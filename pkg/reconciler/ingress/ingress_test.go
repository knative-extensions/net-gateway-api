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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"

	fakegwapiclientset "knative.dev/net-gateway-api/pkg/client/injection/client/fake"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
	"knative.dev/net-gateway-api/pkg/status"
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

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"

	. "knative.dev/net-gateway-api/pkg/reconciler/testing"
	. "knative.dev/pkg/reconciler/testing"
)

var (
	publicSvcIP  = "1.2.3.4"
	privateSvcIP = "5.6.7.8"
	publicSvc    = network.GetServiceHostname(publicName, testNamespace)
	privateSvc   = network.GetServiceHostname(privateName, testNamespace)

	fakeStatusKey struct{}
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
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: "ns",
			},
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}, {
		Name: "reconcile ready ingress",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, makeItReady, withFinalizer),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass)),
		}, servicesAndEndpoints...),
		// no extra update
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			gatewayLister:   listers.GetGatewayLister(),
			statusManager: &fakeStatusManager{
				FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
					return status.ProbeState{Ready: true}, nil
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

func TestReconcileTLS(t *testing.T) {
	// The gateway API annoyingly has a number of
	secretName := "name-WE-STICK-A-LONG-UID-HERE"
	nsName := "ns"
	deleteTime := time.Now().Add(-10 * time.Second)
	table := TableTest{{
		Name: "Happy TLS",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing(withBasicSpec, withGatewayAPIClass, withTLS()),
			secret(secretName, nsName),
			gw(defaultListener),
		},
		WantCreates: []runtime.Object{
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withTLS())),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: gw(defaultListener, tlsListener("example.com", nsName, secretName)),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: "ns",
			},
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIClass, withTLS(), func(i *v1alpha1.Ingress) {
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
			Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			Eventf(corev1.EventTypeNormal, "Created", `Created HTTPRoute "example.com"`),
		},
	}, {
		Name: "Already Configured",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing(withBasicSpec, withFinalizer, withGatewayAPIClass, withTLS()),
			secret(secretName, nsName),
			gw(defaultListener, tlsListener("example.com", nsName, secretName)),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withTLS())),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{
			// None
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withFinalizer, withGatewayAPIClass, withTLS(), func(i *v1alpha1.Ingress) {
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
			// None
		},
	}, {
		Name:                    "Cleanup Listener",
		Key:                     "ns/name",
		SkipNamespaceValidation: true,
		Objects: []runtime.Object{
			ing(withBasicSpec, withGatewayAPIClass, withTLS(), func(i *v1alpha1.Ingress) {
				i.DeletionTimestamp = &metav1.Time{
					Time: deleteTime,
				}
			}),
			secret(secretName, nsName),
			gw(defaultListener, tlsListener("secure.example.com", nsName, secretName)),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withTLS())),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: gw(defaultListener),
		}},
	}, {
		Name:    "No Gateway",
		Key:     "ns/name",
		WantErr: true,
		Objects: []runtime.Object{
			ing(withBasicSpec, withGatewayAPIClass, withTLS()),
			secret(secretName, nsName),
		},
		WantCreates: []runtime.Object{
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withTLS())),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{
			// None
		},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: "ns",
			},
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIClass, withTLS(), func(i *v1alpha1.Ingress) {
				i.Status.InitializeConditions()
				i.Status.MarkIngressNotReady("ReconcileIngressFailed", "Ingress reconciliation failed")
			}),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			Eventf(corev1.EventTypeNormal, "Created", `Created HTTPRoute "example.com"`),
			Eventf(corev1.EventTypeWarning, "GatewayMissing", `Unable to update Gateway istio-system/istio-gateway`),
			Eventf(corev1.EventTypeWarning, "InternalError", `Gateway istio-system/istio-gateway does not exist: gateway.gateway.networking.k8s.io "istio-gateway" not found`),
		},
	}}

	table.Test(t, GatewayFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher, tr *TableRow) controller.Reconciler {
		r := &Reconciler{
			gwapiclient:          fakegwapiclientset.Get(ctx),
			httprouteLister:      listers.GetHTTPRouteLister(),
			referenceGrantLister: listers.GetReferenceGrantLister(),
			gatewayLister:        listers.GetGatewayLister(),
			statusManager: &fakeStatusManager{
				FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
					return status.ProbeState{Ready: true}, nil
				}},
		}
		// The fake tracker's `Add` method incorrectly pluralizes "gatewaies" using UnsafeGuessKindToResource,
		// so create this via explicit call (per note in client-go/testing/fixture.go in tracker.Add)
		fakeCreates := []runtime.Object{}
		for _, x := range tr.Objects {
			myGw, ok := x.(*gatewayapi.Gateway)
			if ok {
				fakegwapiclientset.Get(ctx).GatewayV1beta1().Gateways(myGw.Namespace).Create(ctx, myGw, metav1.CreateOptions{})
				tr.SkipNamespaceValidation = true
				fakeCreates = append(fakeCreates, myGw)
			}
		}
		tr.WantCreates = append(fakeCreates, tr.WantCreates...)

		ingr := ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakeingressclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, gatewayAPIIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: defaultConfig,
				}})

		return ingr
	}))
}

func TestReconcileProbing(t *testing.T) {
	table := TableTest{{
		Name: "first reconciler probe returns false",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
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
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: "ns",
			},
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}, {
		Name: "first reconcile probe returns an error",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, errors.New("this is the error")
			},
		}),
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
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: "ns",
			},
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
			Eventf(corev1.EventTypeWarning, "InternalError", "failed to probe Ingress: this is the error"),
		},
	}, {
		Name: "prober callback all endpoints ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, withFinalizer, withInitialConditions),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
		}, servicesAndEndpoints...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{
			{Object: ing(withBasicSpec, withGatewayAPIclass, withFinalizer, makeItReady)},
		},
	}, {
		Name: "updated ingress - new backends used for endpoint probing",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withSecondRevisionSpec, withGatewayAPIclass, withFinalizer, makeItReady),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
		}, servicesAndEndpoints...),
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true, Version: "previous"}, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		statusManager := ctx.Value(fakeStatusKey).(status.Manager)
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			gatewayLister:   listers.GetGatewayLister(),
			statusManager:   statusManager,
		}
		return ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakeingressclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, gatewayAPIIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: defaultConfig,
				}})
	}))
}

func makeLoadBalancerNotReady(i *v1alpha1.Ingress) {
	i.Status.MarkLoadBalancerNotReady()
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

func httpRouteReady(h *gatewayapi.HTTPRoute) {
	h.Status.Parents = []gatewayapi.RouteParentStatus{{
		Conditions: []metav1.Condition{{
			Type:   string(gatewayapi.RouteConditionAccepted),
			Status: metav1.ConditionTrue,
		}},
	}}
}

type HTTPRouteOption func(h *gatewayapi.HTTPRoute)

func withGatewayAPIclass(i *v1alpha1.Ingress) {
	withAnnotation(map[string]string{
		networking.IngressClassAnnotationKey: gatewayAPIIngressClassName,
	})(i)
}

func withStatusManager(f *fakeStatusManager) context.Context {
	return context.WithValue(context.Background(), fakeStatusKey, f)
}

type fakeStatusManager struct {
	FakeDoProbes      func(context.Context, status.Backends) (status.ProbeState, error)
	FakeIsProbeActive func(types.NamespacedName) (status.ProbeState, bool)
}

func (m *fakeStatusManager) DoProbes(ctx context.Context, backends status.Backends) (status.ProbeState, error) {
	return m.FakeDoProbes(ctx, backends)
}

func (m *fakeStatusManager) IsProbeActive(ing types.NamespacedName) (status.ProbeState, bool) {
	return m.FakeIsProbeActive(ing)
}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

// We need to inject the row's `Objects` to work-around improper pluralization in UnsafeGuessKindToResource
func GatewayFactory(ctor func(context.Context, *Listers, configmap.Watcher, *TableRow) controller.Reconciler) Factory {
	return func(t *testing.T, r *TableRow) (
		controller.Reconciler, ActionRecorderList, EventList,
	) {
		shim := func(c context.Context, l *Listers, cw configmap.Watcher) controller.Reconciler {
			return ctor(c, l, cw, r)
		}
		return MakeFactory(shim)(t, r)
	}
}

type GatewayOption func(*gatewayapi.Gateway)

func gw(opts ...GatewayOption) *gatewayapi.Gateway {
	g := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      publicName,
			Namespace: testNamespace,
		},
		Spec: gatewayapi.GatewaySpec{
			GatewayClassName: gatewayAPIIngressClassName,
		},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func defaultListener(g *gatewayapi.Gateway) {
	g.Spec.Listeners = append(g.Spec.Listeners, gatewayapi.Listener{
		Name:     "http",
		Port:     80,
		Protocol: "HTTP",
	})
}

func tlsListener(hostname, nsName, secretName string) GatewayOption {
	return func(g *gatewayapi.Gateway) {
		g.Spec.Listeners = append(g.Spec.Listeners, gatewayapi.Listener{
			Name:     gatewayapi.SectionName("kni-"),
			Hostname: (*gatewayapi.Hostname)(&hostname),
			Port:     443,
			Protocol: "HTTPS",
			TLS: &gatewayapi.GatewayTLSConfig{
				Mode: (*gatewayapi.TLSModeType)(pointer.String("Terminate")),
				CertificateRefs: []gatewayapi.SecretObjectReference{{
					Group:     (*gatewayapi.Group)(pointer.String("")),
					Kind:      (*gatewayapi.Kind)(pointer.String("Secret")),
					Name:      gatewayapi.ObjectName(secretName),
					Namespace: (*gatewayapi.Namespace)(&nsName),
				}},
				Options: map[gatewayapi.AnnotationKey]gatewayapi.AnnotationValue{},
			},
			AllowedRoutes: &gatewayapi.AllowedRoutes{
				Namespaces: &gatewayapi.RouteNamespaces{
					From: (*gatewayapi.FromNamespaces)(pointer.String("Selector")),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": nsName,
						},
					},
				},
				Kinds: []gatewayapi.RouteGroupKind{},
			},
		})
	}
}

var withInitialConditions = func(i *v1alpha1.Ingress) {
	i.Status.InitializeConditions()
}
var withFinalizer = func(i *v1alpha1.Ingress) {
	i.Finalizers = append(i.Finalizers, "ingresses.networking.internal.knative.dev")
}

func withTLS() IngressOption {
	return func(i *v1alpha1.Ingress) {
		i.Spec.TLS = append(i.Spec.TLS, v1alpha1.IngressTLS{
			Hosts:           []string{"example.com"},
			SecretName:      "name-WE-STICK-A-LONG-UID-HERE",
			SecretNamespace: "ns",
		})
	}
}

func secret(name, ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		StringData: map[string]string{
			"ca.crt": "signed thing",
			"ca.key": "private thing",
		},
		Type: "kubernetes.io/tls",
	}
}

func rp(to *corev1.Secret) *gatewayapi.ReferenceGrant {
	t := true
	return &gatewayapi.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      to.Name + "-" + testNamespace,
			Namespace: to.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "networking.internal.knative.dev/v1alpha1",
				Kind:               "Ingress",
				Name:               "name",
				Controller:         &t,
				BlockOwnerDeletion: &t,
			}},
		},
		Spec: gatewayapi.ReferenceGrantSpec{
			From: []gatewayapi.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "Gateway",
				Namespace: gatewayapi.Namespace(testNamespace),
			}},
			To: []gatewayapi.ReferenceGrantTo{{
				Group: gatewayapi.Group(""),
				Kind:  gatewayapi.Kind("Secret"),
				Name:  (*gatewayapi.ObjectName)(&to.Name),
			}},
		},
	}
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
