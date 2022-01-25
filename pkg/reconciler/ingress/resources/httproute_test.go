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
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/reconciler"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	testNamespace    = "test-ns"
	testIngressName  = "test-ingress"
	testGatewayClass = "test-class"
)

var (
	externalHost      = gwv1alpha2.Hostname(testHosts[0])
	localHostShortest = gwv1alpha2.Hostname(testLocalHosts[0])
	localHostShort    = gwv1alpha2.Hostname(testLocalHosts[1])
	localHostFull     = gwv1alpha2.Hostname(testLocalHosts[2])

	testLocalHosts = []string{
		"hello-example.default",
		"hello-example.default.svc",
		"hello-example.default.svc.cluster.local",
	}

	testHosts = []string{"hello-example.default.example.com"}
)

func TestMakeHTTPRoute(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ing      *v1alpha1.Ingress
		expected []*gwv1alpha2.HTTPRoute
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
			expected: []*gwv1alpha2.HTTPRoute{
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
					Spec: gwv1alpha2.HTTPRouteSpec{
						Hostnames: []gwv1alpha2.Hostname{externalHost},
						Rules: []gwv1alpha2.HTTPRouteRule{{
							BackendRefs: []gwv1alpha2.HTTPBackendRef{{
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(123),
										Name: gwv1alpha2.ObjectName("goo"),
									},
									Weight: pointer.Int32Ptr(int32(12)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{
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
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(124),
										Name: gwv1alpha2.ObjectName("doo"),
									},
									Weight: pointer.Int32Ptr(int32(88)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blurg",
											},
										},
									}}},
							}},
							Filters: []gwv1alpha2.HTTPRouteFilter{{
								Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
									Set: []gwv1alpha2.HTTPHeader{
										{
											Name:  "Foo",
											Value: "bar",
										},
									},
								}}},
							Matches: []gwv1alpha2.HTTPRouteMatch{
								{
									Path: &gwv1alpha2.HTTPPathMatch{
										Type:  pathMatchTypePtr(gwv1alpha2.PathMatchPathPrefix),
										Value: pointer.StringPtr("/"),
									},
									Headers: []gwv1alpha2.HTTPHeaderMatch{},
								},
							},
						}},
						CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
							ParentRefs: []gwv1alpha2.ParentRef{{
								Namespace: namespacePtr("test-ns"),
								Name:      gwv1alpha2.ObjectName("foo"),
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
					Spec: gwv1alpha2.HTTPRouteSpec{
						Hostnames: []gwv1alpha2.Hostname{localHostShortest, localHostShort, localHostFull},
						Rules: []gwv1alpha2.HTTPRouteRule{{
							BackendRefs: []gwv1alpha2.HTTPBackendRef{{
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(123),
										Name: gwv1alpha2.ObjectName("goo"),
									},
									Weight: pointer.Int32Ptr(int32(12)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{
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
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(124),
										Name: gwv1alpha2.ObjectName("doo"),
									},
									Weight: pointer.Int32Ptr(int32(88)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{
											{
												Name:  "Baz",
												Value: "blurg",
											},
										},
									}}},
							}},
							Filters: []gwv1alpha2.HTTPRouteFilter{{
								Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
									Set: []gwv1alpha2.HTTPHeader{
										{
											Name:  "Foo",
											Value: "bar",
										},
									},
								}}},
							Matches: []gwv1alpha2.HTTPRouteMatch{{
								Path: &gwv1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gwv1alpha2.PathMatchPathPrefix),
									Value: pointer.StringPtr("/"),
								},
								Headers: []gwv1alpha2.HTTPHeaderMatch{},
							}},
						}},
						CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
							ParentRefs: []gwv1alpha2.ParentRef{{
								Namespace: namespacePtr("test-ns"),
								Name:      gwv1alpha2.ObjectName("foo-local"),
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
			expected: []*gwv1alpha2.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LongestHost(testHosts),
					Namespace: testNamespace,
					Labels: map[string]string{
						networking.IngressLabelKey:          testIngressName,
						"networking.knative.dev/visibility": "",
					},
					Annotations: map[string]string{},
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					Hostnames: []gwv1alpha2.Hostname{externalHost},
					Rules: []gwv1alpha2.HTTPRouteRule{
						{
							BackendRefs: []gwv1alpha2.HTTPBackendRef{{
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(123),
										Name: gwv1alpha2.ObjectName("goo"),
									},
									Weight: pointer.Int32Ptr(int32(100)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{}}}},
							}},
							Matches: []gwv1alpha2.HTTPRouteMatch{
								{
									Path: &gwv1alpha2.HTTPPathMatch{
										Type:  pathMatchTypePtr(gwv1alpha2.PathMatchPathPrefix),
										Value: pointer.StringPtr("/"),
									},
									Headers: []gwv1alpha2.HTTPHeaderMatch{{
										Type:  headerMatchTypePtr(gwv1alpha2.HeaderMatchExact),
										Name:  gwv1alpha2.HTTPHeaderName("tag"),
										Value: "goo",
									}},
								}},
						}, {
							BackendRefs: []gwv1alpha2.HTTPBackendRef{{
								BackendRef: gwv1alpha2.BackendRef{
									BackendObjectReference: gwv1alpha2.BackendObjectReference{
										Port: portNumPtr(124),
										Name: gwv1alpha2.ObjectName("doo"),
									},
									Weight: pointer.Int32Ptr(int32(100)),
								},
								Filters: []gwv1alpha2.HTTPRouteFilter{{
									Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
										Set: []gwv1alpha2.HTTPHeader{}}}},
							}},
							Matches: []gwv1alpha2.HTTPRouteMatch{
								{
									Path: &gwv1alpha2.HTTPPathMatch{
										Type:  pathMatchTypePtr(gwv1alpha2.PathMatchPathPrefix),
										Value: pointer.StringPtr("/doo"),
									},
									Headers: []gwv1alpha2.HTTPHeaderMatch{{
										Type:  headerMatchTypePtr(gwv1alpha2.HeaderMatchExact),
										Name:  gwv1alpha2.HTTPHeaderName("tag"),
										Value: "doo",
									}},
								}},
						},
					},
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentRef{{
							Namespace: namespacePtr("test-ns"),
							Name:      gwv1alpha2.ObjectName("foo"),
						}},
					},
				},
			}},
		}} {
		t.Run(tc.name, func(t *testing.T) {
			for i, rule := range tc.ing.Spec.Rules {
				rule := rule
				tcs := &testConfigStore{config: testConfig}
				ctx := tcs.ToContext(context.Background())

				route, err := MakeHTTPRoute(ctx, tc.ing, &rule)
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

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var testConfig = &config.Config{
	Gateway: &config.Gateway{
		Gateways: map[v1alpha1.IngressVisibility]config.GatewayConfig{
			v1alpha1.IngressVisibilityExternalIP: {
				GatewayClass: testGatewayClass,
				Gateway:      &types.NamespacedName{Namespace: "test-ns", Name: "foo"},
			},
			v1alpha1.IngressVisibilityClusterLocal: {
				GatewayClass: testGatewayClass,
				Gateway:      &types.NamespacedName{Namespace: "test-ns", Name: "foo-local"},
			},
		}},
}

var _ reconciler.ConfigStore = (*testConfigStore)(nil)
