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
	"strings"
	"testing"
	"unicode"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/configmap"
)

const (
	controlNamespace = "knative-serving"
	domain           = ".example.com"
)

func TestNetGatewayAPIConfigNoService(t *testing.T) {
	if !(strings.Contains(test.ServingFlags.IngressClass, "gateway-api")) {
		t.Skip("Skip this test for non-gateway-api ingress.")
	}

	clients := test.Setup(t)
	ctx := context.Background()

	var configGateway, original *corev1.ConfigMap

	original, err := clients.KubeClient.CoreV1().ConfigMaps(controlNamespace).Get(ctx, "config-gateway", v1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get original config-gateway ConfigMap: %v", err)
	}

	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir")
	}
	testdata := pwd + "/testdata/"

	// set up configmap for ingress
	if ingress, ok := os.LookupEnv("INGRESS"); ok {
		switch ingress {
		case "contour":
			configGateway = ConfigMapFromTestFile(t, testdata+"contour-no-service-vis.yaml", "visibility")
		case "istio":
			configGateway = ConfigMapFromTestFile(t, testdata+"istio-no-service-vis.yaml", "visibility")
		case "default":
			t.Fatalf("value for INGRESS (%s) not supported", ingress)
		}

		configGateway.Name = "config-gateway"
	}

	updated, err := clients.KubeClient.CoreV1().ConfigMaps(controlNamespace).Update(ctx, configGateway, v1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update config-gateway ConfigMap: %v", err)
	}

	svcName, svcPort, svcCancel := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	defer svcCancel()

	_, client, ingressCancel := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts: []string{svcName + domain},
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
	defer ingressCancel()

	url := apis.HTTP(svcName + domain)

	// Verify the new service is accessible via the ingress.
	ingress.RuntimeRequest(ctx, t, client, url.URL().String())

	// restore the old configmap
	updated.Data = original.Data
	_, err = clients.KubeClient.CoreV1().ConfigMaps(controlNamespace).Update(ctx, updated, v1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to restore config-gateway ConfigMap: %v", err)
	}
}

// ConfigMapFromTestFile creates a corev1.ConfigMap resources from the config
// file read from the filepath
func ConfigMapFromTestFile(t testing.TB, name string, allowed ...string) *corev1.ConfigMap {
	t.Helper()

	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal("ReadFile() =", err)
	}

	var orig corev1.ConfigMap

	// Use sigs.k8s.io/yaml since it reads json struct
	// tags so things unmarshal properly.
	if err := yaml.Unmarshal(b, &orig); err != nil {
		t.Fatal("yaml.Unmarshal() =", err)
	}

	if len(orig.Data) != len(allowed) {
		// See here for why we only check in empty ConfigMaps:
		// https://github.com/knative/serving/issues/2668
		t.Errorf("Data = %v, wanted %v", orig.Data, allowed)
	}
	allow := sets.NewString(allowed...)
	for key := range orig.Data {
		if !allow.Has(key) {
			t.Errorf("Encountered key %q in %q that wasn't on the allowed list", key, name)
		}
	}
	// With the length and membership checks, we know that the keyspace matches.

	exampleBody, hasExampleBody := orig.Data[configmap.ExampleKey]
	// Check that exampleBody does not have lines that end in a trailing space.
	for i, line := range strings.Split(exampleBody, "\n") {
		if strings.TrimRightFunc(line, unicode.IsSpace) != line {
			t.Errorf("line %d of %q example contains trailing spaces", i, name)
		}
	}

	// Check that the hashed exampleBody matches the assigned annotation, if present.
	gotChecksum, hasExampleChecksumAnnotation := orig.Annotations[configmap.ExampleChecksumAnnotation]
	if hasExampleBody && hasExampleChecksumAnnotation {
		wantChecksum := configmap.Checksum(exampleBody)
		if gotChecksum != wantChecksum {
			t.Errorf("example checksum annotation = %s, want %s (you may need to re-run ./hack/update-codegen.sh)", gotChecksum, wantChecksum)
		}
	}

	return &orig
}
