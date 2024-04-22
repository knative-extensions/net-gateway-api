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

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/status"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/ingress"
	"knative.dev/pkg/network"
	pkgreconciler "knative.dev/pkg/reconciler"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewaylisters "sigs.k8s.io/gateway-api/pkg/client/listers/apis/v1"
	gatewaylistersv1beta1 "sigs.k8s.io/gateway-api/pkg/client/listers/apis/v1beta1"
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

	referenceGrantLister gatewaylistersv1beta1.ReferenceGrantLister

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
	pluginConfig := config.FromContext(ctx).GatewayPlugin

	// We currently only support TLS on the external IP
	return c.clearGatewayListeners(ctx, ingress, pluginConfig.ExternalGateway().NamespacedName)
}

func (c *Reconciler) reconcileIngress(ctx context.Context, ing *v1alpha1.Ingress) error {
	pluginConfig := config.FromContext(ctx).GatewayPlugin

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	ing.SetDefaults(ctx)
	ing.Status.InitializeConditions()

	var (
		ingressHash string
		err         error
	)

	if ingressHash, err = ingress.InsertProbe(ing); err != nil {
		return fmt.Errorf("failed to add knative probe header: %w", err)
	}

	routesReady := true

	for _, rule := range ing.Spec.Rules {
		rule := rule

		httproute, probeTargets, err := c.reconcileHTTPRoute(ctx, ingressHash, ing, &rule)
		if err != nil {
			return err
		}

		if isHTTPRouteReady(httproute) {
			ing.Status.MarkNetworkConfigured()

			state, err := c.statusManager.DoProbes(ctx, probeTargets)
			if err != nil {
				return fmt.Errorf("failed to probe Ingress: %w", err)
			}

			routesReady = routesReady && state.Ready
		} else {
			routesReady = false
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
			ctx, listeners, ing, pluginConfig.ExternalGateway().NamespacedName)
		if err != nil {
			return err
		}
	}

	// TODO: check Gateway readiness before reporting Ingress ready
	if routesReady {
		var publicLbs, privateLbs []v1alpha1.LoadBalancerIngressStatus

		externalGateway := pluginConfig.ExternalGateway()
		localGateway := pluginConfig.LocalGateway()

		publicLbs, err = c.determineLoadBalancerIngressStatus(externalGateway)
		if err != nil {
			if apierrs.IsNotFound(err) {
				ing.Status.MarkLoadBalancerFailed(
					"GatewayDoesNotExist",
					fmt.Sprintf(
						"could not find Gateway %s/%s",
						externalGateway.Namespace,
						externalGateway.Name,
					),
				)
				return fmt.Errorf("Gateway %s does not exist: %w", externalGateway.Name, err) //nolint:stylecheck
			}
			ing.Status.MarkLoadBalancerNotReady()
			return err
		}

		privateLbs, err = c.determineLoadBalancerIngressStatus(localGateway)
		if err != nil {
			if apierrs.IsNotFound(err) {
				ing.Status.MarkLoadBalancerFailed(
					"GatewayDoesNotExist",
					fmt.Sprintf(
						"could not find Gateway %s/%s",
						localGateway.Namespace,
						localGateway.Name,
					),
				)
				return fmt.Errorf("Gateway %s does not exist: %w", localGateway.Name, err) //nolint:stylecheck
			}
			ing.Status.MarkLoadBalancerNotReady()
			return err
		}

		ing.Status.MarkLoadBalancerReady(publicLbs, privateLbs)
	} else {
		ing.Status.MarkLoadBalancerNotReady()
	}

	return nil
}

// determineLoadBalancerIngressStatus will return the address for the Gateway.
// If a service is provided, it will return the address of the service.
// Otherwise, it will return the first address in the Gateway status.
func (c *Reconciler) determineLoadBalancerIngressStatus(gwc config.Gateway) ([]v1alpha1.LoadBalancerIngressStatus, error) {
	if gwc.Service != nil {
		return []v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: network.GetServiceHostname(gwc.Service.Name, gwc.Service.Namespace)},
		}, nil
	}
	gw, err := c.gatewayLister.Gateways(gwc.Namespace).Get(gwc.Name)
	if err != nil {
		return nil, err
	}

	var lbis v1alpha1.LoadBalancerIngressStatus

	if len(gw.Status.Addresses) > 0 {
		switch *gw.Status.Addresses[0].Type {
		case gatewayapi.IPAddressType:
			lbis = v1alpha1.LoadBalancerIngressStatus{IP: gw.Status.Addresses[0].Value}
		default:
			// Should this actually be under Domain? It seems like the rest of the code expects DomainInternal though...
			lbis = v1alpha1.LoadBalancerIngressStatus{DomainInternal: gw.Status.Addresses[0].Value}
		}

		return []v1alpha1.LoadBalancerIngressStatus{lbis}, nil
	}

	return nil, fmt.Errorf("Gateway %q does not have an address in status", gwc.NamespacedName) //nolint:stylecheck

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
