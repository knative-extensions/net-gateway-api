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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/configmap"
	"sigs.k8s.io/yaml"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

const (
	// GatewayConfigName is the config map name for the gateway configuration.
	GatewayConfigName = "config-gateway"

	externalGatewaysKey = "external-gateways"
	localGatewaysKey    = "local-gateways"
)

func defaultExternalGateways() []Gateway {
	return []Gateway{{
		NamespacedName: types.NamespacedName{
			Name:      "knative-gateway",
			Namespace: "istio-system",
		},

		Class: "istio",
		Service: &types.NamespacedName{
			Name:      "istio-ingressgateway",
			Namespace: "istio-system",
		},
	}}
}

func defaultLocalGateways() []Gateway {
	return []Gateway{{
		NamespacedName: types.NamespacedName{
			Name:      "knative-local-gateway",
			Namespace: "istio-system",
		},
		Class: "istio",
		Service: &types.NamespacedName{
			Name:      "knative-local-gateway",
			Namespace: "istio-system",
		},
	}}
}

// GatewayPlugin specifies which Gateways are used for external/local traffic
type GatewayPlugin struct {
	ExternalGateways []Gateway
	LocalGateways    []Gateway
}

func (g *GatewayPlugin) ExternalGateway() Gateway {
	return g.ExternalGateways[0]
}

func (g *GatewayPlugin) LocalGateway() Gateway {
	return g.LocalGateways[0]
}

type Gateway struct {
	types.NamespacedName

	Class   string
	Service *types.NamespacedName
}

// FromConfigMap creates a GatewayPlugin config from the supplied ConfigMap
func FromConfigMap(cm *corev1.ConfigMap) (*GatewayPlugin, error) {
	var (
		err    error
		config = &GatewayPlugin{}
	)

	if data, ok := cm.Data[externalGatewaysKey]; ok {
		config.ExternalGateways, err = parseGatewayConfig(data)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse %q: %w", externalGatewaysKey, err)
		}
	}

	if data, ok := cm.Data[localGatewaysKey]; ok {
		config.LocalGateways, err = parseGatewayConfig(data)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse %q: %w", localGatewaysKey, err)
		}
	}

	switch len(config.ExternalGateways) {
	case 0:
		config.ExternalGateways = defaultExternalGateways()
	case 1:
	default:
		return nil, fmt.Errorf("Only a single external gateway is supported")
	}

	switch len(config.LocalGateways) {
	case 0:
		config.LocalGateways = defaultLocalGateways()
	case 1:
	default:
		return nil, fmt.Errorf("Only a single local gateway is supported")
	}

	return config, nil
}

func parseGatewayConfig(data string) ([]Gateway, error) {
	var entries []map[string]string

	if err := yaml.Unmarshal([]byte(data), &entries); err != nil {
		return nil, err
	}

	gws := make([]Gateway, 0, len(entries))

	for i, entry := range entries {
		fmt.Println(entry)
		gw := Gateway{}

		err := configmap.Parse(entry,
			configmap.AsString("class", &gw.Class),
			configmap.AsNamespacedName("gateway", &gw.NamespacedName),
			configmap.AsOptionalNamespacedName("service", &gw.Service),
		)
		if err != nil {
			return nil, err
		}

		if len(strings.TrimSpace(gw.Class)) == 0 {
			return nil, fmt.Errorf(`entry [%d] field "class" is required`, i)
		}

		gws = append(gws, gw)
	}

	return gws, nil
}
