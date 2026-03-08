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
	"bytes"
	"cmp"
	"slices"
	"strings"
	"text/template"

	netcfg "knative.dev/networking/pkg/config"
)

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
func LongestHost[S ~[]E, E cmp.Ordered](hosts S) E {
	slices.Sort(hosts)
	return hosts[len(hosts)-1]
}

func executeTagTemplate(tmpl *template.Template, name, tag string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, netcfg.TagTemplateValues{Name: name, Tag: tag}); err != nil {
		return ""
	}
	return buf.String()
}

// TagOfHost extracts the traffic tag name from the first host in hosts
// by using the tag template. It executes the template with a sentinel
// value to discover where .Tag appears, then extracts and verifies
// the tag from the actual hostname.
// Returns empty string if no tag is found or hosts is empty.
//
// Note: templates that reference .Tag more than once are not supported
// and will always return "". In practice, tag templates use .Tag exactly once.
func TagOfHost(hosts []string, ingressName string, tmpl *template.Template) string {
	if len(hosts) == 0 || tmpl == nil {
		return ""
	}
	host := strings.SplitN(hosts[0], ".", 2)[0]

	// No tag if template with empty tag matches
	if host == executeTagTemplate(tmpl, ingressName, "") {
		return ""
	}

	// Execute with a sentinel to discover where .Tag appears in the output.
	const sentinel = "\x00TAG\x00"
	result := executeTagTemplate(tmpl, ingressName, sentinel)
	prefix, suffix, found := strings.Cut(result, sentinel)
	if !found {
		return "" // template doesn't use .Tag at all
	}

	// Check that the host has the expected prefix and suffix.
	if !strings.HasPrefix(host, prefix) || !strings.HasSuffix(host, suffix) {
		return ""
	}

	// Extract the tag by stripping prefix and suffix.
	tag := host[len(prefix):]
	if len(suffix) > 0 {
		if len(tag) < len(suffix) {
			return ""
		}
		tag = tag[:len(tag)-len(suffix)]
	}
	if tag == "" {
		return ""
	}

	// Forward-verify: re-execute the template with the extracted tag
	// and confirm it reproduces the host exactly.
	if executeTagTemplate(tmpl, ingressName, tag) != host {
		return ""
	}

	return tag
}
