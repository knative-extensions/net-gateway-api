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
	"net/http"
	"sort"

	"knative.dev/pkg/kmap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"

	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
)

const redirectHTTPRoutePostfix = "-redirect"

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
	gatewayRef gatewayapi.ParentReference,
) (*gatewayapi.HTTPRoute, error) {

	visibility := ""
	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		visibility = "cluster-local"
	}

	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts),
			Namespace: ing.Namespace,
			Labels: kmap.Union(ing.Labels, map[string]string{
				networking.VisibilityLabelKey: visibility,
			}),
			Annotations: kmap.Filter(ing.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: makeHTTPRouteSpec(rule, gatewayRef),
	}, nil
}

func makeHTTPRouteSpec(
	rule *netv1alpha1.IngressRule,
	gatewayRef gatewayapi.ParentReference,
) gatewayapi.HTTPRouteSpec {

	hostnames := make([]gatewayapi.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gatewayapi.Hostname(hostname))
	}

	rules := makeHTTPRouteRule(rule)

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
					Name:  gatewayapiv1.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			preFilters = []gatewayapi.HTTPRouteFilter{{
				Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: headers,
				}}}
		}

		if path.RewriteHost != "" {
			preFilters = append(preFilters, gatewayapi.HTTPRouteFilter{
				Type: gatewayapiv1.HTTPRouteFilterURLRewrite,
				URLRewrite: &gatewayapi.HTTPURLRewriteFilter{
					Hostname: (*gatewayapi.PreciseHostname)(&path.RewriteHost),
				},
			})
		}

		for _, split := range path.Splits {
			headers := []gatewayapi.HTTPHeader{}
			for k, v := range split.AppendHeaders {
				header := gatewayapi.HTTPHeader{
					Name:  gatewayapiv1.HTTPHeaderName(k),
					Value: v,
				}
				headers = append(headers, header)
			}

			// Sort HTTPHeader as the order is random.
			sort.Sort(HTTPHeaderList(headers))

			name := split.ServiceName
			backendRef := gatewayapi.HTTPBackendRef{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Group: (*gatewayapi.Group)(ptr.To("")),
						Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
						Port:  ptr.To(gatewayapiv1.PortNumber(split.ServicePort.IntValue())),
						Name:  gatewayapi.ObjectName(name),
					},
					Weight: ptr.To(int32(split.Percent)),
				},
				Filters: []gatewayapi.HTTPRouteFilter{{
					Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
						Set: headers,
					}},
				}}
			backendRefs = append(backendRefs, backendRef)
		}

		matches := matchesFromRulePath(path)
		rule := gatewayapi.HTTPRouteRule{
			BackendRefs: backendRefs,
			Filters:     preFilters,
			Matches:     matches,
		}
		rules = append(rules, rule)
	}
	return rules
}

// MakeRedirectHTTPRoute creates a HTTPRoute with a redirection filter.
func MakeRedirectHTTPRoute(
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
	gatewayRef gatewayapi.ParentReference,
) (*gatewayapi.HTTPRoute, error) {

	visibility := ""
	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		visibility = "cluster-local"
	}

	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts) + redirectHTTPRoutePostfix,
			Namespace: ing.Namespace,
			Labels: kmap.Union(ing.Labels, map[string]string{
				networking.VisibilityLabelKey: visibility,
			}),
			Annotations: kmap.Filter(ing.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: makeRedirectHTTPRouteSpec(rule, gatewayRef),
	}, nil
}

func makeRedirectHTTPRouteSpec(
	rule *netv1alpha1.IngressRule,
	gatewayRef gatewayapi.ParentReference,
) gatewayapi.HTTPRouteSpec {
	hostnames := make([]gatewayapi.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gatewayapi.Hostname(hostname))
	}

	rules := makeRedirectHTTPRouteRule(rule)

	return gatewayapi.HTTPRouteSpec{
		Hostnames: hostnames,
		Rules:     rules,
		CommonRouteSpec: gatewayapi.CommonRouteSpec{ParentRefs: []gatewayapi.ParentReference{
			gatewayRef,
		}},
	}
}

func makeRedirectHTTPRouteRule(rule *netv1alpha1.IngressRule) []gatewayapi.HTTPRouteRule {
	rules := []gatewayapi.HTTPRouteRule{}

	for _, path := range rule.HTTP.Paths {
		preFilters := []gatewayapi.HTTPRouteFilter{
			{
				Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
					Scheme:     ptr.To("https"),
					Port:       ptr.To(gatewayapi.PortNumber(443)),
					StatusCode: ptr.To(http.StatusMovedPermanently),
				},
			}}

		matches := matchesFromRulePath(path)
		rule := gatewayapi.HTTPRouteRule{
			Filters: preFilters,
			Matches: matches,
		}
		rules = append(rules, rule)
	}
	return rules
}

func matchesFromRulePath(path netv1alpha1.HTTPIngressPath) []gatewayapi.HTTPRouteMatch {
	pathPrefix := "/"
	if path.Path != "" {
		pathPrefix = path.Path
	}
	pathMatch := gatewayapi.HTTPPathMatch{
		Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
		Value: ptr.To(pathPrefix),
	}

	var headerMatchList []gatewayapi.HTTPHeaderMatch
	for k, v := range path.Headers {
		headerMatch := gatewayapi.HTTPHeaderMatch{
			Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
			Name:  gatewayapiv1.HTTPHeaderName(k),
			Value: v.Exact,
		}
		headerMatchList = append(headerMatchList, headerMatch)
	}

	// Sort HTTPHeaderMatch as the order is random.
	sort.Sort(HTTPHeaderMatchList(headerMatchList))

	return []gatewayapi.HTTPRouteMatch{{Path: &pathMatch, Headers: headerMatchList}}
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
