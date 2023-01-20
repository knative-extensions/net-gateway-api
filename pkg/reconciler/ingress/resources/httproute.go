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
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
)

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ctx context.Context,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayapi.HTTPRoute, error) {

	visibility := ""
	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		visibility = "cluster-local"
	}

	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts),
			Namespace: ing.Namespace,
			Labels: kmeta.UnionMaps(ing.Labels, map[string]string{
				networking.VisibilityLabelKey: visibility,
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
) gatewayapi.HTTPRouteSpec {

	hostnames := make([]gatewayapi.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gatewayapi.Hostname(hostname))
	}

	rules := makeHTTPRouteRule(rule)

	gatewayConfig := config.FromContext(ctx).Gateway
	namespacedNameGateway := gatewayConfig.Gateways[rule.Visibility].Gateway

	gatewayRef := gatewayapi.ParentReference{
		Group:     (*gatewayapi.Group)(&gatewayapi.GroupVersion.Group),
		Kind:      (*gatewayapi.Kind)(pointer.String("Gateway")),
		Namespace: ptr(gatewayapi.Namespace(namespacedNameGateway.Namespace)),
		Name:      gatewayapi.ObjectName(namespacedNameGateway.Name),
	}

	return gatewayapi.HTTPRouteSpec{
		Hostnames: hostnames,
		Rules:     rules,
		CommonRouteSpec: gatewayapi.CommonRouteSpec{ParentRefs: []gatewayapi.ParentReference{
			gatewayRef,
		}},
	}
}

func makeHTTPRouteRule(rule *netv1alpha1.IngressRule) []gatewayapi.HTTPRouteRule {
	rules := []gatewayapi.HTTPRouteRule{}

	for _, path := range rule.HTTP.Paths {
		backendRefs := make([]gatewayapi.HTTPBackendRef, 0, len(path.Splits))
		var preFilters []gatewayapi.HTTPRouteFilter

		if path.AppendHeaders != nil {
			headers := []gatewayapi.HTTPHeader{}
			for k, v := range path.AppendHeaders {
				header := gatewayapi.HTTPHeader{
					Name:  gatewayapi.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			preFilters = []gatewayapi.HTTPRouteFilter{{
				Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: headers,
				}}}
		}

		for _, split := range path.Splits {
			headers := []gatewayapi.HTTPHeader{}
			for k, v := range split.AppendHeaders {
				header := gatewayapi.HTTPHeader{
					Name:  gatewayapi.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			name := split.IngressBackend.ServiceName
			backendRef := gatewayapi.HTTPBackendRef{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Group: (*gatewayapi.Group)(pointer.String("")),
						Kind:  (*gatewayapi.Kind)(pointer.String("Service")),
						Port:  portNumPtr(split.ServicePort.IntValue()),
						Name:  gatewayapi.ObjectName(name),
					},
					Weight: pointer.Int32(int32(split.Percent)),
				},
				Filters: []gatewayapi.HTTPRouteFilter{{
					Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
						Set: headers,
					}},
				}}
			backendRefs = append(backendRefs, backendRef)
		}

		pathPrefix := "/"
		if path.Path != "" {
			pathPrefix = path.Path
		}
		pathMatch := gatewayapi.HTTPPathMatch{
			Type:  ptr(gatewayapi.PathMatchPathPrefix),
			Value: pointer.String(pathPrefix),
		}

		var headerMatchList []gatewayapi.HTTPHeaderMatch
		for k, v := range path.Headers {
			headerMatch := gatewayapi.HTTPHeaderMatch{
				Type:  ptr(gatewayapi.HeaderMatchExact),
				Name:  gatewayapi.HTTPHeaderName(k),
				Value: v.Exact,
			}
			headerMatchList = append(headerMatchList, headerMatch)
		}

		// Sort HTTPHeaderMatch as the order is random.
		sort.Sort(HTTPHeaderMatchList(headerMatchList))

		matches := []gatewayapi.HTTPRouteMatch{{Path: &pathMatch, Headers: headerMatchList}}

		rule := gatewayapi.HTTPRouteRule{
			BackendRefs: backendRefs,
			Filters:     preFilters,
			Matches:     matches,
		}
		rules = append(rules, rule)
	}
	return rules
}

type HTTPHeaderList []gatewayapi.HTTPHeader

func (h HTTPHeaderList) Len() int {
	return len(h)
}

func (h HTTPHeaderList) Less(i, j int) bool {
	return h[i].Name > h[j].Name
}

func (h HTTPHeaderList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

type HTTPHeaderMatchList []gatewayapi.HTTPHeaderMatch

func (h HTTPHeaderMatchList) Len() int {
	return len(h)
}

func (h HTTPHeaderMatchList) Less(i, j int) bool {
	return h[i].Name > h[j].Name
}

func (h HTTPHeaderMatchList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
