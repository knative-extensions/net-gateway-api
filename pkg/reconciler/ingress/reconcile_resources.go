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
	"net/url"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
	"knative.dev/net-gateway-api/pkg/status"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/pkg/controller"
)

const listenerPrefix = "kni-"

func probeTargets(
	hash string,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
	r *gatewayapi.HTTPRoute,
) status.Backends {

	backends := status.Backends{
		Version: hash,
		Key:     resources.HTTPRouteKey(ing, rule),
		CallbackKey: types.NamespacedName{
			Name:      ing.Name,
			Namespace: ing.Namespace,
		},
	}

	visibility := rule.Visibility
	if visibility == "" {
		visibility = netv1alpha1.IngressVisibilityExternalIP
	}

	for _, rule := range r.Spec.Rules {
	match_loop:
		for _, match := range rule.Matches {
			for _, headers := range match.Headers {
				// Skip non-probe matches
				if headers.Name != header.HashKey {
					continue
				}

				if visibility == netv1alpha1.IngressVisibilityClusterLocal {
					host := resources.LongestHost(r.Spec.Hostnames)
					url := url.URL{Host: string(host), Path: *match.Path.Value}
					backends.AddURL(visibility, url)
					continue match_loop
				}

				for _, hostname := range r.Spec.Hostnames {
					url := url.URL{Host: string(hostname), Path: *match.Path.Value}
					backends.AddURL(visibility, url)
				}
			}
		}
	}
	return backends
}

// reconcileHTTPRoute reconciles HTTPRoute.
func (c *Reconciler) reconcileHTTPRoute(
	ctx context.Context,
	hash string,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayapi.HTTPRoute, status.Backends, error) {

	recorder := controller.GetEventRecorder(ctx)

	httproute, err := c.httprouteLister.HTTPRoutes(ing.Namespace).Get(resources.LongestHost(rule.Hosts))
	if apierrs.IsNotFound(err) {
		desired, err := resources.MakeHTTPRoute(ctx, ing, rule)
		if err != nil {
			return nil, status.Backends{}, err
		}
		httproute, err = c.gwapiclient.GatewayV1().HTTPRoutes(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "CreationFailed", "Failed to create HTTPRoute: %v", err)
			return nil, status.Backends{}, fmt.Errorf("failed to create HTTPRoute: %w", err)
		}

		recorder.Eventf(ing, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", httproute.GetName())
		return httproute, probeTargets(hash, ing, rule, httproute), nil
	} else if err != nil {
		return nil, status.Backends{}, err
	}

	return c.reconcileHTTPRouteUpdate(ctx, hash, ing, rule, httproute.DeepCopy())
}

func (c *Reconciler) reconcileHTTPRouteUpdate(
	ctx context.Context,
	hash string,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
	httproute *gatewayapi.HTTPRoute,
) (*gatewayapi.HTTPRoute, status.Backends, error) {

	const (
		endpointPrefix   = "ep-"
		transitionPrefix = "tr-"
	)

	var (
		desired *gatewayapi.HTTPRoute
		err     error

		original = httproute.DeepCopy()
		recorder = controller.GetEventRecorder(ctx)

		probeKey = types.NamespacedName{
			Name:      httproute.Name,
			Namespace: httproute.Namespace,
		}

		probe, _           = c.statusManager.IsProbeActive(probeKey)
		wasEndpointProbe   = strings.HasPrefix(probe.Version, endpointPrefix)
		wasTransitionProbe = strings.HasPrefix(probe.Version, transitionPrefix)
	)

	probeHash := strings.TrimPrefix(probe.Version, endpointPrefix)
	probeHash = strings.TrimPrefix(probeHash, transitionPrefix)

	newBackends, oldBackends := computeBackends(httproute, rule)

	if wasTransitionProbe && probeHash == hash && probe.Ready {
		desired, err = resources.MakeHTTPRoute(ctx, ing, rule)
	} else if wasEndpointProbe && probeHash == hash && probe.Ready {
		hash = transitionPrefix + hash

		desired, err = resources.MakeHTTPRoute(ctx, ing, rule)
		resources.UpdateProbeHash(desired, hash)

		resources.RemoveEndpointProbes(httproute)
		for _, backend := range newBackends {
			resources.AddEndpointProbe(desired, hash, backend)
		}
		for _, backend := range oldBackends {
			resources.AddOldBackend(desired, hash, backend)
		}
	} else if len(newBackends) > 0 {
		hash = endpointPrefix + hash
		desired = httproute.DeepCopy()
		resources.UpdateProbeHash(desired, hash)
		resources.RemoveEndpointProbes(desired)
		for _, backend := range newBackends {
			resources.AddEndpointProbe(desired, hash, backend)
		}
		for _, backend := range oldBackends {
			resources.AddOldBackend(desired, hash, backend)
		}
	} else if probeHash != hash {
		desired, err = resources.MakeHTTPRoute(ctx, ing, rule)
	} else {
		// noop - preserve current probing
		if probe.Version != "" {
			hash = probe.Version
		}
		return httproute, probeTargets(hash, ing, rule, httproute), nil
	}

	if err != nil {
		return nil, status.Backends{}, err
	}

	if !equality.Semantic.DeepEqual(original.Spec, desired.Spec) ||
		!equality.Semantic.DeepEqual(original.Annotations, desired.Annotations) ||
		!equality.Semantic.DeepEqual(original.Labels, desired.Labels) {

		// Don't modify the informers copy.
		original.Spec = desired.Spec
		original.Annotations = desired.Annotations
		original.Labels = desired.Labels

		updated, err := c.gwapiclient.GatewayV1().HTTPRoutes(original.Namespace).
			Update(ctx, original, metav1.UpdateOptions{})

		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "UpdateFailed", "Failed to update HTTPRoute: %v", err)
			return nil, status.Backends{}, fmt.Errorf("failed to update HTTPRoute: %w", err)
		}
		return updated, probeTargets(hash, ing, rule, updated), nil
	}

	return httproute, probeTargets(hash, ing, rule, httproute), nil
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
	mode := gatewayapi.TLSModeTerminate
	selector := gatewayapi.NamespacesFromSelector
	listeners := make([]*gatewayapi.Listener, 0, len(tls.Hosts))
	for _, h := range tls.Hosts {
		h := h
		listener := gatewayapi.Listener{
			Name:     gatewayapi.SectionName(listenerPrefix + ing.GetUID()),
			Hostname: (*gatewayapi.Hostname)(&h),
			Port:     443,
			Protocol: gatewayapi.HTTPSProtocolType,
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
		_, err := c.gwapiclient.GatewayV1().Gateways(update.Namespace).Update(
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
		_, err := c.gwapiclient.GatewayV1().Gateways(update.Namespace).Update(ctx, update, metav1.UpdateOptions{})
		if err != nil {
			recorder.Eventf(ing, corev1.EventTypeWarning, "GatewayUpdateFailed", "Failed to remove Listener from Gateway %s: %v", gwName, err)
			return fmt.Errorf("failed to update Gateway %s/%s: %w", update.Namespace, update.Name, err)
		}
	}

	return nil
}

func computeBackends(
	route *gatewayapi.HTTPRoute,
	rule *netv1alpha1.IngressRule,
) ([]netv1alpha1.IngressBackendSplit, []gatewayapi.HTTPBackendRef) {
	newBackends := []netv1alpha1.IngressBackendSplit{}
	oldBackends := []gatewayapi.HTTPBackendRef{}
	oldNames := sets.Set[types.NamespacedName]{}

oldbackends:
	for _, rule := range route.Spec.Rules {
		// We want to skip probes
		for _, match := range rule.Matches {
			for _, headers := range match.Headers {
				if headers.Name == header.HashKey {
					continue oldbackends
				}
			}
		}

		for _, backend := range rule.BackendRefs {
			nn := types.NamespacedName{
				Name: string(backend.Name),
			}
			if backend.Namespace != nil {
				nn.Namespace = string(*backend.Namespace)
			} else {
				nn.Namespace = route.Namespace

			}
			oldNames.Insert(nn)
			oldBackends = append(oldBackends, backend)
		}
	}

newbackends:
	for _, path := range rule.HTTP.Paths {
		// We want to skip probes
		for k := range path.Headers {
			if k == header.HashKey {
				continue newbackends
			}
		}

		for _, split := range path.Splits {
			service := types.NamespacedName{
				Name:      split.ServiceName,
				Namespace: split.ServiceNamespace,
			}

			if oldNames.Has(service) {
				continue
			}

			newBackends = append(newBackends, split)
		}
	}

	slices.SortFunc(newBackends, func(a, b netv1alpha1.IngressBackendSplit) int {
		return strings.Compare(a.ServiceName, b.ServiceName)
	})
	return newBackends, oldBackends
}
