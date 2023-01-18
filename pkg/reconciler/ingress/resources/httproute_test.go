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
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/reconciler"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"
)

const (
	testNamespace    = "test-ns"
	testIngressName  = "test-ingress"
	testGatewayClass = "test-class"
)

var (
	externalHost      = gatewayapi.Hostname(testHosts[0])
	localHostShortest = gatewayapi.Hostname(testLocalHosts[0])
	localHostShort    = gatewayapi.Hostname(testLocalHosts[1])
	localHostFull     = gatewayapi.Hostname(testLocalHosts[2])

	testLocalHosts = []string{
		"hello-example.default",
		"hello-example.default.svc",
		"hello-example.default.svc.cluster.local",
	}

	testHosts = []string{"hello-example.default.example.com"}

	testIngress = &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIngressName,
			Namespace: testNamespace,
			Labels: map[string]string{
				networking.IngressLabelKey: testIngressName,
			},
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{{
				Hosts:      testHosts,
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP: &v1alpha1.HTTPIngressRuleValue{
					Paths: []v1alpha1.HTTPIngressPath{{
						AppendHeaders: map[string]string{
							"Foo": "bar",
						},
						Splits: []v1alpha1.IngressBackendSplit{{
							IngressBackend: v1alpha1.IngressBackend{
								ServiceName:      "goo",
								ServiceNamespace: testNamespace,
								ServicePort:      intstr.FromInt(123),
							},
							Percent: 12,
							AppendHeaders: map[string]string{
								"Baz":   "blah",
								"Bleep": "bloop",
							},
						}, {
							IngressBackend: v1alpha1.IngressBackend{
								ServiceName:      "doo",
								ServiceNamespace: testNamespace,
								ServicePort:      intstr.FromInt(124),
							},
							Percent: 88,
							AppendHeaders: map[string]string{
								"Baz": "blurg",
							},
						}},
					}},
				},
			}},
		},
	}
)

func TestMakeHTTPRoute(t *testing.T) {
	for _, tc := range []struct {
		name         string
		ing          *v1alpha1.Ingress
		expected     []*gatewayapi.HTTPRoute
		changeConfig func(gw *config.Config)
	}{
		{
			name: "single external domain with split and cluster local",
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{
					Rules: []v1alpha1.IngressRule{
						{
							Hosts:      testHosts,
							Visibility: v1alpha1.IngressVisibilityExternalIP,
							HTTP: &v1alpha1.HTTPIngressRuleValue{
								Paths: []v1alpha1.HTTPIngressPath{{
									AppendHeaders: map[string]string{
										"Foo": "bar",
									},
									Splits: []v1alpha1.IngressBackendSplit{{
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "goo",
											ServicePort: intstr.FromInt(123),
										},
										Percent: 12,
										AppendHeaders: map[string]string{
											"Baz":   "blah",
											"Bleep": "bloop",
										},
									}, {
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "doo",
											ServicePort: intstr.FromInt(124),
										},
										Percent: 88,
										AppendHeaders: map[string]string{
											"Baz": "blurg",
										},
									}},
								}},
							},
						}, {
							Hosts:      testLocalHosts,
							Visibility: v1alpha1.IngressVisibilityClusterLocal,
							HTTP: &v1alpha1.HTTPIngressRuleValue{
								Paths: []v1alpha1.HTTPIngressPath{{
									AppendHeaders: map[string]string{
										"Foo": "bar",
									},
									Splits: []v1alpha1.IngressBackendSplit{{
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "goo",
											ServicePort: intstr.FromInt(123),
										},
										Percent: 12,
										AppendHeaders: map[string]string{
											"Bleep": "bloop",
											"Baz":   "blah",
										},
									}, {
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "doo",
											ServicePort: intstr.FromInt(124),
										},
										Percent: 88,
										AppendHeaders: map[string]string{
											"Baz": "blurg",
										},
									}},
								}},
							},
						},
					}},
			},
			expected: []*gatewayapi.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      LongestHost(testHosts),
						Namespace: testNamespace,
						Labels: map[string]string{
							networking.IngressLabelKey:          testIngressName,
							"networking.knative.dev/visibility": "",
						},
						Annotations: map[string]string{},
					},
					Spec: gatewayapi.HTTPRouteSpec{
						Hostnames: []gatewayapi.Hostname{externalHost},
						Rules: []gatewayapi.HTTPRouteRule{{
							BackendRefs: []gatewayapi.HTTPBackendRef{{
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Name:  gatewayapi.ObjectName("goo"),
										Port:  ptr.To[gatewayapi.PortNumber](123),
									},
									Weight: ptr.To(int32(12)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blah",
											},
											{
												Name:  "Bleep",
												Value: "bloop",
											},
										}}}},
							}, {
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapi.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(88)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blurg",
											},
										},
									}}},
							}},
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{
										{
											Name:  "Foo",
											Value: "bar",
										},
									},
								}}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								},
							},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{{
								Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
								Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
								Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
								Name:      gatewayapi.ObjectName("foo"),
							}},
						},
					},
				}, {
					ObjectMeta: metav1.ObjectMeta{
						Name:      LongestHost(testLocalHosts),
						Namespace: testNamespace,
						Labels: map[string]string{
							networking.IngressLabelKey:          testIngressName,
							"networking.knative.dev/visibility": "cluster-local",
						},
						Annotations: map[string]string{},
					},
					Spec: gatewayapi.HTTPRouteSpec{
						Hostnames: []gatewayapi.Hostname{localHostShortest, localHostShort, localHostFull},
						Rules: []gatewayapi.HTTPRouteRule{{
							BackendRefs: []gatewayapi.HTTPBackendRef{{
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapi.PortNumber](123),
										Name:  gatewayapi.ObjectName("goo"),
									},
									Weight: ptr.To(int32(12)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blah",
											},
											{
												Name:  "Bleep",
												Value: "bloop",
											},
										}}}},
							}, {
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapi.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(88)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blurg",
											},
										},
									}}},
							}},
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{
										{
											Name:  "Foo",
											Value: "bar",
										},
									},
								}}},
							Matches: []gatewayapi.HTTPRouteMatch{{
								Path: &gatewayapi.HTTPPathMatch{
									Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							}},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{{
								Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
								Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
								Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
								Name:      gatewayapi.ObjectName("foo-local"),
							}},
						},
					},
				},
			},
		}, {
			name: "multiple paths with header conditions",
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
					Hosts:      testHosts,
					Visibility: v1alpha1.IngressVisibilityExternalIP,
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{{
							Headers: map[string]v1alpha1.HeaderMatch{
								"tag": {
									Exact: "goo",
								},
							},
							Splits: []v1alpha1.IngressBackendSplit{{
								IngressBackend: v1alpha1.IngressBackend{
									ServiceName: "goo",
									ServicePort: intstr.FromInt(123),
								},
								Percent: 100,
							}},
						}, {
							Path: "/doo",
							Headers: map[string]v1alpha1.HeaderMatch{
								"tag": {
									Exact: "doo",
								},
							},
							Splits: []v1alpha1.IngressBackendSplit{{
								IngressBackend: v1alpha1.IngressBackend{
									ServiceName: "doo",
									ServicePort: intstr.FromInt(124),
								},
								Percent: 100,
							}},
						}},
					},
				}}},
			},
			expected: []*gatewayapi.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LongestHost(testHosts),
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey:          testIngressName,
						"networking.knative.dev/visibility": "",
					},
					Annotations: map[string]string{},
				},
				Spec: gatewayapi.HTTPRouteSpec{
					Hostnames: []gatewayapi.Hostname{externalHost},
					Rules: []gatewayapi.HTTPRouteRule{
						{
							BackendRefs: []gatewayapi.HTTPBackendRef{{
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapi.PortNumber](123),
										Name:  gatewayapi.ObjectName("goo"),
									},
									Weight: ptr.To(int32(100)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{}}}},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapi.HeaderMatchExact),
										Name:  gatewayapi.HTTPHeaderName("tag"),
										Value: "goo",
									}},
								}},
						}, {
							BackendRefs: []gatewayapi.HTTPBackendRef{{
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapi.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(100)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{}}}},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/doo"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapi.HeaderMatchExact),
										Name:  gatewayapi.HTTPHeaderName("tag"),
										Value: "doo",
									}},
								}},
						},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{{
							Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
							Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
							Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
							Name:      gatewayapi.ObjectName("foo"),
						}},
					},
				},
			}},
		}, {
			name: "path with host rewrites",
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
					Hosts:      testHosts,
					Visibility: v1alpha1.IngressVisibilityExternalIP,
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{{
							RewriteHost: "hello-example.example.com",
						}},
					},
				}}},
			},
			expected: []*gatewayapi.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LongestHost(testHosts),
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey:          testIngressName,
						"networking.knative.dev/visibility": "",
					},
					Annotations: map[string]string{},
				},
				Spec: gatewayapi.HTTPRouteSpec{
					Hostnames: []gatewayapi.Hostname{externalHost},
					Rules: []gatewayapi.HTTPRouteRule{{
						Filters: []gatewayapi.HTTPRouteFilter{{
							Type: gatewayapi.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayapi.HTTPURLRewriteFilter{
								Hostname: (*gatewayapi.PreciseHostname)(ptr.To("hello-example.example.com")),
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{},
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}}},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{{
							Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
							Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
							Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
							Name:      gatewayapi.ObjectName("foo"),
						}},
					},
				},
			}},
		}, {
			name: "gateway supports HTTPRouteRequestTimeout",
			changeConfig: func(c *config.Config) {
				gateways := c.GatewayPlugin.ExternalGateways

				for _, gateway := range gateways {
					gateway.SupportedFeatures.Insert(features.SupportHTTPRouteRequestTimeout)
				}
			},
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
					Hosts:      testHosts,
					Visibility: v1alpha1.IngressVisibilityExternalIP,
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{{
							Path: "/",
						}},
					},
				}}},
			},
			expected: []*gatewayapi.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LongestHost(testHosts),
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey:          testIngressName,
						"networking.knative.dev/visibility": "",
					},
					Annotations: map[string]string{},
				},
				Spec: gatewayapi.HTTPRouteSpec{
					Hostnames: []gatewayapi.Hostname{externalHost},
					Rules: []gatewayapi.HTTPRouteRule{{
						Timeouts: &gatewayapi.HTTPRouteTimeouts{
							Request: ptr.To[gatewayapi.Duration]("0s"),
						},
						BackendRefs: []gatewayapi.HTTPBackendRef{},
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}}},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{{
							Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
							Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
							Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
							Name:      gatewayapi.ObjectName("foo"),
						}},
					},
				},
			}},
		}} {
		t.Run(tc.name, func(t *testing.T) {
			for i, rule := range tc.ing.Spec.Rules {
				rule := rule
				cfg := testConfig.DeepCopy()
				if tc.changeConfig != nil {
					tc.changeConfig(cfg)

					fmt.Printf("%#v", cfg.GatewayPlugin.ExternalGateways)
				}
				tcs := &testConfigStore{config: cfg}
				ctx := tcs.ToContext(context.Background())

				route, err := MakeHTTPRoute(ctx, tc.ing, &rule, nil)
				if err != nil {
					t.Fatal("MakeHTTPRoute failed:", err)
				}
				tc.expected[i].OwnerReferences = []metav1.OwnerReference{*kmeta.NewControllerRef(tc.ing)}
				if diff := cmp.Diff(tc.expected[i], route); diff != "" {
					t.Error("Unexpected HTTPRoute (-want +got):", diff)
				}
			}
		})
	}
}

func TestAddEndpointProbes(t *testing.T) {
	tcs := &testConfigStore{config: testConfig}
	ctx := tcs.ToContext(context.Background())

	ing := testIngress.DeepCopy()
	rule := &ing.Spec.Rules[0]
	route, err := MakeHTTPRoute(ctx, ing, rule, nil)
	if err != nil {
		t.Fatal("MakeHTTPRoute failed:", err)
	}

	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[0])
	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[1])

	expected := &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(testHosts),
			Namespace: testNamespace,
			Labels: map[string]string{
				networking.IngressLabelKey:          testIngressName,
				"networking.knative.dev/visibility": "",
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: gatewayapi.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi.CommonRouteSpec{
				ParentRefs: []gatewayapi.ParentReference{{
					Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
					Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
					Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
					Name:      gatewayapi.ObjectName("foo"),
				}},
			},
			Hostnames: []gatewayapi.Hostname{externalHost},
			Rules: []gatewayapi.HTTPRouteRule{{
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/"),
					},
				}},
				Filters: []gatewayapi.HTTPRouteFilter{{
					Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
						Set: []gatewayapi.HTTPHeader{{
							Name:  "Foo",
							Value: "bar",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](12),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(123)),
							Name:  "goo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blah",
							}, {
								Name:  "Bleep",
								Value: "bloop",
							}},
						}},
					},
				}, {
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](88),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(124)),
							Name:  "doo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blurg",
							}},
						}},
					}},
				},
			}, {
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/.well-known/knative/revision/test-ns/goo"),
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
							Value: "hash",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](100),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: ptr.To[gatewayapi.Group](""),
							Kind:  ptr.To[gatewayapi.Kind]("Service"),
							Name:  "goo",
							Port:  ptr.To[gatewayapi.PortNumber](123),
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blah",
							}, {
								Name:  "Bleep",
								Value: "bloop",
							}},
						},
					}},
				}},
			}, {
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/.well-known/knative/revision/test-ns/doo"),
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
							Value: "hash",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](100),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: ptr.To[gatewayapi.Group](""),
							Kind:  ptr.To[gatewayapi.Kind]("Service"),
							Name:  "doo",
							Port:  ptr.To[gatewayapi.PortNumber](124),
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blurg",
							}},
						},
					}},
				}},
			}},
		},
	}

	if diff := cmp.Diff(expected, route); diff != "" {
		t.Fatal("Unexpected (-want, +got): ", diff)
	}
}

func TestRemoveEndpointProbes(t *testing.T) {
	tcs := &testConfigStore{config: testConfig}
	ctx := tcs.ToContext(context.Background())

	ing := testIngress.DeepCopy()
	rule := &ing.Spec.Rules[0]
	route, err := MakeHTTPRoute(ctx, ing, rule, nil)
	if err != nil {
		t.Fatal("MakeHTTPRoute failed:", err)
	}

	expected := route.DeepCopy()

	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[0])
	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[1])
	RemoveEndpointProbes(route)

	if diff := cmp.Diff(expected, route); diff != "" {
		t.Fatal("Unexpected (-want, +got): ", diff)
	}
}

func TestUpdateProbeHash(t *testing.T) {
	tcs := &testConfigStore{config: testConfig}
	ctx := tcs.ToContext(context.Background())
	ing := testIngress.DeepCopy()
	rule := &ing.Spec.Rules[0]
	route, err := MakeHTTPRoute(ctx, ing, rule, nil)
	if err != nil {
		t.Fatal("MakeHTTPRoute failed:", err)
	}

	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[0])
	AddEndpointProbe(route, "hash", rule.HTTP.Paths[0].Splits[1])
	UpdateProbeHash(route, "second-hash")

	expected := &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(testHosts),
			Namespace: testNamespace,
			Labels: map[string]string{
				networking.IngressLabelKey:          testIngressName,
				"networking.knative.dev/visibility": "",
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: gatewayapi.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi.CommonRouteSpec{
				ParentRefs: []gatewayapi.ParentReference{{
					Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
					Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
					Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
					Name:      gatewayapi.ObjectName("foo"),
				}},
			},
			Hostnames: []gatewayapi.Hostname{externalHost},
			Rules: []gatewayapi.HTTPRouteRule{{
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/"),
					},
				}},
				Filters: []gatewayapi.HTTPRouteFilter{{
					Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
						Set: []gatewayapi.HTTPHeader{{
							Name:  "Foo",
							Value: "bar",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](12),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(123)),
							Name:  "goo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blah",
							}, {
								Name:  "Bleep",
								Value: "bloop",
							}},
						}},
					},
				}, {
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](88),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(124)),
							Name:  "doo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blurg",
							}},
						}},
					}},
				},
			}, {
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/.well-known/knative/revision/test-ns/goo"),
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
							Value: "second-hash",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](100),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: ptr.To[gatewayapi.Group](""),
							Kind:  ptr.To[gatewayapi.Kind]("Service"),
							Name:  "goo",
							Port:  ptr.To[gatewayapi.PortNumber](123),
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blah",
							}, {
								Name:  "Bleep",
								Value: "bloop",
							}},
						},
					}},
				}},
			}, {
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/.well-known/knative/revision/test-ns/doo"),
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
							Value: "second-hash",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](100),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: ptr.To[gatewayapi.Group](""),
							Kind:  ptr.To[gatewayapi.Kind]("Service"),
							Name:  "doo",
							Port:  ptr.To[gatewayapi.PortNumber](124),
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blurg",
							}},
						},
					}},
				}},
			}},
		},
	}

	if diff := cmp.Diff(expected, route); diff != "" {
		t.Fatal("Unexpected (-want, +got): ", diff)
	}
}

func TestAddOldBackend(t *testing.T) {
	tcs := &testConfigStore{config: testConfig}
	ctx := tcs.ToContext(context.Background())
	ing := testIngress.DeepCopy()

	rule := &ing.Spec.Rules[0]
	route, err := MakeHTTPRoute(ctx, ing, rule, nil)
	if err != nil {
		t.Fatal("MakeHTTPRoute failed:", err)
	}

	AddOldBackend(route, "hash", gatewayapi.HTTPBackendRef{
		BackendRef: gatewayapi.BackendRef{
			Weight: ptr.To[int32](100),
			BackendObjectReference: gatewayapi.BackendObjectReference{
				Group:     ptr.To[gatewayapi.Group](""),
				Kind:      ptr.To[gatewayapi.Kind]("Service"),
				Name:      "blah",
				Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
				Port:      ptr.To[gatewayapi.PortNumber](127),
			},
		},
		Filters: []gatewayapi.HTTPRouteFilter{{
			Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
				Set: []gatewayapi.HTTPHeader{{
					Name:  "Foo",
					Value: "bar",
				}},
			},
		}},
	})

	expected := &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LongestHost(testHosts),
			Namespace: testNamespace,
			Labels: map[string]string{
				networking.IngressLabelKey:          testIngressName,
				"networking.knative.dev/visibility": "",
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: gatewayapi.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi.CommonRouteSpec{
				ParentRefs: []gatewayapi.ParentReference{{
					Group:     (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
					Kind:      (*gatewayapi.Kind)(ptr.To("Gateway")),
					Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
					Name:      gatewayapi.ObjectName("foo"),
				}},
			},
			Hostnames: []gatewayapi.Hostname{externalHost},
			Rules: []gatewayapi.HTTPRouteRule{{
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/"),
					},
				}},
				Filters: []gatewayapi.HTTPRouteFilter{{
					Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
						Set: []gatewayapi.HTTPHeader{{
							Name:  "Foo",
							Value: "bar",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](12),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(123)),
							Name:  "goo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blah",
							}, {
								Name:  "Bleep",
								Value: "bloop",
							}},
						}},
					},
				}, {
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](88),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group: (*gatewayapi.Group)(ptr.To("")),
							Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
							Port:  ptr.To(gatewayapi.PortNumber(124)),
							Name:  "doo",
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Baz",
								Value: "blurg",
							}},
						}},
					}},
				},
			}, {
				Matches: []gatewayapi.HTTPRouteMatch{{
					Path: &gatewayapi.HTTPPathMatch{
						Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
						Value: ptr.To("/.well-known/knative/revision/test-ns/blah"),
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
							Value: "hash",
						}},
					},
				}},
				BackendRefs: []gatewayapi.HTTPBackendRef{{
					BackendRef: gatewayapi.BackendRef{
						Weight: ptr.To[int32](100),
						BackendObjectReference: gatewayapi.BackendObjectReference{
							Group:     ptr.To[gatewayapi.Group](""),
							Kind:      ptr.To[gatewayapi.Kind]("Service"),
							Name:      "blah",
							Namespace: ptr.To[gatewayapi.Namespace]("test-ns"),
							Port:      ptr.To[gatewayapi.PortNumber](127),
						},
					},
					Filters: []gatewayapi.HTTPRouteFilter{{
						Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
							Set: []gatewayapi.HTTPHeader{{
								Name:  "Foo",
								Value: "bar",
							}},
						},
					}},
				}},
			}},
		},
	}

	if diff := cmp.Diff(expected, route); diff != "" {
		t.Fatal("Unexpected (-want, +got): ", diff)
	}
}

func TestMakeRedirectHTTPRoute(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ing      *v1alpha1.Ingress
		expected []*gatewayapi.HTTPRoute
	}{
		{
			name: "single external domain and cluster local",
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{
					Rules: []v1alpha1.IngressRule{
						{
							Hosts:      testHosts,
							Visibility: v1alpha1.IngressVisibilityExternalIP,
							HTTP: &v1alpha1.HTTPIngressRuleValue{
								Paths: []v1alpha1.HTTPIngressPath{{
									Splits: []v1alpha1.IngressBackendSplit{{
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "goo",
											ServicePort: intstr.FromInt(123),
										},
										Percent: 100,
									}},
								}},
							},
						}, {
							Hosts:      testLocalHosts,
							Visibility: v1alpha1.IngressVisibilityClusterLocal,
							HTTP: &v1alpha1.HTTPIngressRuleValue{
								Paths: []v1alpha1.HTTPIngressPath{{
									Splits: []v1alpha1.IngressBackendSplit{{
										IngressBackend: v1alpha1.IngressBackend{
											ServiceName: "goo",
											ServicePort: intstr.FromInt(123),
										},
										Percent: 100,
									}},
								}},
							},
						},
					}},
			},
			expected: []*gatewayapi.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      LongestHost(testHosts) + redirectHTTPRoutePostfix,
						Namespace: testNamespace,
						Labels: map[string]string{
							networking.IngressLabelKey:          testIngressName,
							"networking.knative.dev/visibility": "",
						},
						Annotations: map[string]string{},
					},
					Spec: gatewayapi.HTTPRouteSpec{
						Hostnames: []gatewayapi.Hostname{externalHost},
						Rules: []gatewayapi.HTTPRouteRule{{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								},
							},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{{
								Group:       (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
								Kind:        (*gatewayapi.Kind)(ptr.To("Gateway")),
								Namespace:   ptr.To[gatewayapi.Namespace]("test-ns"),
								Name:        gatewayapi.ObjectName("foo"),
								SectionName: ptr.To[gatewayapi.SectionName]("http"),
							}},
						},
					},
				},
			},
		}, {
			name: "multiple paths with header conditions",
			ing: &v1alpha1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngressName,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey: testIngressName,
					},
				},
				Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
					Hosts:      testHosts,
					Visibility: v1alpha1.IngressVisibilityExternalIP,
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{{
							Headers: map[string]v1alpha1.HeaderMatch{
								"tag": {
									Exact: "goo",
								},
							},
							Splits: []v1alpha1.IngressBackendSplit{{
								IngressBackend: v1alpha1.IngressBackend{
									ServiceName: "goo",
									ServicePort: intstr.FromInt(123),
								},
								Percent: 100,
							}},
						}, {
							Path: "/doo",
							Headers: map[string]v1alpha1.HeaderMatch{
								"tag": {
									Exact: "doo",
								},
							},
							Splits: []v1alpha1.IngressBackendSplit{{
								IngressBackend: v1alpha1.IngressBackend{
									ServiceName: "doo",
									ServicePort: intstr.FromInt(124),
								},
								Percent: 100,
							}},
						}},
					},
				}}},
			},
			expected: []*gatewayapi.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LongestHost(testHosts) + redirectHTTPRoutePostfix,
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey:          testIngressName,
						"networking.knative.dev/visibility": "",
					},
					Annotations: map[string]string{},
				},
				Spec: gatewayapi.HTTPRouteSpec{
					Hostnames: []gatewayapi.Hostname{externalHost},
					Rules: []gatewayapi.HTTPRouteRule{
						{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapi.HeaderMatchExact),
										Name:  gatewayapi.HTTPHeaderName("tag"),
										Value: "goo",
									}},
								}},
						}, {
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
										Value: ptr.To("/doo"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapi.HeaderMatchExact),
										Name:  gatewayapi.HTTPHeaderName("tag"),
										Value: "doo",
									}},
								}},
						},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{{
							Group:       (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
							Kind:        (*gatewayapi.Kind)(ptr.To("Gateway")),
							Namespace:   ptr.To[gatewayapi.Namespace]("test-ns"),
							Name:        gatewayapi.ObjectName("foo"),
							SectionName: ptr.To[gatewayapi.SectionName]("http"),
						}},
					},
				},
			}},
		}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, exp := range tc.expected {
				exp.OwnerReferences = []metav1.OwnerReference{*kmeta.NewControllerRef(tc.ing)}
			}
			for i, rule := range tc.ing.Spec.Rules {
				if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
					rule := rule
					cfg := testConfig.DeepCopy()
					tcs := &testConfigStore{config: cfg}
					ctx := tcs.ToContext(context.Background())

					route, err := MakeRedirectHTTPRoute(ctx, tc.ing, &rule)
					if err != nil {
						t.Fatal("MakeRedirectHTTPRoute failed:", err)
					}
					if diff := cmp.Diff(tc.expected[i], route); diff != "" {
						t.Error("Unexpected redirect HTTPRoute (-want +got):", diff)
					}
				}
			}
		})
	}
}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var testConfig = &config.Config{
	GatewayPlugin: &config.GatewayPlugin{
		ExternalGateways: []config.Gateway{{
			NamespacedName:    types.NamespacedName{Namespace: "test-ns", Name: "foo"},
			Class:             testGatewayClass,
			HTTPListenerName:  "http",
			SupportedFeatures: sets.New[features.SupportedFeature](),
		}},
		LocalGateways: []config.Gateway{{
			NamespacedName:    types.NamespacedName{Namespace: "test-ns", Name: "foo-local"},
			Class:             testGatewayClass,
			HTTPListenerName:  "http",
			SupportedFeatures: sets.New[features.SupportedFeature](),
		}},
	},
}

var _ reconciler.ConfigStore = (*testConfigStore)(nil)
