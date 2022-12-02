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

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func ptr[T any](val T) *T {
	return &val
}

func portNumPtr(port int) *gatewayapi.PortNumber {
	pn := gatewayapi.PortNumber(port)
	return &pn
}

// LongestHost returns the most specific host.
// The length is:
// 1. the length of the hostnames.
// 2. the first alphabetical order.
//
// For example, "hello-example.default.svc.cluster.local" will be
// returned from the following hosts in KIngress.
//
// - hosts:
//   - hello.default
//   - hello.default.svc
//   - hello.default.svc.cluster.local
func LongestHost(hosts []string) string {
	sort.Strings(hosts)
	return hosts[len(hosts)-1]
}
