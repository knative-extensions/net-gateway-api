/*
Copyright 2026 The Knative Authors

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
	"encoding/json"
	"slices"

	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

// tagForHost returns the tag name for the given rule by looking up its hosts
// in the TagToHostAnnotationKey annotation on the Ingress.
// The annotation is a JSON map of tag names to host lists, e.g.:
//
//	{"blue":["blue-myservice.example.com"],"green":["green-myservice.example.com"]}
//
// Each rule's hosts are assumed to belong to at most one tag. This invariant
// is guaranteed by how knative/serving builds IngressRules (one rule per
// traffic target). Returns an empty string if no matching tag is found.
func tagForHost(ing *v1alpha1.Ingress, rule *v1alpha1.IngressRule) string {
	serialized := ing.GetAnnotations()[networking.TagToHostAnnotationKey]
	if serialized == "" {
		return ""
	}

	parsed := make(map[string][]string)
	if err := json.Unmarshal([]byte(serialized), &parsed); err != nil {
		return ""
	}

	for tag, hosts := range parsed {
		for _, host := range hosts {
			if slices.Contains(rule.Hosts, host) {
				return tag
			}
		}
	}
	return ""
}
