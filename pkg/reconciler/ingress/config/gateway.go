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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/yaml"

	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

const (
	// GatewayConfigName is the config map name for the gateway configuration.
	GatewayConfigName = "config-gateway"

	visibilityConfigKey = "visibility"

	// defaultGatewayClass is the gatewayclass name for the gateway.
	defaultGatewayClass = "istio"

	// defaultClusterLocalHTTPListener is the name of the listener for HTTP traffic
	defaultClusterLocalHTTPListener = "default"

	// defaultExternalIPHTTPListener
	defaultExternalIPHTTPListener = "default"
)

var (
	// defaultIstioGateway is the default gateway.
	defaultIstioGateway = &types.NamespacedName{Namespace: "istio-system", Name: "knative-gateway"}

	// defaultIstioLocalGateway is the default local gateway:
	defaultIstioLocalGateway = &types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"}

	// defaultLocalGatewayService holds the default local gateway service.
	defaultLocalGatewayService = &types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"}

	// defaultGatewayService is the default gateway service.
	defaultGatewayService = &types.NamespacedName{Namespace: "istio-system", Name: "istio-ingressgateway"}
)

type GatewayConfig struct {
	GatewayClass     string
	Gateway          *types.NamespacedName
	Service          *types.NamespacedName
	HTTPListenerName string
}

type visibilityValue struct {
	GatewayClass     string `json:"class,omitempty"`
	Gateway          string `json:"gateway,omitempty"`
	Service          string `json:"service,omitempty"`
	HTTPListenerName string `json:"httpListenerName,omitempty"`
}

// Gateway maps gateways to routes by matching the gateway's
// label selectors to the route's labels.
type Gateway struct {
	// Gateways map from gateway to label selector.  If a route has
	// labels matching a particular selector, it will use the
	// corresponding gateway.  If multiple selectors match, we choose
	// the most specific selector.
	Gateways map[v1alpha1.IngressVisibility]GatewayConfig
}

// NewGatewayFromConfigMap creates a Gateway from the supplied ConfigMap
func NewGatewayFromConfigMap(configMap *corev1.ConfigMap) (*Gateway, error) {
	v, ok := configMap.Data[visibilityConfigKey]
	if !ok {
		// These are the defaults.
		return &Gateway{
			Gateways: map[v1alpha1.IngressVisibility]GatewayConfig{
				v1alpha1.IngressVisibilityExternalIP: {
					GatewayClass:     defaultGatewayClass,
					Gateway:          defaultIstioGateway,
					Service:          defaultGatewayService,
					HTTPListenerName: defaultExternalIPHTTPListener,
				},
				v1alpha1.IngressVisibilityClusterLocal: {
					GatewayClass:     defaultGatewayClass,
					Gateway:          defaultIstioLocalGateway,
					Service:          defaultLocalGatewayService,
					HTTPListenerName: defaultClusterLocalHTTPListener,
				},
			},
		}, nil
	}

	visConfig := make(map[v1alpha1.IngressVisibility]visibilityValue)
	if err := yaml.Unmarshal([]byte(v), &visConfig); err != nil {
		return nil, err
	}

	for _, vis := range []v1alpha1.IngressVisibility{
		v1alpha1.IngressVisibilityClusterLocal,
		v1alpha1.IngressVisibilityExternalIP,
	} {
		if _, ok := visConfig[vis]; !ok {
			return nil, fmt.Errorf("visibility %q must not be empty", vis)
		}
	}

	entry := make(map[v1alpha1.IngressVisibility]GatewayConfig)
	for key, value := range visConfig {
		// Check that the visibility makes sense.
		switch key {
		case v1alpha1.IngressVisibilityClusterLocal, v1alpha1.IngressVisibilityExternalIP:
		default:
			return nil, fmt.Errorf("unrecognized visibility: %q", key)
		}
		if value.GatewayClass == "" {
			return nil, fmt.Errorf("visibility %q must set gatewayclass", key)
		}
		gateway, err := parseNamespacedName(value.Gateway)
		if err != nil {
			return nil, fmt.Errorf("visibility %q failed to parse gateway: %w", key, err)
		}
		service, err := parseNamespacedName(value.Service)
		if err != nil {
			return nil, fmt.Errorf("visibility %q failed to parse service: %w", key, err)
		}
		if value.HTTPListenerName == "" {
			return nil, fmt.Errorf("visibility %q must set httpListenerName", key)
		}
		entry[key] = GatewayConfig{
			GatewayClass:     value.GatewayClass,
			Gateway:          gateway,
			Service:          service,
			HTTPListenerName: value.HTTPListenerName,
		}
	}
	return &Gateway{Gateways: entry}, nil
}

func parseNamespacedName(namespacedName string) (*types.NamespacedName, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(namespacedName)
	if err != nil {
		return nil, err
	} else if namespace == "" || name == "" {
		return nil, fmt.Errorf("missing namespace or name in %q", namespacedName)
	}
	return &types.NamespacedName{Namespace: namespace, Name: name}, nil
}
