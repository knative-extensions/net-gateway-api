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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Grant the resource "to" access to the resource "from"
func MakeReferenceGrant(_ context.Context, ing *netv1alpha1.Ingress, to, from metav1.PartialObjectMetadata) *gatewayv1beta1.ReferenceGrant {
	name := fmt.Sprintf("%s-%s", ing.Name, to.Name)
	if len(name)+len(from.Namespace) > 62 {
		name = name[:62-len(from.Namespace)]
	}
	name += "-" + from.Namespace

	return &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       to.Namespace,
			Labels:          to.Labels,
			Annotations:     to.Annotations,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing)},
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group(from.GroupVersionKind().Group),
				Kind:      gatewayv1beta1.Kind(from.Kind),
				Namespace: gatewayv1beta1.Namespace(from.Namespace),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(to.GroupVersionKind().Group),
				Kind:  gatewayv1beta1.Kind(to.Kind),
				Name:  (*gatewayv1beta1.ObjectName)(&to.Name),
			}},
		},
	}
}
