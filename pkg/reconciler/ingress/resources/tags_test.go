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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

func ingressWithTagAnnotation(annotation string) *v1alpha1.Ingress {
	ing := &v1alpha1.Ingress{ObjectMeta: metav1.ObjectMeta{}}
	if annotation != "" {
		ing.ObjectMeta.Annotations = map[string]string{
			networking.TagToHostAnnotationKey: annotation,
		}
	}
	return ing
}

func TestTagForHost(t *testing.T) {
	cases := []struct {
		name       string
		annotation string
		hosts      []string
		want       string
	}{
		{
			name:  "no annotation",
			hosts: []string{"blue.example.com"},
			want:  "",
		},
		{
			name:       "malformed annotation",
			annotation: `{"blue":`,
			hosts:      []string{"blue.example.com"},
			want:       "",
		},
		{
			name:       "matching host returns its tag",
			annotation: `{"blue":["blue.example.com"]}`,
			hosts:      []string{"blue.example.com"},
			want:       "blue",
		},
		{
			name:       "host not present in any tag",
			annotation: `{"blue":["blue.example.com"]}`,
			hosts:      []string{"main.example.com"},
			want:       "",
		},
		{
			name:       "matches the right tag among several",
			annotation: `{"blue":["blue.example.com"],"green":["green.example.com"]}`,
			hosts:      []string{"green.example.com"},
			want:       "green",
		},
		{
			name:       "empty tag map",
			annotation: `{}`,
			hosts:      []string{"blue.example.com"},
			want:       "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ing := ingressWithTagAnnotation(tc.annotation)
			rule := &v1alpha1.IngressRule{Hosts: tc.hosts}

			if got := tagForHost(ing, rule); got != tc.want {
				t.Errorf("tagForHost() = %q, want %q", got, tc.want)
			}
		})
	}
}
