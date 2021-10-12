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
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"knative.dev/networking/pkg"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ctx context.Context,
	ing *netv1alpha1.Ingress,
	rule *netv1alpha1.IngressRule,
) (*gwv1alpha1.HTTPRoute, error) {

	visibility := ""
	if rule.Visibility == netv1alpha1.IngressVisibilityClusterLocal {
		visibility = "cluster-local"
	}

	return &gwv1alpha1.HTTPRoute{
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
) gwv1alpha1.HTTPRouteSpec {

	hostnames := make([]gwv1alpha1.Hostname, 0, len(rule.Hosts))
	for _, hostname := range rule.Hosts {
		hostnames = append(hostnames, gwv1alpha1.Hostname(hostname))
	}

	rules := makeHTTPRouteRule(rule)

	gatewayConfig := config.FromContext(ctx).Gateway
	ns, name, _ := gatewayConfig.LookupGateway(rule.Visibility)

	gatewayRef := gwv1alpha1.GatewayReference{
		Namespace: ns,
		Name:      name,
	}

	return gwv1alpha1.HTTPRouteSpec{
		Hostnames: hostnames,
		Rules:     rules,
		Gateways: &gwv1alpha1.RouteGateways{
			Allow:       gatewayAllowTypePtr(gwv1alpha1.GatewayAllowFromList),
			GatewayRefs: []gwv1alpha1.GatewayReference{gatewayRef},
		},
	}
}

func makeHTTPRouteRule(rule *netv1alpha1.IngressRule) []gwv1alpha1.HTTPRouteRule {
	rules := []gwv1alpha1.HTTPRouteRule{}

	for _, path := range rule.HTTP.Paths {
		forwards := make([]gwv1alpha1.HTTPRouteForwardTo, 0, len(path.Splits))
		var preFilters []gwv1alpha1.HTTPRouteFilter

		if path.AppendHeaders != nil {
			preFilters = []gwv1alpha1.HTTPRouteFilter{{
				Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
					Set: path.AppendHeaders,
				}}}
		}

		set := map[string]string{}
		if path.RewriteHost != "" {
			set = map[string]string{"Host": path.RewriteHost, ":Authority": path.RewriteHost}
		}
		for _, split := range path.Splits {
			name := split.IngressBackend.ServiceName
			forward := gwv1alpha1.HTTPRouteForwardTo{
				Port:        portNumPtr(split.ServicePort.IntValue()),
				ServiceName: &name,
				Weight:      pointer.Int32Ptr(int32(split.Percent)),
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: kmeta.UnionMaps(split.AppendHeaders, set),
					}},
				}}
			forwards = append(forwards, forward)
		}

		pathPrefix := "/"
		if path.Path != "" {
			pathPrefix = path.Path
		}
		pathMatch := gwv1alpha1.HTTPPathMatch{
			Type:  pathMatchTypePtr(gwv1alpha1.PathMatchPrefix),
			Value: pointer.StringPtr(pathPrefix),
		}

		var headersMatch *gwv1alpha1.HTTPHeaderMatch
		if path.Headers != nil {
			header := map[string]string{}
			for k, v := range path.Headers {
				header[k] = v.Exact
			}
			headersMatch = &gwv1alpha1.HTTPHeaderMatch{
				Type:   headerMatchTypePtr(gwv1alpha1.HeaderMatchExact),
				Values: header,
			}
		}

		matches := []gwv1alpha1.HTTPRouteMatch{{Path: &pathMatch, Headers: headersMatch}}

		rule := gwv1alpha1.HTTPRouteRule{
			ForwardTo: forwards,
			Filters:   preFilters,
			Matches:   matches,
		}
		rules = append(rules, rule)
	}
	return rules
}
