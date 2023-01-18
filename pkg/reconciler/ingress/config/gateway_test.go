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

	corev1 "k8s.io/api/core/v1"
	. "knative.dev/pkg/configmap/testing"
)

func TestFromConfigMap(t *testing.T) {
	cm, example := ConfigMapsFromTestFile(t, GatewayConfigName)

	if _, err := FromConfigMap(cm); err != nil {
		t.Error("FromConfigMap(actual) =", err)
	}

	if _, err := FromConfigMap(example); err != nil {
		t.Error("FromConfigMap(example) =", err)
	}
}

func TestFromConfigMapErrors(t *testing.T) {
	cases := []struct {
		name string
		data map[string]string
		want string
	}{{
		name: "external-gateways bad yaml",
		data: map[string]string{
			"external-gateways": `{`,
		},
		want: `unable to parse "external-gateways"`,
	}, {
		name: "local-gateways bad yaml",
		data: map[string]string{
			"local-gateways": `{`,
		},
		want: `unable to parse "local-gateways"`,
	}, {
		name: "external-gateways multiple entries",
		data: map[string]string{
			"external-gateways": `[{
					"class":"boo",
					"http-listener-name":"http",
					"gateway": "ns/n"
				},{
					"class":"boo",
					"http-listener-name":"http",
					"gateway": "ns/n"
				}]`,
		},
		want: `only a single external gateway is supported`,
	}, {
		name: "local-gateways multiple entries",
		data: map[string]string{
			"local-gateways": `[{
					"class":"boo",
					"http-listener-name":"http",
					"gateway": "ns/n"
				},{
					"class":"boo",
					"http-listener-name":"http",
					"gateway": "ns/n"
				}]`,
		},
		want: `only a single local gateway is supported`,
	}, {
		name: "missing gateway class",
		data: map[string]string{
			"local-gateways": `[{"gateway": "namespace/name"}]`,
		},
		want: `unable to parse "local-gateways": entry [0] field "class" is required`,
	}, {
		name: "missing gateway name",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "namespace/"}]`,
		},
		want: `unable to parse "local-gateways": failed to parse "gateway"`,
	}, {
		name: "missing gateway namespace",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "/name"}]`,
		},
		want: `unable to parse "local-gateways": failed to parse "gateway"`,
	}, {
		name: "bad gateway entry",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "name"}]`,
		},
		want: `unable to parse "local-gateways"`,
	}, {
		name: "missing service name",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "ns/n", "service":"ns/"}]`,
		},
		want: `unable to parse "local-gateways": failed to parse "service"`,
	}, {
		name: "missing service namespace",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "ns/n", "service":"/name"}]`,
		},
		want: `unable to parse "local-gateways": failed to parse "service"`,
	}, {
		name: "bad service entry",
		data: map[string]string{
			"local-gateways": `[{"class": "class", "gateway": "ns/n", "service":"name"}]`,
		},
		want: `unable to parse "local-gateways"`,
	}, {
		name: "missing gateway http-listener-name",
		data: map[string]string{
			"local-gateways": `[{"class":"class", "gateway": "namespace/name"}]`,
		},
		want: `unable to parse "local-gateways": entry [0] field "http-listener-name" is required`,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := FromConfigMap(&corev1.ConfigMap{Data: tc.data})

			if got == nil {
				t.Fatal("Expected an error to occur")
			}
			if !strings.HasPrefix(got.Error(), tc.want) {
				t.Errorf("Unexpected error message %q - wanted prefix %q", got, tc.want)
			}
		})
	}

}

func TestGatewayNoService(t *testing.T) {
	_, err := FromConfigMap(&corev1.ConfigMap{
		Data: map[string]string{
			"external-gateways": `
      - class: istio
        http-listener-name: http
        gateway: istio-system/knative-gateway`,
			"local-gateways": `
      - class: istio
        http-listener-name: http
        gateway: istio-system/knative-local-gateway`,
		},
	})

	if err != nil {
		t.Errorf("FromConfigMap(noService) = %v", err)
	}
}
