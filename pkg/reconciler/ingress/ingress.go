/*
Copyright 2020 The Knative Authors

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

	"go.uber.org/zap"
	v2clientset "knative.dev/net-ingressv2/pkg/client/clientset/versioned"
	v2listers "knative.dev/net-ingressv2/pkg/client/listers/apis/v1alpha1"
	"knative.dev/net-ingressv2/pkg/reconciler/ingress/resources"
	"knative.dev/net-ingressv2/pkg/reconciler/ingressv2"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/pkg/logging"
	pkgnet "knative.dev/pkg/network"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"
)

const (
	//	httpRoutesNotReconciled = "ReconcileHTTPRoutesFailed"
	notReconciledReason  = "ReconcileIngressFailed"
	notReconciledMessage = "Ingress reconciliation failed"
)

// TODO: Istio cannot choose gateway now. See - https://github.com/istio/istio/issues/29078
const (
	istioGatewayName      = "istio-ingressgateway"
	istioGatewayNamespace = "istio-system"
)

// Reconciler implements addressableservicereconciler.Interface for
// AddressableService resources.
type Reconciler struct {
	// Tracker builds an index of what resources are watching other resources
	// so that we can immediately react to changes tracked resources.
	Tracker tracker.Interface

	v2ClientSet v2clientset.Interface
	httpLister  v2listers.HTTPRouteLister
}

var (
	_ ingressreconciler.Interface = (*Reconciler)(nil)
	_ ingressv2.HTTPRouteAccessor = (*Reconciler)(nil)
)

// GetHTTPRouteClient returns the client to access service-apis resources.
func (r *Reconciler) GetHTTPRouteClient() v2clientset.Interface {
	return r.v2ClientSet
}

// GetHTTPRouteLister returns the lister for HTTPRoute.
func (r *Reconciler) GetHTTPRouteLister() v2listers.HTTPRouteLister {
	return r.httpLister
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, ingress *v1alpha1.Ingress) reconciler.Event {
	logger := logging.FromContext(ctx)

	reconcileErr := r.reconcileIngress(ctx, ingress)
	if reconcileErr != nil {
		logger.Errorw("Failed to reconcile Ingress: ", zap.Error(reconcileErr))
		ingress.Status.MarkIngressNotReady(notReconciledReason, notReconciledMessage)
		return reconcileErr
	}

	return nil
}

func (r *Reconciler) reconcileIngress(ctx context.Context, ing *v1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	ing.SetDefaults(ctx)

	ing.Status.InitializeConditions()
	logger.Infof("Reconciling ingress: %#v", ing)

	httpRoutes := resources.MakeHTTPRoutes(ing)
	if err := r.reconcileHTTPRoutes(ctx, ing, httpRoutes); err != nil {
		return err
	}

	ing.Status.MarkNetworkConfigured()

	publicLbs := []v1alpha1.LoadBalancerIngressStatus{
		{DomainInternal: pkgnet.GetServiceHostname(istioGatewayName, istioGatewayNamespace)},
	}
	privateLbs := []v1alpha1.LoadBalancerIngressStatus{
		{DomainInternal: pkgnet.GetServiceHostname(istioGatewayName, istioGatewayNamespace)},
	}

	ing.Status.MarkLoadBalancerReady(publicLbs, privateLbs)

	return nil
}

func (r *Reconciler) reconcileHTTPRoutes(ctx context.Context, ing *v1alpha1.Ingress, desired []*servicev1alpha1.HTTPRoute) error {
	logger := logging.FromContext(ctx)

	for _, d := range desired {
		if d.GetAnnotations()[networking.IngressClassAnnotationKey] != resources.V2IngressClassName {
			// We do not create resources that do not have ingressv2 ingress class annotation.
			// As a result, obsoleted resources will be cleaned up.
			continue
		}
		logger.Info("Creating/Updating HTTPRoutes")
		if _, err := ingressv2.ReconcileHTTPRoute(ctx, ing, d, r); err != nil {
			if ingressv2.IsNotOwned(err) {
				ing.Status.MarkResourceNotOwned("HTTPRoute", d.Name)
			}
			return err
		}
	}
	return nil
}
