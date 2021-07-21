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

package test

import (
	"os"

	netTest "knative.dev/networking/test"
)

var (
	// defaultGatewayAddress is a named gateway address for the default gateway.
	defaultGatewayAddress = "istio-ingressgateway.istio-system"

	// defaultGatewayClass is a GatewayClass name for the default gateway.
	defaultGatewayClass = "istio"

	// defaultGatewayAddress is a named gateway address for the local visibility gateway.
	defaultLocalGatewayAddress = "knative-local-gateway.istio-system"

	// defaultGatewayClass is a GatewayClass name for the local visibility gateway.
	defaultLocalGatewayClass = "istio"
)

func GatewayAddress() string {
	address := defaultGatewayAddress
	if gatewayOverride := os.Getenv("GATEWAY_ADDRESS_OVERRIDE"); gatewayOverride != "" {
		address = gatewayOverride
	}
	return address + ".svc." + netTest.NetworkingFlags.ClusterSuffix
}

func LocalGatewayAddress() string {
	address := defaultLocalGatewayAddress
	if gatewayOverride := os.Getenv("LOCAL_GATEWAY_ADDRESS_OVERRIDE"); gatewayOverride != "" {
		address = gatewayOverride
	}
	return address + ".svc." + netTest.NetworkingFlags.ClusterSuffix
}

func GatewayClass() string {
	if gatewayClassOverride := os.Getenv("GATEWAY_CLASS_OVERRIDE"); gatewayClassOverride != "" {
		return gatewayClassOverride
	}
	return defaultGatewayClass
}

func LocalGatewayClass() string {
	if gatewayClassOverride := os.Getenv("LOCAL_GATEWAY_CLASS_OVERRIDE"); gatewayClassOverride != "" {
		return gatewayClassOverride
	}
	return defaultLocalGatewayClass
}

// GatewayNamespace overrides the namespace where Gateway is deployed.
// For example, Gateway for Istio requires to be deployed in the same namespace with the ingress services.
func GatewayNamespace(namespace string) string {
	if gatewayOverride := os.Getenv("GATEWAY_NAMESPACE_OVERRIDE"); gatewayOverride != "" {
		return gatewayOverride
	}
	return namespace
}
