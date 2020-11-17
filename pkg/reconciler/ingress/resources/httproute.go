package resources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"
)

const (
	// ServingGroupName is the group name for Knative serving labels
	// and annotations
	ServingGroupName = "serving.knative.dev"
	// RouteLabelKey is the label key attached to a Configuration
	// indicating by which Route it is configured as traffic target.
	// The key is also attached to Revision resources to indicate they
	// are directly referenced by a Route, or are a child of a
	// Configuration which is referenced by a Route.  The key can also
	// be attached to Ingress resources to indicate which Route
	// triggered their creation.  The key is also attached to k8s
	// Service resources to indicate which Route triggered their
	// creation.
	RouteLabelKey = ServingGroupName + "/route"
	// RouteNamespaceLabelKey is the label key attached to a Ingress
	// by a Route to indicate which namespace the Route was created in.
	RouteNamespaceLabelKey = ServingGroupName + "/routeNamespace"
)

const V2IngressClassName = "ingressv2.ingress.networking.knative.dev"

// MakeHTTPRoutes creates ...
func MakeHTTPRoutes(ctx context.Context, ing *v1alpha1.Ingress) ([]*servicev1alpha1.HTTPRoute, error) {
	httpRoutes, err := makeHTTPRoute(ctx, ing)
	if err != nil {
		return nil, fmt.Errorf("todo: %w", err)
	}

	return httpRoutes, nil
}

// makeHTTPRoutes creates ...
func makeHTTPRoute(ctx context.Context, ing *v1alpha1.Ingress) ([]*servicev1alpha1.HTTPRoute, error) {
	var httpRoutes []*servicev1alpha1.HTTPRoute

	for _, rule := range ing.Spec.Rules {
		//TODO: https://github.com/istio/istio/issues/29078
		if rule.Visibility == v1alpha1.IngressVisibilityClusterLocal {
			continue
		}
		var rules []servicev1alpha1.HTTPRouteRule
		for _, path := range rule.HTTP.Paths {
			var forwards []servicev1alpha1.HTTPRouteForwardTo
			var filters []servicev1alpha1.HTTPRouteFilter
			// TODO:
			// When requestHeaderModifier in forwards.filters does not work when weight 100.
			// see: https://github.com/istio/istio/issues/29111
			if len(path.Splits) == 1 {
				split := path.Splits[0]
				name := split.IngressBackend.ServiceName
				forward := servicev1alpha1.HTTPRouteForwardTo{
					Port:        servicev1alpha1.PortNumber(split.ServicePort.IntValue()),
					ServiceName: &name,
					Weight:      int32(split.Percent),
				}
				forwards = append(forwards, forward)
				filters = []servicev1alpha1.HTTPRouteFilter{{
					Type: servicev1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &servicev1alpha1.HTTPRequestHeaderFilter{
						Add: split.AppendHeaders,
					}}}
			} else {
				for _, split := range path.Splits {
					name := split.IngressBackend.ServiceName
					forward := servicev1alpha1.HTTPRouteForwardTo{
						Port:        servicev1alpha1.PortNumber(split.ServicePort.IntValue()),
						ServiceName: &name,
						Weight:      int32(split.Percent),
						Filters: []servicev1alpha1.HTTPRouteFilter{{
							Type: servicev1alpha1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &servicev1alpha1.HTTPRequestHeaderFilter{
								Add: split.AppendHeaders,
							}},
						}}
					forwards = append(forwards, forward)
				}
			}
			rule := servicev1alpha1.HTTPRouteRule{
				ForwardTo: forwards,
				Filters:   filters,
			}
			rules = append(rules, rule)
		}

		var hostnames []servicev1alpha1.Hostname
		for _, host := range rule.Hosts {
			hostnames = append(hostnames, servicev1alpha1.Hostname(host))
		}
		spec := servicev1alpha1.HTTPRouteSpec{
			Hostnames: hostnames,
			Rules:     rules,
		}

		httpRoute := &servicev1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:            ing.Name, // TODO: Should not be same name with ingress?
				Namespace:       ing.Namespace,
				OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
				Labels:          ing.GetLabels(),
				Annotations:     ing.GetAnnotations(),
			},
			Spec: spec,
		}
		httpRoutes = append(httpRoutes, httpRoute)

	}
	return httpRoutes, nil
}
