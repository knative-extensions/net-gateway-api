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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/http/header"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

type RuleBuilder interface {
	Build() gatewayapi.HTTPRouteRule
}

type HTTPRoute struct {
	Namespace        string
	Name             string
	Hostnames        []string
	Hostname         string
	Rules            []RuleBuilder
	StatusConditions []metav1.Condition
	ClusterLocal     bool
}

func (r HTTPRoute) Build() *gatewayapi.HTTPRoute {
	hostnames := r.Hostnames

	if len(hostnames) == 0 && r.Hostname == "" {
		hostnames = []string{"example.com"}
	}

	if r.Hostname != "" {
		hostnames = append(hostnames, r.Hostname)
	}

	route := gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Annotations: map[string]string{
				networking.IngressClassAnnotationKey: gatewayAPIIngressClassName,
			},
			Labels: map[string]string{
				networking.VisibilityLabelKey: "",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "networking.internal.knative.dev/v1alpha1",
				Kind:               "Ingress",
				Name:               "name",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: gatewayapi.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi.CommonRouteSpec{
				ParentRefs: []gatewayapi.ParentReference{{
					Group:     ptr.To[gatewayapi.Group]("gateway.networking.k8s.io"),
					Kind:      ptr.To[gatewayapi.Kind]("Gateway"),
					Namespace: ptr.To[gatewayapi.Namespace]("istio-system"),
					Name:      "istio-gateway",
				}},
			},
		},
	}

	if r.ClusterLocal {
		route.Labels[networking.VisibilityLabelKey] = "cluster-local"
		route.Spec.CommonRouteSpec.ParentRefs[0].Name = gatewayapi.ObjectName(privateName)
	}

	for _, hostname := range hostnames {
		route.Spec.Hostnames = append(
			route.Spec.Hostnames,
			gatewayapi.Hostname(hostname),
		)
	}

	if route.Status.Parents == nil {
		route.Status.Parents = []gatewayapi.RouteParentStatus{{}}
	}

	route.Status.RouteStatus.Parents[0].Conditions = append(
		route.Status.RouteStatus.Parents[0].Conditions,
		r.StatusConditions...,
	)

	for _, rule := range r.Rules {
		route.Spec.Rules = append(route.Spec.Rules, rule.Build())
	}

	return &route
}

type EndpointProbeRule struct {
	Namespace string
	Name      string
	Hash      string
	Path      string
	Port      int
	Headers   []string
}

func (p EndpointProbeRule) Build() gatewayapi.HTTPRouteRule {
	path := p.Path
	if path == "" {
		path = "/"
	}
	rule := gatewayapi.HTTPRouteRule{
		Matches: []gatewayapi.HTTPRouteMatch{{
			Path: &gatewayapi.HTTPPathMatch{
				Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
				Value: ptr.To(path),
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
					Value: p.Hash,
				}},
			},
		}},
		BackendRefs: []gatewayapi.HTTPBackendRef{{
			Filters: []gatewayapi.HTTPRouteFilter{{
				Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: []gatewayapi.HTTPHeader{{
						Name:  "K-Serving-Namespace",
						Value: p.Namespace,
					}, {
						Name:  "K-Serving-Revision",
						Value: p.Name,
					}},
				},
			}},
			BackendRef: gatewayapi.BackendRef{
				Weight: ptr.To[int32](100),
				BackendObjectReference: gatewayapi.BackendObjectReference{
					Group: ptr.To[gatewayapi.Group](""),
					Kind:  ptr.To[gatewayapi.Kind]("Service"),
					Name:  gatewayapi.ObjectName(p.Name),
					Port:  ptr.To[gatewayapi.PortNumber](gatewayapi.PortNumber(p.Port)),
				},
			},
		}},
	}

	for i := 0; i < len(p.Headers); i += 2 {
		k, v := p.Headers[i], p.Headers[i+1]
		rule.BackendRefs[0].Filters[0].RequestHeaderModifier.Set = append(
			rule.BackendRefs[0].Filters[0].RequestHeaderModifier.Set,
			gatewayapi.HTTPHeader{Name: gatewayapi.HTTPHeaderName(k), Value: v},
		)
	}

	return rule
}

type NormalRule struct {
	Namespace string
	Name      string
	Path      string
	Port      int
	Headers   []string
	Weight    int
}

func (p NormalRule) Build() gatewayapi.HTTPRouteRule {
	path := p.Path
	if path == "" {
		path = "/"
	}
	rule := gatewayapi.HTTPRouteRule{
		Matches: []gatewayapi.HTTPRouteMatch{{
			Path: &gatewayapi.HTTPPathMatch{
				Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
				Value: ptr.To(path),
			},
		}},
		BackendRefs: []gatewayapi.HTTPBackendRef{{
			Filters: []gatewayapi.HTTPRouteFilter{{
				Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
					Set: []gatewayapi.HTTPHeader{{
						Name:  "K-Serving-Namespace",
						Value: p.Namespace,
					}, {
						Name:  "K-Serving-Revision",
						Value: p.Name,
					}},
				},
			}},
			BackendRef: gatewayapi.BackendRef{
				BackendObjectReference: gatewayapi.BackendObjectReference{
					Group: ptr.To[gatewayapi.Group](""),
					Kind:  ptr.To[gatewayapi.Kind]("Service"),
					Name:  gatewayapi.ObjectName(p.Name),
					Port:  ptr.To[gatewayapi.PortNumber](gatewayapi.PortNumber(p.Port)),
				},
				Weight: ptr.To[int32](int32(p.Weight)),
			},
		}},
	}

	for i := 0; i < len(p.Headers); i += 2 {
		k, v := p.Headers[i], p.Headers[i+1]
		rule.BackendRefs[0].Filters[0].RequestHeaderModifier.Set = append(
			rule.BackendRefs[0].Filters[0].RequestHeaderModifier.Set,
			gatewayapi.HTTPHeader{Name: gatewayapi.HTTPHeaderName(k), Value: v},
		)
	}

	return rule
}
