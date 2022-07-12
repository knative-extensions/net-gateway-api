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
	"k8s.io/utils/pointer"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/controller"
)

const listenerPrefix = "kni-"

// reconcileHTTPRoute reconciles HTTPRoute.
func (c *Reconciler) reconcileHTTPRoute(
	ctx context.Context, ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayv1alpha2.HTTPRoute, error) {
	recorder := controller.GetEventRecorder(ctx)

	httproute, err := c.httprouteLister.HTTPRoutes(ing.Namespace).Get(resources.LongestHost(rule.Hosts))
	if apierrs.IsNotFound(err) {
		desired, err := resources.MakeHTTPRoute(ctx, ing, rule)
		if err != nil {
			return nil, err
		}
		httproute, err = c.gwapiclient.GatewayV1alpha2().HTTPRoutes(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "CreationFailed", "Failed to create HTTPRoute: %v", err)
			return nil, fmt.Errorf("failed to create HTTPRoute: %w", err)
		}

		recorder.Eventf(ing, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", httproute.GetName())
		return httproute, nil
	} else if err != nil {
		return nil, err
	} else {
		desired, err := resources.MakeHTTPRoute(ctx, ing, rule)
		if err != nil {
			return nil, err
		}

		if !equality.Semantic.DeepEqual(httproute.Spec, desired.Spec) ||
			!equality.Semantic.DeepEqual(httproute.Annotations, desired.Annotations) ||
			!equality.Semantic.DeepEqual(httproute.Labels, desired.Labels) {

			// Don't modify the informers copy.
			origin := httproute.DeepCopy()
			origin.Spec = desired.Spec
			origin.Annotations = desired.Annotations
			origin.Labels = desired.Labels

			updated, err := c.gwapiclient.GatewayV1alpha2().HTTPRoutes(origin.Namespace).Update(
				ctx, origin, metav1.UpdateOptions{})
			if err != nil {
				recorder.Eventf(ing, corev1.EventTypeWarning, "UpdateFailed", "Failed to update HTTPRoute: %v", err)
				return nil, fmt.Errorf("failed to update HTTPRoute: %w", err)
			}
			return updated, nil
		}
	}

	return httproute, err
}

func (c *Reconciler) reconcileTLS(
	ctx context.Context, tls *netv1alpha1.IngressTLS, ing *netv1alpha1.Ingress,
) (
	[]*gatewayv1alpha2.Listener, error) {
	recorder := controller.GetEventRecorder(ctx)
	gatewayConfig := config.FromContext(ctx).Gateway.Gateways
	externalGw := gatewayConfig[netv1alpha1.IngressVisibilityExternalIP]

	gateway := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayv1alpha2.GroupVersion.String(),
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

	rp, err := c.referencePolicyLister.ReferencePolicies(desired.Namespace).Get(desired.Name)

	if apierrs.IsNotFound(err) {
		rp, err = c.gwapiclient.GatewayV1alpha2().ReferencePolicies(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})

		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "CreationFailed", "Failed to create ReferencePolicy: %v", err)
			return nil, fmt.Errorf("failed to create ReferencePolicy: %w", err)
		}
	} else if err != nil {
		return nil, err
	}

	if !metav1.IsControlledBy(rp, ing) {
		recorder.Eventf(ing, corev1.EventTypeWarning, "NotOwned", "ReferencePolicy %s not owned by this object", desired.Name)
		return nil, fmt.Errorf("ReferencePolicy %s not owned by %s", rp.Name, ing.Name)
	}

	if !equality.Semantic.DeepEqual(rp.Spec, desired.Spec) {
		update := rp.DeepCopy()
		update.Spec = desired.Spec

		_, err := c.gwapiclient.GatewayV1alpha2().ReferencePolicies(update.Namespace).Update(ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "UpdateFailed", "Failed to update ReferencePolicy: %v", err)
			return nil, fmt.Errorf("failed to update ReferencePolicy: %w", err)
		}
	}

	// Gateway API loves typed pointers and constants, so we need to copy the constants
	// to something we can reference
	mode := gatewayv1alpha2.TLSModeTerminate
	selector := gatewayv1alpha2.NamespacesFromSelector
	listeners := make([]*gatewayv1alpha2.Listener, 0, len(tls.Hosts))
	for _, h := range tls.Hosts {
		h := h
		listener := gatewayv1alpha2.Listener{
			Name:     gatewayv1alpha2.SectionName(listenerPrefix + ing.GetUID()),
			Hostname: (*gatewayv1alpha2.Hostname)(&h),
			Port:     443,
			Protocol: gatewayv1alpha2.HTTPSProtocolType,
			TLS: &gatewayv1alpha2.GatewayTLSConfig{
				Mode: &mode,
				CertificateRefs: []gatewayv1alpha2.SecretObjectReference{{
					Group:     (*gatewayv1alpha2.Group)(pointer.String("")),
					Kind:      (*gatewayv1alpha2.Kind)(pointer.String("Secret")),
					Name:      gatewayv1alpha2.ObjectName(tls.SecretName),
					Namespace: (*gatewayv1alpha2.Namespace)(&tls.SecretNamespace),
				}},
			},
			AllowedRoutes: &gatewayv1alpha2.AllowedRoutes{
				Namespaces: &gatewayv1alpha2.RouteNamespaces{
					From: &selector,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							corev1.LabelMetadataName: ing.Namespace,
						},
					},
				},
				Kinds: []gatewayv1alpha2.RouteGroupKind{},
			},
		}
		listeners = append(listeners, &listener)
	}

	return listeners, err
}

func (c *Reconciler) reconcileGatewayListeners(
	ctx context.Context, listeners []*gatewayv1alpha2.Listener,
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

	lmap := map[string]*gatewayv1alpha2.Listener{}
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
		_, err := c.gwapiclient.GatewayV1alpha2().Gateways(update.Namespace).Update(
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
		_, err := c.gwapiclient.GatewayV1alpha2().Gateways(update.Namespace).Update(ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "GatewayUpdateFailed", "Failed to remove Listener from Gateway %s: %v", gwName, err)
			return fmt.Errorf("failed to update Gateway %s/%s: %w", update.Namespace, update.Name, err)
		}
	}

	return nil
}
