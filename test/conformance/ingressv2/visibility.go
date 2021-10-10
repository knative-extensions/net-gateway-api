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

package ingress

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"knative.dev/net-gateway-api/test"
	"knative.dev/networking/pkg/apis/networking"
	nettest "knative.dev/networking/test"
	"knative.dev/pkg/pool"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestVisibility(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	// Create the private backend
	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// Generate a different hostname for each of these tests, so that they do not fail when run concurrently.
	var privateHostNames = map[string]gwv1alpha1.Hostname{
		"fqdn":     gwv1alpha1.Hostname(test.ObjectNameForTest(t) + "." + test.ServingNamespace + ".svc." + nettest.NetworkingFlags.ClusterSuffix),
		"short":    gwv1alpha1.Hostname(test.ObjectNameForTest(t) + "." + test.ServingNamespace + ".svc"),
		"shortest": gwv1alpha1.Hostname(test.ObjectNameForTest(t) + "." + test.ServingNamespace),
	}

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testLocalGateway,
		Hostnames: []gwv1alpha1.Hostname{privateHostNames["fqdn"], privateHostNames["short"], privateHostNames["shortest"]},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        portNumPtr(port),
				ServiceName: &name,
			}},
		}},
	}, OverrideHTTPRouteLabel(gatewayLocalLabel))

	// Ensure the service is not publicly accessible
	for _, privateHostName := range privateHostNames {
		RuntimeRequestWithExpectations(ctx, t, client, "http://"+string(privateHostName), []ResponseExpectation{StatusCodeExpectation(sets.NewInt(http.StatusNotFound))}, true)
	}

	for name := range privateHostNames {
		privateHostName := privateHostNames[name] // avoid the Go iterator capture issue.
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			testProxyToHelloworld(ctx, t, clients, string(privateHostName))
		})
	}
}

func testProxyToHelloworld(ctx context.Context, t *testing.T, clients *test.Clients, privateHostName string) {

	namespace, name := getClusterIngress()
	loadbalancerAddress := fmt.Sprintf("%s.%s.svc.%s", name, namespace, nettest.NetworkingFlags.ClusterSuffix)
	proxyName, proxyPort, _ := CreateProxyService(ctx, t, clients, privateHostName, loadbalancerAddress)

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	publicHostName := gwv1alpha1.Hostname(test.ObjectNameForTest(t) + ".publicproxy.example.com")

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{publicHostName},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        portNumPtr(proxyPort),
				ServiceName: &proxyName,
			}},
		}},
	})

	// Ensure the service is accessible from within the cluster.
	RuntimeRequest(ctx, t, client, "http://"+string(publicHostName))
}

func TestVisibilitySplit(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	// Use a post-split injected header to establish which split we are sending traffic to.
	const headerName = "Foo-Bar-Baz"

	backends := make([]gwv1alpha1.HTTPRouteForwardTo, 0, 10)
	weights := make(map[string]float64, len(backends))

	// Double the percentage of the split each iteration until it would overflow, and then
	// give the last route the remainder.
	percent, total := int32(1), int32(0)
	for i := 0; i < 10; i++ {
		weight := percent
		name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
		backends = append(backends, gwv1alpha1.HTTPRouteForwardTo{
			ServiceName: &name,
			Port:        portNumPtr(port),
			Weight:      &weight,

			// Append different headers to each split, which lets us identify
			// which backend we hit.
			Filters: []gwv1alpha1.HTTPRouteFilter{{
				Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
					Set: map[string]string{headerName: name},
				},
			}},
		})
		weights[name] = float64(percent)

		total += percent
		percent *= 2
		// Cap the final non-zero bucket so that we total 100%
		// After that, this will zero out remaining buckets.
		if total+percent > 100 {
			percent = 100 - total
		}
	}

	name := test.ObjectNameForTest(t)

	// Create a simple Ingress over the 10 Services.
	privateHostName := fmt.Sprintf("%s.%s.svc.%s", name, test.ServingNamespace, nettest.NetworkingFlags.ClusterSuffix)
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testLocalGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(privateHostName)},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: backends,
		}},
	}, OverrideHTTPRouteLabel(gatewayLocalLabel))

	// Ensure we can't connect to the private resources
	RuntimeRequestWithExpectations(ctx, t, client, "http://"+privateHostName, []ResponseExpectation{StatusCodeExpectation(sets.NewInt(http.StatusNotFound))}, true)

	namespace, name := getClusterIngress()
	loadbalancerAddress := fmt.Sprintf("%s.%s.svc.%s", name, namespace, nettest.NetworkingFlags.ClusterSuffix)
	proxyName, proxyPort, _ := CreateProxyService(ctx, t, clients, privateHostName, loadbalancerAddress)

	publicHostName := fmt.Sprintf("%s.%s", name, "example.com")
	_, client, _ = CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(publicHostName)},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        portNumPtr(proxyPort),
				ServiceName: &proxyName,
			}},
		}},
	})

	// Create a large enough population of requests that we can reasonably assess how
	// well the Ingress respected the percentage split.
	seen := make(map[string]float64, len(backends))

	const (
		// The total number of requests to make (as a float to avoid conversions in later computations).
		totalRequests = 1000.0
		// The increment to make for each request, so that the values of seen reflect the
		// percentage of the total number of requests we are making.
		increment = 100.0 / totalRequests
		// Allow the Ingress to be within 10% of the configured value.
		margin = 10.0
	)
	wg := pool.NewWithCapacity(8, totalRequests)
	resultCh := make(chan string, totalRequests)

	for i := 0.0; i < totalRequests; i++ {
		wg.Go(func() error {
			ri := RuntimeRequest(ctx, t, client, "http://"+publicHostName)
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

	for r := range resultCh {
		seen[r] += increment
	}

	for name, want := range weights {
		got := seen[name]
		switch {
		case want == 0.0 && got > 0.0:
			// For 0% targets, we have tighter requirements.
			t.Errorf("Target %q received traffic, wanted none (0%% target).", name)
		case math.Abs(got-want) > margin:
			t.Errorf("Target %q received %f%%, wanted %f +/- %f", name, got, want, margin)
		}
	}
}

func TestVisibilityPath(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	// For /foo
	fooName, fooPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// For /bar
	barName, barPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// For /baz
	bazName, bazPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	mainName, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	// Use a post-split injected header to establish which split we are sending traffic to.
	const headerName = "Which-Backend"

	name := test.ObjectNameForTest(t)
	privateHostName := fmt.Sprintf("%s.%s.svc.%s", name, test.ServingNamespace, nettest.NetworkingFlags.ClusterSuffix)
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testLocalGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(privateHostName)},
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
					ServiceName: &mainName,
				}},
				// Append different headers to each split, which lets us identify
				// which backend we hit.
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{headerName: mainName},
					},
				}},
			},
		},
	}, OverrideHTTPRouteLabel(gatewayLocalLabel))

	// Ensure we can't connect to the private resources
	for _, path := range []string{"", "/foo", "/bar", "/baz"} {
		RuntimeRequestWithExpectations(ctx, t, client, "http://"+privateHostName+path, []ResponseExpectation{StatusCodeExpectation(sets.NewInt(http.StatusNotFound))}, true)
	}

	namespace, name := getClusterIngress()
	loadbalancerAddress := fmt.Sprintf("%s.%s.svc.%s", name, namespace, nettest.NetworkingFlags.ClusterSuffix)
	proxyName, proxyPort, _ := CreateProxyService(ctx, t, clients, privateHostName, loadbalancerAddress)

	publicHostName := fmt.Sprintf("%s.%s", name, "example.com")
	_, client, _ = CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(publicHostName)},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        portNumPtr(proxyPort),
				ServiceName: &proxyName,
			}},
		}},
	})

	tests := map[string]string{
		"/foo":  fooName,
		"/bar":  barName,
		"/baz":  bazName,
		"":      mainName,
		"/asdf": mainName,
	}

	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			ri := RuntimeRequest(ctx, t, client, "http://"+publicHostName+path)
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
