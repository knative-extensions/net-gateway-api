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
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestIsHTTPRouteReady(t *testing.T) {
	tests := []struct {
		name          string
		expect        bool
		gatewayStatus []gatewayv1alpha2.RouteParentStatus
	}{
		{
			name: "Zero gateway - it does not have status condition",
		}, {
			name:   "One gateway - it has Admitted condition true",
			expect: true,
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{{
				ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
				Conditions: []metav1.Condition{{
					Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
					Status: metav1.ConditionTrue,
				}},
			}},
		}, {
			name: "One gateway - it has Admitted condition false",
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{{
				ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
				Conditions: []metav1.Condition{{
					Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
					Status: metav1.ConditionFalse,
				}},
			}},
		}, {
			name: "One gateway - it does not have Admitted condition",
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{{
				ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
			}},
		}, {
			name:   "Two gateways - both have Admitted condition true",
			expect: true,
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{
				{
					ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				}, {
					ParentRef: gatewayv1alpha2.ParentRef{Name: "bar", Namespace: namespacePtr("bar")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		}, {
			name: "Two gateways - one has Admitted condition false",
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{
				{
					ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				}, {
					ParentRef: gatewayv1alpha2.ParentRef{Name: "bar", Namespace: namespacePtr("bar")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		}, {
			name: "Two gateways - one does not have Admitted condition",
			gatewayStatus: []gatewayv1alpha2.RouteParentStatus{
				{
					ParentRef: gatewayv1alpha2.ParentRef{Name: "foo", Namespace: namespacePtr("foo")},
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.ConditionRouteAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				}, {
					ParentRef: gatewayv1alpha2.ParentRef{Name: "bar", Namespace: namespacePtr("bar")},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpRoute := &gatewayv1alpha2.HTTPRoute{
				Status: gatewayv1alpha2.HTTPRouteStatus{
					RouteStatus: gatewayv1alpha2.RouteStatus{Parents: test.gatewayStatus},
				},
			}
			got, _ := IsHTTPRouteReady(httpRoute)
			if got != test.expect {
				t.Errorf("Got = %v, want = %v", got, test.expect)
			}
		})
	}
}
