package ingress

import (
	"context"
	"testing"

	fakenetworkingclient "knative.dev/networking/pkg/client/injection/client/fake"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"

	"knative.dev/net-ingressv2/pkg/reconciler/ingress/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	v2client "knative.dev/net-ingressv2/pkg/client/injection/client"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"

	_ "knative.dev/net-ingressv2/pkg/client/injection/informers/apis/v1alpha1/httproute/fake"

	corev1 "k8s.io/api/core/v1"
	clientgotesting "k8s.io/client-go/testing"

	duckv1 "knative.dev/pkg/apis/duck/v1"

	//	"knative.dev/pkg/apis"
	pkgnet "knative.dev/pkg/network"

	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"

	. "knative.dev/net-ingressv2/pkg/reconciler/testing"
	. "knative.dev/pkg/reconciler/testing"
)

const (
	testNS = "test-ns"
)

var (
	serviceName  = "test-service"
	ingressRules = []v1alpha1.IngressRule{{
		Hosts: []string{
			"host-tls.example.com",
			"host-tls.test-ns.svc.cluster.local",
		},
		HTTP: &v1alpha1.HTTPIngressRuleValue{
			Paths: []v1alpha1.HTTPIngressPath{{
				Splits: []v1alpha1.IngressBackendSplit{{
					IngressBackend: v1alpha1.IngressBackend{
						ServiceNamespace: testNS,
						ServiceName:      "test-service",
						ServicePort:      intstr.FromInt(80),
					},
					Percent: 100,
				}},
			}},
		},
		Visibility: v1alpha1.IngressVisibilityExternalIP,
	}}
)

func addAnnotations(ing *v1alpha1.Ingress, annos map[string]string) *v1alpha1.Ingress {
	// UnionMaps(a, b) where value from b wins. Use annos for second arg.
	ing.ObjectMeta.Annotations = kmeta.UnionMaps(ing.ObjectMeta.Annotations, annos)
	return ing
}

func ing(name string) *v1alpha1.Ingress {
	return ingressWithStatus(name, v1alpha1.IngressStatus{})
}

func ingressWithStatus(name string, status v1alpha1.IngressStatus) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
			Labels: map[string]string{
				resources.RouteLabelKey:          "test-route",
				resources.RouteNamespaceLabelKey: testNS,
			},
			Annotations:     map[string]string{networking.IngressClassAnnotationKey: resources.V2IngressClassName},
			ResourceVersion: "v1",
		},
		Spec: v1alpha1.IngressSpec{
			Rules: ingressRules,
		},
		Status: status,
	}
}

func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "skip ingress not matching class key",
		Objects: []runtime.Object{
			addAnnotations(ing("no-virtualservice-yet"),
				map[string]string{networking.IngressClassAnnotationKey: "fake-controller"}),
		},
	}, {
		Name: "reconcile HTTPRoutes to match desired one",
		/*
			WithReactors: []clientgotesting.ReactionFunc{
				InduceFailure("update", "virtualservices"),
			},
		*/
		Objects: []runtime.Object{
			ingressWithStatus("reconcile-httproute",
				v1alpha1.IngressStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   v1alpha1.IngressConditionNetworkConfigured,
							Status: corev1.ConditionTrue,
						}, {
							Type:   v1alpha1.IngressConditionReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			),
			&servicev1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcile-httproute",
					Namespace: testNS,
					Labels: map[string]string{
						resources.RouteLabelKey:          "test-route",
						resources.RouteNamespaceLabelKey: testNS,
					},
					Annotations:     map[string]string{networking.IngressClassAnnotationKey: resources.V2IngressClassName},
					OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing("reconcile-httproute"))},
				},
				Spec: servicev1alpha1.HTTPRouteSpec{
					Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("foo.example.com")},
				},
			},
			&servicev1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcile-httproute-extra",
					Namespace: testNS,
					Labels: map[string]string{
						resources.RouteLabelKey:          "test-route",
						resources.RouteNamespaceLabelKey: testNS,
					},
					Annotations:     map[string]string{networking.IngressClassAnnotationKey: resources.V2IngressClassName},
					OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing("reconcile-httproute"))},
				},
				Spec: servicev1alpha1.HTTPRouteSpec{
					Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("foo.example.com")},
				},
			},
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: &servicev1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcile-httproute",
					Namespace: testNS,
					Labels: map[string]string{
						resources.RouteLabelKey:          "test-route",
						resources.RouteNamespaceLabelKey: testNS,
					},
					Annotations:     map[string]string{networking.IngressClassAnnotationKey: resources.V2IngressClassName},
					OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(ing("reconcile-httproute"))},
				},
				Spec: servicev1alpha1.HTTPRouteSpec{
					Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("host-tls.example.com"), servicev1alpha1.Hostname("host-tls.test-ns.svc.cluster.local")},
					Rules: []servicev1alpha1.HTTPRouteRule{{
						ForwardTo: []servicev1alpha1.HTTPRouteForwardTo{{
							Port:        servicev1alpha1.PortNumber(80),
							ServiceName: &serviceName,
							Weight:      int32(100),
						}},
						Filters: []servicev1alpha1.HTTPRouteFilter{{
							Type: servicev1alpha1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &servicev1alpha1.HTTPRequestHeaderFilter{
								Add: nil,
							}},
						},
					}},
				},
			},
		}},
		/*
			WantDeletes: []clientgotesting.DeleteActionImpl{{
				ActionImpl: clientgotesting.ActionImpl{
					Namespace: testNS,
					Verb:      "delete",
					//	Resource:  v1alpha3.SchemeGroupVersion.WithResource("virtualservices"),
				},
				Name: "reconcile-httproute-extra",
			}},
		*/
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ingressWithStatus("reconcile-httproute",
				v1alpha1.IngressStatus{
					PublicLoadBalancer: &v1alpha1.LoadBalancerStatus{
						Ingress: []v1alpha1.LoadBalancerIngressStatus{
							{DomainInternal: pkgnet.GetServiceHostname("istio-ingressgateway", "istio-system")},
						},
					},
					PrivateLoadBalancer: &v1alpha1.LoadBalancerStatus{Ingress: []v1alpha1.LoadBalancerIngressStatus{}},
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   v1alpha1.IngressConditionLoadBalancerReady,
							Status: corev1.ConditionTrue,
						}, {
							Type:   v1alpha1.IngressConditionNetworkConfigured,
							Status: corev1.ConditionTrue,
						}, {
							Type:   v1alpha1.IngressConditionReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated HTTPRoute %s/%s", testNS, "reconcile-httproute"),
		},
		Key: "test-ns/reconcile-httproute",
	}}

	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers) controller.Reconciler {
		r := &Reconciler{
			httpLister:  listers.GetHttpRoutetLister(),
			v2ClientSet: v2client.Get(ctx),
			Tracker:     &NullTracker{},
		}

		return ingressreconciler.NewReconciler(ctx, logging.FromContext(ctx), fakenetworkingclient.Get(ctx),
			listers.GetIngressLister(), controller.GetEventRecorder(ctx), r, resources.V2IngressClassName)
	}))
}
