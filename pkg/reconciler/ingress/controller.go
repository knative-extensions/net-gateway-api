/*
  pyright 2019 The Knative Authors

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

	"k8s.io/client-go/tools/cache"
	v2client "knative.dev/net-ingressv2/pkg/client/injection/client"
	httprouteinformer "knative.dev/net-ingressv2/pkg/client/injection/informers/apis/v1alpha1/httproute"
	"knative.dev/net-ingressv2/pkg/reconciler/ingress/resources"
	"knative.dev/networking/pkg/apis/networking"
	ingressinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	httprouteInformer := httprouteinformer.Get(ctx)

	c := &Reconciler{
		//	IngressLister: ingressInformer.Lister(),
		httpLister:  httprouteInformer.Lister(),
		v2ClientSet: v2client.Get(ctx),
	}

	filterFunc := reconciler.AnnotationFilterFunc(networking.IngressClassAnnotationKey, resources.V2IngressClassName, true)

	impl := ingressreconciler.NewImpl(ctx, c, resources.V2IngressClassName, func(impl *controller.Impl) controller.Options {
		return controller.Options{}
	})

	logger.Info("Setting up Ingress event handlers")
	ingressHandler := cache.FilteringResourceEventHandler{
		FilterFunc: filterFunc,
		Handler:    controller.HandleAll(impl.Enqueue),
	}
	ingressInformer.Informer().AddEventHandler(ingressHandler)

	httprouteInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: filterFunc,
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	})

	tracker := tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))
	c.Tracker = tracker

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// Cancel probing when a Ingress is deleted
		DeleteFunc: combineFunc(
			tracker.OnDeletedObserver,
		),
	})
	return impl
}

// TODO: not necessary?
func combineFunc(functions ...func(interface{})) func(interface{}) {
	return func(obj interface{}) {
		for _, f := range functions {
			f(obj)
		}
	}
}
