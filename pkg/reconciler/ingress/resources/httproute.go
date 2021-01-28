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
