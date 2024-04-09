/*
Copyright 2021 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ingress

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/ingress"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/network"
	pkgreconciler "knative.dev/pkg/reconciler"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewaylisters "sigs.k8s.io/gateway-api/pkg/client/listers/apis/v1beta1"
)

const (
	notReconciledReason  = "ReconcileIngressFailed"
	notReconciledMessage = "Ingress reconciliation failed"
)

// Reconciler implements controller.Reconciler for Route resources.
type Reconciler struct {
	statusManager status.Manager

	gwapiclient gatewayclientset.Interface

	// Listers index properties about resources
	httprouteLister gatewaylisters.HTTPRouteLister

	referenceGrantLister gatewaylisters.ReferenceGrantLister

	gatewayLister gatewaylisters.GatewayLister
}

var (
	_ ingressreconciler.Interface = (*Reconciler)(nil)
)

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, ingress *v1alpha1.Ingress) pkgreconciler.Event {
	reconcileErr := c.reconcileIngress(ctx, ingress)

	if reconcileErr != nil {
		ingress.Status.MarkIngressNotReady(notReconciledReason, notReconciledMessage)
		return reconcileErr
	}

	return nil
}

// FinalizeKind implements Interface.FinalizeKind
func (c *Reconciler) FinalizeKind(ctx context.Context, ingress *v1alpha1.Ingress) pkgreconciler.Event {
	gatewayConfig := config.FromContext(ctx).Gateway

	// We currently only support TLS on the external IP
	return c.clearGatewayListeners(ctx, ingress, gatewayConfig.Gateways[v1alpha1.IngressVisibilityExternalIP].Gateway)
}

func (c *Reconciler) reconcileIngress(ctx context.Context, ing *v1alpha1.Ingress) error {
	gatewayConfig := config.FromContext(ctx).Gateway

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	ing.SetDefaults(ctx)
	before := ing.DeepCopy()

	ing.Status.InitializeConditions()

	if _, err := ingress.InsertProbe(ing); err != nil {
		return fmt.Errorf("failed to add knative probe header: %w", err)
	}

	for _, rule := range ing.Spec.Rules {
		rule := rule

		workloadHTTPRoute, err := c.reconcileWorkloadRoute(ctx, ing, &rule)
		if err != nil {
			return err
		}

		// For now, we only generate the redirected HTTPRoute for external visibility,
		// because currently we do not (yet) support HTTPs on cluster-local domains.
		var redirectHTTPRoute *gatewayapi.HTTPRoute
		if ing.Spec.HTTPOption == v1alpha1.HTTPOptionRedirected && rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			redirectHTTPRoute, err = c.reconcileRedirectHTTPRoute(ctx, ing, &rule)
			if err != nil {
				return err
			}
		}

		if isHTTPRouteReady(workloadHTTPRoute) && (redirectHTTPRoute == nil || isHTTPRouteReady(redirectHTTPRoute)) {
			ing.Status.MarkNetworkConfigured()
		} else {
			ing.Status.MarkIngressNotReady("HTTPRouteNotReady", "Waiting for HTTPRoute becomes Ready.")
		}
	}

	externalIngressTLS := ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP)
	listeners := make([]*gatewayapi.Listener, 0, len(externalIngressTLS))
	for _, tls := range externalIngressTLS {
		tls := tls

		l, err := c.reconcileTLS(ctx, &tls, ing)
		if err != nil {
			return err
		}
		listeners = append(listeners, l...)
	}

	if len(listeners) > 0 {
		// For now, we only reconcile the external visibility, because there's
		// no way to provide TLS for internal listeners.
		err := c.reconcileGatewayListeners(
			ctx, listeners, ing, *gatewayConfig.Gateways[v1alpha1.IngressVisibilityExternalIP].Gateway)
		if err != nil {
			return err
		}
	}

	// TODO: check Gateway readiness before reporting Ingress ready

	ready, err := c.statusManager.IsReady(ctx, before)
	if err != nil {
		return fmt.Errorf("failed to probe Ingress: %w", err)
	}

	if ready {
		namespacedNameService := gatewayConfig.Gateways[v1alpha1.IngressVisibilityExternalIP].Service
		publicLbs := []v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: network.GetServiceHostname(namespacedNameService.Name, namespacedNameService.Namespace)},
		}

		namespacedNameLocalService := gatewayConfig.Gateways[v1alpha1.IngressVisibilityClusterLocal].Service
		privateLbs := []v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: network.GetServiceHostname(namespacedNameLocalService.Name, namespacedNameLocalService.Namespace)},
		}

		ing.Status.MarkLoadBalancerReady(publicLbs, privateLbs)
	} else {
		ing.Status.MarkLoadBalancerNotReady()
	}

	return nil
}

// isHTTPRouteReady will check the status conditions of the ingress and return true if
// all gateways have been admitted.
func isHTTPRouteReady(r *gatewayapi.HTTPRoute) bool {
	if r.Status.Parents == nil {
		return false
	}
	for _, gw := range r.Status.Parents {
		if !isGatewayAdmitted(gw) {
			// Return false if _any_ of the gateways isn't admitted yet.
			return false
		}
	}
	return true
}

func isGatewayAdmitted(gw gatewayapi.RouteParentStatus) bool {
	for _, condition := range gw.Conditions {
		if condition.Type == string(gatewayapi.RouteConditionAccepted) {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}
