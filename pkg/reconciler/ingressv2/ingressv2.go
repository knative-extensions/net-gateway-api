package ingressv2

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	httpclientset "knative.dev/net-ingressv2/pkg/client/clientset/versioned"
	httplisters "knative.dev/net-ingressv2/pkg/client/listers/apis/v1alpha1"
	ingressv1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/controller"
	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"
)

// HTTPRouteAccessor is an interface for accessing HTTPRoute.
type HTTPRouteAccessor interface {
	GetHTTPRouteClient() httpclientset.Interface
	GetHTTPRouteLister() httplisters.HTTPRouteLister
}

func hasDesiredDiff(current, desired *servicev1alpha1.HTTPRoute) bool {
	return !equality.Semantic.DeepEqual(current.Spec, desired.Spec) ||
		!equality.Semantic.DeepEqual(current.Labels, desired.Labels) ||
		!equality.Semantic.DeepEqual(current.Annotations, desired.Annotations)
}

// ReconcileHTTPRoute reconciles HTTPRoute to the desired status.
func ReconcileHTTPRoute(ctx context.Context, owner *ingressv1.Ingress, desired *servicev1alpha1.HTTPRoute,
	httpAccessor HTTPRouteAccessor) (*servicev1alpha1.HTTPRoute, error) {

	recorder := controller.GetEventRecorder(ctx)
	if recorder == nil {
		return nil, fmt.Errorf("recoder for reconciling HTTPRoute %s/%s is not created", desired.Namespace, desired.Name)
	}
	ns := desired.Namespace
	name := desired.Name
	hr, err := httpAccessor.GetHTTPRouteLister().HTTPRoutes(ns).Get(name)
	if apierrs.IsNotFound(err) {
		hr, err = httpAccessor.GetHTTPRouteClient().NetworkingV1alpha1().HTTPRoutes(ns).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(owner, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create HTTPRoute %s/%s: %v", ns, name, err)
			return nil, fmt.Errorf("failed to create HTTPRoute: %w", err)
		}
		recorder.Eventf(owner, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", desired.Name)
	} else if err != nil {
		return nil, err
	} else if !metav1.IsControlledBy(hr, owner) {
		// Return an error with NotControlledBy information.
		return nil, NewIngressv2Error(
			fmt.Errorf("owner: %s with Type %T does not own HTTPRoute: %q", owner.GetName(), owner, name),
			NotOwnResource)
	} else if hasDesiredDiff(hr, desired) {
		// Don't modify the informers copy
		existing := hr.DeepCopy()
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		hr, err = httpAccessor.GetHTTPRouteClient().NetworkingV1alpha1().HTTPRoutes(ns).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update HTTPRoute %s/%s: %w", ns, existing.Name, err)
		}
		recorder.Eventf(owner, corev1.EventTypeNormal, "Updated", "Updated HTTPRoute %s/%s", ns, name)
	}
	return hr, nil
}
