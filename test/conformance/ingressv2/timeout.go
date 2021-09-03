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
	"fmt"
	"net/http"
	"testing"
	"time"

	"knative.dev/net-ingressv2/test"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TestTimeout verifies that an Ingress implements "no timeout".
func TestTimeout(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateTimeoutService(ctx, t, clients)

	// Create a simple HTTPRoute over the Service.
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentRef{
			testGateway,
		}},
		Hostnames: []gatewayv1alpha2.Hostname{gatewayv1alpha2.Hostname(name + ".example.com")},
		Rules: []gatewayv1alpha2.HTTPRouteRule{{
			BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
				BackendRef: gatewayv1alpha2.BackendRef{
					BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
						Port: portNumPtr(port),
						Name: name,
					}}},
			},
		}},
	})

	const timeout = 10 * time.Second

	tests := []struct {
		name         string
		code         int
		initialDelay time.Duration
		delay        time.Duration
	}{{
		name: "no delays is OK",
		code: http.StatusOK,
	}, {
		name:         "large delay before headers is ok",
		code:         http.StatusOK,
		initialDelay: timeout,
	}, {
		name:  "large delay after headers is ok",
		code:  http.StatusOK,
		delay: timeout,
	}}

	// TODO: https://github.com/knative-sandbox/net-ingressv2/issues/18
	// As Ingress v2 does not have prober, it needs to make sure backend is ready.
	waitForBackend(t, client, "http://"+name+".example.com?initialTimeout=0&timeout=0")

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			checkTimeout(ctx, t, client, name, test.code, test.initialDelay, test.delay)
		})
	}
}

func checkTimeout(ctx context.Context, t *testing.T, client *http.Client, name string, code int, initial time.Duration, timeout time.Duration) {
	t.Helper()

	resp, err := client.Get(fmt.Sprintf("http://%s.example.com?initialTimeout=%d&timeout=%d",
		name, initial.Milliseconds(), timeout.Milliseconds()))
	if err != nil {
		t.Fatal("Error making GET request:", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != code {
		t.Errorf("Unexpected status code: %d, wanted %d", resp.StatusCode, code)
		DumpResponse(ctx, t, resp)
	}
}
