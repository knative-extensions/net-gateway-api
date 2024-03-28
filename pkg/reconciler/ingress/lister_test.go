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
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/kmeta"

	ingress "knative.dev/net-gateway-api/pkg/reconciler/testing"
)

var (
	testNamespace = "istio-system"
	publicName    = "istio-gateway"
	privateName   = "knative-local-gateway"
)

func TestListProbeTargets(t *testing.T) {
	tests := []struct {
		name    string
		ing     *v1alpha1.Ingress
		objects []runtime.Object
		want    []status.ProbeTarget
		wantErr error
	}{{
		name: "single address to probe",
		objects: []runtime.Object{
			privateEndpointsOneAddr,
			publicEndpointsOneAddr,
		},
		ing: ing(withBasicSpec, withGatewayAPIClass),
		want: []status.ProbeTarget{
			{
				PodIPs:  sets.New("1.2.3.4"),
				PodPort: "8080",
				URLs: []*url.URL{{
					Scheme: "http",
					Host:   "example.com",
					Path:   "/",
				}},
			},
		},
	}, {
		name: "no local endpoint to probe",
		objects: []runtime.Object{
			publicEndpointsOneAddr,
		},
		ing:     ing(withBasicSpec, withInternalSpec, withGatewayAPIClass),
		wantErr: fmt.Errorf("failed to get endpoints: endpoints %q not found", privateName),
	}, {
		name: "no external endpoint to probe",
		objects: []runtime.Object{
			privateEndpointsOneAddr,
		},
		ing:     ing(withBasicSpec, withGatewayAPIClass),
		wantErr: fmt.Errorf("failed to get endpoints: endpoints %q not found", "istio-gateway"),
	}, {
		name: "local endpoint without address to probe",
		objects: []runtime.Object{
			privateEndpointsNoAddr,
			publicEndpointsOneAddr,
		},
		ing:     ing(withBasicSpec, withInternalSpec, withGatewayAPIClass),
		wantErr: fmt.Errorf("no gateway pods available"),
	}, {
		name: "external endpoint without address to probe",
		objects: []runtime.Object{
			privateEndpointsOneAddr,
			publicEndpointsNoAddr,
		},
		ing:     ing(withBasicSpec, withGatewayAPIClass),
		wantErr: fmt.Errorf("no gateway pods available"),
	}, {
		name: "endpoint with single address to probe (https redirected)",
		objects: []runtime.Object{
			privateEndpointsOneAddr,
			publicSslEndpointsOneAddr,
		},
		ing: ing(withBasicSpec, withGatewayAPIClass, withHTTPOption(v1alpha1.HTTPOptionRedirected)),
		want: []status.ProbeTarget{{
			PodIPs:  sets.New("1.2.3.4"),
			PodPort: "8443",
			URLs: []*url.URL{{
				Scheme: "https",
				Host:   "example.com",
				Path:   "/",
			}},
		}},
	}, {
		name: "endpoint with multiple addresses and subsets to probe",
		objects: []runtime.Object{
			privateEndpointsMultiAddrMultiSubset,
			publicEndpointsMultiAddrMultiSubset,
		},
		ing: ing(withBasicSpec, withGatewayAPIClass),
		want: []status.ProbeTarget{
			{
				PodIPs:  sets.New("2.3.4.5"),
				PodPort: "1234",
				URLs: []*url.URL{{
					Scheme: "http",
					Host:   "example.com",
					Path:   "/",
				}},
			}, {
				PodIPs:  sets.New("3.4.5.6", "4.3.2.1"),
				PodPort: "4321",
				URLs: []*url.URL{{
					Scheme: "http",
					Host:   "example.com",
					Path:   "/",
				}},
			}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tl := ingress.NewListers(test.objects)

			l := &gatewayPodTargetLister{
				endpointsLister: tl.GetEndpointsLister(),
			}

			cfg := defaultConfig.DeepCopy()
			ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

			got, gotErr := l.ListProbeTargets(ctx, test.ing)
			if (gotErr != nil) != (test.wantErr != nil) {
				t.Fatalf("ListProbeTargets() = %v, wanted %v", gotErr, test.wantErr)
			} else if gotErr != nil && test.wantErr != nil && gotErr.Error() != test.wantErr.Error() {
				t.Fatalf("ListProbeTargets() = %v, wanted %v", gotErr, test.wantErr)
			}

			if !cmp.Equal(test.want, got) {
				t.Error("ListProbeTargets (-want, +got) =", cmp.Diff(test.want, got))
			}
		})
	}
}

var (
	privateEndpointsOneAddr = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      privateName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "http",
				Port: 8081,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "1.2.3.4",
			}},
		}},
	}

	publicEndpointsOneAddr = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      publicName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "http",
				Port: 8080,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "1.2.3.4",
			}},
		}},
	}

	publicSslEndpointsOneAddr = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      publicName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "http",
				Port: 8443,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "1.2.3.4",
			}},
		}},
	}

	privateEndpointsMultiAddrMultiSubset = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      privateName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "asdf",
				Port: 1234,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "2.3.4.5",
			}},
		}, {
			Ports: []corev1.EndpointPort{{
				Name: "http2",
				Port: 4321,
			}, {
				Name: "admin",
				Port: 1337,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "3.4.5.6",
			}, {
				IP: "4.3.2.1",
			}},
		}},
	}
	publicEndpointsMultiAddrMultiSubset = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      publicName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "asdf",
				Port: 1234,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "2.3.4.5",
			}},
		}, {
			Ports: []corev1.EndpointPort{{
				Name: "asdf",
				Port: 4321,
			}},
			Addresses: []corev1.EndpointAddress{{
				IP: "3.4.5.6",
			}, {
				IP: "4.3.2.1",
			}},
		}},
	}
	privateEndpointsNoAddr = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      privateName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "fdsa",
				Port: 32,
			}},
		}},
	}
	publicEndpointsNoAddr = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      publicName,
		},
		Subsets: []corev1.EndpointSubset{{
			Ports: []corev1.EndpointPort{{
				Name: "fdsa",
				Port: 32,
			}},
		}},
	}
)

func withBasicSpec(i *v1alpha1.Ingress) {
	i.Spec.HTTPOption = v1alpha1.HTTPOptionEnabled
	i.Spec.Rules = append(i.Spec.Rules, v1alpha1.IngressRule{
		Hosts:      []string{"example.com"},
		Visibility: v1alpha1.IngressVisibilityExternalIP,
		HTTP: &v1alpha1.HTTPIngressRuleValue{
			Paths: []v1alpha1.HTTPIngressPath{{
				Splits: []v1alpha1.IngressBackendSplit{{
					IngressBackend: v1alpha1.IngressBackend{
						ServiceName:      "goo",
						ServiceNamespace: i.Namespace,
						ServicePort:      intstr.FromInt(123),
					},
					Percent: 100,
				}},
			}},
		},
	})
}

func withHTTPOptionRedirected(i *v1alpha1.Ingress) {
	i.Spec.HTTPOption = v1alpha1.HTTPOptionRedirected
}

func withInternalSpec(i *v1alpha1.Ingress) {
	i.Spec.Rules = append(i.Spec.Rules, v1alpha1.IngressRule{
		Hosts:      []string{"foo.svc", "foo.svc.cluster.local"},
		Visibility: v1alpha1.IngressVisibilityClusterLocal,
		HTTP: &v1alpha1.HTTPIngressRuleValue{
			Paths: []v1alpha1.HTTPIngressPath{{
				Splits: []v1alpha1.IngressBackendSplit{{
					IngressBackend: v1alpha1.IngressBackend{
						ServiceName:      "goo",
						ServiceNamespace: i.Namespace,
						ServicePort:      intstr.FromInt(124),
					},
					Percent: 100,
				}},
			}},
		},
	})
}

type IngressOption func(*v1alpha1.Ingress)

func ing(opts ...IngressOption) *v1alpha1.Ingress {
	i := &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "ns",
		},
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

func withGatewayAPIClass(i *v1alpha1.Ingress) {
	withAnnotation(map[string]string{
		networking.IngressClassAnnotationKey: gatewayAPIIngressClassName,
	})(i)
}

func withAnnotation(ann map[string]string) IngressOption {
	return func(i *v1alpha1.Ingress) {
		i.Annotations = kmeta.UnionMaps(i.Annotations, ann)
	}
}

func withHTTPOption(option v1alpha1.HTTPOption) IngressOption {
	return func(i *v1alpha1.Ingress) {
		i.Spec.HTTPOption = option
	}
}
