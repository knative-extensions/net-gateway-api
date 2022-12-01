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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestIsHTTPRouteReady(t *testing.T) {
	tests := []struct {
		name          string
		expect        bool
		gatewayStatus []gatewayapi.RouteParentStatus
	}{
		{
			name: "Zero gateway - it does not have status condition",
		}, {
			name:   "One gateway - it has Admitted condition true",
			expect: true,
			gatewayStatus: []gatewayapi.RouteParentStatus{{
				ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
				Conditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}},
		}, {
			name: "One gateway - it has Admitted condition false",
			gatewayStatus: []gatewayapi.RouteParentStatus{{
				ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
				Conditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionFalse,
				}},
			}},
		}, {
			name: "One gateway - it does not have Admitted condition",
			gatewayStatus: []gatewayapi.RouteParentStatus{{
				ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
			}},
		}, {
			name:   "Two gateways - both have Admitted condition true",
			expect: true,
			gatewayStatus: []gatewayapi.RouteParentStatus{
				{
					ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				}, {
					ParentRef: gatewayapi.ParentReference{Name: "bar", Namespace: namespacePtr("bar")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		}, {
			name: "Two gateways - one has Admitted condition false",
			gatewayStatus: []gatewayapi.RouteParentStatus{
				{
					ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				}, {
					ParentRef: gatewayapi.ParentReference{Name: "bar", Namespace: namespacePtr("bar")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		}, {
			name: "Two gateways - one does not have Admitted condition",
			gatewayStatus: []gatewayapi.RouteParentStatus{
				{
					ParentRef: gatewayapi.ParentReference{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				}, {
					ParentRef: gatewayapi.ParentReference{Name: "bar", Namespace: namespacePtr("bar")},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpRoute := &gatewayapi.HTTPRoute{
				Status: gatewayapi.HTTPRouteStatus{
					RouteStatus: gatewayapi.RouteStatus{Parents: test.gatewayStatus},
				},
			}
			got, _ := IsHTTPRouteReady(httpRoute)
			if got != test.expect {
				t.Errorf("Got = %v, want = %v", got, test.expect)
			}
		})
	}
}
