/*
Copyright 2018 The Knative Authors

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

	// defaultIstioGateway is the default gateway.
	defaultIstioGateway = "istio-system/test-gateway"

	// defaultIstioLocalGateway is the default local gateway:
	defaultIstioLocalGateway = "istio-system/test-local-gateway"

	// defaultLocalGatewayService holds the default local gateway service.
	defaultLocalGatewayService = "istio-system/knative-local-gateway"

	// defaultGatewayService is the default gateway service.
	defaultGatewayService = "istio-system/istio-ingressgateway"
)

type GatewayConfig struct {
	GatewayClass string `json:"class,omitempty"`
	Gateway      string `json:"gateway,omitempty"`
	Service      string `json:"service,omitempty"`
}

// Gateway maps gateways to routes by matching the gateway's
// label selectors to the route's labels.
type Gateway struct {
	// Gateways map from gateway to label selector.  If a route has
	// labels matching a particular selector, it will use the
	// corresponding gateway.  If multiple selectors match, we choose
	// the most specific selector.
	Gateways map[v1alpha1.IngressVisibility]*GatewayConfig
}

// NewGatewayFromConfigMap creates a Gateway from the supplied ConfigMap
func NewGatewayFromConfigMap(configMap *corev1.ConfigMap) (*Gateway, error) {
	v, ok := configMap.Data[visibilityConfigKey]
	if !ok {
		// These are the defaults.
		return &Gateway{
			Gateways: map[v1alpha1.IngressVisibility]*GatewayConfig{
				v1alpha1.IngressVisibilityExternalIP:   {GatewayClass: defaultGatewayClass, Gateway: defaultIstioGateway, Service: defaultGatewayService},
				v1alpha1.IngressVisibilityClusterLocal: {GatewayClass: defaultGatewayClass, Gateway: defaultIstioLocalGateway, Service: defaultLocalGatewayService},
			},
		}, nil
	}

	entry := make(map[v1alpha1.IngressVisibility]GatewayConfig)
	if err := yaml.Unmarshal([]byte(v), &entry); err != nil {
		return nil, err
	}

	for _, vis := range []v1alpha1.IngressVisibility{
		v1alpha1.IngressVisibilityClusterLocal,
		v1alpha1.IngressVisibilityExternalIP,
	} {
		if _, ok := entry[vis]; !ok {
			return nil, fmt.Errorf("visibility %q must not be empty", vis)
		}
	}
	c := Gateway{Gateways: map[v1alpha1.IngressVisibility]*GatewayConfig{}}

	for key, value := range entry {
		key, value := key, value
		// Check that the visibility makes sense.
		switch key {
		case v1alpha1.IngressVisibilityClusterLocal, v1alpha1.IngressVisibilityExternalIP:
		default:
			return nil, fmt.Errorf("unrecognized visibility: %q", key)
		}

		// See if the Service is a valid namespace/name token.
		if _, _, err := cache.SplitMetaNamespaceKey(value.Service); err != nil {
			return nil, err
		}

		// See if the Gateway is a valid namespace/name token.
		if _, _, err := cache.SplitMetaNamespaceKey(value.Gateway); err != nil {
			return nil, err
		}

		if value.GatewayClass == "" {
			// TODO: set default instead of error?
			return nil, fmt.Errorf("visibility %q must set class", key)
		}

		c.Gateways[key] = &value
	}
	return &c, nil
}

// LookupGateway returns a gateway given a visibility config.
func (c *Gateway) LookupGateway(visibility v1alpha1.IngressVisibility) (string, string, error) {
	if c.Gateways[visibility] == nil {
		return "", "", nil
	}
	return cache.SplitMetaNamespaceKey(c.Gateways[visibility].Gateway)
}

// LookupGatewayClass returns a gatewayclass given a visibility config.
func (c *Gateway) LookupGatewayClass(visibility v1alpha1.IngressVisibility) string {
	if c.Gateways[visibility] == nil {
		return ""
	}
	return c.Gateways[visibility].GatewayClass
}

// LookupService returns a gateway service address given a visibility config.
func (c *Gateway) LookupService(visibility v1alpha1.IngressVisibility) string {
	if c.Gateways[visibility] == nil {
		return ""
	}
	return c.Gateways[visibility].Service
}
