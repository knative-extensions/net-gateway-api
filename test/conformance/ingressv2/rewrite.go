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
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TestRewriteHost verifies that a RewriteHost rule can be used to implement vanity URLs.
func TestRewriteHost(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	privateServiceName := test.ObjectNameForTest(t)
	privateHostName := privateServiceName + "." + test.ServingNamespace + ".svc.cluster.local"

	// Create a simple Ingress over the Service.
	_, _, _ = CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentRef{
			testLocalGateway,
		}},
		Hostnames: []gatewayv1alpha2.Hostname{gatewayv1alpha2.Hostname(privateHostName)},
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

	// Slap an ExternalName service in front of the kingress
	namespace, name := getIngress()
	loadbalancerAddress := name + "." + namespace + ".svc"
	createExternalNameService(ctx, t, clients, privateHostName, loadbalancerAddress)

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	hosts := []gatewayv1alpha2.Hostname{
		gatewayv1alpha2.Hostname(name + "." + "vanity.ismy.name"),
		gatewayv1alpha2.Hostname(name + "." + "vanity.isalsomy.number"),
	}

	// Now create a RewriteHost ingress to point a custom Host at the Service
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gatewayv1alpha2.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{ParentRefs: []gatewayv1alpha2.ParentRef{
			testGateway,
		}},
		Hostnames: hosts,
		Rules: []gatewayv1alpha2.HTTPRouteRule{{
			BackendRefs: []gatewayv1alpha2.HTTPBackendRef{{
				BackendRef: gatewayv1alpha2.BackendRef{
					BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
						Port: portNumPtr(80),
						Name: privateServiceName,
					}}},
			},
			Filters: []gatewayv1alpha2.HTTPRouteFilter{{
				Type: gatewayv1alpha2.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayv1alpha2.HTTPRequestHeaderFilter{
					Set: []gatewayv1alpha2.HTTPHeader{
						{
							Name:  "Host",
							Value: privateHostName,
						},
						// This is invalid since v1alpha2. We need to wait for https://github.com/kubernetes-sigs/gateway-api/pull/731
						{
							Name:  ":Authority",
							Value: privateHostName,
						},
					},
				}}},
		}},
	})

	for _, host := range hosts {
		RuntimeRequest(ctx, t, client, "http://"+string(host))
	}
}
