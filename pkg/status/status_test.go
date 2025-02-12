/*
Copyright 2019 The Knative Authors

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

package status

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/pkg/http/probe"
	"knative.dev/pkg/logging"

	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ingressNN = types.NamespacedName{
		Namespace: "default",
		Name:      "whatever",
	}
	ingTemplate = &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "whatever",
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{{
				Hosts: []string{
					"foo.bar.com",
				},
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP:       &v1alpha1.HTTPIngressRuleValue{},
			}},
		},
	}
)

func TestBackends(t *testing.T) {
	var backends Backends

	backends.AddURL("external", url.URL{Host: "www.example.com"})
	backends.AddURL("cluster", url.URL{Host: "www.example.com"})
	backends.AddURL("cluster", url.URL{Host: "www.blah.com"})

	expected := map[Visibility]URLSet{
		"external": sets.New(
			url.URL{Host: "www.example.com"},
		),
		"cluster": sets.New(
			url.URL{Host: "www.example.com"},
			url.URL{Host: "www.blah.com"},
		),
	}

	if diff := cmp.Diff(expected, backends.URLs); diff != "" {
		t.Error("unexpected diff (-want,+got): ", diff)
	}
}

func TestProbeAllHosts(t *testing.T) {
	const hostA = "foo.bar.com"
	const hostB = "ksvc.test.dev"
	var hostBEnabled atomic.Bool

	hash := "some-hash"
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

	// Failing handler returning HTTP 500 (it should never be called during probing)
	failedRequests := make(chan *http.Request)
	failHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedRequests <- r
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Actual probe handler used in Activator and Queue-Proxy
	probeHandler := probe.NewHandler(failHandler)

	// Probes to hostA always succeed and probes to hostB only succeed if hostBEnabled is true
	probeRequests := make(chan *http.Request)
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeRequests <- r
		if !strings.HasPrefix(r.Host, hostA) &&
			(!hostBEnabled.Load() || !strings.HasPrefix(r.Host, hostB)) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		r.Header.Set(header.HashKey, hash)
		probeHandler.ServeHTTP(w, r)
	})

	ts := httptest.NewServer(finalHandler)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", ts.URL, err)
	}

	hostAURL := *tsURL
	hostAURL.Host = hostA
	hostBURL := *tsURL
	hostBURL.Host = hostB

	ready := make(chan types.NamespacedName)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		fakeProbeTargetLister{
			PodIPs:  sets.New(tsURL.Hostname()),
			PodPort: tsURL.Port(),
		},
		func(ing types.NamespacedName) {
			ready <- ing
		})

	done := make(chan struct{})
	cancelled := prober.Start(done)
	defer func() {
		close(done)
		<-cancelled
	}()

	// The first call to DoProbes must succeed and return false
	backends := Backends{
		Key:     ingressNN,
		Version: hash,
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				hostAURL, hostBURL,
			),
		},
	}

	state, active := prober.IsProbeActive(ingressNN)
	if active {
		t.Error("probe as active when it should have been false")
	}
	if diff := cmp.Diff(ProbeState{}, state); diff != "" {
		t.Error("inactive probe should have empty state: ", diff)
	}

	pstate, err := prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if pstate.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	state, active = prober.IsProbeActive(ingressNN)
	if !active {
		t.Error("active probe should report active")
	}
	if diff := cmp.Diff(ProbeState{Version: hash, Ready: false}, state); diff != "" {
		t.Error("probe shouldn't be ready: ", diff)
	}

	// Wait for both hosts to be probed
	hostASeen, hostBSeen := false, false
	for req := range probeRequests {
		switch req.Host {
		case hostA:
			hostASeen = true
		case hostB:
			hostBSeen = true
		default:
			t.Fatalf("Host header = %q, want %q or %q", req.Host, hostA, hostB)
		}

		if hostASeen && hostBSeen {
			break
		}
	}

	select {
	case <-ready:
		// Since HostB doesn't return 200, the prober shouldn't be ready
		t.Fatal("Prober shouldn't be ready")
	case <-time.After(1 * time.Second):
		// Not ideal but it gives time to the prober to write to ready
		break
	}

	// Make probes to hostB succeed
	hostBEnabled.Store(true)

	// Just drain the requests in the channel to not block the handler
	go func() {
		for range probeRequests {
		}
	}()

	select {
	case <-ready:
		// Wait for the probing to eventually succeed
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	state, active = prober.IsProbeActive(ingressNN)
	if !active {
		t.Error("active probe should report active")
	}
	if diff := cmp.Diff(ProbeState{Version: hash, Ready: true}, state); diff != "" {
		t.Error("probe should ready: ", diff)
	}
}

func TestProbeLifecycle(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

	ing := ingTemplate.DeepCopy()
	hash := "some-hash"
	hostA := "foo.bar.com"
	// Simulate that the latest configuration is not applied yet by returning a different
	// hash once and then the by returning the expected hash.
	hashes := make(chan string, 1)
	hashes <- "not-the-hash-you-are-looking-for"
	go func() {
		for {
			hashes <- hash
		}
	}()

	// Failing handler returning HTTP 500 (it should never be called during probing)
	failedRequests := make(chan *http.Request)
	failHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedRequests <- r
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Actual probe handler used in Activator and Queue-Proxy
	probeHandler := probe.NewHandler(failHandler)

	// Test handler keeping track of received requests, mimicking AppendHeader of K-Network-Hash
	// and simulate a non-existing host by returning 404.
	probeRequests := make(chan *http.Request)
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, hostA) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		probeRequests <- r
		r.Header.Set(header.HashKey, <-hashes)
		probeHandler.ServeHTTP(w, r)
	})

	ts := httptest.NewServer(finalHandler)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", ts.URL, err)
	}

	hostAURL := *tsURL
	hostAURL.Host = hostA

	ready := make(chan types.NamespacedName)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		fakeProbeTargetLister{
			PodIPs:  sets.New(tsURL.Hostname()),
			PodPort: tsURL.Port(),
		},
		func(nn types.NamespacedName) {
			ready <- nn
		})

	done := make(chan struct{})
	cancelled := prober.Start(done)
	defer func() {
		close(done)
		<-cancelled
	}()

	backends := Backends{
		CallbackKey: ingressNN,
		Key:         ingressNN,
		Version:     hash,
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				hostAURL,
			),
		},
	}

	// The first call to DoProbes must succeed and return false
	state, err := prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	// Wait for the first (failing) and second (success) requests to be executed and validate Host header
	for range 2 {
		req := <-probeRequests
		if req.Host != hostA {
			t.Fatalf("Host header = %q, want %q", req.Host, hostA)
		}
	}

	select {
	case <-ready:
		// Wait for the probing to eventually succeed
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	// The subsequent calls to DoProbes must succeed and return true
	for range 5 {
		if state, err = prober.DoProbes(ctx, backends); err != nil {
			t.Fatal("DoProbes failed:", err)
		}
		if !state.Ready {
			t.Fatal("Probing should be ready")
		}
	}

	// Cancel Ingress probing -> deletes the cached state
	prober.CancelIngressProbing(ing)

	select {
	// Validate that no probe requests were issued (cached)
	case <-probeRequests:
		t.Fatal("An unexpected probe request was received")
	// Validate that no requests went through the probe handler
	case <-failedRequests:
		t.Fatal("An unexpected request went through the probe handler")
	default:
	}

	// The state has been removed and DoProbes must return False
	state, err = prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	// Wait for the first request (success) to be executed
	<-probeRequests

	select {
	case <-ready:
		// Wait for the probing to eventually succeed
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	select {
	// Validate that no requests went through the probe handler
	case <-failedRequests:
		t.Fatal("An unexpected request went through the probe handler")
	default:
		break
	}
}

func TestProbeListerFail(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

	ready := make(chan types.NamespacedName)
	defer close(ready)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		notFoundLister{},
		func(ing types.NamespacedName) {
			ready <- ing
		})

	backends := Backends{
		Key:     ingressNN,
		Version: "some-hash",
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				url.URL{Scheme: "http", Host: "foo.bar.com"},
			),
		},
	}

	// If we can't list, this  must fail and return false
	state, err := prober.DoProbes(ctx, backends)
	if err == nil {
		t.Fatal("DoProbes returned unexpected success")
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}
}

func TestCancelPodProbing(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

	type timedRequest struct {
		*http.Request
		Time time.Time
	}

	// Handler keeping track of received requests and mimicking an Ingress not ready
	requests := make(chan *timedRequest, 100)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- &timedRequest{
			Time:    time.Now(),
			Request: r.Clone(context.Background()),
		}
		w.WriteHeader(http.StatusNotFound)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", ts.URL, err)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "gateway",
		},
		Status: v1.PodStatus{
			PodIP: tsURL.Hostname(),
		},
	}

	ready := make(chan types.NamespacedName)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		fakeProbeTargetLister{
			PodIPs:  sets.New(tsURL.Hostname()),
			PodPort: tsURL.Port(),
		},
		func(ing types.NamespacedName) {
			ready <- ing
		})

	done := make(chan struct{})
	cancelled := prober.Start(done)
	defer func() {
		close(done)
		<-cancelled
	}()

	backends := Backends{
		Key:     ingressNN,
		Version: "some-hash",
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				url.URL{Scheme: "http", Host: "foo.bar.com"},
			),
		},
	}
	state, err := prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	select {
	case <-requests:
		// Wait for the first probe request
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	// Create a new version of the Ingress (to replace the original Ingress)
	const otherDomain = "blabla.net"
	backends = Backends{
		Key:     ingressNN,
		Version: "a-new-hash",
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				url.URL{Scheme: "http", Host: otherDomain},
			),
		},
	}

	// Create a different Ingress (to be probed in parallel)
	const parallelDomain = "parallel.net"
	func() {
		parallelNN := ingressNN
		parallelNN.Name = "something"
		parallelBackends := Backends{
			Key:     parallelNN,
			Version: "another-hash",
			URLs: map[v1alpha1.IngressVisibility]URLSet{
				v1alpha1.IngressVisibilityExternalIP: sets.New(
					url.URL{Scheme: "http", Host: parallelDomain},
				),
			},
		}

		state, err := prober.DoProbes(ctx, parallelBackends)
		if err != nil {
			t.Fatal("DoProbes failed:", err)
		}
		if state.Ready {
			t.Fatal("Probing returned ready but should be false")
		}
	}()

	// Check that probing is unsuccessful
	select {
	case <-ready:
		t.Fatal("Probing succeeded while it should not have succeeded")
	default:
	}

	state, err = prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	// Drain requests for the old version
	start := time.Now()
	for req := range requests {
		if strings.HasPrefix(req.Host, otherDomain) {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("haven't received requests from", otherDomain, "after 5 seconds")
		}
	}

	// Cancel Pod probing
	prober.CancelPodProbing(pod)
	cancelTime := time.Now()

	// Check that there are no requests for the old Ingress and the requests predate cancellation
	for {
		select {
		case req := <-requests:
			if !strings.HasPrefix(req.Host, otherDomain) &&
				!strings.HasPrefix(req.Host, parallelDomain) {
				t.Fatalf("Host = %s, want: %s or %s", req.Host, otherDomain, parallelDomain)
			} else if req.Time.Sub(cancelTime) > 0 {
				t.Fatal("Request was made after cancellation")
			}
		default:
			return
		}
	}
}

func TestPartialPodCancellation(t *testing.T) {
	hash := "some-hash"
	hostA := "foo.bar.com"
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

	// Simulate a probe target returning HTTP 200 OK and the correct hash
	requests := make(chan *http.Request, 100)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.Header().Set(header.HashKey, hash)
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", ts.URL, err)
	}
	hostAURL := *tsURL
	hostAURL.Host = hostA

	// pods[0] will be probed successfully, pods[1] will never be probed successfully
	pods := []*v1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "pod0",
		},
		Status: v1.PodStatus{
			PodIP: tsURL.Hostname(),
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "pod1",
		},
		Status: v1.PodStatus{
			PodIP: "198.51.100.1",
		},
	}}

	ready := make(chan types.NamespacedName)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		fakeProbeTargetLister{
			PodIPs:  sets.New(pods[0].Status.PodIP, pods[1].Status.PodIP),
			PodPort: tsURL.Port(),
		},
		func(ing types.NamespacedName) {
			ready <- ing
		})

	done := make(chan struct{})
	cancelled := prober.Start(done)
	defer func() {
		close(done)
		<-cancelled
	}()

	backends := Backends{
		Key:     ingressNN,
		Version: hash,
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				hostAURL,
			),
		},
	}
	state, err := prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	select {
	case <-requests:
		// Wait for the first probe request
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	// Check that probing is unsuccessful
	select {
	case <-ready:
		t.Fatal("Probing succeeded while it should not have succeeded")
	default:
	}

	// Cancel probing of pods[1]
	prober.CancelPodProbing(pods[1])

	// Check that probing was successful
	select {
	case <-ready:
		break
	case <-time.After(5 * time.Second):
		t.Fatal("Probing was not successful even after waiting")
	}
}

func TestCancelIngressProbing(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())
	// Handler keeping track of received requests and mimicking an Ingress not ready
	requests := make(chan *http.Request, 100)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.WriteHeader(http.StatusNotFound)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse URL %q: %v", ts.URL, err)
	}

	ready := make(chan types.NamespacedName)
	prober := NewProber(
		zaptest.NewLogger(t).Sugar(),
		fakeProbeTargetLister{
			PodIPs:  sets.New(tsURL.Hostname()),
			PodPort: tsURL.Port(),
		},
		func(ing types.NamespacedName) {
			ready <- ing
		})

	done := make(chan struct{})
	cancelled := prober.Start(done)
	defer func() {
		close(done)
		<-cancelled
	}()

	backends := Backends{
		Key:     ingressNN,
		Version: "some-hash",
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				url.URL{Scheme: "http", Host: "foo.bar.com"},
			),
		},
	}
	state, err := prober.DoProbes(ctx, backends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	select {
	case <-requests:
		// Wait for the first probe request
	case <-time.After(5 * time.Second):
		t.Error("Timed out waiting for probing to succeed.")
	}

	const domain = "blabla.net"

	newBackends := Backends{
		Key:     ingressNN,
		Version: "second-hash",
		URLs: map[v1alpha1.IngressVisibility]URLSet{
			v1alpha1.IngressVisibilityExternalIP: sets.New(
				url.URL{Scheme: "http", Host: domain},
			),
		},
	}

	// Check that probing is unsuccessful
	select {
	case <-ready:
		t.Fatal("Probing succeeded while it should not have succeeded")
	default:
	}

	state, err = prober.DoProbes(ctx, newBackends)
	if err != nil {
		t.Fatal("DoProbes failed:", err)
	}
	if state.Ready {
		t.Fatal("Probing returned ready but should be false")
	}

	// Drain requests for the old version.
	for req := range requests {
		t.Log("req.Host:", req.Host)
		if strings.HasPrefix(req.Host, domain) {
			break
		}
	}

	// Cancel Ingress probing.
	prober.CancelIngressProbing(ingTemplate)

	// Check that the requests were for the new version.
	close(requests)
	for req := range requests {
		if !strings.HasPrefix(req.Host, domain) {
			t.Fatalf("Host = %s, want: %s", req.Host, domain)
		}
	}
}

func TestProbeVerifier(t *testing.T) {
	const hash = "Hi! I am hash!"
	prober := NewProber(zaptest.NewLogger(t).Sugar(), nil, nil)
	verifier := prober.probeVerifier(&workItem{
		routeState: &routeState{
			version: hash,
		},
		podState: nil,
		context:  nil,
		url:      nil,
		podIP:    "",
		podPort:  "",
		logger:   zaptest.NewLogger(t).Sugar(),
	})
	cases := []struct {
		name string
		resp *http.Response
		want bool
	}{{
		name: "HTTP 200 matching hash",
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{header.HashKey: []string{hash}},
		},
		want: true,
	}, {
		name: "HTTP 200 mismatching hash",
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{header.HashKey: []string{"nope"}},
		},
		want: false,
	}, {
		name: "HTTP 200 missing header",
		resp: &http.Response{
			StatusCode: http.StatusOK,
		},
		want: true,
	}, {
		name: "HTTP 404",
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
		},
		want: false,
	}, {
		name: "HTTP 503",
		resp: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
		},
		want: false,
	}, {
		name: "HTTP 403",
		resp: &http.Response{
			StatusCode: http.StatusForbidden,
		},
		want: true,
	}, {
		name: "HTTP 503",
		resp: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
		},
		want: false,
	}, {
		name: "HTTP 301",
		resp: &http.Response{
			StatusCode: http.StatusMovedPermanently,
		},
		want: true,
	}, {
		name: "HTTP 302",
		resp: &http.Response{
			StatusCode: http.StatusFound,
		},
		want: true,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := verifier(c.resp, nil)
			if got != c.want {
				t.Errorf("got: %v, want: %v", got, c.want)
			}
		})
	}
}

type fakeProbeTargetLister struct {
	PodIPs  sets.Set[string]
	PodPort string
}

func (l fakeProbeTargetLister) BackendsToProbeTargets(_ context.Context, backends Backends) ([]ProbeTarget, error) {
	targets := []ProbeTarget{}

	for _, urls := range backends.URLs {
		newTarget := ProbeTarget{
			PodIPs:  l.PodIPs,
			PodPort: l.PodPort,
		}

		for url := range urls {
			newTarget.URLs = append(newTarget.URLs, &url)
		}
		targets = append(targets, newTarget)
	}
	return targets, nil
}

type notFoundLister struct{}

func (l notFoundLister) BackendsToProbeTargets(context.Context, Backends) ([]ProbeTarget, error) {
	return nil, errors.New("not found")
}
