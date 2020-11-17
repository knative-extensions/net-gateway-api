package resources

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	network "knative.dev/networking/pkg"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"

	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"
	/*
		"k8s.io/apimachinery/pkg/util/intstr"
		"k8s.io/apimachinery/pkg/util/sets"
		"knative.dev/net-istio/pkg/reconciler/ingress/config"
		"knative.dev/networking/pkg/ingress"
		"knative.dev/pkg/system"
		_ "knative.dev/pkg/system/testing"
	*/)

var (
	serviceName = "test-service"
)

func TestMakeHTTPRoute_CorrectMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ing      *v1alpha1.Ingress
		expected []metav1.ObjectMeta
	}{{

		name: "propagate label and annotations from Ingress",
		ing: &v1alpha1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: "test-ns",
				Labels: map[string]string{
					RouteLabelKey:              "test-route",
					RouteNamespaceLabelKey:     "test-ns",
					networking.IngressLabelKey: "test-ingress",
				},
				Annotations: map[string]string{networking.IngressClassAnnotationKey: network.IstioIngressClassName},
			},
			Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
				Hosts: []string{
					"test-route.test-ns.svc.cluster.local",
				},
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP:       &v1alpha1.HTTPIngressRuleValue{},
			}}},
		},
		expected: []metav1.ObjectMeta{{
			Name:      "test-ingress",
			Namespace: "test-ns",
			Labels: map[string]string{
				RouteLabelKey:              "test-route",
				RouteNamespaceLabelKey:     "test-ns",
				networking.IngressLabelKey: "test-ingress",
			},
			Annotations: map[string]string{networking.IngressClassAnnotationKey: network.IstioIngressClassName},
		}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			hr, err := MakeHTTPRoutes(context.Background(), tc.ing)
			if err != nil {
				t.Fatal("MakeHTTPRoutes failed:", err)
			}
			if len(hr) != len(tc.expected) {
				t.Fatalf("Expected %d HTTPRoutes, saw %d", len(tc.expected), len(hr))
			}
			for i := range tc.expected {
				tc.expected[i].OwnerReferences = []metav1.OwnerReference{*kmeta.NewControllerRef(tc.ing)}
				if diff := cmp.Diff(tc.expected[i], hr[i].ObjectMeta); diff != "" {
					t.Error("Unexpected metadata (-want +got):", diff)
				}
			}
		})
	}
}

func TestMakeHTTPRoute_CorrectSpec(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ing      *v1alpha1.Ingress
		expected []servicev1alpha1.HTTPRouteSpec
	}{{

		name: "propagate label and annotations from Ingress",
		ing: &v1alpha1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: "test-ns",
				Labels: map[string]string{
					RouteLabelKey:              "test-route",
					RouteNamespaceLabelKey:     "test-ns",
					networking.IngressLabelKey: "test-ingress",
				},
				Annotations: map[string]string{networking.IngressClassAnnotationKey: network.IstioIngressClassName},
			},
			Spec: v1alpha1.IngressSpec{Rules: []v1alpha1.IngressRule{{
				Hosts: []string{
					"test-route.test-ns.svc.cluster.local",
				},
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP:       &v1alpha1.HTTPIngressRuleValue{},
			}}},
		},

		expected: []servicev1alpha1.HTTPRouteSpec{{
			Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("test-route.test-ns.svc.cluster.local")},
			Rules: []servicev1alpha1.HTTPRouteRule{{
				ForwardTo: []servicev1alpha1.HTTPRouteForwardTo{{
					Port:        servicev1alpha1.PortNumber(80),
					ServiceName: &serviceName,
					Weight:      int32(100),
					Filters: []servicev1alpha1.HTTPRouteFilter{{
						Type: servicev1alpha1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &servicev1alpha1.HTTPRequestHeaderFilter{
							Add: map[string]string{"foo": "bar"},
						}},
					}}},
			}},
		}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			hr, err := MakeHTTPRoutes(context.Background(), tc.ing)
			if err != nil {
				t.Fatal("MakeHTTPRoutes failed:", err)
			}
			if len(hr) != len(tc.expected) {
				t.Fatalf("Expected %d HTTPRoutes, saw %d", len(tc.expected), len(hr))
			}
			for i := range tc.expected {
				if diff := cmp.Diff(tc.expected[i].Hostnames, hr[i].Spec.Hostnames); diff != "" {
					t.Error("Unexpected hostnames (-want +got):", diff)
				}
				// TODO: Rueles
			}
		})
	}
}
