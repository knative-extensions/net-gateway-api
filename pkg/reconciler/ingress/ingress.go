/*
Copyright 2018 The Knative Authors

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
	"k8s.io/client-go/tools/cache"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/ingress"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/network"
	pkgreconciler "knative.dev/pkg/reconciler"

	gwapiclientset "knative.dev/net-gateway-api/pkg/client/gatewayapi/clientset/versioned"
	gwlisters "knative.dev/net-gateway-api/pkg/client/gatewayapi/listers/apis/v1alpha1"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

const (
	notReconciledReason  = "ReconcileIngressFailed"
	notReconciledMessage = "Ingress reconciliation failed"
)

// Reconciler implements controller.Reconciler for Route resources.
type Reconciler struct {
	statusManager status.Manager

	gwapiclient gwapiclientset.Interface

	// Listers index properties about resources
	httprouteLister gwlisters.HTTPRouteLister
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

func (c *Reconciler) reconcileIngress(ctx context.Context, ing *v1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)

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

	logger.Infof("Reconciling ingress: %#v", ing)

	for _, rule := range ing.Spec.Rules {
		rule := rule

		httproutes, err := c.reconcileHTTPRoute(ctx, ing, &rule)
		if err != nil {
			return err
		}

		ready, err := IsHTTPRouteReady(httproutes)
		if err != nil {
			return err
		}

		if ready {
			ing.Status.MarkNetworkConfigured()
		} else {
			ing.Status.MarkIngressNotReady("HTTPRouteNotReady", "Waiting for HTTPRoute becomes Ready.")
		}
		logger.Infof("HTTPRoute successfully synced %v", httproutes)
	}

	ready, err := c.statusManager.IsReady(ctx, before)
	if err != nil {
		return fmt.Errorf("failed to probe Ingress: %w", err)
	}

	if ready {
		gatewayConfig := config.FromContext(ctx).Gateway

		ns, name, _ := cache.SplitMetaNamespaceKey(gatewayConfig.LookupService(v1alpha1.IngressVisibilityExternalIP))
		publicLbs := []v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: network.GetServiceHostname(name, ns)},
		}

		ns, name, _ = cache.SplitMetaNamespaceKey(gatewayConfig.LookupService(v1alpha1.IngressVisibilityClusterLocal))
		privateLbs := []v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: network.GetServiceHostname(name, ns)},
		}

		ing.Status.MarkLoadBalancerReady(publicLbs, privateLbs)
	} else {
		ing.Status.MarkLoadBalancerNotReady()
	}

	return nil
}

// IsHTTPRouteReady will check the status conditions of the ingress and return true if
// all gateways have been admitted.
func IsHTTPRouteReady(r *gatewayv1alpha1.HTTPRoute) (bool, error) {
	if r.Status.Gateways == nil {
		return false, nil
	}
	for _, gw := range r.Status.Gateways {
		if !isGatewayAdmitted(gw) {
			// Return false if _any_ of the gateways isn't admitted yet.
			return false, nil
		}
	}
	return true, nil
}

func isGatewayAdmitted(gw gatewayv1alpha1.RouteGatewayStatus) bool {
	for _, condition := range gw.Conditions {
		if condition.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}
