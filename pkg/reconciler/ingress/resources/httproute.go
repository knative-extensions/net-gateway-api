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

package resources

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
)

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ctx context.Context,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gwv1alpha2.HTTPRoute, error) {

	visibility := ""
	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		visibility = "cluster-local"
	}

	return &gwv1alpha2.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts),
			Namespace: ing.Namespace,
			Labels: kmeta.UnionMaps(ing.Labels, map[string]string{
				pkg.VisibilityLabelKey: visibility,
			}),
			Annotations: kmeta.FilterMap(ing.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: makeHTTPRouteSpec(ctx, rule),
	}, nil
}

func makeHTTPRouteSpec(
	ctx context.Context,
	rule *netv1alpha1.IngressRule,
) gwv1alpha2.HTTPRouteSpec {

	hostnames := make([]gwv1alpha2.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gwv1alpha2.Hostname(hostname))
	}

	rules := makeHTTPRouteRule(rule)

	gatewayConfig := config.FromContext(ctx).Gateway
	namespacedNameGateway := gatewayConfig.Gateways[rule.Visibility].Gateway

	gatewayRef := gwv1alpha2.ParentRef{
		Namespace: namespacePtr(gwv1alpha2.Namespace(namespacedNameGateway.Namespace)),
		Name:      gwv1alpha2.ObjectName(namespacedNameGateway.Name),
	}

	return gwv1alpha2.HTTPRouteSpec{
		Hostnames: hostnames,
		Rules:     rules,
		CommonRouteSpec: gwv1alpha2.CommonRouteSpec{ParentRefs: []gwv1alpha2.ParentRef{
			gatewayRef,
		}},
	}
}

func makeHTTPRouteRule(rule *netv1alpha1.IngressRule) []gwv1alpha2.HTTPRouteRule {
	rules := []gwv1alpha2.HTTPRouteRule{}

	for _, path := range rule.HTTP.Paths {
		backendRefs := make([]gwv1alpha2.HTTPBackendRef, 0, len(path.Splits))
		var preFilters []gwv1alpha2.HTTPRouteFilter

		if path.AppendHeaders != nil {
			headers := []gwv1alpha2.HTTPHeader{}
			for k, v := range path.AppendHeaders {
				header := gwv1alpha2.HTTPHeader{
					Name:  gwv1alpha2.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			preFilters = []gwv1alpha2.HTTPRouteFilter{{
				Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
					Set: headers,
				}}}
		}

		for _, split := range path.Splits {
			headers := []gwv1alpha2.HTTPHeader{}
			for k, v := range split.AppendHeaders {
				header := gwv1alpha2.HTTPHeader{
					Name:  gwv1alpha2.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			name := split.IngressBackend.ServiceName
			backendRef := gwv1alpha2.HTTPBackendRef{
				BackendRef: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Port: portNumPtr(split.ServicePort.IntValue()),
						Name: gwv1alpha2.ObjectName(name),
					},
					Weight: pointer.Int32Ptr(int32(split.Percent)),
				},
				Filters: []gwv1alpha2.HTTPRouteFilter{{
					Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
						Set: headers,
					}},
				}}
			backendRefs = append(backendRefs, backendRef)
		}

		pathPrefix := "/"
		if path.Path != "" {
			pathPrefix = path.Path
		}
		pathMatch := gwv1alpha2.HTTPPathMatch{
			Type:  pathMatchTypePtr(gwv1alpha2.PathMatchPathPrefix),
			Value: pointer.StringPtr(pathPrefix),
		}

		headerMatchList := []gwv1alpha2.HTTPHeaderMatch{}
		for k, v := range path.Headers {
			headerMatch := gwv1alpha2.HTTPHeaderMatch{
				Type:  headerMatchTypePtr(gwv1alpha2.HeaderMatchExact),
				Name:  gwv1alpha2.HTTPHeaderName(k),
				Value: v.Exact,
			}
			headerMatchList = append(headerMatchList, headerMatch)
		}

		// Sort HTTPHeaderMatch as the order is random.
		sort.Sort(HTTPHeaderMatchList(headerMatchList))

		matches := []gwv1alpha2.HTTPRouteMatch{{Path: &pathMatch, Headers: headerMatchList}}

		rule := gwv1alpha2.HTTPRouteRule{
			BackendRefs: backendRefs,
			Filters:     preFilters,
			Matches:     matches,
		}
		rules = append(rules, rule)
	}
	return rules
}

type HTTPHeaderList []gwv1alpha2.HTTPHeader

func (h HTTPHeaderList) Len() int {
	return len(h)
}

func (h HTTPHeaderList) Less(i, j int) bool {
	return h[i].Name > h[j].Name
}

func (h HTTPHeaderList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

type HTTPHeaderMatchList []gwv1alpha2.HTTPHeaderMatch

func (h HTTPHeaderMatchList) Len() int {
	return len(h)
}

func (h HTTPHeaderMatchList) Less(i, j int) bool {
	return h[i].Name > h[j].Name
}

func (h HTTPHeaderMatchList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
