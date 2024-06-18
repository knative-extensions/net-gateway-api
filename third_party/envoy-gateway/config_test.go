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

package main

import (
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

func TestFromConfigMap(t *testing.T) {
	bytes, err := os.ReadFile(config.GatewayConfigName + ".yaml")
	if err != nil {
		t.Fatalf("failed to read %q: %s", config.GatewayConfigName, err)
	}

	cm := &corev1.ConfigMap{}
	err = yaml.Unmarshal(bytes, cm)
	if err != nil {
		t.Fatalf("failed to unmarshal %q: %s", config.GatewayConfigName, err)
	}

	if _, err := config.FromConfigMap(cm); err != nil {
		t.Error("FromConfigMap(actual) =", err)
	}
}
