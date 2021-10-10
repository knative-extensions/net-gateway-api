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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"knative.dev/net-gateway-api/test"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/pool"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
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

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        portNumPtr(fooPort),
					ServiceName: &fooName,
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gwv1alpha1.HTTPRouteFilter{{
						Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
							Set: map[string]string{headerName: fooName},
						},
					}},
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Path: &gwv1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gwv1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/foo"),
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        portNumPtr(barPort),
					ServiceName: &barName,
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gwv1alpha1.HTTPRouteFilter{{
						Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
							Set: map[string]string{headerName: barName},
						},
					}},
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Path: &gwv1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gwv1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/bar"),
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        portNumPtr(bazPort),
					ServiceName: &bazName,
					// Append different headers to each split, which lets us identify
					// which backend we hit.
					Filters: []gwv1alpha1.HTTPRouteFilter{{
						Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
							Set: map[string]string{headerName: bazName},
						},
					}},
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Path: &gwv1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gwv1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/baz"),
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        portNumPtr(port),
					ServiceName: &name,
				}},
				// Append different headers to each split, which lets us identify
				// which backend we hit.
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{headerName: name},
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

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{
					{
						Port:        portNumPtr(fooPort),
						ServiceName: &fooName,
						Weight:      pointer.Int32Ptr(1),
						// Append different headers to each split, which lets us identify
						// which backend we hit.
						Filters: []gwv1alpha1.HTTPRouteFilter{{
							Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
								Set: map[string]string{headerName: fooName},
							},
						}},
					},
					{
						Port:        portNumPtr(barPort),
						ServiceName: &barName,
						Weight:      pointer.Int32Ptr(1),
						// Append different headers to each split, which lets us identify
						// which backend we hit.
						Filters: []gwv1alpha1.HTTPRouteFilter{{
							Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
								Set: map[string]string{headerName: barName},
							},
						}},
					},
				},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Path: &gwv1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gwv1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/foo"),
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        portNumPtr(port),
					ServiceName: &name,
					Weight:      pointer.Int32Ptr(1),
				}},
				// Append different headers to each split, which lets us identify
				// which backend we hit.
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{headerName: name},
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

	wg := pool.NewWithCapacity(8, total)

	for i := 0; i < total; i++ {
		wg.Go(func() error {
			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com/foo")
			if ri == nil {
				return errors.New("failed to request")
			}
			resultCh <- ri.Request.Headers.Get(headerName)
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
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
