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
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/net-ingressv2/test"
	network "knative.dev/networking/pkg"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/ptr"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// TestTagHeaders verifies that an Ingress properly dispatches to backends based on the tag header
//
// See proposal doc for reference:
// https://docs.google.com/document/d/12t_3NE4EqvW_l0hfVlQcAGKkwkAM56tTn2wN_JtHbSQ/edit?usp=sharing
func TestTagHeaders(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	const (
		tagName           = "the-tag"
		backendHeader     = "Which-Backend"
		backendWithTag    = "tag"
		backendWithoutTag = "no-tag"
	)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        &port,
					ServiceName: &name,
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Headers: &gwv1alpha1.HTTPHeaderMatch{
						Type:   gwv1alpha1.HeaderMatchExact,
						Values: map[string]string{network.TagHeaderName: tagName},
					},
					// This should be removed once https://github.com/kubernetes-sigs/gateway-api/issues/563 was solved.
					Path: gwv1alpha1.HTTPPathMatch{
						Type:  gwv1alpha1.PathMatchPrefix,
						Value: "/",
					},
				}},
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{backendHeader: backendWithTag},
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        &port,
					ServiceName: &name,
				}},
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{backendHeader: backendWithoutTag},
					},
				}},
			},
		},
	})

	tests := []struct {
		Name        string
		TagHeader   *string
		WantBackend string
	}{{
		Name:        "matching tag header",
		TagHeader:   ptr.String(tagName),
		WantBackend: backendWithTag,
	}, {
		Name:        "no tag header",
		WantBackend: backendWithoutTag,
	}, {
		// Note: Behavior may change in Phase 2 (see Proposal doc)
		Name:        "empty tag header",
		TagHeader:   ptr.String(""),
		WantBackend: backendWithoutTag,
	}, {
		// Note: Behavior may change in Phase 2 (see Proposal doc)
		Name:        "non-matching tag header",
		TagHeader:   ptr.String("not-" + tagName),
		WantBackend: backendWithoutTag,
	}}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			ros := []RequestOption{}

			if tt.TagHeader != nil {
				ros = append(ros, func(r *http.Request) {
					r.Header.Set(network.TagHeaderName, *tt.TagHeader)
				})
			}

			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com", ros...)
			if ri == nil {
				t.Error("Couldn't make request")
				return
			}

			if got, want := ri.Request.Headers.Get(backendHeader), tt.WantBackend; got != want {
				t.Errorf("Header[%q] = %q, wanted %q", backendHeader, got, want)
			}
		})
	}

}

// TestPreSplitSetHeaders verifies that an Ingress that specified AppendHeaders pre-split has the appropriate header(s) set.
func TestPreSplitSetHeaders(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	const headerName = "Foo-Bar-Baz"

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        &port,
				ServiceName: &name,
			}},
			Filters: []gwv1alpha1.HTTPRouteFilter{{
				Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
					Set: map[string]string{headerName: name},
				}}},
		}},
	})

	t.Run("Check without passing header", func(t *testing.T) {
		t.Parallel()

		ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com")
		if ri == nil {
			return
		}

		if got, want := ri.Request.Headers.Get(headerName), name; got != want {
			t.Errorf("Headers[%q] = %q, wanted %q", headerName, got, want)
		}
	})

	t.Run("Check with passing header", func(t *testing.T) {
		t.Parallel()

		ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com", func(req *http.Request) {
			// Specify a value for the header to verify that implementations
			// use set vs. append semantics.
			req.Header.Set(headerName, "bogus")
		})
		if ri == nil {
			return
		}

		if got, want := ri.Request.Headers.Get(headerName), name; got != want {
			t.Errorf("Headers[%q] = %q, wanted %q", headerName, got, want)
		}
	})
}

// TestPostSplitSetHeaders verifies that an Ingress that specified AppendHeaders post-split has the appropriate header(s) set.
func TestPostSplitSetHeaders(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	const (
		headerName  = "Foo-Bar-Baz"
		splits      = 4
		maxRequests = 100
	)

	forwards := make([]gwv1alpha1.HTTPRouteForwardTo, 0, splits)

	names := make(sets.String, splits)
	for i := 0; i < splits; i++ {
		name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

		forwards = append(forwards,
			gwv1alpha1.HTTPRouteForwardTo{
				Port:        &port,
				ServiceName: &name,
				Weight:      100 / splits,
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{headerName: name},
					}},
				}})
		names.Insert(name)
	}

	// Create a simple Ingress over the 10 Services.
	name := test.ObjectNameForTest(t)
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: forwards,
		}},
	})

	t.Run("Check without passing header", func(t *testing.T) {
		t.Parallel()

		// Make enough requests that the likelihood of us seeing each variation is high,
		// but don't check the distribution of requests, as that isn't the point of this
		// particular test.
		seen := make(sets.String, len(names))
		for i := 0; i < maxRequests; i++ {
			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com")
			if ri == nil {
				return
			}
			seen.Insert(ri.Request.Headers.Get(headerName))
			if seen.Equal(names) {
				// Short circuit if we've seen all headers.
				return
			}
		}
		// Us getting here means we haven't seen all headers, print the diff.
		t.Errorf("(over %d requests) Header[%q] (-want, +got) = %s",
			maxRequests, headerName, cmp.Diff(names, seen))
	})

	t.Run("Check with passing header", func(t *testing.T) {
		t.Parallel()

		// Make enough requests that the likelihood of us seeing each variation is high,
		// but don't check the distribution of requests, as that isn't the point of this
		// particular test.
		seen := make(sets.String, len(names))
		for i := 0; i < maxRequests; i++ {
			ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com", func(req *http.Request) {
				// Specify a value for the header to verify that implementations
				// use set vs. append semantics.
				req.Header.Set(headerName, "bogus")
			})
			if ri == nil {
				return
			}
			seen.Insert(ri.Request.Headers.Get(headerName))
			if seen.Equal(names) {
				// Short circuit if we've seen all headers.
				return
			}
		}
		// Us getting here means we haven't seen all headers, print the diff.
		t.Errorf("(over %d requests) Header[%q] (-want, +got) = %s",
			maxRequests, headerName, cmp.Diff(names, seen))
	})
}
