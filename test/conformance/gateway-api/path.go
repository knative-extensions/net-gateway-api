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
	"errors"
	"math"
	"testing"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"knative.dev/net-gateway-api/test"
	"knative.dev/networking/pkg/apis/networking"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TestPath verifies that an Ingress properly dispatches to backends based on the path of the URL.
func TestPath(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	// For /foo
	fooName, fooPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// For /bar
	barName, barPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// For /baz
	bazName, bazPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// Use a post-split injected header to establish which split we are sending traffic to.
	const headerName = "Which-Backend"

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentReference{
			testGateway,
		}},
		Hostnames: []gatewayv1alpha2.Hostname{gatewayv1alpha2.Hostname(name + ".example.com")},
		Rules: []gatewayv1alpha2.HTTPRouteRule{
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(fooPort),
							Name: gatewayv1alpha2.ObjectName(fooName),
						}},
					// Append different headers to each split, which lets us identify
					// which backend we hit.
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
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/foo"),
					},
				}},
			},
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(barPort),
							Name: gatewayv1alpha2.ObjectName(barName),
						}},
					// Append different headers to each split, which lets us identify
					// which backend we hit.
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
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/bar"),
					},
				}},
			},
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(bazPort),
							Name: gatewayv1alpha2.ObjectName(bazName),
						}},
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gatewayv1alpha2.HTTPRouteFilter{{
						Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
							Set: []gatewayv1alpha2.HTTPHeader{{
								Name:  headerName,
								Value: bazName,
							}},
						}},
					},
				}},
				Matches: []gatewayv1alpha2.HTTPRouteMatch{{
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/baz"),
					},
				}},
			},
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(port),
							Name: gatewayv1alpha2.ObjectName(name),
						}},
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gatewayv1alpha2.HTTPRouteFilter{{
						Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
							Set: []gatewayv1alpha2.HTTPHeader{{
								Name:  headerName,
								Value: name,
							}},
						}},
					},
				}},
			},
		},
	})

	tests := map[string]string{
		"/foo":  fooName,
		"/bar":  barName,
		"/baz":  bazName,
		"":      name,
		"/asdf": name,
	}

	for path, want := range tests {
		path, want := path, want
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com"+path)
			if ri == nil {
				return
			}

			got := ri.Request.Headers.Get(headerName)
			if got != want {
				t.Errorf("Header[%q] = %q, wanted %q", headerName, got, want)
			}
		})
	}
}

func TestPathAndPercentageSplit(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	fooName, fooPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	barName, barPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// Use a post-split injected header to establish which split we are sending traffic to.
	const headerName = "Which-Backend"

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentReference{
			testGateway,
		}},
		Hostnames: []gatewayv1alpha2.Hostname{gatewayv1alpha2.Hostname(name + ".example.com")},
		Rules: []gatewayv1alpha2.HTTPRouteRule{
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{
					{
						BackendRef: gatewayv1alpha2.BackendRef{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Port: portNumPtr(fooPort),
								Name: gatewayv1alpha2.ObjectName(fooName),
							},
							Weight: pointer.Int32Ptr(1),
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
					},
					{
						BackendRef: gatewayv1alpha2.BackendRef{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Port: portNumPtr(barPort),
								Name: gatewayv1alpha2.ObjectName(barName),
							},
							Weight: pointer.Int32Ptr(1),
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
					},
				},
				Matches: []gatewayv1alpha2.HTTPRouteMatch{{
					Path: &gatewayv1alpha2.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayv1alpha2.PathMatchPathPrefix),
						Value: pointer.StringPtr("/foo"),
					},
				}},
			},
			{
				BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
					BackendRef: gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Port: portNumPtr(port),
							Name: gatewayv1alpha2.ObjectName(name),
						}},
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gatewayv1alpha2.HTTPRouteFilter{{
						Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
							Set: []gatewayv1alpha2.HTTPHeader{{
								Name:  headerName,
								Value: name,
							}},
						}},
					},
				}},
			},
		},
	})

	const (
		total     = 1000
		totalHalf = total / 2
		tolerance = total * 0.15
	)
	wantKeys := sets.NewString(fooName, barName)
	resultCh := make(chan string, total)

	var g errgroup.Group
	g.SetLimit(8)

	for i := 0; i < total; i++ {
		g.Go(func() error {
			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com/foo")
			if ri == nil {
				return errors.New("failed to request")
			}
			resultCh <- ri.Request.Headers.Get(headerName)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Error("Error while sending requests:", err)
	}
	close(resultCh)

	got := make(map[string]float64, len(wantKeys))
	for r := range resultCh {
		got[r]++
	}
	for k, v := range got {
		if !wantKeys.Has(k) {
			t.Errorf("%s is not in the expected header say %v", k, wantKeys)
		}
		if math.Abs(v-totalHalf) > tolerance {
			t.Errorf("Header %s got: %v times, want in [%v, %v] range", k, v, totalHalf-tolerance, totalHalf+tolerance)
		}
	}
}
