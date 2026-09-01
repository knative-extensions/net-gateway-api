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
	"slices"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/pkg/kmeta"
)

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

// AddEndpointProbe copies a production route rule and narrows it to the
// backend at backendIndex. Only the rule name, match, backend weight, and probe
// hash differ from the production rule, so route and backend filters stay in
// sync.
func AddEndpointProbe(
	r *gatewayapi.HTTPRoute,
	hash string,
	sourceRule gatewayapi.HTTPRouteRule,
	backendIndex int,
) {
	rule := sourceRule.DeepCopy()
	backend := rule.BackendRefs[backendIndex]
	backend.Weight = ptr.To[int32](100)
	rule.Name = nil

	backendNamespace := r.Namespace
	if backend.Namespace != nil {
		backendNamespace = string(*backend.Namespace)
	}

	rule.Matches = []gatewayapi.HTTPRouteMatch{{
		Path: &gatewayapi.HTTPPathMatch{
			Type: ptr.To(gatewayapi.PathMatchPathPrefix),
			Value: ptr.To(fmt.Sprintf("/.well-known/knative/revision/%s/%s",
				backendNamespace, backend.Name)),
		},
		Headers: []gatewayapi.HTTPHeaderMatch{{
			Type:  ptr.To(gatewayapi.HeaderMatchExact),
			Name:  header.HashKey,
			Value: header.HashValueOverride,
		}},
	}}
	rule.BackendRefs = []gatewayapi.HTTPBackendRef{backend}
	setProbeHash(rule, hash)

	r.Spec.Rules = append(r.Spec.Rules, *rule)
}

func setProbeHash(rule *gatewayapi.HTTPRouteRule, hash string) {
	for i := range rule.Filters {
		filter := &rule.Filters[i]
		if filter.Type != gatewayapi.HTTPRouteFilterRequestHeaderModifier ||
			filter.RequestHeaderModifier == nil {
			continue
		}

		for j := range filter.RequestHeaderModifier.Set {
			if filter.RequestHeaderModifier.Set[j].Name == header.HashKey {
				filter.RequestHeaderModifier.Set[j].Value = hash
				return
			}
		}

		filter.RequestHeaderModifier.Set = append(filter.RequestHeaderModifier.Set, gatewayapi.HTTPHeader{
			Name:  header.HashKey,
			Value: hash,
		})
		slices.SortFunc(filter.RequestHeaderModifier.Set, compareHTTPHeader)
		return
	}

	probeFilter := gatewayapi.HTTPRouteFilter{
		Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
			Set: []gatewayapi.HTTPHeader{{
				Name:  header.HashKey,
				Value: hash,
			}},
		},
	}
	rule.Filters = append([]gatewayapi.HTTPRouteFilter{probeFilter}, rule.Filters...)
}

func appendHostRewriteFilter(filters []gatewayapi.HTTPRouteFilter, host string) []gatewayapi.HTTPRouteFilter {
	if host == "" {
		return filters
	}

	return append(filters, gatewayapi.HTTPRouteFilter{
		Type: gatewayapi.HTTPRouteFilterURLRewrite,
		URLRewrite: &gatewayapi.HTTPURLRewriteFilter{
			Hostname: ptr.To(gatewayapi.PreciseHostname(host)),
		},
	})
}

func HTTPRouteKey(ing *netv1alpha1.Ingress, rule *netv1alpha1.IngressRule) types.NamespacedName {
	return types.NamespacedName{
		Name:      LongestHost(rule.Hosts),
		Namespace: ing.Namespace,
	}
}

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

	extraLabels := map[string]string{
		networking.IngressLabelKey:    ing.Name,
		networking.VisibilityLabelKey: visibility,
	}

	if tag := tagForHost(ing, rule); tag != "" {
		extraLabels[TagLabelKey] = tag
	}

	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(rule.Hosts),
			Namespace: ing.Namespace,
			Labels:    kmeta.UnionMaps(ing.Labels, extraLabels),
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
				},
			}}
		}

		preFilters = appendHostRewriteFilter(preFilters, path.RewriteHost)

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
						Name:  gatewayapi.ObjectName(name),
						Group: (*gatewayapi.Group)(ptr.To("")),
						Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
						//nolint:gosec // port numbers are bounded
						Port: ptr.To(gatewayapi.PortNumber(split.ServicePort.IntValue())),
					},
					Weight: ptr.To(int32(split.Percent)), //nolint:gosec // percent is bounded [0,100]
				},
				Filters: []gatewayapi.HTTPRouteFilter{
					{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: headers,
						},
					},
				},
			}
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

// TagLabelKey is the label key used to identify which tag a host-based
// HTTPRoute belongs to. The networking package does not define this constant
// yet, so it is defined locally.
const TagLabelKey = "networking.knative.dev/tag"

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
