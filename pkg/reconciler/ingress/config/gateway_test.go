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

package config

import (
	"testing"

	ktesting "knative.dev/pkg/configmap/testing"
)

func TestGateway(t *testing.T) {
	cm, example := ktesting.ConfigMapsFromTestFile(t, GatewayConfigName)

	if _, err := NewGatewayFromConfigMap(cm); err != nil {
		t.Error("NewContourFromConfigMap(actual) =", err)
	}

	if _, err := NewGatewayFromConfigMap(example); err != nil {
		t.Error("NewContourFromConfigMap(example) =", err)
	}
}
