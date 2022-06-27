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

	"knative.dev/net-gateway-api/test"
	"knative.dev/networking/pkg/apis/networking"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TestMultipleHosts verifies that an Ingress can respond to multiple hosts.
func TestMultipleHosts(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	hosts := []gatewayv1alpha2.Hostname{
		"foo.com",
		"www.foo.com",
		"a-b-1.something-really-really-long.knative.dev",
		"add.your.interesting.domain.here.io",
	}

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	for i, host := range hosts {
		hosts[i] = gatewayv1alpha2.Hostname(name + "." + string(host))
	}

	// Create a simple HTTPRoute over the Service.
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentReference{
			testGateway,
		}},
		Hostnames: hosts,
		Rules: []gatewayv1alpha2.HTTPRouteRule{{
			BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
				BackendRef: gatewayv1alpha2.BackendRef{
					BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
						Port: portNumPtr(port),
						Name: gatewayv1alpha2.ObjectName(name),
					}}},
			},
		}},
	})

	for _, host := range hosts {
		RuntimeRequest(ctx, t, client, "http://"+string(host))
	}
}
