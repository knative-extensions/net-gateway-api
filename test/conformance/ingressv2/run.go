/*
Copyright 2020 The Knative Authors

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

package ingress

import (
	"strings"
	"testing"

	"knative.dev/networking/test"
)

var istioStableTests = map[string]func(t *testing.T){
	"basics":                       TestBasics,
	"basics/http2":                 TestBasicsHTTP2,
	"headers/pre-split":            TestPreSplitSetHeaders,
	"headers/post-split":           TestPostSplitSetHeaders,
	"dispatch/path":                TestPath,
	"dispatch/percentage":          TestPercentage,
	"dispatch/path_and_percentage": TestPathAndPercentageSplit,
	"dispatch/rule":                TestRule,
	"hosts/multiple":               TestMultipleHosts,
	"timeout":                      TestTimeout,
	"websocket":                    TestWebsocket,
	"websocket/split":              TestWebsocketSplit,
	"grpc":                         TestGRPC,
	"grpc/split":                   TestGRPCSplit,
}

var contourStableTests = map[string]func(t *testing.T){
	"basics": TestBasics,
}

var stableTests = map[string]func(t *testing.T){
	"basics":                       TestBasics,
	"basics/http2":                 TestBasicsHTTP2,
	"headers/pre-split":            TestPreSplitSetHeaders,
	"headers/post-split":           TestPostSplitSetHeaders,
	"dispatch/path":                TestPath,
	"dispatch/percentage":          TestPercentage,
	"dispatch/path_and_percentage": TestPathAndPercentageSplit,
	"dispatch/rule":                TestRule,
	"hosts/multiple":               TestMultipleHosts,
	"timeout":                      TestTimeout,
	"websocket":                    TestWebsocket,
	"websocket/split":              TestWebsocketSplit,
	"grpc":                         TestGRPC,
	"grpc/split":                   TestGRPCSplit,
	/*
		"headers/probe":                TestProbeHeaders,
		"retry":                        TestRetry,
		"tls":                          TestIngressTLS,
		"update":                       TestUpdate,
		"visibility":                   TestVisibility,
		"visibility/split":             TestVisibilitySplit,
		"visibility/path":              TestVisibilityPath,
		"ingressclass":                 TestIngressClass,
	*/
}

var contourBetaTests = map[string]func(t *testing.T){}

var istioBetaTests = map[string]func(t *testing.T){
	"headers/tags": TestTagHeaders,
	"host-rewrite": TestRewriteHost,
}

var betaTests = map[string]func(t *testing.T){
	// Add your conformance test for beta features
	"headers/tags": TestTagHeaders,
	"host-rewrite": TestRewriteHost,
}

var alphaTests = map[string]func(t *testing.T){
	// Add your conformance test for alpha features
}

// RunConformance will run ingress conformance tests
//
// Depending on the options it may test alpha and beta features
func RunConformance(t *testing.T) {

	var stables map[string]func(t *testing.T)
	var betas map[string]func(t *testing.T)
	switch test.NetworkingFlags.IngressClass {
	case "istio":
		stables = istioStableTests
		betas = istioBetaTests
	case "contour":
		stables = contourStableTests
		betas = contourBetaTests
	default:
		stables = stableTests
		betas = betaTests
	}

	for name, test := range stables {
		t.Run(name, test)
	}

	skipTests := skipTests()

	// TODO(dprotaso) we'll need something more robust
	// in the long term that lets downstream
	// implementations to better select which tests
	// should be run -  selection across various
	// dimensions
	// ie. state - alpha, beta, ga
	// ie. requirement - must, should, may
	if test.NetworkingFlags.EnableBetaFeatures {
		for name, test := range betas {
			if _, ok := skipTests[name]; ok {
				t.Run(name, skipFunc)
				continue
			}
			t.Run(name, test)
		}
	}

	if test.NetworkingFlags.EnableAlphaFeatures {
		for name, test := range alphaTests {
			if _, ok := skipTests[name]; ok {
				t.Run(name, skipFunc)
				continue
			}
			t.Run(name, test)
		}
	}
}

var skipFunc = func(t *testing.T) {
	t.Skip("Skipping the test in skip-test flag")
}

func skipTests() map[string]struct{} {
	skipArray := strings.Split(test.NetworkingFlags.SkipTests, ",")
	skipMap := make(map[string]struct{}, len(skipArray))
	for _, name := range skipArray {
		skipMap[name] = struct{}{}
	}
	return skipMap
}
