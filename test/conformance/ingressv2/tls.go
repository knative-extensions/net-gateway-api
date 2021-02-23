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

	"knative.dev/net-ingressv2/test"
	"knative.dev/networking/pkg/apis/networking"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// TestIngressTLS verifies that the HTTPRoute properly handles the TLS field.
func TestIngressTLS(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	hosts := []string{name + ".example.com"}

	secretName, _ := CreateTLSSecret(ctx, t, clients, hosts)

	_, client, _ := CreateHTTPRouteReady(ctx, t, clients, gwv1alpha1.HTTPRouteSpec{
		Gateways:  testGateway,
		Hostnames: []gwv1alpha1.Hostname{gwv1alpha1.Hostname(name + ".example.com")},
		TLS: &gwv1alpha1.RouteTLSConfig{CertificateRef: gwv1alpha1.LocalObjectReference{
			Group: "core", // empty string "" does not work. see https://github.com/kubernetes-sigs/gateway-api/pull/562
			Kind:  "Secret",
			Name:  secretName,
		}},
		Rules: []gwv1alpha1.HTTPRouteRule{{
			ForwardTo: []gwv1alpha1.HTTPRouteForwardTo{{
				Port:        &port,
				ServiceName: &name,
			}},
		}},
	})

	// Check without TLS.
	RuntimeRequest(ctx, t, client, "http://"+name+".example.com")

	// Check with TLS.
	RuntimeRequest(ctx, t, client, "https://"+name+".example.com")
}

// TODO(mattmoor): Consider adding variants where we have multiple hosts with distinct certificates.
