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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/controller"
)

const listenerPrefix = "kni-"

// reconcileWorkloadRoute reconciles the HTTPRoute for the workload traffic
func (c *Reconciler) reconcileWorkloadRoute(
	ctx context.Context, ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayapi.HTTPRoute, error) {
	gatewayConfig := config.FromContext(ctx).Gateway.Gateways[rule.Visibility]
	gatewayRef := gatewayapi.ParentReference{
		Group:     (*gatewayapi.Group)(&gatewayapi.GroupVersion.Group),
		Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
		Namespace: (*gatewayapi.Namespace)(&gatewayConfig.Gateway.Namespace),
		Name:      gatewayapi.ObjectName(gatewayConfig.Gateway.Name),
	}

	// If http > https redirect is enabled, this route must only be bound to the TLS listener on the gateway.
	// For now, we only generate the TLS Listener on the external traffic gateway
	// because there's no way to provide TLS for internal listeners.
	if ing.Spec.HTTPOption == netv1alpha1.HTTPOptionRedirected && rule.Visibility == netv1alpha1.IngressVisibilityExternalIP {
		sectionName := gatewayapi.SectionName(listenerPrefix + ing.GetUID())
		gatewayRef.SectionName = &sectionName
	}

	desired, err := resources.MakeHTTPRoute(ing, rule, gatewayRef)
	if err != nil {
		return nil, err
	}
	return c.reconcileHTTPRoute(ctx, ing, desired)
}

// reconcileRedirectHTTPRoute reconciles the HTTPRoute for the http->https redirect
func (c *Reconciler) reconcileRedirectHTTPRoute(
	ctx context.Context, ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayapi.HTTPRoute, error) {

	if ing.Spec.TLS == nil || len(ing.Spec.TLS) == 0 {
		return nil, fmt.Errorf("no TLS configuration provided in `spec.tls`. Failed to create HTTPRoute for HTTPS redirection")
	}

	gatewayConfig := config.FromContext(ctx).Gateway.Gateways[rule.Visibility]
	gatewayRef := gatewayapi.ParentReference{
		Group:     (*gatewayapi.Group)(&gatewayapi.GroupVersion.Group),
		Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
		Namespace: (*gatewayapi.Namespace)(&gatewayConfig.Gateway.Namespace),
		Name:      gatewayapi.ObjectName(gatewayConfig.Gateway.Name),

		// Redirect routes must only be bound on the http listener of the gateway
		SectionName: (*gatewayapi.SectionName)(&gatewayConfig.HTTPListenerName),
	}

	desired, err := resources.MakeRedirectHTTPRoute(ing, rule, gatewayRef)
	if err != nil {
		return nil, err
	}
	return c.reconcileHTTPRoute(ctx, ing, desired)
}

// reconcileHTTPRoute reconciles the desired HTTPRoute.
func (c *Reconciler) reconcileHTTPRoute(ctx context.Context,
	ing *netv1alpha1.Ingress,
	desired *gatewayapi.HTTPRoute,
) (*gatewayapi.HTTPRoute, error) {

	recorder := controller.GetEventRecorder(ctx)

	httpRoute, err := c.httprouteLister.HTTPRoutes(ing.Namespace).Get(desired.Name)
	if apierrs.IsNotFound(err) {
		httpRoute, err = c.gwapiclient.GatewayV1beta1().HTTPRoutes(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "CreationFailed", "Failed to create HTTPRoute: %v", err)
			return nil, fmt.Errorf("failed to create HTTPRoute: %w", err)
		}

		recorder.Eventf(ing, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", httpRoute.GetName())
		return httpRoute, nil
	} else if err != nil {
		return nil, err
	}

	if !equality.Semantic.DeepEqual(httpRoute.Spec, desired.Spec) ||
		!equality.Semantic.DeepEqual(httpRoute.Annotations, desired.Annotations) ||
		!equality.Semantic.DeepEqual(httpRoute.Labels, desired.Labels) {

		// Don't modify the informers copy.
		origin := httpRoute.DeepCopy()
		origin.Spec = desired.Spec
		origin.Annotations = desired.Annotations
		origin.Labels = desired.Labels

		updated, err := c.gwapiclient.GatewayV1beta1().HTTPRoutes(origin.Namespace).Update(
			ctx, origin, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "UpdateFailed", "Failed to update HTTPRoute: %v", err)
			return nil, fmt.Errorf("failed to update HTTPRoute: %w", err)
		}
		return updated, nil
	}

	return httpRoute, err
}

func (c *Reconciler) reconcileTLS(
	ctx context.Context, tls *netv1alpha1.IngressTLS, ing *netv1alpha1.Ingress,
) (
	[]*gatewayapi.Listener, error) {
	recorder := controller.GetEventRecorder(ctx)
	gatewayConfig := config.FromContext(ctx).Gateway.Gateways
	externalGw := gatewayConfig[netv1alpha1.IngressVisibilityExternalIP]

	gateway := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayapi.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalGw.Gateway.Name,
			Namespace: externalGw.Gateway.Namespace,
		},
	}
	secret := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tls.SecretName,
			Namespace: tls.SecretNamespace,
		},
	}

	desired := resources.MakeReferenceGrant(ctx, ing, secret, gateway)

	rp, err := c.referenceGrantLister.ReferenceGrants(desired.Namespace).Get(desired.Name)

	if apierrs.IsNotFound(err) {
		rp, err = c.gwapiclient.GatewayV1beta1().ReferenceGrants(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})

		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "CreationFailed", "Failed to create ReferenceGrant: %v", err)
			return nil, fmt.Errorf("failed to create ReferenceGrant: %w", err)
		}
	} else if err != nil {
		return nil, err
	}

	if !metav1.IsControlledBy(rp, ing) {
		recorder.Eventf(ing, corev1.EventTypeWarning, "NotOwned", "ReferenceGrant %s not owned by this object", desired.Name)
		return nil, fmt.Errorf("ReferenceGrant %s not owned by %s", rp.Name, ing.Name)
	}

	if !equality.Semantic.DeepEqual(rp.Spec, desired.Spec) {
		update := rp.DeepCopy()
		update.Spec = desired.Spec

		_, err := c.gwapiclient.GatewayV1beta1().ReferenceGrants(update.Namespace).Update(ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "UpdateFailed", "Failed to update ReferenceGrant: %v", err)
			return nil, fmt.Errorf("failed to update ReferenceGrant: %w", err)
		}
	}

	// Gateway API loves typed pointers and constants, so we need to copy the constants
	// to something we can reference
	mode := gatewayapiv1.TLSModeTerminate
	selector := gatewayapiv1.NamespacesFromSelector
	listeners := make([]*gatewayapi.Listener, 0, len(tls.Hosts))
	for _, h := range tls.Hosts {
		h := h
		listener := gatewayapi.Listener{
			Name:     gatewayapi.SectionName(listenerPrefix + ing.GetUID()),
			Hostname: (*gatewayapi.Hostname)(&h),
			Port:     443,
			Protocol: gatewayapiv1.HTTPSProtocolType,
			TLS: &gatewayapi.GatewayTLSConfig{
				Mode: &mode,
				CertificateRefs: []gatewayapi.SecretObjectReference{{
					Group:     (*gatewayapi.Group)(ptr.To("")),
					Kind:      (*gatewayapi.Kind)(ptr.To("Secret")),
					Name:      gatewayapi.ObjectName(tls.SecretName),
					Namespace: (*gatewayapi.Namespace)(&tls.SecretNamespace),
				}},
			},
			AllowedRoutes: &gatewayapi.AllowedRoutes{
				Namespaces: &gatewayapi.RouteNamespaces{
					From: &selector,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							corev1.LabelMetadataName: ing.Namespace,
						},
					},
				},
				Kinds: []gatewayapi.RouteGroupKind{},
			},
		}
		listeners = append(listeners, &listener)
	}

	return listeners, err
}

func (c *Reconciler) reconcileGatewayListeners(
	ctx context.Context, listeners []*gatewayapi.Listener,
	ing *netv1alpha1.Ingress, gwName types.NamespacedName,
) error {
	recorder := controller.GetEventRecorder(ctx)
	gw, err := c.gatewayLister.Gateways(gwName.Namespace).Get(gwName.Name)
	if apierrs.IsNotFound(err) {
		recorder.Eventf(ing, corev1.EventTypeWarning, "GatewayMissing", "Unable to update Gateway %s", gwName.String())
		return fmt.Errorf("Gateway %s does not exist: %w", gwName, err) //nolint:stylecheck
	} else if err != nil {
		return err
	}

	update := gw.DeepCopy()

	lmap := map[string]*gatewayapi.Listener{}
	for _, l := range listeners {
		lmap[string(l.Name)] = l
	}
	// TODO: how do we track and remove listeners if they are removed from the KIngress spec?
	// Tracked in https://github.com/knative-sandbox/net-gateway-api/issues/319

	updated := false
	for i, l := range gw.Spec.Listeners {
		l := l
		desired, ok := lmap[string(l.Name)]
		if !ok {
			// This listener doesn't match any that we control.
			continue
		}
		delete(lmap, string(l.Name))
		if equality.Semantic.DeepEqual(&l, desired) {
			// Already present and correct
			continue
		}
		update.Spec.Listeners[i] = *desired
		updated = true
	}

	for _, l := range lmap {
		// Add all remaining listeners
		update.Spec.Listeners = append(update.Spec.Listeners, *l)
		updated = true
	}

	if updated {
		_, err := c.gwapiclient.GatewayV1beta1().Gateways(update.Namespace).Update(
			ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "GatewayUpdateFailed", "Failed to update Gateway %s: %v", gwName, err)
			return fmt.Errorf("failed to update Gateway %s/%s: %w", update.Namespace, update.Name, err)
		}
	}

	return nil
}

func (c *Reconciler) clearGatewayListeners(ctx context.Context, ing *netv1alpha1.Ingress, gwName *types.NamespacedName) error {
	recorder := controller.GetEventRecorder(ctx)

	gw, err := c.gatewayLister.Gateways(gwName.Namespace).Get(gwName.Name)
	if apierrs.IsNotFound(err) {
		// Nothing to clean up, all done!
		return nil
	} else if err != nil {
		return err
	}

	listenerName := listenerPrefix + string(ing.GetUID())
	update := gw.DeepCopy()

	numListeners := len(update.Spec.Listeners)
	for i := numListeners - 1; i >= 0; i-- {
		// March backwards down the list removing items by swapping in the last item and trimming the list
		// A generic list.remove(func) would be nice here.
		l := update.Spec.Listeners[i]
		if string(l.Name) == listenerName {
			update.Spec.Listeners[i] = update.Spec.Listeners[len(update.Spec.Listeners)-1]
			update.Spec.Listeners = update.Spec.Listeners[:len(update.Spec.Listeners)-1]
		}
	}

	if len(update.Spec.Listeners) != numListeners {
		_, err := c.gwapiclient.GatewayV1beta1().Gateways(update.Namespace).Update(ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "GatewayUpdateFailed", "Failed to remove Listener from Gateway %s: %v", gwName, err)
			return fmt.Errorf("failed to update Gateway %s/%s: %w", update.Namespace, update.Name, err)
		}
	}

	return nil
}
