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
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
)

// routeTagHeaderValue returns the value of the route tag header match.
func routeTagHeaderValue(headers map[string]v1alpha1.HeaderMatch) string {
	if len(headers) == 0 {
		return ""
	}
	match, ok := headers[header.RouteTagKey]
	if !ok {
		return ""
	}
	return match.Exact
}

// routeTagAppendValue returns the value of the route tag append header.
func routeTagAppendValue(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	return headers[header.RouteTagKey]
}

// hostRouteTags returns the set of tags already represented as host-based paths.
func hostRouteTags(rule *v1alpha1.IngressRule) sets.Set[string] {
	tags := sets.New[string]()
	if rule.HTTP == nil {
		return tags
	}
	for _, path := range rule.HTTP.Paths {
		tag := routeTagAppendValue(path.AppendHeaders)
		if tag == "" || routeTagHeaderValue(path.Headers) != "" {
			continue
		}
		tags.Insert(tag)
	}
	return tags
}
