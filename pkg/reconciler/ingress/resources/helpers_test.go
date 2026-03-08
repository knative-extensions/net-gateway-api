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
	"testing"
	"text/template"
)

func TestTagOfHost(t *testing.T) {
	defaultTmpl := "{{.Tag}}-{{.Name}}"

	for _, tc := range []struct {
		name        string
		hosts       []string
		ingressName string
		tmpl        string
		want        string
	}{
		// --- Existing tests (default template) ---
		{
			name:        "no tag",
			hosts:       []string{"helloworld-go.default.example.com"},
			ingressName: "helloworld-go",
			want:        "",
		}, {
			name:        "tagged host",
			hosts:       []string{"auth-helloworld-go.default.example.com"},
			ingressName: "helloworld-go",
			want:        "auth",
		}, {
			name:        "tag with hyphens",
			hosts:       []string{"my-tag-helloworld-go.default.example.com"},
			ingressName: "helloworld-go",
			want:        "my-tag",
		}, {
			name:        "empty hosts",
			hosts:       []string{},
			ingressName: "helloworld-go",
			want:        "",
		}, {
			name:        "ingress name with hyphens",
			hosts:       []string{"v2-my-app.default.example.com"},
			ingressName: "my-app",
			want:        "v2",
		}, {
			name:        "host without dot",
			hosts:       []string{"auth-myapp"},
			ingressName: "myapp",
			want:        "auth",
		}, {
			name:        "no suffix match",
			hosts:       []string{"other.default.example.com"},
			ingressName: "helloworld-go",
			want:        "",
		}, {
			name:        "nil hosts",
			hosts:       nil,
			ingressName: "helloworld-go",
			want:        "",
		}, {
			name:        "partial ingress name match",
			hosts:       []string{"test-ingressextra.default.example.com"},
			ingressName: "ingress",
			want:        "",
		},

		// --- Custom template: reversed order {{.Name}}-{{.Tag}} ---
		{
			name:        "reversed template: tag extracted",
			hosts:       []string{"myapp-auth.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Name}}-{{.Tag}}",
			want:        "auth",
		}, {
			name:        "reversed template: no tag",
			hosts:       []string{"myapp.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Name}}-{{.Tag}}",
			want:        "",
		},

		// --- Custom template: double-dash separator {{.Tag}}--{{.Name}} ---
		{
			name:        "double-dash separator: tag extracted",
			hosts:       []string{"auth--helloworld-go.default.example.com"},
			ingressName: "helloworld-go",
			tmpl:        "{{.Tag}}--{{.Name}}",
			want:        "auth",
		},

		// --- DomainMapping-like hostnames (default template) ---
		{
			name:        "domain mapping: no false tag for custom.example.com",
			hosts:       []string{"custom.example.com"},
			ingressName: "custom.example.com",
			want:        "",
		}, {
			name:        "domain mapping: no false tag for foo-bar.example.com",
			hosts:       []string{"foo-bar.example.com"},
			ingressName: "foo-bar.example.com",
			want:        "",
		},

		// --- Forward verification prevents ambiguous matches (default template) ---
		{
			name:        "ambiguous match: ingressName=c resolves tag=a-b",
			hosts:       []string{"a-b-c.default.example.com"},
			ingressName: "c",
			want:        "a-b",
		}, {
			name:        "ambiguous match: ingressName=b-c resolves tag=a",
			hosts:       []string{"a-b-c.default.example.com"},
			ingressName: "b-c",
			want:        "a",
		},

		// --- Template with no separator (edge case): {{.Tag}}{{.Name}} ---
		{
			name:        "no separator template: tag extracted",
			hosts:       []string{"authmyapp.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Tag}}{{.Name}}",
			want:        "auth",
		},

		// --- Complex template with conditional ---
		{
			name:        "conditional template: tag extracted",
			hosts:       []string{"auth-myapp.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{if .Tag}}{{.Tag}}-{{end}}{{.Name}}",
			want:        "auth",
		}, {
			name:        "conditional template: no tag",
			hosts:       []string{"myapp.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{if .Tag}}{{.Tag}}-{{end}}{{.Name}}",
			want:        "",
		},

		// --- Multi-tag and edge-case templates ---
		{
			name:        "multi-tag template: returns empty",
			hosts:       []string{"auth-myapp-auth.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Tag}}-{{.Name}}-{{.Tag}}",
			want:        "",
		}, {
			name:        "tag-only template: extracts tag",
			hosts:       []string{"auth.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Tag}}",
			want:        "auth",
		}, {
			name:        "multi-name template: extracts tag",
			hosts:       []string{"auth-myapp-myapp.default.example.com"},
			ingressName: "myapp",
			tmpl:        "{{.Tag}}-{{.Name}}-{{.Name}}",
			want:        "auth",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmplSrc := tc.tmpl
			if tmplSrc == "" {
				tmplSrc = defaultTmpl
			}
			tmpl := template.Must(template.New("tag-template").Parse(tmplSrc))
			got := TagOfHost(tc.hosts, tc.ingressName, tmpl)
			if got != tc.want {
				t.Errorf("TagOfHost(%v, %q, tmpl=%q) = %q, want %q", tc.hosts, tc.ingressName, tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestTagOfHostNilTemplate(t *testing.T) {
	got := TagOfHost([]string{"auth-myapp.default.example.com"}, "myapp", nil)
	if got != "" {
		t.Errorf("TagOfHost with nil template = %q, want empty string", got)
	}
}
