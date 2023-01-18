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
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"

	fakegwapiclientset "knative.dev/net-gateway-api/pkg/client/injection/client/fake"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/resources"
	"knative.dev/net-gateway-api/pkg/status"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	fakeingressclient "knative.dev/networking/pkg/client/injection/client/fake"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	networkcfg "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/pkg/ingress"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/network"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	gwtesting "knative.dev/net-gateway-api/pkg/reconciler/testing"
	ktesting "knative.dev/pkg/reconciler/testing"
)

var (
	publicSvcIP  = "1.2.3.4"
	privateSvcIP = "5.6.7.8"
	publicSvc    = network.GetServiceHostname(publicName, testNamespace)
	privateSvc   = network.GetServiceHostname(privateName, testNamespace)

	fakeStatusKey struct{}

	publicGatewayAddress  = "11.22.33.44"
	publicGatewayHostname = "off.cluster.gateway"
	privateGatewayAddress = "55.66.77.88"
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
	table := ktesting.TableTest{{
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
				i.Status.MarkIngressNotReady("HTTPRouteNotReady", "Waiting for HTTPRoute becomes Ready.")
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
			ktesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}, {
		Name: "reconcile ready ingress",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, makeItReady, withFinalizer),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
		}, servicesAndEndpoints...),
		// no extra update
	}}

	table.Test(t, gwtesting.MakeFactory(func(ctx context.Context, listers *gwtesting.Listers, cmw configmap.Watcher) controller.Reconciler {
		r := &Reconciler{
			gwapiclient: fakegwapiclientset.Get(ctx),
			// Listers index properties about resources
			httprouteLister: listers.GetHTTPRouteLister(),
			gatewayLister:   listers.GetGatewayLister(),
			statusManager: &fakeStatusManager{
				FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
					return status.ProbeState{Ready: true}, nil
				},
				FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
					return status.ProbeState{Ready: true}, true
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
	table := ktesting.TableTest{{
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
				i.Status.MarkIngressNotReady("HTTPRouteNotReady", "Waiting for HTTPRoute becomes Ready.")
				i.Status.MarkLoadBalancerNotReady()
			}),
		}},
		WantEvents: []string{
			ktesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", `Created HTTPRoute "example.com"`),
		},
	}, {
		Name: "Already Configured",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing(withBasicSpec, withFinalizer, withGatewayAPIClass, withTLS(), makeItReady),
			secret(secretName, nsName),
			gw(defaultListener, tlsListener("example.com", nsName, secretName)),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withTLS()), httpRouteReady),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{
			// None
		},
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
			ktesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", `Created HTTPRoute "example.com"`),
			ktesting.Eventf(corev1.EventTypeWarning, "GatewayMissing", `Unable to update Gateway istio-system/istio-gateway`),
			ktesting.Eventf(corev1.EventTypeWarning, "InternalError", `Gateway istio-system/istio-gateway does not exist: gateway.gateway.networking.k8s.io "istio-gateway" not found`),
		},
	}, {
		Name: "TLS ingress with httpOption redirected",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIClass, withHTTPOptionRedirected, withTLS()),
			secret(secretName, nsName),
			gw(defaultListener),
		}, servicesAndEndpoints...),
		WantCreates: []runtime.Object{
			httpRoute(t, ing(withBasicSpec, withGatewayAPIClass, withHTTPOptionRedirected, withTLS()), withSectionName("kni-")),
			httpRedirectRoute(t, ing(withBasicSpec, withGatewayAPIClass, withHTTPOptionRedirected, withTLS()), withSectionName("http")),
			rp(secret(secretName, nsName)),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: gw(defaultListener, tlsListener("example.com", nsName, secretName)),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec, withGatewayAPIClass, withHTTPOptionRedirected, withTLS(), func(i *v1alpha1.Ingress) {
				// These are the things we expect to change in status.
				i.Status.InitializeConditions()
				i.Status.MarkIngressNotReady("HTTPRouteNotReady", "Waiting for HTTPRoute becomes Ready.")
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
			ktesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", "Created redirect HTTPRoute \"example.com-redirect\""),
		},
	}}

	table.Test(t, GatewayFactory(func(ctx context.Context, listers *gwtesting.Listers, cmw configmap.Watcher, tr *ktesting.TableRow) controller.Reconciler {
		r := &Reconciler{
			gwapiclient:          fakegwapiclientset.Get(ctx),
			httprouteLister:      listers.GetHTTPRouteLister(),
			referenceGrantLister: listers.GetReferenceGrantLister(),
			gatewayLister:        listers.GetGatewayLister(),
			statusManager: &fakeStatusManager{
				FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
					return status.ProbeState{Ready: true}, nil
				},
				FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
					return status.ProbeState{Ready: true}, true
				},
			},
		}
		// The fake tracker's `Add` method incorrectly pluralizes "gatewaies" using UnsafeGuessKindToResource,
		// so create this via explicit call (per note in client-go/testing/fixture.go in tracker.Add)
		fakeCreates := []runtime.Object{}
		for _, x := range tr.Objects {
			myGw, ok := x.(*gatewayapi.Gateway)
			if ok {
				fakegwapiclientset.Get(ctx).GatewayV1().Gateways(myGw.Namespace).Create(ctx, myGw, metav1.CreateOptions{})
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
	table := ktesting.TableTest{{
		Name: "first reconciler probe returns false",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: false}, false
			},
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
			ktesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "name" finalizers`),
			ktesting.Eventf(corev1.EventTypeNormal, "Created", "Created HTTPRoute \"example.com\""),
		},
	}, {
		Name: "prober callback all endpoints ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true}, true
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
			ing(withBasicSpec, withSecondRevisionSpec, withGatewayAPIclass, withFinalizer, makeItReady),
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
			Object: ing(
				withBasicSpec,
				withSecondRevisionSpec,
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "steady state ingress - endpoint probing still not ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{
					Ready:   false,
					Version: "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
				}, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
	}, {
		Name: "endpoints are ready - transition to new backends",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "steady state - transition probing still not ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: false, Version: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			&gatewayapi.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example.com",
					Namespace: "ns",
					Annotations: map[string]string{
						networking.IngressClassAnnotationKey: gatewayAPIIngressClassName,
					},
					Labels: map[string]string{
						networking.VisibilityLabelKey: "",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         "networking.internal.knative.dev/v1alpha1",
						Kind:               "Ingress",
						Name:               "name",
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					}},
				},
				Spec: gatewayapi.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi.CommonRouteSpec{
						ParentRefs: []gatewayapi.ParentReference{{
							Group:     ptr.To[gatewayapi.Group]("gateway.networking.k8s.io"),
							Kind:      ptr.To[gatewayapi.Kind]("Gateway"),
							Namespace: ptr.To[gatewayapi.Namespace]("istio-system"),
							Name:      "istio-gateway",
						}},
					},
					Hostnames: []gatewayapi.Hostname{"example.com"},
					Rules: []gatewayapi.HTTPRouteRule{{
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							Headers: []gatewayapi.HTTPHeaderMatch{{
								Type:  ptr.To(gatewayapi.HeaderMatchExact),
								Name:  header.HashKey,
								Value: header.HashValueOverride,
							}},
						}},
						Filters: []gatewayapi.HTTPRouteFilter{{
							Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
								Set: []gatewayapi.HTTPHeader{{
									Name:  header.HashKey,
									Value: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
								}},
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{{
										Name:  "K-Serving-Revision",
										Value: "second-revision",
									}, {
										Name:  "K-Serving-Namespace",
										Value: "ns",
									}},
								},
							}},
							BackendRef: gatewayapi.BackendRef{
								BackendObjectReference: gatewayapi.BackendObjectReference{
									Group: ptr.To[gatewayapi.Group](""),
									Kind:  ptr.To[gatewayapi.Kind]("Service"),
									Name:  "second-revision",
									Port:  ptr.To[gatewayapi.PortNumber](123),
								},
								Weight: ptr.To[int32](100),
							},
						}},
					}, {
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{{
										Name:  "K-Serving-Revision",
										Value: "second-revision",
									}, {
										Name:  "K-Serving-Namespace",
										Value: "ns",
									}},
								},
							}},
							BackendRef: gatewayapi.BackendRef{
								BackendObjectReference: gatewayapi.BackendObjectReference{
									Group: ptr.To[gatewayapi.Group](""),
									Kind:  ptr.To[gatewayapi.Kind]("Service"),
									Name:  "second-revision",
									Port:  ptr.To[gatewayapi.PortNumber](123),
								},
								Weight: ptr.To[int32](100),
							},
						}},
					}, {
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/.well-known/knative/revision/ns/second-revision"),
							},
							Headers: []gatewayapi.HTTPHeaderMatch{{
								Type:  ptr.To(gatewayapi.HeaderMatchExact),
								Name:  header.HashKey,
								Value: header.HashValueOverride,
							}},
						}},
						Filters: []gatewayapi.HTTPRouteFilter{{
							Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
								Set: []gatewayapi.HTTPHeader{{
									Name:  header.HashKey,
									Value: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
								}},
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{{
										Name:  "K-Serving-Namespace",
										Value: "ns",
									}, {
										Name:  "K-Serving-Revision",
										Value: "second-revision",
									}},
								},
							}},
							BackendRef: gatewayapi.BackendRef{
								Weight: ptr.To[int32](100),
								BackendObjectReference: gatewayapi.BackendObjectReference{
									Group: ptr.To[gatewayapi.Group](""),
									Kind:  ptr.To[gatewayapi.Kind]("Service"),
									Name:  "second-revision",
									Port:  ptr.To[gatewayapi.PortNumber](123),
								},
							},
						}},
					}, {
						Matches: []gatewayapi.HTTPRouteMatch{{
							Path: &gatewayapi.HTTPPathMatch{
								Type:  ptr.To(gatewayapi.PathMatchPathPrefix),
								Value: ptr.To("/.well-known/knative/revision/ns/goo"),
							},
							Headers: []gatewayapi.HTTPHeaderMatch{{
								Type:  ptr.To(gatewayapi.HeaderMatchExact),
								Name:  header.HashKey,
								Value: header.HashValueOverride,
							}},
						}},
						Filters: []gatewayapi.HTTPRouteFilter{{
							Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
								Set: []gatewayapi.HTTPHeader{{
									Name:  header.HashKey,
									Value: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
								}},
							},
						}},
						BackendRefs: []gatewayapi.HTTPBackendRef{{
							Filters: []gatewayapi.HTTPRouteFilter{{
								Type: gatewayapi.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi.HTTPHeaderFilter{
									Set: []gatewayapi.HTTPHeader{{
										Name:  "K-Serving-Namespace",
										Value: "ns",
									}, {
										Name:  "K-Serving-Revision",
										Value: "goo",
									}},
								},
							}},
							BackendRef: gatewayapi.BackendRef{
								Weight: ptr.To[int32](100),
								BackendObjectReference: gatewayapi.BackendObjectReference{
									Group: ptr.To[gatewayapi.Group](""),
									Kind:  ptr.To[gatewayapi.Kind]("Service"),
									Name:  "goo",
									Port:  ptr.To[gatewayapi.PortNumber](123),
								},
							},
						}},
					}},
				},
				Status: gatewayapi.HTTPRouteStatus{
					RouteStatus: gatewayapi.RouteStatus{
						Parents: []gatewayapi.RouteParentStatus{{
							Conditions: []metav1.Condition{{
								Type:   string(gatewayapi.RouteConditionAccepted),
								Status: metav1.ConditionTrue,
							}},
						}},
					},
				},
			},
		},
			servicesAndEndpoints...),
	}, {
		Name: "transition probe complete - drop probes",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		},
			servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "dropping probes complete - mark ingress ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
		}),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
			),
		}},
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		},
			servicesAndEndpoints...),
	}, {
		Name: "endpoints are ready - wrong hash",
		// When the endpoints are ready but the hash is incorrect we do
		// not transition the backend
		Key: "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "bad-hash"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Path:      "/.well-known/knative/revision/ns/goo",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
	}, {
		Name: "updated ingress - while endpoint probing in progress",
		// Here we want the existing probe to stop and then new backends added
		// to the endpoint probes
		Key: "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: false, Version: "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"}, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withThirdRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Path:      "/.well-known/knative/revision/ns/second-revision",
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Path:      "/.well-known/knative/revision/ns/goo",
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		},
			servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-40e40e812e47b79d9bae1f1d0ecec5bcb481030dad90a1aa6200f3389c31d374",
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "third-revision",
						Hash:      "ep-40e40e812e47b79d9bae1f1d0ecec5bcb481030dad90a1aa6200f3389c31d374",
						Path:      "/.well-known/knative/revision/ns/third-revision",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-40e40e812e47b79d9bae1f1d0ecec5bcb481030dad90a1aa6200f3389c31d374",
						Path:      "/.well-known/knative/revision/ns/goo",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "updated ingress - backend headers change",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				withBackendAppendHeaders("key", "value")),
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
			Object: ing(withBasicSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
				withBackendAppendHeaders("key", "value"),
			),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Name:      "goo",
						Namespace: "ns",
						Hash:      "3531718c72349578ea2293f8ec1cd980d551f70295c1b0b4c10abfc0b2a248f8",
						Headers:   []string{"key", "value"},
						Port:      123,
					},
					NormalRule{
						Name:      "goo",
						Namespace: "ns",
						Headers:   []string{"key", "value"},
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "multiple visibility - updated ingress - new backends used for endpoint probing",
		Key:  "ns/name",
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "first-hash",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "first-hash",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      124,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
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
			Object: ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
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
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
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
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, {
			Object: HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "multiple visibility - steady state ingress - endpoint probing still not ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{
					Ready:   false,
					Version: "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
				}, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
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
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
	}, {
		Name: "multiple visibility - endpoints are ready - transition to new backends",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
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
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, {
			Object: HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "multiple visibility - steady state - transition probing still not ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: false, Version: "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
	}, {
		Name: "multiple visibility - transition complete - drop probes",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, {
			Object: HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "multiple visibility - dropping probes complete - mark ingress ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				state := status.ProbeState{Ready: true, Version: "ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970"}
				return state, true
			},
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      123,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing(
				withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
			),
		}},
	}, {
		Name: "multiple visibility - steady state ingress - probe state flips while reconciliing",
		// Probes are tied to the HTTPRoute so they can have different hashes
		Key: "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: ProbeIsReadyAfter{
				Attempts: 1,
				Hash:     "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
			}.Build(),
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: false}, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withInternalSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
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
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
			HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "goo",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: HTTPRoute{
				Name:         "foo.svc.cluster.local",
				Namespace:    "ns",
				Hostnames:    []string{"foo.svc", "foo.svc.cluster.local"},
				ClusterLocal: true,
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					NormalRule{
						Namespace: "ns",
						Name:      "second-revision",
						Port:      124,
						Weight:    100,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "second-revision",
						Path:      "/.well-known/knative/revision/ns/second-revision",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "tr-ff3cee4d49fbd4547b85c63d56e88eb866d4043951761f069d6afe14a2e61970",
						Port:      124,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}},
	}, {
		Name: "stale informer cache",
		// A stale httproute in the informer cache can result in probing to get stuck
		//
		// The following events need to happen
		// 1. Endpoint probe (version: ep-*) succeeds for an HTTPRoute (generation 2)
		// 2. We trigger an HTTPRoute update to move new route backends into the
		//    main rules. API server has HTTPRoute with generation 3.
		// 3. Start probing (version: tr-*)
		// 4. Some event triggers reconciliation of the parent Ingress. The hash
		//    remains the same so there are no spec changes.
		// 5. Informer cache still has HTTPRoute (generation 2)
		Key: "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{
					Ready:   false,
					Version: "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
				}, false
			},
			FakeDoProbes: func(ctx context.Context, s status.Backends) (status.ProbeState, error) {
				state := status.ProbeState{}
				expectedHash := "tr-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2"

				if s.Version != expectedHash {
					panic(fmt.Sprintf("Expected DoProbes to be called with the same hash got: %q want: %q",
						s.Version,
						expectedHash,
					))
				}

				return state, nil
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec,
				withSecondRevisionSpec,
				withGatewayAPIclass,
				withFinalizer,
				makeItReady,
				makeLoadBalancerNotReady,
			),
			HTTPRoute{
				Name:      "example.com",
				Namespace: "ns",
				Hostname:  "example.com",
				Rules: []RuleBuilder{
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
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
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
					EndpointProbeRule{
						Namespace: "ns",
						Name:      "goo",
						Path:      "/.well-known/knative/revision/ns/goo",
						Hash:      "ep-9333a9a68409bb44f2a5f538d2d7c617e5338b6b6c1ebc5e00a19612a5c962c2",
						Port:      123,
					},
				},
				StatusConditions: []metav1.Condition{{
					Type:   string(gatewayapi.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				}},
			}.Build(),
		}, servicesAndEndpoints...),
		WantUpdates: nil, // No updates
	}}

	table.Test(t, gwtesting.MakeFactory(func(ctx context.Context, listers *gwtesting.Listers, cmw configmap.Watcher) controller.Reconciler {
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

type ProbeIsReadyAfter struct {
	Attempts int
	Hash     string
}

func (p ProbeIsReadyAfter) Build() func(types.NamespacedName) (status.ProbeState, bool) {
	return func(types.NamespacedName) (status.ProbeState, bool) {
		ready := p.Attempts <= 0
		p.Attempts--
		return status.ProbeState{Ready: ready, Version: p.Hash}, true
	}
}

func makeLoadBalancerNotReady(i *v1alpha1.Ingress) {
	i.Status.MarkLoadBalancerNotReady()
}
func TestReconcileProbingOffClusterGateway(t *testing.T) {
	table := ktesting.TableTest{{
		Name: "prober callback all endpoints ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true}, true
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, withFinalizer, withInitialConditions),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
			gw(defaultListener, setStatusPublicAddressIP),
			gw(privateGw, defaultListener, setStatusPrivateAddress),
		}, servicesAndEndpoints...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{
			{Object: ing(withBasicSpec, withGatewayAPIclass, withFinalizer, makeItReadyOffClusterGateway)},
		},
	}, {
		Name: "gateway has hostname in address",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true}, true
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, withFinalizer, withInitialConditions),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
			gw(defaultListener, setStatusPublicAddressHostname),
			gw(privateGw, defaultListener, setStatusPrivateAddress),
		}, servicesAndEndpoints...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{
			{Object: ing(withBasicSpec, withGatewayAPIclass, withFinalizer, makeItReadyOffClusterGatewayHostname)},
		},
	}, {
		Name: "gateway not ready",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true}, true
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, withFinalizer, withInitialConditions),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
			gw(defaultListener),
			gw(privateGw, defaultListener),
		}, servicesAndEndpoints...),
		WantErr: true,
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{
			{Object: ing(
				withBasicSpec,
				withGatewayAPIClass,
				withFinalizer,
				func(i *v1alpha1.Ingress) {
					i.Status.InitializeConditions()
					i.Status.MarkLoadBalancerNotReady()
					i.Status.MarkNetworkConfigured()
					i.Status.MarkIngressNotReady("ReconcileIngressFailed", "Ingress reconciliation failed")
				},
			)},
		},
		WantEvents: []string{
			ktesting.Eventf(corev1.EventTypeWarning, "InternalError", `no address found in status of Gateway istio-system/istio-gateway`),
		},
	}, {
		Name: "gateway doesn't exist",
		Key:  "ns/name",
		Ctx: withStatusManager(&fakeStatusManager{
			FakeDoProbes: func(context.Context, status.Backends) (status.ProbeState, error) {
				return status.ProbeState{Ready: true}, nil
			},
			FakeIsProbeActive: func(types.NamespacedName) (status.ProbeState, bool) {
				return status.ProbeState{Ready: true}, true
			},
		}),
		Objects: append([]runtime.Object{
			ing(withBasicSpec, withGatewayAPIclass, withFinalizer, withInitialConditions),
			httpRoute(t, ing(withBasicSpec, withGatewayAPIclass), httpRouteReady),
		}, servicesAndEndpoints...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{Object: ing(
			withBasicSpec,
			withGatewayAPIClass,
			withFinalizer,
			func(i *v1alpha1.Ingress) {
				i.Status.InitializeConditions()
				i.Status.MarkLoadBalancerFailed("GatewayDoesNotExist", "could not find Gateway istio-system/istio-gateway")
				i.Status.MarkNetworkConfigured()
			})}},
	}}

	table.Test(t, gwtesting.MakeFactory(func(ctx context.Context, listers *gwtesting.Listers, cmw configmap.Watcher) controller.Reconciler {
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
					config: configNoService,
				}})
	}))
}

func makeItReadyOffClusterGateway(i *v1alpha1.Ingress) {
	i.Status.InitializeConditions()
	i.Status.MarkNetworkConfigured()
	i.Status.MarkLoadBalancerReady(
		[]v1alpha1.LoadBalancerIngressStatus{{
			IP: publicGatewayAddress,
		}},
		[]v1alpha1.LoadBalancerIngressStatus{{
			IP: privateGatewayAddress,
		}})
}

func makeItReadyOffClusterGatewayHostname(i *v1alpha1.Ingress) {
	i.Status.InitializeConditions()
	i.Status.MarkNetworkConfigured()
	i.Status.MarkLoadBalancerReady(
		[]v1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: publicGatewayHostname,
		}},
		[]v1alpha1.LoadBalancerIngressStatus{{
			IP: privateGatewayAddress,
		}})
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
	httpRoute, _ := resources.MakeHTTPRoute(ctx, i, &i.Spec.Rules[0], nil)
	for _, opt := range opts {
		opt(httpRoute)
	}
	return httpRoute
}

func httpRedirectRoute(t *testing.T, i *v1alpha1.Ingress, opts ...HTTPRouteOption) runtime.Object {
	t.Helper()
	ingress.InsertProbe(i)
	ctx := (&testConfigStore{config: defaultConfig}).ToContext(context.Background())
	httpRedirectRoute, _ := resources.MakeRedirectHTTPRoute(ctx, i, &i.Spec.Rules[0])
	for _, opt := range opts {
		opt(httpRedirectRoute)
	}
	return httpRedirectRoute
}

func withSectionName(sectionName string) HTTPRouteOption {
	return func(h *gatewayapi.HTTPRoute) {
		h.Spec.CommonRouteSpec.ParentRefs[0].SectionName = (*gatewayapi.SectionName)(ptr.To(sectionName))
	}
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
func GatewayFactory(ctor func(context.Context, *gwtesting.Listers, configmap.Watcher, *ktesting.TableRow) controller.Reconciler) ktesting.Factory {
	return func(t *testing.T, r *ktesting.TableRow) (
		controller.Reconciler, ktesting.ActionRecorderList, ktesting.EventList,
	) {
		shim := func(c context.Context, l *gwtesting.Listers, cw configmap.Watcher) controller.Reconciler {
			return ctor(c, l, cw, r)
		}
		return gwtesting.MakeFactory(shim)(t, r)
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

func privateGw(g *gatewayapi.Gateway) {
	g.Name = privateName
}

func setStatusPrivateAddress(g *gatewayapi.Gateway) {
	g.Status.Addresses = append(g.Status.Addresses, gatewayapi.GatewayStatusAddress{
		Type:  ptr.To[gatewayapi.AddressType](gatewayapi.IPAddressType),
		Value: privateGatewayAddress,
	})
}

func setStatusPublicAddressIP(g *gatewayapi.Gateway) {
	g.Status.Addresses = append(g.Status.Addresses, gatewayapi.GatewayStatusAddress{
		Type:  ptr.To[gatewayapi.AddressType](gatewayapi.IPAddressType),
		Value: publicGatewayAddress,
	})
}

func setStatusPublicAddressHostname(g *gatewayapi.Gateway) {
	g.Status.Addresses = append(g.Status.Addresses, gatewayapi.GatewayStatusAddress{
		Type:  ptr.To[gatewayapi.AddressType](gatewayapi.HostnameAddressType),
		Value: publicGatewayHostname,
	})
}

func tlsListener(hostname, nsName, secretName string) GatewayOption {
	return func(g *gatewayapi.Gateway) {
		g.Spec.Listeners = append(g.Spec.Listeners, gatewayapi.Listener{
			Name:     "kni-",
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

func rp(to *corev1.Secret) *gatewayapiv1beta1.ReferenceGrant {
	t := true
	return &gatewayapiv1beta1.ReferenceGrant{
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
		Spec: gatewayapiv1beta1.ReferenceGrantSpec{
			From: []gatewayapiv1beta1.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "Gateway",
				Namespace: gatewayapi.Namespace(testNamespace),
			}},
			To: []gatewayapiv1beta1.ReferenceGrantTo{{
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
		GatewayPlugin: &config.GatewayPlugin{
			ExternalGateways: []config.Gateway{{
				Service:          &types.NamespacedName{Namespace: "istio-system", Name: "istio-gateway"},
				NamespacedName:   types.NamespacedName{Namespace: "istio-system", Name: "istio-gateway"},
				HTTPListenerName: "http",
			}},
			LocalGateways: []config.Gateway{{
				Service:          &types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"},
				NamespacedName:   types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"},
				HTTPListenerName: "http",
			}},
		},
	}

	configNoService = &config.Config{
		Network: &networkcfg.Config{},
		GatewayPlugin: &config.GatewayPlugin{
			ExternalGateways: []config.Gateway{{
				NamespacedName:   types.NamespacedName{Namespace: "istio-system", Name: "istio-gateway"},
				HTTPListenerName: "http",
			}},
			LocalGateways: []config.Gateway{{
				NamespacedName:   types.NamespacedName{Namespace: "istio-system", Name: "knative-local-gateway"},
				HTTPListenerName: "http",
			}},
		},
	}
)
