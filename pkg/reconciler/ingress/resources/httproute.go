/*
Copyright 2020 The Knative Authors

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

package resources

import (
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

// MakeHTTPRoutes creates HTTPRoute to control an HTTP traffic.
func MakeHTTPRoutes(ing *v1alpha1.Ingress) []*servicev1alpha1.HTTPRoute {
	return makeHTTPRoute(ing)
}

func makeHTTPRoute(ing *v1alpha1.Ingress) []*servicev1alpha1.HTTPRoute {
	httpRoutes := []*servicev1alpha1.HTTPRoute{}

	for _, rule := range ing.Spec.Rules {
		var rules []servicev1alpha1.HTTPRouteRule
		for _, path := range rule.HTTP.Paths {
			var forwards []servicev1alpha1.HTTPRouteForwardTo
			var preFilters []servicev1alpha1.HTTPRouteFilter
			if path.AppendHeaders != nil {
				preFilters = []servicev1alpha1.HTTPRouteFilter{{
					Type: servicev1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &servicev1alpha1.HTTPRequestHeaderFilter{
						Add: path.AppendHeaders,
					}}}
			}

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

			rule := servicev1alpha1.HTTPRouteRule{
				ForwardTo: forwards,
				Filters:   preFilters,
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

		hrName := ing.Name
		if rule.Visibility == v1alpha1.IngressVisibilityClusterLocal {
			hrName = ing.Name + "-private"
		}

		httpRoute := &servicev1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:            hrName,
				Namespace:       ing.Namespace,
				OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
				Labels:          ing.GetLabels(),
				Annotations:     ing.GetAnnotations(),
			},
			Spec: spec,
		}
		httpRoutes = append(httpRoutes, httpRoute)

	}
	return httpRoutes
}
