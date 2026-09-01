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

func TestIsProbeMatchRequiresOverrideValue(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		headerName gatewayapi.HTTPHeaderName
		headerVal  string
		want       bool
	}{
		{
			name:       "network configuration probe on application path",
			path:       "/",
			headerName: header.HashKey,
			headerVal:  header.HashValueOverride,
			want:       true,
		}, {
			name:       "endpoint probe on reserved path",
			path:       "/.well-known/knative/revision/ns/backend",
			headerName: header.HashKey,
			headerVal:  header.HashValueOverride,
			want:       true,
		}, {
			name:       "application route using the hash header with another value",
			path:       "/",
			headerName: header.HashKey,
			headerVal:  "application-value",
			want:       false,
		}, {
			name:       "unrelated header using the override value",
			path:       "/",
			headerName: "X-Probe",
			headerVal:  header.HashValueOverride,
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := gatewayapi.HTTPRouteMatch{
				Path: &gatewayapi.HTTPPathMatch{
					Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
					Value: ptr.To(test.path),
				},
				Headers: []gatewayapi.HTTPHeaderMatch{{
					Type:  ptr.To(gatewayapi.HeaderMatchExact),
					Name:  test.headerName,
					Value: test.headerVal,
				}},
			}

			if got := isProbeMatch(match); got != test.want {
				t.Errorf("isProbeMatch() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestComputeProbeTargetsKeepsCompleteRouteRules(t *testing.T) {
	previous := HTTPRoute{
		Name:      "example.com",
		Namespace: "ns",
		Hostname:  "example.com",
		Rules: []RuleBuilder{NormalRule{
			Namespace:   "ns",
			Name:        "goo",
			Port:        123,
			Weight:      100,
			RewriteHost: "goo.ns.svc.cluster.local",
		}},
	}.Build()
	desired := HTTPRoute{
		Name:      "example.com",
		Namespace: "ns",
		Hostname:  "example.com",
		Rules: []RuleBuilder{NormalRule{
			Namespace: "ns",
			Name:      "second-revision",
			Port:      123,
			Weight:    100,
		}},
	}.Build()

	newBackends, oldBackends := computeProbeTargets(previous, desired)
	if len(newBackends) != 1 {
		t.Fatalf("got %d new backends, want 1", len(newBackends))
	}
	if got, want := string(newBackends[0].backendRef().Name), "second-revision"; got != want {
		t.Errorf("new backend service = %q, want %q", got, want)
	}
	if diff := cmp.Diff(desired.Spec.Rules[0], newBackends[0].sourceRule); diff != "" {
		t.Errorf("new backend route rule was not preserved (-want, +got):\n%s", diff)
	}

	if len(oldBackends) != 1 {
		t.Fatalf("got %d old backends, want 1", len(oldBackends))
	}
	if got, want := string(oldBackends[0].backendRef().Name), "goo"; got != want {
		t.Errorf("old backend service = %q, want %q", got, want)
	}
	if diff := cmp.Diff(previous.Spec.Rules[0], oldBackends[0].sourceRule); diff != "" {
		t.Errorf("old backend route rule was not preserved (-want, +got):\n%s", diff)
	}
}

func TestComputeProbeTargetsKeepsNormalHashMatch(t *testing.T) {
	previousRule := NormalRule{
		Namespace: "ns",
		Name:      "old-backend",
		Port:      80,
		Weight:    100,
	}.Build()
	previousProbe := *previousRule.DeepCopy()
	previousProbe.Matches[0].Headers = []gatewayapi.HTTPHeaderMatch{{
		Type:  ptr.To(gatewayapi.HeaderMatchExact),
		Name:  header.HashKey,
		Value: header.HashValueOverride,
	}}

	desiredRule := NormalRule{
		Namespace: "ns",
		Name:      "new-backend",
		Port:      80,
		Weight:    100,
	}.Build()
	desiredRule.Matches[0].Headers = []gatewayapi.HTTPHeaderMatch{{
		Type:  ptr.To(gatewayapi.HeaderMatchExact),
		Name:  header.HashKey,
		Value: "application-value",
	}}
	desiredProbe := *desiredRule.DeepCopy()
	desiredProbe.Matches[0].Headers[0].Value = header.HashValueOverride

	previous := &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: gatewayapi.HTTPRouteSpec{
			Rules: []gatewayapi.HTTPRouteRule{previousProbe, previousRule},
		},
	}
	desired := &gatewayapi.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: gatewayapi.HTTPRouteSpec{
			Rules: []gatewayapi.HTTPRouteRule{desiredProbe, desiredRule},
		},
	}

	newBackends, oldBackends := computeProbeTargets(previous, desired)
	if got, want := len(newBackends), 1; got != want {
		t.Fatalf("new backend count = %d, want %d", got, want)
	}
	if got, want := string(newBackends[0].backendRef().Name), "new-backend"; got != want {
		t.Errorf("new backend = %q, want %q", got, want)
	}
	if got, want := len(oldBackends), 1; got != want {
		t.Fatalf("old backend count = %d, want %d", got, want)
	}
	if got, want := string(oldBackends[0].backendRef().Name), "old-backend"; got != want {
		t.Errorf("old backend = %q, want %q", got, want)
	}
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

			current := &gatewayapi.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: gatewayapi.HTTPRouteSpec{Rules: []gatewayapi.HTTPRouteRule{{
					BackendRefs: []gatewayapi.HTTPBackendRef{currentBackend},
				}}},
			}
			desired := &gatewayapi.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: gatewayapi.HTTPRouteSpec{Rules: []gatewayapi.HTTPRouteRule{{
					BackendRefs: []gatewayapi.HTTPBackendRef{desiredBackend},
				}}},
			}

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
