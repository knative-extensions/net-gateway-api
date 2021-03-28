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

	"knative.dev/net-ingressv2/test"
	"knative.dev/networking/pkg/apis/networking"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// TestRule verifies that an Ingress properly dispatches to backends based on different rules.
func TestRule(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	fooName, fooPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	barName, barPort, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(fooName + ".example.com"), gwv1alpha1.Hostname(barName + ".example.com")},
		Rules: []gwv1alpha1.HTTPRouteRule{
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        &fooPort,
					ServiceName: &fooName,
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Headers: &gwv1alpha1.HTTPHeaderMatch{
						Type:   gwv1alpha1.HeaderMatchExact,
						Values: map[string]string{"Host": fooName + ".example.com"},
					},
					// This should be removed once https://github.com/kubernetes-sigs/gateway-api/issues/563 was solved.
					Path: gwv1alpha1.HTTPPathMatch{
						Type:  gwv1alpha1.PathMatchPrefix,
						Value: "/",
					},
				}},
			},
			{
				ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
					Port:        &barPort,
					ServiceName: &barName,
				}},
				Matches: []gwv1alpha1.HTTPRouteMatch{{
					Headers: &gwv1alpha1.HTTPHeaderMatch{
						Type:   gwv1alpha1.HeaderMatchExact,
						Values: map[string]string{"Host": barName + ".example.com"},
					},
					// This should be removed once https://github.com/kubernetes-sigs/gateway-api/issues/563 was solved.
					Path: gwv1alpha1.HTTPPathMatch{
						Type:  gwv1alpha1.PathMatchPrefix,
						Value: "/",
					},
				}},
			},
		},
	})

	RuntimeRequest(ctx, t, client, "http://"+fooName+".example.com")
	RuntimeRequest(ctx, t, client, "http://"+barName+".example.com")
}
