//go:build e2e
// +build e2e

/*
Copyright 2024 The Knative Authors

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

package e2e

import (
	"context"
	"os"
	"testing"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	. "knative.dev/networking/test/defaultsystem"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/system"
)

func TestGatewayWithNoService(t *testing.T) {
	clients := test.Setup(t)
	ctx := context.Background()

	var configGateway, original *corev1.ConfigMap

	original, err := clients.KubeClient.CoreV1().ConfigMaps(system.Namespace()).Get(ctx, "config-gateway", v1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get original config-gateway ConfigMap: %v", err)
	}

	// set up configmap for ingress
	if ingress, ok := os.LookupEnv("INGRESS"); ok {
		switch ingress {
		case "contour":
			configGateway = ConfigMapFromTestFile(t, "testdata/contour-no-service-vis.yaml")
		case "istio":
			configGateway = ConfigMapFromTestFile(t, "testdata/istio-no-service-vis.yaml")
		case "default":
			t.Fatalf("value for INGRESS (%s) not supported", ingress)
		}

		configGateway.Name = "config-gateway"
	}

	updated, err := clients.KubeClient.CoreV1().ConfigMaps(system.Namespace()).Update(ctx, configGateway, v1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update config-gateway ConfigMap: %v", err)
	}

	svcName, svcPort, svcCancel := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	_, client, ingressCancel := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts: []string{svcName + test.NetworkingFlags.ServiceDomain},
			HTTP: &v1alpha1.HTTPIngressRuleValue{
				Paths: []v1alpha1.HTTPIngressPath{{
					Splits: []v1alpha1.IngressBackendSplit{{
						IngressBackend: v1alpha1.IngressBackend{
							ServiceName:      svcName,
							ServiceNamespace: test.ServingNamespace,
							ServicePort:      intstr.FromInt(svcPort),
						},
					}},
				}},
			},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
		}},
	})

	test.EnsureCleanup(t, func() {
		// restore the old configmap
		updated.Data = original.Data
		_, err = clients.KubeClient.CoreV1().ConfigMaps(system.Namespace()).Update(ctx, updated, v1.UpdateOptions{})
		if err != nil {
			t.Fatalf("failed to restore config-gateway ConfigMap: %v", err)
		}

		svcCancel()
		ingressCancel()
	})

	url := apis.HTTP(svcName + test.NetworkingFlags.ServiceDomain)

	// Verify the new service is accessible via the ingress.
	ingress.RuntimeRequest(ctx, t, client, url.URL().String())

}

// ConfigMapFromTestFile creates a corev1.ConfigMap resources from the config
// file read from the filepath
func ConfigMapFromTestFile(t testing.TB, name string) *corev1.ConfigMap {
	t.Helper()

	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal("ReadFile() =", err)
	}

	var orig corev1.ConfigMap

	if err := yaml.Unmarshal(b, &orig); err != nil {
		t.Fatal("yaml.Unmarshal() =", err)
	}

	return &orig
}
