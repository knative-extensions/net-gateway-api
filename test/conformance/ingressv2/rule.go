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
	"context"
	"testing"

	"k8s.io/utils/pointer"
	"knative.dev/net-gateway-api/test"
	"knative.dev/networking/pkg/apis/networking"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TestRule verifies that an Ingress properly dispatches to backends based on different rules.
func TestRule(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	// Use a pre-split injected header to establish which rule we are sending traffic to.
	const headerName = "Foo-Bar-Baz"

	fooName, fooPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	barName, barPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentRef{
			testGateway,
		}},
		Hostnames: []gatewayv1alpha2.Hostname{gatewayv1alpha2.Hostname(fooName + ".example.com"), gatewayv1alpha2.Hostname(barName + ".example.com")},
		Rules: []gatewayv1alpha2.HTTPRouteRule{
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(fooPort),
							Name: gatewayv1alpha2.ObjectName(fooName),
						},
					},
					Filters: []gatewayv1alpha2.HTTPRouteFilter{{
						Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
							Set: []gatewayv1alpha2.HTTPHeader{{
								Name:  headerName,
								Value: fooName,
							}},
						}},
					},
				}},
				Matches: []gatewayv1alpha2.HTTPRouteMatch{{
					Headers: []gatewayv1alpha2.HTTPHeaderMatch{{
						Type:  headerMatchTypePtr(gatewayv1alpha2.HeaderMatchExact),
						Name:  "Host",
						Value: fooName + ".example.com",
					}},
					// This should be removed once https://github.com/kubernetes-sigs/gateway-api/issues/563 was solved.
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
			},
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(barPort),
							Name: gatewayv1alpha2.ObjectName(barName),
						},
					},
					Filters: []gatewayv1alpha2.HTTPRouteFilter{{
						Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
							Set: []gatewayv1alpha2.HTTPHeader{{
								Name:  headerName,
								Value: barName,
							}},
						}},
					},
				}},
				Matches: []gatewayv1alpha2.HTTPRouteMatch{{
					Headers: []gatewayv1alpha2.HTTPHeaderMatch{{
						Type:  headerMatchTypePtr(gatewayv1alpha2.HeaderMatchExact),
						Name:  "Host",
						Value: barName + ".example.com",
					}},
					// This should be removed once https://github.com/kubernetes-sigs/gateway-api/issues/563 was solved.
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
			},
		},
	})

	ri := RuntimeRequest(ctx, t, client, "http://"+fooName+".example.com")
	if got := ri.Request.Headers.Get(headerName); got != fooName {
		t.Errorf("Header[Host] = %q, wanted %q", got, fooName)
	}

	ri = RuntimeRequest(ctx, t, client, "http://"+barName+".example.com")
	if got := ri.Request.Headers.Get(headerName); got != barName {
		t.Errorf("Header[Host] = %q, wanted %q", got, barName)
	}
}
