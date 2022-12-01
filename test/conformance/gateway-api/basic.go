/*
Copyright 2019 The Knative Authors

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
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// TestBasics verifies that a no frills HTTPRoute exposes a simple Pod/Service via the public load balancer.
func TestBasics(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayapi.HTTPRouteSpec{
		CommonRouteSpec: gatewayapi.CommonRouteSpec{ParentRefs: []gatewayapi.ParentReference{
			testGateway,
		}},
		Hostnames: []gatewayapi.Hostname{gatewayapi.Hostname(name + ".example.com")},
		Rules: []gatewayapi.HTTPRouteRule{{
			BackendRefs: []gatewayapi.HTTPBackendRef{{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Port: portNumPtr(port),
						Name: gatewayapi.ObjectName(name),
					}}},
			},
		}},
	})

	RuntimeRequest(ctx, t, client, "http://"+name+".example.com")
}

// TestBasicsHTTP2 verifies that the same no-frills HTTPRoute over a Service with http/2 configured
// will see a ProtoMajor of 2.
func TestBasicsHTTP2(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameH2C)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayapi.HTTPRouteSpec{
		CommonRouteSpec: gatewayapi.CommonRouteSpec{ParentRefs: []gatewayapi.ParentReference{
			testGateway,
		}},
		Hostnames: []gatewayapi.Hostname{gatewayapi.Hostname(name + ".example.com")},
		Rules: []gatewayapi.HTTPRouteRule{{
			BackendRefs: []gatewayapi.HTTPBackendRef{{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Port: portNumPtr(port),
						Name: gatewayapi.ObjectName(name),
					}}},
			},
		}},
	})

	ri := RuntimeRequest(ctx, t, client, "http://"+name+".example.com")
	if ri == nil {
		return
	}

	if want, got := 2, ri.Request.ProtoMajor; want != got {
		t.Errorf("ProtoMajor = %d, wanted %d", got, want)
	}
}
