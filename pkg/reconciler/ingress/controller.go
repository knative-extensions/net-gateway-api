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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	networkcfg "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/status"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"

	gwapiclient "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/client"
	gatewayinformer "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/informers/apis/v1alpha2/gateway"
	httprouteinformer "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/informers/apis/v1alpha2/httproute"
	referencegrantinformer "knative.dev/net-gateway-api/pkg/client/gatewayapi/injection/informers/apis/v1alpha2/referencegrant"
	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

const (
	// gatewayAPIIngressClassName is the class name to reconcile.
	gatewayAPIIngressClassName = "gateway-api.ingress.networking.knative.dev"
)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	httprouteInformer := httprouteinformer.Get(ctx)
	referenceGrantInformer := referencegrantinformer.Get(ctx)
	gatewayInformer := gatewayinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)

	c := &Reconciler{
		gwapiclient:          gwapiclient.Get(ctx),
		httprouteLister:      httprouteInformer.Lister(),
		referenceGrantLister: referenceGrantInformer.Lister(),
		gatewayLister:        gatewayInformer.Lister(),
	}

	filterFunc := reconciler.AnnotationFilterFunc(networking.IngressClassAnnotationKey, gatewayAPIIngressClassName, false)

	impl := ingressreconciler.NewImpl(ctx, c, gatewayAPIIngressClassName, func(impl *controller.Impl) controller.Options {
		configsToResync := []interface{}{
			&networkcfg.Config{},
			&config.Gateway{},
		}
		resync := configmap.TypeFilter(configsToResync...)(func(string, interface{}) {
			impl.GlobalResync(ingressInformer.Informer())
		})
		configStore := config.NewStore(logging.WithLogger(ctx, logger.Named("config-store")), resync)
		configStore.WatchConfigs(cmw)
		return controller.Options{
			ConfigStore:       configStore,
			PromoteFilterFunc: filterFunc,
		}
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
	gatewayInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: filterFunc,
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	})

	statusProber := status.NewProber(
		logger.Named("status-manager"),
		NewProbeTargetLister(logger, endpointsInformer.Lister()),
		func(ing *v1alpha1.Ingress) {
			logger.Debugf("Ready callback triggered for ingress: %s/%s", ing.Namespace, ing.Name)
			impl.EnqueueKey(types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name})
		})
	c.statusManager = statusProber
	// TODO: Bring up gateway-api community to discuss about probing.
	// related to https://github.com/knative-sandbox/net-gateway-api/issues/18
	statusProber.Start(ctx.Done())

	// Make sure trackers are deleted once the observers are removed.
	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: impl.Tracker.OnDeletedObserver,
	})

	return impl
}
