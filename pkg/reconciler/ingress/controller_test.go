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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	networkcfg "knative.dev/networking/pkg/config"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/system"

	_ "knative.dev/net-gateway-api/pkg/client/injection/informers/apis/v1beta1/gateway/fake"
	_ "knative.dev/net-gateway-api/pkg/client/injection/informers/apis/v1beta1/httproute/fake"
	_ "knative.dev/net-gateway-api/pkg/client/injection/informers/apis/v1beta1/referencegrant/fake"
	_ "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints/fake"

	. "knative.dev/pkg/reconciler/testing"
)

func TestNew(t *testing.T) {
	ctx, _ := SetupFakeContext(t)

	c := NewController(ctx, configmap.NewStaticWatcher(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      config.GatewayConfigName,
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      networkcfg.ConfigMapName,
		},
	}))

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
