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
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmap"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/pkg/kmeta"
)

const redirectHTTPRoutePostfix = "-redirect"

func UpdateProbeHash(r *gatewayapi.HTTPRoute, hash string) {
	// Note: we use indices and references to avoid mutating copies
	for rIdx := range r.Spec.Rules {
		rule := &r.Spec.Rules[rIdx]

		for fIdx := range rule.Filters {
			filter := &rule.Filters[fIdx]

			if filter.Type != gatewayapi.HTTPRouteFilterRequestHeaderModifier {
				continue
			}

			if filter.RequestHeaderModifier == nil {
				continue
			}

			for hIdx := range filter.RequestHeaderModifier.Set {
				h := &filter.RequestHeaderModifier.Set[hIdx]
				if h.Name == header.HashKey {
					h.Value = hash
				}
			}
		}
	}
}

func RemoveEndpointProbes(r *gatewayapi.HTTPRoute) {
	rules := r.Spec.Rules
	r.Spec.Rules = make([]gatewayapi.HTTPRouteRule, 0, len(rules))

	// Remove old endpoint probes
outer:
	for _, rule := range rules {
		for _, match := range rule.Matches {
			if match.Path != nil && match.Path.Value != nil &&
				strings.HasPrefix(*match.Path.Value, "/.well-known/knative") {
				continue outer
			}
			r.Spec.Rules = append(r.Spec.Rules, rule)
		}
	}
}

func AddEndpointProbe(r *gatewayapi.HTTPRoute, hash string, backend netv1alpha1.IngressBackendSplit) {
	rule := gatewayapi.HTTPRouteRule{
		Matches: []gatewayapi.HTTPRouteMatch{{
			Path: &gatewayapi.HTTPPathMatch{
				Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
				Value: ptr.To(fmt.Sprintf("/.well-known/knative/revision/%s/%s", backend.ServiceNamespace, backend.ServiceName)),
			},
			Headers: []gatewayapi.HTTPHeaderMatch{{
				Type:  ptr.To(gatewayapi.HeaderMatchExact),
				Name:  header.HashKey,
				Value: header.HashValueOverride,
			}},
		}},
		Filters: []gatewayapi.HTTPRouteFilter{{
			Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
				Set: []gatewayapi.HTTPHeader{{
					Name:  header.HashKey,
					Value: hash,
				}},
			},
		}},
		BackendRefs: []gatewayapi.HTTPBackendRef{{
			BackendRef: gatewayapi.BackendRef{
				Weight: ptr.To[int32](100),
				BackendObjectReference: gatewayapi.BackendObjectReference{
					Group: ptr.To[gatewayapi.Group](""),
					Kind:  ptr.To[gatewayapi.Kind]("Service"),
					Name:  gatewayapi.ObjectName(backend.ServiceName),
					Port:  ptr.To[gatewayapi.PortNumber](gatewayapi.PortNumber(backend.ServicePort.IntValue())),
				},
			},
		}},
	}

	if len(backend.AppendHeaders) > 0 {
		headers := make([]gatewayapi.HTTPHeader, 0, len(backend.AppendHeaders))

		for k, v := range backend.AppendHeaders {
			headers = append(headers, gatewayapi.HTTPHeader{
				Name:  gatewayapi.HTTPHeaderName(k),
				Value: v,
			})
		}

		slices.SortFunc(headers, compareHTTPHeader)

		rule.BackendRefs[0].Filters = append(rule.BackendRefs[0].Filters,
			gatewayapi.HTTPRouteFilter{
				Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: headers,
				},
			},
		)
	}

	r.Spec.Rules = append(r.Spec.Rules, rule)
}

func AddOldBackend(r *gatewayapi.HTTPRoute, hash string, old gatewayapi.HTTPBackendRef) {
	backend := *old.DeepCopy()
	backend.Weight = ptr.To[int32](100)

	// KIngress only supports AppendHeaders so there's only this filter
	for _, filters := range backend.Filters {
		if filters.RequestHeaderModifier != nil {

			slices.SortFunc(filters.RequestHeaderModifier.Set, func(a, b gatewayapi.HTTPHeader) int {
				return strings.Compare(string(a.Name), string(b.Name))
			})
		}
	}

	rule := gatewayapi.HTTPRouteRule{
		Matches: []gatewayapi.HTTPRouteMatch{{
			Path: &gatewayapi.HTTPPathMatch{
				Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
				Value: ptr.To(fmt.Sprintf("/.well-known/knative/revision/%s/%s", r.Namespace, backend.Name)),
			},
			Headers: []gatewayapi.HTTPHeaderMatch{{
				Type:  ptr.To(gatewayapi.HeaderMatchExact),
				Name:  header.HashKey,
				Value: header.HashValueOverride,
			}},
		}},
		Filters: []gatewayapi.HTTPRouteFilter{{
			Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
				Set: []gatewayapi.HTTPHeader{{
					Name:  header.HashKey,
					Value: hash,
				}},
			},
		}},
		BackendRefs: []gatewayapi.HTTPBackendRef{backend},
	}

	r.Spec.Rules = append(r.Spec.Rules, rule)
}

func HTTPRouteKey(ing *netv1alpha1.Ingress, rule *netv1alpha1.IngressRule) types.NamespacedName {
	return types.NamespacedName{
		Name:      LongestHost(rule.Hosts),
		Namespace: ing.Namespace,
	}
}

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(ctx context.Context, ing *netv1alpha1.Ingress, rule *netv1alpha1.IngressRule, sectionName *gatewayapi.SectionName) (*gatewayapi.HTTPRoute, error) {

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
		Spec: makeHTTPRouteSpec(ctx, rule, sectionName),
	}, nil
}

// MakeRedirectHTTPRoute creates a HTTPRoute with a redirection filter.
func MakeRedirectHTTPRoute(
	ctx context.Context,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gatewayapi.HTTPRoute, error) {
	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts) + redirectHTTPRoutePostfix,
			Namespace: ing.Namespace,
			Labels: kmap.Union(ing.Labels, map[string]string{
				networking.VisibilityLabelKey: "",
			}),
			Annotations: kmap.Filter(ing.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: makeRedirectHTTPRouteSpec(ctx, rule),
	}, nil
}

func makeHTTPRouteSpec(ctx context.Context, rule *netv1alpha1.IngressRule, sectionName *gatewayapi.SectionName) gatewayapi.HTTPRouteSpec {

	hostnames := make([]gatewayapi.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gatewayapi.Hostname(hostname))
	}

	pluginConfig := config.FromContext(ctx).GatewayPlugin

	var gateway config.Gateway

	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		gateway = pluginConfig.LocalGateway()
	} else {
		gateway = pluginConfig.ExternalGateway()
	}

	rules := makeHTTPRouteRule(gateway, rule)

	gatewayRef := gatewayapi.ParentReference{
		Group:     (*gatewayapi.Group)(&gatewayapi.GroupVersion.Group),
		Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
		Namespace: ptr.To(gatewayapi.Namespace(gateway.Namespace)),
		Name:      gatewayapi.ObjectName(gateway.Name),
	}

	if sectionName != nil {
		gatewayRef.SectionName = sectionName
	}

	return gatewayapi.HTTPRouteSpec{
		Hostnames: hostnames,
		Rules:     rules,
		CommonRouteSpec: gatewayapi.CommonRouteSpec{ParentRefs: []gatewayapi.ParentReference{
			gatewayRef,
		}},
	}
}

func makeHTTPRouteRule(gw config.Gateway, rule *netv1alpha1.IngressRule) []gatewayapi.HTTPRouteRule {
	rules := []gatewayapi.HTTPRouteRule{}

	for _, path := range rule.HTTP.Paths {
		path := path
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
			slices.SortFunc(headers, compareHTTPHeader)

			preFilters = []gatewayapi.HTTPRouteFilter{{
				Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: headers,
				}}}
		}

		if path.RewriteHost != "" {
			preFilters = append(preFilters, gatewayapi.HTTPRouteFilter{
				Type: gatewayapi.HTTPRouteFilterURLRewrite,
				URLRewrite: &gatewayapi.HTTPURLRewriteFilter{
					Hostname: (*gatewayapi.PreciseHostname)(&path.RewriteHost),
				},
			})
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
			slices.SortFunc(headers, compareHTTPHeader)

			name := split.ServiceName
			backendRef := gatewayapi.HTTPBackendRef{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Group: (*gatewayapi.Group)(ptr.To("")),
						Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
						Port:  ptr.To(gatewayapi.PortNumber(split.ServicePort.IntValue())),
						Name:  gatewayapi.ObjectName(name),
					},
					Weight: ptr.To(int32(split.Percent)),
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
			Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
			Value: ptr.To(pathPrefix),
		}

		var headerMatchList []gatewayapi.HTTPHeaderMatch
		for k, v := range path.Headers {
			headerMatch := gatewayapi.HTTPHeaderMatch{
				Type:  ptr.To(gatewayapi.HeaderMatchExact),
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

		if gw.SupportedFeatures.Has(features.SupportHTTPRouteRequestTimeout) {
			rule.Timeouts = &gatewayapi.HTTPRouteTimeouts{
				Request: ptr.To[gatewayapi.Duration]("0s"),
			}
		}

		rules = append(rules, rule)
	}
	return rules
}

func makeRedirectHTTPRouteSpec(
	ctx context.Context,
	rule *netv1alpha1.IngressRule,
) gatewayapi.HTTPRouteSpec {
	hostnames := make([]gatewayapi.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gatewayapi.Hostname(hostname))
	}

	pluginConfig := config.FromContext(ctx).GatewayPlugin

	var gateway config.Gateway

	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		gateway = pluginConfig.LocalGateway()
	} else {
		gateway = pluginConfig.ExternalGateway()
	}

	rules := makeRedirectHTTPRouteRule(rule)

	gatewayRef := gatewayapi.ParentReference{
		Group:     (*gatewayapi.Group)(&gatewayapi.GroupVersion.Group),
		Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
		Namespace: ptr.To(gatewayapi.Namespace(gateway.Namespace)),
		Name:      gatewayapi.ObjectName(gateway.Name),
		// Redirect routes are only added on to the http listener
		SectionName: ptr.To(gatewayapi.SectionName(gateway.HTTPListenerName)),
	}

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
				Type: gatewayapi.HTTPRouteFilterRequestRedirect,
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
		Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
		Value: ptr.To(pathPrefix),
	}

	var headerMatchList []gatewayapi.HTTPHeaderMatch
	for k, v := range path.Headers {
		headerMatch := gatewayapi.HTTPHeaderMatch{
			Type:  ptr.To(gatewayapi.HeaderMatchExact),
			Name:  gatewayapi.HTTPHeaderName(k),
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

func compareHTTPHeader(a, b gatewayapi.HTTPHeader) int {
	return strings.Compare(string(a.Name), string(b.Name))
}
