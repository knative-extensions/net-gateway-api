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
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	testNamespace   = "test-ns"
	testIngressName = "test-ingress"
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

	gatewayRef = gatewayapi.ParentReference{
		Group:       (*gatewayapi.Group)(ptr.To("gateway.networking.k8s.io")),
		Kind:        (*gatewayapi.Kind)(ptr.To("Gateway")),
		Namespace:   ptr.To[gatewayapi.Namespace]("test-ns"),
		Name:        gatewayapi.ObjectName("foo"),
		SectionName: ptr.To[gatewayapi.SectionName]("http"),
	}
)

func TestMakeHTTPRoute(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ing      *v1alpha1.Ingress
		expected []*gatewayapi.HTTPRoute
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
										Port:  ptr.To[gatewayapiv1.PortNumber](123),
									},
									Weight: ptr.To(int32(12)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Bleep",
												Value: "bloop",
											},
											{
												Name:  "Baz",
												Value: "blah",
											},
										}}}},
							}, {
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapiv1.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(88)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
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
								Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
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
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								},
							},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{gatewayRef},
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
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{
											{
												Name:  "Bleep",
												Value: "bloop",
											},
											{
												Name:  "Baz",
												Value: "blah",
											},
										}}}},
							}, {
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapiv1.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(88)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
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
								Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
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
									Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							}},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{gatewayRef},
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
										Port:  ptr.To[gatewayapiv1.PortNumber](123),
										Name:  gatewayapi.ObjectName("goo"),
									},
									Weight: ptr.To(int32(100)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{}}}},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
										Name:  gatewayapiv1.HTTPHeaderName("tag"),
										Value: "goo",
									}},
								}},
						}, {
							BackendRefs: []gatewayapi.HTTPBackendRef{{
								BackendRef: gatewayapi.BackendRef{
									BackendObjectReference: gatewayapi.BackendObjectReference{
										Group: (*gatewayapi.Group)(ptr.To("")),
										Kind:  (*gatewayapi.Kind)(ptr.To("Service")),
										Port:  ptr.To[gatewayapiv1.PortNumber](124),
										Name:  gatewayapi.ObjectName("doo"),
									},
									Weight: ptr.To(int32(100)),
								},
								Filters: []gatewayapi.HTTPRouteFilter{{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
										Set: []gatewayapi.HTTPHeader{}}}},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/doo"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
										Name:  gatewayapiv1.HTTPHeaderName("tag"),
										Value: "doo",
									}},
								}},
						},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{gatewayRef},
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
							Type: gatewayapiv1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayapi.HTTPURLRewriteFilter{
								Hostname: (*gatewayapi.PreciseHostname)(ptr.To("hello-example.example.com")),
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{},
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}}},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{gatewayRef},
					},
				},
			}},
		}} {
		t.Run(tc.name, func(t *testing.T) {
			for i, rule := range tc.ing.Spec.Rules {
				rule := rule
				route, err := MakeHTTPRoute(tc.ing, &rule, gatewayRef)
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
								Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								},
							},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{gatewayRef},
						},
					},
				}, {
					ObjectMeta: metav1.ObjectMeta{
						Name:      LongestHost(testLocalHosts) + redirectHTTPRoutePostfix,
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
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{{
								Path: &gatewayapi.HTTPPathMatch{
									Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							}},
						}},
						CommonRouteSpec: gatewayapi.CommonRouteSpec{
							ParentRefs: []gatewayapi.ParentReference{gatewayRef},
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
								Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
										Name:  gatewayapiv1.HTTPHeaderName("tag"),
										Value: "goo",
									}},
								}},
						}, {
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi.HTTPRequestRedirectFilter{
									Scheme:     ptr.To("https"),
									Port:       ptr.To(gatewayapi.PortNumber(443)),
									StatusCode: ptr.To(http.StatusMovedPermanently),
								},
							}},
							Matches: []gatewayapi.HTTPRouteMatch{
								{
									Path: &gatewayapi.HTTPPathMatch{
										Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
										Value: ptr.To("/doo"),
									},
									Headers: []gatewayapi.HTTPHeaderMatch{{
										Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
										Name:  gatewayapiv1.HTTPHeaderName("tag"),
										Value: "doo",
									}},
								}},
						},
					},
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{gatewayRef},
					},
				},
			}},
		}} {
		t.Run(tc.name, func(t *testing.T) {
			for i, rule := range tc.ing.Spec.Rules {
				rule := rule
				route, err := MakeRedirectHTTPRoute(tc.ing, &rule, gatewayRef)
				if err != nil {
					t.Fatal("MakeRedirectHTTPRoute failed:", err)
				}
				tc.expected[i].OwnerReferences = []metav1.OwnerReference{*kmeta.NewControllerRef(tc.ing)}
				if diff := cmp.Diff(tc.expected[i], route); diff != "" {
					t.Error("Unexpected HTTPRoute (-want +got):", diff)
				}
			}
		})
	}
}
