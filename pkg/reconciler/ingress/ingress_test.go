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
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	fakeingressclient "knative.dev/networking/pkg/client/injection/client/fake"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	networkcfg "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/ingress"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/network"

	. "knative.dev/net-gateway-api/pkg/reconciler/testing"
	. "knative.dev/pkg/reconciler/testing"

	fakegwapiclientset "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/client/fake"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
)

var (
	publicSvcIP  = "1.2.3.4"
	privateSvcIP = "5.6.7.8"
	publicSvc    = network.GetServiceHostname(publicName, testNamespace)
	privateSvc   = network.GetServiceHostname(privateName, testNamespace)
)

var (
	services = []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "goo",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name: "http",
				}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "doo",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name: "http2",
				}},
			},
		},
		// Contour Control Plane Services
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      publicName,
				Namespace: testNamespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: publicSvcIP,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      privateName,
				Namespace: testNamespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: privateSvcIP,
			},
		},
	}
	endpoints = []runtime.Object{
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "goo",
			},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{
					IP: "10.0.0.1",
				}},
			}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "doo",
			},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{
					IP: "192.168.1.1",
				}},
			}},
		},
	}
	servicesAndEndpoints = append(append([]runtime.Object{}, services...), endpoints...)
)

// TODO: Add more tests - e.g. invalid ingress, delete ingress, etc.
func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "skip ingress not matching class key",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing(withBasicSpec, withAnnotation(map[string]string{
				networking.IngressClassAnnotationKey: "fake-controller",
			})),
		},
	}, {
		Name: "skip ingress marked for deletion",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, func(i *v1alpha1.Ingress) {
				i.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
			}),
		},
	}, {
		Name: "first reconcile basic ingress",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass),
		}, servicesAndEndpoints...),
		WantCreates: []runtime.Object{httpRoute(t, ing(withBasicSpec, withGatewayAPIclass))},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIclass, func(i *v1alpha1.Ingress) {
				// These are the things we expect to change in status.
				i.Status.InitializeConditions()
				i.Status.MarkLoadBalancerReady(
					[]v1alpha1.LoadBalancerIngressStatus{{
						DomainInternal: publicSvc,
					}},
					[]v1alpha1.LoadBalancerIngressStatus{{
						DomainInternal: privateSvc,
					}})
			}),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}, {
		Name: "reconcile ready ingress",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, makeItReady),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass)),
		}, servicesAndEndpoints...),
		// no extra update
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			statusManager: &fakeStatusManager{
				FakeIsReady: func(context.Context, *v1alpha1.Ingress) (bool, error) {
					return true, nil
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

func TestReconcileProberNotReady(t *testing.T) {
	table := TableTest{{
		Name: "first reconcile basic ingress wth prober",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass),
		}, servicesAndEndpoints...),
		WantCreates: []runtime.Object{httpRoute(t, ing(withBasicSpec, withGatewayAPIclass))},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIclass, func(i *v1alpha1.Ingress) {
				i.Status.InitializeConditions()
				i.Status.MarkLoadBalancerNotReady()
			}),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			statusManager: &fakeStatusManager{
				FakeIsReady: func(context.Context, *v1alpha1.Ingress) (bool, error) {
					return false, nil
				},
			},
		}
		return ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakeingressclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, gatewayAPIIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: defaultConfig,
				}})
	}))
}

func TestReconcileProbeError(t *testing.T) {
	theError := errors.New("this is the error")

	table := TableTest{{
		Name:    "first reconcile basic ingress",
		Key:     "ns/name",
		WantErr: true,
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass),
		}, servicesAndEndpoints...),
		WantCreates: []runtime.Object{httpRoute(t, ing(withBasicSpec, withGatewayAPIclass))},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIclass, func(i *v1alpha1.Ingress) {
				i.Status.InitializeConditions()
				i.Status.MarkIngressNotReady(notReconciledReason, notReconciledMessage)
			}),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
			Eventf(corev1.EventTypeWarning, "InternalError", fmt.Sprintf("failed to probe Ingress: %v", theError)),
		},
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
		return ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakeingressclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, gatewayAPIIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: defaultConfig,
				}})
	}))
}

func makeItReady(i *v1alpha1.Ingress) {
	i.Status.InitializeConditions()
	i.Status.MarkNetworkConfigured()
	i.Status.MarkLoadBalancerReady(
		[]v1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: publicSvc,
		}},
		[]v1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: privateSvc,
		}})
}

func httpRoute(t *testing.T, i *v1alpha1.Ingress, opts ...HTTPRouteOption) runtime.Object {
	t.Helper()
	ingress.InsertProbe(i)
	ctx := (&testConfigStore{config: defaultConfig}).ToContext(context.Background())
	httpRoute, _ := resources.MakeHTTPRoute(ctx, i, &i.Spec.Rules[0])
	for _, opt := range opts {
		opt(httpRoute)
	}
	return httpRoute
}

type HTTPRouteOption func(h *gatewayv1alpha2.HTTPRoute)

func withGatewayAPIclass(i *v1alpha1.Ingress) {
	withAnnotation(map[string]string{
		networking.IngressClassAnnotationKey: gatewayAPIIngressClassName,
	})(i)
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
		Network: &networkcfg.Config{},
		Gateway: &config.Gateway{
			Gateways: map[v1alpha1.IngressVisibility]config.GatewayConfig{
				v1alpha1.IngressVisibilityExternalIP: {
					Service: &types.NamespacedName{Namespace: "istio-system", Name: "istio-gateway"},
					Gateway: &types.NamespacedName{Namespace: "istio-system", Name: "istio-gateway"},
				},
				v1alpha1.IngressVisibilityClusterLocal: {
					Service: &types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"},
					Gateway: &types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"},
				},
			},
		},
	}
)
