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
	"embed"
	"testing"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"

	"k8s.io/apimachinery/pkg/util/sets"
)

//go:embed *
var Manifests embed.FS

// KnativeConformanceProfile is a ConformanceProfile that covers testing Gateway API features
// that Knative require
var (
	KnativeConformanceProfile = suite.ConformanceProfile{
		Name: "KNATIVE",
		CoreFeatures: sets.New(
			// Core HTTPRoute Features
			suite.SupportGateway,
			suite.SupportReferenceGrant,
			suite.SupportHTTPRoute,

			// HTTPRoute Core Features to be merged
			// ----
			// ServiceBackend Support https://github.com/kubernetes-sigs/gateway-api/pull/2828
			// HTTPRoute Weights      https://github.com/kubernetes-sigs/gateway-api/pull/2814

			// Needed for traffic through the activator (scale to zero, handling burst traffic)
			suite.SupportHTTPRouteBackendRequestHeaderModification,

			// HTTPRoute Experimental Features
			suite.SupportHTTPRouteBackendProtocolH2C,
			suite.SupportHTTPRouteBackendProtocolWebSocket,
		),
		ExtendedFeatures: sets.New(
			// This feature is required for DomainMapping
			// HTTPRoute Extended Features
			suite.SupportHTTPRouteHostRewrite,

			// Optional - testing to check for support
			suite.SupportHTTPRouteRequestTimeout,
			suite.SupportHTTPRouteSchemeRedirect,

			// This _could_ be used for in cluster deployments
			// suite.SupportGatewayInfrastructureMetadata, // https://github.com/kubernetes-sigs/gateway-api/pull/2845
		),
	}
)

func init() {
	suite.RegisterConformanceProfile(KnativeConformanceProfile)
}

func TestConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)
	opts.ManifestFS = append(opts.ManifestFS, Manifests)
	opts.ConformanceProfiles.Insert(KnativeConformanceProfile.Name)
	conformance.RunConformanceWithOptions(t, opts)
}
