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

package resources

import (
	"sort"

	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// Visibility converts netv1alpha1.IngressVisibility to string.
func Visibility(visibility netv1alpha1.IngressVisibility) string {
	switch visibility {
	case netv1alpha1.IngressVisibilityClusterLocal:
		return "cluster-local"
	case netv1alpha1.IngressVisibilityExternalIP:
		return ""
	}
	return ""
}

func gatewayAllowTypePtr(val gwv1alpha1.GatewayAllowType) *gwv1alpha1.GatewayAllowType {
	return &val
}

func headerMatchTypePtr(val gwv1alpha1.HeaderMatchType) *gwv1alpha1.HeaderMatchType {
	return &val
}

func portNumPtr(port int) *gwv1alpha1.PortNumber {
	pn := gwv1alpha1.PortNumber(port)
	return &pn
}

func pathMatchTypePtr(val gwv1alpha1.PathMatchType) *gwv1alpha1.PathMatchType {
	return &val
}

func stringPtr(s string) *string {
	return &s
}

// LongestHost returns the "longest" host.
// The length is:
// 1. the length of the hostnames.
// 2. the first alphabetical order.
func LongestHost(hosts []string) string {
	sort.Strings(hosts)
	return hosts[0]
}
