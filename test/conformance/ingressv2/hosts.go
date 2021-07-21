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

// TestMultipleHosts verifies that an Ingress can respond to multiple hosts.
func TestMultipleHosts(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	hosts := []gwv1alpha1.Hostname{
		"foo.com",
		"www.foo.com",
		"a-b-1.something-really-really-long.knative.dev",
		"add.your.interesting.domain.here.io",
	}

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	for i, host := range hosts {
		hosts[i] = gwv1alpha1.Hostname(name + "." + string(host))
	}

	// Create a simple HTTPRoute over the Service.
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Hostnames: hosts,
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        portNumPtr(port),
				ServiceName: &name,
			}},
		}},
	})

	for _, host := range hosts {
		RuntimeRequest(ctx, t, client, "http://"+string(host))
	}
}
