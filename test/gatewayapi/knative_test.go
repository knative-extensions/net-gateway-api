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

package gatewayapi

import (
	"testing"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/pkg/features"

	"k8s.io/apimachinery/pkg/util/sets"
)

// KnativeConformanceProfile is a ConformanceProfile that covers testing Gateway API features
// that Knative require
var (
	KnativeConformanceProfile = suite.ConformanceProfile{
		Name: "KNATIVE",
		CoreFeatures: sets.New(
			// Core HTTPRoute Features
			features.SupportGateway,
			features.SupportReferenceGrant,
			features.SupportHTTPRoute,

			// Needed for traffic through the activator (scale to zero, handling burst traffic)
			features.SupportHTTPRouteBackendRequestHeaderModification,

			// HTTPRoute Experimental Features
			// Required to support the different backend protocols we need
			features.SupportHTTPRouteBackendProtocolH2C,
			features.SupportHTTPRouteBackendProtocolWebSocket,

			// This feature is required for DomainMapping
			// HTTPRoute Extended Features
			features.SupportHTTPRouteHostRewrite,

			// Optional - testing to check for support
			features.SupportHTTPRouteRequestTimeout,
			features.SupportHTTPRouteSchemeRedirect,

		// This _could_ be used for in cluster deployments
		// suite.SupportGatewayInfrastructureMetadata, // https://github.com/kubernetes-sigs/gateway-api/pull/2845
		),
	}
)

func init() {
	suite.RegisterConformanceProfile(KnativeConformanceProfile)
}

func TestGatewayConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)
	opts.ConformanceProfiles.Insert(KnativeConformanceProfile.Name)
	conformance.RunConformanceWithOptions(t, opts)
}
