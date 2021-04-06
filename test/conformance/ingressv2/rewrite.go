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

// TestRewriteHost verifies that a RewriteHost rule can be used to implement vanity URLs.
func TestRewriteHost(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	portNum := gwv1alpha1.PortNumber(port)

	privateServiceName := test.ObjectNameForTest(t)
	privateHostName := privateServiceName + "." + test.ServingNamespace + ".svc.cluster.local"

	// Create a simple Ingress over the Service.
	// TODO: Make this HTTPRoute cluster local visibility.
	_, _, _ = CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(privateHostName)},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        &portNum,
				ServiceName: &name,
			}},
		}},
	})

	// Slap an ExternalName service in front of the kingress
	namespace, name := getIngress()
	loadbalancerAddress := name + "." + namespace + ".svc"
	createExternalNameService(ctx, t, clients, privateHostName, loadbalancerAddress)

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	hosts := []gwv1alpha1.Hostname{
		gwv1alpha1.Hostname(name + "." + "vanity.ismy.name"),
		gwv1alpha1.Hostname(name + "." + "vanity.isalsomy.number"),
	}

	portNumIng := gwv1alpha1.PortNumber(80)

	// Now create a RewriteHost ingress to point a custom Host at the Service
	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: hosts,
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        &portNumIng,
				ServiceName: &privateServiceName,
			}},
			Filters: []gwv1alpha1.HTTPRouteFilter{
				{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{"Host": privateHostName, ":Authority": privateHostName},
					},
				},
			},
		}},
	})

	for _, host := range hosts {
		RuntimeRequest(ctx, t, client, "http://"+string(host))
	}
}
