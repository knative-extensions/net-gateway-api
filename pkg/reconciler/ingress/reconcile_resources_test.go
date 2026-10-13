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

package ingress

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	"knative.dev/networking/pkg/http/header"
)

func routeWithRules(rules ...gatewayapi.HTTPRouteRule) *gatewayapi.HTTPRoute {
	return &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec:       gatewayapi.HTTPRouteSpec{Rules: rules},
	}
}

func TestComputeProbeTargetsUsesDesiredRuleForRetainedBackends(t *testing.T) {
	type probe struct {
		Backend string
		Rule    gatewayapi.HTTPRouteRule
	}

	const (
		previousHost = "previous.example"
		currentHost  = "current.example"
	)

	backend := func(name, host string, weight int) NormalRule {
		return NormalRule{Namespace: "ns", Name: name, Port: 80, Weight: weight, RewriteHost: host}
	}
	onPath := func(rule NormalRule, path string) NormalRule {
		rule.Path = path
		return rule
	}

	retainedBefore := backend("retained", previousHost, 100)
	retainedAfter := backend("retained", currentHost, 50)
	retainedWithHeaders := retainedAfter
	retainedWithHeaders.Headers = []string{"X-Extra", "value"}
	fresh := backend("fresh", currentHost, 50)

	onlyRetained := SplitRule{retainedBefore}.Build()
	retainedAndFresh := SplitRule{retainedAfter, fresh}.Build()
	headersChanged := SplitRule{retainedWithHeaders, fresh}.Build()
	removedAndRetained := SplitRule{backend("removed", previousHost, 50), retainedBefore}.Build()
	retainedAndTwoFresh := SplitRule{
		retainedAfter,
		backend("fresh-b", currentHost, 25),
		backend("fresh-a", currentHost, 25),
	}.Build()
	retainedOnTwoPaths := []gatewayapi.HTTPRouteRule{
		SplitRule{onPath(retainedBefore, "/a")}.Build(),
		SplitRule{onPath(retainedBefore, "/b")}.Build(),
	}
	freshOnTwoPaths := []gatewayapi.HTTPRouteRule{
		SplitRule{onPath(fresh, "/a")}.Build(),
		SplitRule{onPath(fresh, "/b")}.Build(),
	}

	tests := []struct {
		name    string
		current []gatewayapi.HTTPRouteRule
		desired []gatewayapi.HTTPRouteRule
		wantNew []probe
		wantOld []probe
	}{{
		name:    "retained backend with a changed host rewrite",
		current: []gatewayapi.HTTPRouteRule{onlyRetained},
		desired: []gatewayapi.HTTPRouteRule{retainedAndFresh},
		wantNew: []probe{{"fresh", retainedAndFresh}},
		wantOld: []probe{{"retained", retainedAndFresh}},
	}, {
		name:    "retained backend with changed append headers",
		current: []gatewayapi.HTTPRouteRule{onlyRetained},
		desired: []gatewayapi.HTTPRouteRule{headersChanged},
		wantNew: []probe{{"fresh", headersChanged}},
		wantOld: []probe{{"retained", headersChanged}},
	}, {
		name:    "removed backend keeps its current rule and new backends are sorted",
		current: []gatewayapi.HTTPRouteRule{removedAndRetained},
		desired: []gatewayapi.HTTPRouteRule{retainedAndTwoFresh},
		wantNew: []probe{{"fresh-a", retainedAndTwoFresh}, {"fresh-b", retainedAndTwoFresh}},
		wantOld: []probe{{"removed", removedAndRetained}, {"retained", retainedAndTwoFresh}},
	}, {
		name:    "retained backend in several current rules is probed once",
		current: retainedOnTwoPaths,
		desired: []gatewayapi.HTTPRouteRule{retainedAndFresh},
		wantNew: []probe{{"fresh", retainedAndFresh}},
		wantOld: []probe{{"retained", retainedAndFresh}},
	}, {
		name:    "new backend in several desired rules is probed once",
		current: []gatewayapi.HTTPRouteRule{onlyRetained},
		desired: append([]gatewayapi.HTTPRouteRule{retainedAndFresh}, freshOnTwoPaths...),
		wantNew: []probe{{"fresh", retainedAndFresh}},
		wantOld: []probe{{"retained", retainedAndFresh}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := routeWithRules(test.current...)
			desired := routeWithRules(test.desired...)

			summarize := func(targets []backendToProbe) []probe {
				got := make([]probe, 0, len(targets))
				for _, target := range targets {
					got = append(got, probe{string(target.identity.name), target.sourceRule})
				}
				return got
			}

			newBackends, oldBackends := computeProbeTargets(current, desired)
			if diff := cmp.Diff(test.wantNew, summarize(newBackends)); diff != "" {
				t.Errorf("new backends (-want, +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.wantOld, summarize(oldBackends)); diff != "" {
				t.Errorf("old backends (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestComputeProbeTargetsKeepsNormalHashMatch(t *testing.T) {
	previousBackend := NormalRule{Namespace: "ns", Name: "old-backend", Port: 80, Weight: 100}
	desiredBackend := NormalRule{Namespace: "ns", Name: "new-backend", Port: 80, Weight: 100}

	desiredRule := desiredBackend.Build()
	desiredRule.Matches[0].Headers = []gatewayapi.HTTPHeaderMatch{{
		Type:  ptr.To(gatewayapi.HeaderMatchExact),
		Name:  header.HashKey,
		Value: "application-value",
	}}

	previous := routeWithRules(ProbeRule{Rule: previousBackend}.Build(), previousBackend.Build())
	desired := routeWithRules(ProbeRule{Rule: desiredBackend}.Build(), desiredRule)

	newBackends, oldBackends := computeProbeTargets(previous, desired)
	if diff := cmp.Diff([]string{"new-backend"}, backendNames(newBackends)); diff != "" {
		t.Errorf("new backends (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"old-backend"}, backendNames(oldBackends)); diff != "" {
		t.Errorf("old backends (-want, +got):\n%s", diff)
	}
}

func backendNames(targets []backendToProbe) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, string(target.identity.name))
	}
	return names
}

func TestComputeProbeTargetsUsesCompleteBackendIdentity(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*gatewayapi.HTTPBackendRef)
		wantProbe bool
	}{
		{
			name:   "unchanged backend",
			change: func(*gatewayapi.HTTPBackendRef) {},
		}, {
			name: "explicit default values",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Group = ptr.To(gatewayapi.Group(""))
				backend.Kind = ptr.To(gatewayapi.Kind("Service"))
				backend.Namespace = ptr.To(gatewayapi.Namespace("ns"))
			},
		}, {
			name: "port changed",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Port = ptr.To(gatewayapi.PortNumber(8080))
			},
			wantProbe: true,
		}, {
			name: "group changed",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Group = ptr.To(gatewayapi.Group("example.dev"))
			},
			wantProbe: true,
		}, {
			name: "kind changed",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Kind = ptr.To(gatewayapi.Kind("Backend"))
			},
			wantProbe: true,
		}, {
			name: "namespace changed",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Namespace = ptr.To(gatewayapi.Namespace("other-ns"))
			},
			wantProbe: true,
		}, {
			name: "name changed",
			change: func(backend *gatewayapi.HTTPBackendRef) {
				backend.Name = gatewayapi.ObjectName("other-backend")
			},
			wantProbe: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			currentBackend := gatewayapi.HTTPBackendRef{
				BackendRef: gatewayapi.BackendRef{
					BackendObjectReference: gatewayapi.BackendObjectReference{
						Name: "backend",
						Port: ptr.To(gatewayapi.PortNumber(80)),
					},
				},
			}
			desiredBackend := *currentBackend.DeepCopy()
			test.change(&desiredBackend)

			current := routeWithRules(gatewayapi.HTTPRouteRule{
				BackendRefs: []gatewayapi.HTTPBackendRef{currentBackend},
			})
			desired := routeWithRules(gatewayapi.HTTPRouteRule{
				BackendRefs: []gatewayapi.HTTPBackendRef{desiredBackend},
			})

			newBackends, oldBackends := computeProbeTargets(current, desired)
			if got, want := len(oldBackends), 1; got != want {
				t.Fatalf("old backend count = %d, want %d", got, want)
			}
			wantNewBackendCount := 0
			if test.wantProbe {
				wantNewBackendCount = 1
			}
			if got := len(newBackends); got != wantNewBackendCount {
				t.Fatalf("new backend count = %d, want %d", got, wantNewBackendCount)
			}
		})
	}
}
