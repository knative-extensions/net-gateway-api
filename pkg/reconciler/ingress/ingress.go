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
	"errors"
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

var ErrGatewayNotFound = errors.New("could not find Gateway")

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

		workloadRoute, probeTargets, err := c.reconcileWorkloadRoute(ctx, ingressHash, ing, &rule)
		if err != nil {
			return err
		}

		// For now, we only generate the redirected HTTPRoute for external visibility,
		// because currently we do not (yet) support HTTPs for cluster-local domains in net-gateway-api.
		var redirectRoute *gatewayapi.HTTPRoute
		if ing.Spec.HTTPOption == v1alpha1.HTTPOptionRedirected && rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			redirectRoute, err = c.reconcileRedirectHTTPRoute(ctx, ing, &rule)
			if err != nil {
				return err
			}
		}

		if isHTTPRouteReady(workloadRoute) && (redirectRoute == nil || isHTTPRouteReady(redirectRoute)) {
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
		externalLBs, internalLBs, err := c.lookUpLoadBalancers(ing, pluginConfig)
		if err != nil {
			if ok := errors.Is(err, ErrGatewayNotFound); ok {
				// if we can't find a Gateway, we mark it as failed, and
				// return no error, since there is no point in retrying
				return nil
			}
			ing.Status.MarkLoadBalancerNotReady()
			return err
		}

		ing.Status.MarkLoadBalancerReady(externalLBs, internalLBs)
	} else {
		ing.Status.MarkLoadBalancerNotReady()
	}

	return nil
}

// lookUpLoadBalancers will return a map of visibilites to
// LoadBalancerIngressStatuses for the current Gateways in use.
func (c *Reconciler) lookUpLoadBalancers(ing *v1alpha1.Ingress, gpc *config.GatewayPlugin) ([]v1alpha1.LoadBalancerIngressStatus, []v1alpha1.LoadBalancerIngressStatus, error) {
	externalStatuses, err := c.collectLBIngressStatus(ing, gpc.ExternalGateway())
	if err != nil {
		return nil, nil, err
	}

	internalStatuses, err := c.collectLBIngressStatus(ing, gpc.LocalGateway())
	if err != nil {
		return nil, nil, err
	}

	return externalStatuses, internalStatuses, nil
}

// collectLBIngressStatus will return LoadBalancerIngressStatuses for the
// provided single Gateway config. If a service is available on a Gateway, it will
// return the address of the service. Otherwise, it will return the first
// address in the Gateway status.
func (c *Reconciler) collectLBIngressStatus(ing *v1alpha1.Ingress, gwc config.Gateway) ([]v1alpha1.LoadBalancerIngressStatus, error) {
	statuses := []v1alpha1.LoadBalancerIngressStatus{}

	// TODO: currently only 1 gateway is supported. When the config is updated to
	// support multiple, this code must change to find out which Gateway is
	// appropriate for the given Ingress
	if gwc.Service != nil {
		statuses = append(statuses, v1alpha1.LoadBalancerIngressStatus{
			DomainInternal: network.GetServiceHostname(gwc.Service.Name, gwc.Service.Namespace),
		})
	} else {
		gw, err := c.gatewayLister.Gateways(gwc.Namespace).Get(gwc.Name)
		if err != nil {
			if apierrs.IsNotFound(err) {
				ing.Status.MarkLoadBalancerFailed(
					"GatewayDoesNotExist",
					fmt.Sprintf(
						"could not find Gateway %s/%s",
						gwc.Namespace,
						gwc.Name,
					),
				)
				return nil, fmt.Errorf("error getting Gateway %s/%s: %w", gwc.Namespace, gwc.Name, ErrGatewayNotFound)
			}
			return nil, fmt.Errorf("failed to get Gateway \"%s/%s\": %w", gwc.Namespace, gwc.Name, err)
		}

		if len(gw.Status.Addresses) > 0 {
			switch *gw.Status.Addresses[0].Type {
			case gatewayapi.IPAddressType:
				statuses = append(statuses, v1alpha1.LoadBalancerIngressStatus{IP: gw.Status.Addresses[0].Value})
			default:
				// Should this actually be under Domain? It seems like the rest of the code expects DomainInternal though...
				statuses = append(statuses, v1alpha1.LoadBalancerIngressStatus{DomainInternal: gw.Status.Addresses[0].Value})
			}
		} else {
			return nil, fmt.Errorf("no address found in status of Gateway %s/%s", gwc.Namespace, gwc.Name)
		}
	}

	return statuses, nil
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
