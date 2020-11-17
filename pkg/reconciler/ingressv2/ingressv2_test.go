package ingressv2

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	httpclientset "knative.dev/net-ingressv2/pkg/client/clientset/versioned"
	ingressv2fake "knative.dev/net-ingressv2/pkg/client/clientset/versioned/fake"
	ingressv2informers "knative.dev/net-ingressv2/pkg/client/informers/externalversions"
	fakehttprouteclient "knative.dev/net-ingressv2/pkg/client/injection/client/fake"
	httplisters "knative.dev/net-ingressv2/pkg/client/listers/apis/v1alpha1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/ptr"
	servicev1alpha1 "sigs.k8s.io/service-apis/apis/v1alpha1"

	. "knative.dev/pkg/reconciler/testing"
)

var (
	ownerObj = &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-ns",
			UID:       "abcd",
		},
	}

	ownerRef = metav1.OwnerReference{
		Kind:       ownerObj.Kind,
		Name:       ownerObj.Name,
		UID:        ownerObj.UID,
		Controller: ptr.Bool(true),
	}

	origin = &servicev1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ownerObj.Name,
			Namespace:       ownerObj.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: servicev1alpha1.HTTPRouteSpec{
			Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("origin.example.com")},
		},
	}

	desired = &servicev1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ownerObj.Name,
			Namespace:       ownerObj.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: servicev1alpha1.HTTPRouteSpec{
			Hostnames: []servicev1alpha1.Hostname{servicev1alpha1.Hostname("desired.example.com")},
		},
	}
)

type FakeAccessor struct {
	client   httpclientset.Interface
	hrLister httplisters.HTTPRouteLister
}

func (f *FakeAccessor) GetHTTPRouteClient() httpclientset.Interface {
	return f.client
}

func (f *FakeAccessor) GetHTTPRouteLister() httplisters.HTTPRouteLister {
	return f.hrLister
}

func TestReconcileHTTPRoute_Create(t *testing.T) {
	ctx, _ := SetupFakeContext(t)
	ctx, cancel := context.WithCancel(ctx)

	v2Client := fakehttprouteclient.Get(ctx)

	h := NewHooks()
	h.OnCreate(&v2Client.Fake, "httproutes", func(obj runtime.Object) HookResult {
		got := obj.(*servicev1alpha1.HTTPRoute)
		if diff := cmp.Diff(got, desired); diff != "" {
			t.Log("Unexpected HTTPRoute (-want, +got):", diff)
			return HookIncomplete
		}
		return HookComplete
	})

	accessor, waitInformers := setup(ctx, []*servicev1alpha1.HTTPRoute{}, v2Client, t)
	defer func() {
		cancel()
		waitInformers()
	}()

	ReconcileHTTPRoute(ctx, ownerObj, desired, accessor)

	if err := h.WaitForHooks(3 * time.Second); err != nil {
		t.Error("Failed to Reconcile HTTPRoute:", err)
	}
}

func TestReconcileHTTPRoute_Update(t *testing.T) {
	ctx, _ := SetupFakeContext(t)
	ctx, cancel := context.WithCancel(ctx)

	v2Client := fakehttprouteclient.Get(ctx)
	accessor, waitInformers := setup(ctx, []*servicev1alpha1.HTTPRoute{origin}, v2Client, t)
	defer func() {
		cancel()
		waitInformers()
	}()

	h := NewHooks()
	h.OnUpdate(&v2Client.Fake, "httproutes", func(obj runtime.Object) HookResult {
		got := obj.(*servicev1alpha1.HTTPRoute)
		if diff := cmp.Diff(got, desired); diff != "" {
			t.Log("Unexpected HTTPRoute (-want, +got):", diff)
			return HookIncomplete
		}
		return HookComplete
	})

	ReconcileHTTPRoute(ctx, ownerObj, desired, accessor)

	if err := h.WaitForHooks(3 * time.Second); err != nil {
		t.Error("Failed to Reconcile HTTPRoute:", err)
	}
}

func setup(ctx context.Context, hrs []*servicev1alpha1.HTTPRoute,
	hrClient httpclientset.Interface, t *testing.T) (*FakeAccessor, func()) {

	fake := ingressv2fake.NewSimpleClientset()
	informer := ingressv2informers.NewSharedInformerFactory(fake, 0)
	hrInformer := informer.Networking().V1alpha1().HTTPRoutes()

	for _, hr := range hrs {
		fake.NetworkingV1alpha1().HTTPRoutes(hr.Namespace).Create(ctx, hr, metav1.CreateOptions{})
		hrInformer.Informer().GetIndexer().Add(hr)
	}

	waitInformers, err := controller.RunInformers(ctx.Done(), hrInformer.Informer())
	if err != nil {
		t.Fatal("failed to start httproute informer:", err)
	}

	return &FakeAccessor{
		client:   hrClient,
		hrLister: hrInformer.Lister(),
	}, waitInformers
}
