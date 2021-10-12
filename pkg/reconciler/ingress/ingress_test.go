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
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	network "knative.dev/networking/pkg"

	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	fakeingressclient "knative.dev/networking/pkg/client/injection/client/fake"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	. "knative.dev/net-gateway-api/pkg/reconciler/testing"
	. "knative.dev/pkg/reconciler/testing"

	fakegwapiclientset "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/client/fake"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

// TODO: Add more tests.
func TestReconcile(t *testing.T) {
	theError := errors.New("this is the error")

	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			statusManager: &fakeStatusManager{
				FakeIsReady: func(context.Context, *v1alpha1.Ingress) (bool, error) {
					return false, theError
				},
			},
		}

		ingr := ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakeingressclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, gatewayAPIIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: defaultConfig,
				}})

		return ingr
	}))
}

type fakeStatusManager struct {
	FakeIsReady func(context.Context, *v1alpha1.Ingress) (bool, error)
}

func (m *fakeStatusManager) IsReady(ctx context.Context, ing *v1alpha1.Ingress) (bool, error) {
	return m.FakeIsReady(ctx, ing)
}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var (
	defaultConfig = &config.Config{
		Network: &network.Config{},
		Gateway: &config.Gateway{
			Gateways: map[v1alpha1.IngressVisibility]*config.GatewayConfig{
				v1alpha1.IngressVisibilityExternalIP: {},
				v1alpha1.IngressVisibilityClusterLocal: {
					Service: &types.NamespacedName{"istio-system", "knative-local-gateway"},
				}},
		},
	}
)
