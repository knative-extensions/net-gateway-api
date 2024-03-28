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
	"strings"
	"testing"

	. "knative.dev/pkg/configmap/testing"
)

var badVisibilityEntry = `
this isn't yaml???
`

var emptyVisibilityEntry = `

`

func TestGateway(t *testing.T) {
	cm, example := ConfigMapsFromTestFile(t, GatewayConfigName, defaultTLSSecretKey)

	if _, err := NewGatewayFromConfigMap(cm); err != nil {
		t.Error("NewContourFromConfigMap(actual) =", err)
	}

	cm.Data[defaultTLSSecretKey] = "secret-no-namespace"
	if _, err := NewGatewayFromConfigMap(cm); err == nil {
		t.Error("NewContourFromConfigMap(actual) with bad defaultTLSSecretKey value did not fail")
	}

	delete(cm.Data, defaultTLSSecretKey)

	cm.Data[visibilityConfigKey] = badVisibilityEntry

	_, err := NewGatewayFromConfigMap(cm)
	expectedError := "error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type map[v1alpha1.IngressVisibility]config.visibilityValu"
	if err == nil {
		t.Error("NewContourFromConfigMap(actual) with bad visibility config value did not fail")
	} else if !strings.Contains(err.Error(), expectedError) {
		t.Error("NewContourFromConfigMap(actual) with bad visibility config failed with unexpected error message:", err)
	}

	cm.Data[visibilityConfigKey] = emptyVisibilityEntry

	_, err = NewGatewayFromConfigMap(cm)
	expectedError = "visibility \"ClusterLocal\" must not be empty"
	if err == nil {
		t.Error("NewContourFromConfigMap(actual) with empty visibility config value did not fail")
	} else if !strings.Contains(err.Error(), expectedError) {
		t.Error("NewContourFromConfigMap(actual) with empty visibility config value failed with unexpected error message:", err)
	}

	if _, err := NewGatewayFromConfigMap(example); err != nil {
		t.Error("NewContourFromConfigMap(example) =", err)
	}

}
