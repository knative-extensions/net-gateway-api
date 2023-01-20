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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	networking "knative.dev/networking/pkg/apis/networking/v1alpha1"
	fakeservingclientset "knative.dev/networking/pkg/client/clientset/versioned/fake"
	networkinglisters "knative.dev/networking/pkg/client/listers/networking/v1alpha1"
	"knative.dev/pkg/reconciler/testing"

	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	fakegatewayapiclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
	gatewaylisters "sigs.k8s.io/gateway-api/pkg/client/listers/apis/v1beta1"
)

var clientSetSchemes = []func(*runtime.Scheme) error{
	fakeservingclientset.AddToScheme,
	fakegatewayapiclientset.AddToScheme,
	fakekubeclientset.AddToScheme,
}

type Listers struct {
	sorter testing.ObjectSorter
}

func NewListers(objs []runtime.Object) Listers {
	scheme := NewScheme()

	ls := Listers{
		sorter: testing.NewObjectSorter(scheme),
	}

	ls.sorter.AddObjects(objs...)

	return ls
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	for _, addTo := range clientSetSchemes {
		addTo(scheme)
	}
	return scheme
}

func (*Listers) NewScheme() *runtime.Scheme {
	return NewScheme()
}

// IndexerFor returns the indexer for the given object.
func (l *Listers) IndexerFor(obj runtime.Object) cache.Indexer {
	return l.sorter.IndexerForObjectType(obj)
}

func (l *Listers) GetNetworkingObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakeservingclientset.AddToScheme)
}

func (l *Listers) GetKubeObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakekubeclientset.AddToScheme)
}

func (l *Listers) GetGatewayAPIObjects() []runtime.Object {
	return l.sorter.ObjectsForSchemeFunc(fakegatewayapiclientset.AddToScheme)
}

// GetIngressLister get lister for Ingress resource.
func (l *Listers) GetIngressLister() networkinglisters.IngressLister {
	return networkinglisters.NewIngressLister(l.IndexerFor(&networking.Ingress{}))
}

// GetHTTPRouteLister get lister for HTTPProxy resource.
func (l *Listers) GetHTTPRouteLister() gatewaylisters.HTTPRouteLister {
	return gatewaylisters.NewHTTPRouteLister(l.IndexerFor(&gatewayv1beta1.HTTPRoute{}))
}

// GetEndpointsLister get lister for K8s Endpoints resource.
func (l *Listers) GetEndpointsLister() corev1listers.EndpointsLister {
	return corev1listers.NewEndpointsLister(l.IndexerFor(&corev1.Endpoints{}))
}

func (l *Listers) GetGatewayLister() gatewaylisters.GatewayLister {
	return gatewaylisters.NewGatewayLister(l.IndexerFor(&gatewayv1beta1.Gateway{}))
}

func (l *Listers) GetReferenceGrantLister() gatewaylisters.ReferenceGrantLister {
	return gatewaylisters.NewReferenceGrantLister(l.IndexerFor(&gatewayv1beta1.ReferenceGrant{}))
}
