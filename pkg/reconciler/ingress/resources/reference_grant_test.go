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
	"context"
	"knative.dev/pkg/ptr"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestMakeReferenceGrant(t *testing.T) {
	ing := &netv1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-namespace",
		},
	}

	to := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "to-resource",
			Namespace: "to-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
	}

	from := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "from-namespace",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
	}

	want := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-ingress-to-resource-from-namespace",
			Namespace:   "to-namespace",
			Labels:      to.Labels,
			Annotations: to.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "networking.internal.knative.dev/v1alpha1",
					Kind:               "Ingress",
					Name:               "test-ingress",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				},
			},
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group("networking.k8s.io"),
				Kind:      gatewayv1beta1.Kind("Ingress"),
				Namespace: gatewayv1beta1.Namespace("from-namespace"),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("Service"),
				Name:  (*gatewayv1beta1.ObjectName)(&to.Name),
			}},
		},
	}

	got := MakeReferenceGrant(context.TODO(), ing, to, from)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("MakeReferenceGrant (-want, +got):\n%s", diff)
	}
}
