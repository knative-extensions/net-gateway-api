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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"

	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	nethttp "knative.dev/networking/pkg/http"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/pkg/prober"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
)

const (
	// probeConcurrency defines how many probing calls can be issued simultaneously
	probeConcurrency = 15
	// probeTimeout defines the maximum amount of time a request will wait
	probeTimeout = 1 * time.Second
	// initialDelay defines the delay before enqueuing a probing request the first time.
	// It gives times for the change to propagate and prevents unnecessary retries.
	initialDelay = 200 * time.Millisecond
)

var dialContext = (&net.Dialer{Timeout: probeTimeout}).DialContext

// ingressState represents the probing state of an Ingress
type routeState struct {
	version     string
	key         types.NamespacedName
	callbackKey types.NamespacedName

	// pendingCount is the number of pods that haven't been successfully probed yet
	pendingCount atomic.Int64
	lastAccessed time.Time

	cancel func()
}

// podState represents the probing state of a Pod (for a specific Ingress)
type podState struct {
	// pendingCount is the number of probes for the Pod
	pendingCount atomic.Int64

	cancel func()
}

// cancelContext is a pair of a Context and its cancel function
type cancelContext struct {
	context context.Context
	cancel  func()
}

type workItem struct {
	routeState *routeState
	podState   *podState
	context    context.Context
	url        *url.URL
	podIP      string
	podPort    string
	logger     *zap.SugaredLogger
}

// ProbeTarget contains the URLs to probes for a set of Pod IPs serving out of the same port.
type ProbeTarget struct {
	PodIPs  sets.Set[string]
	PodPort string
	Port    string
	URLs    []*url.URL
}

type ProbeState struct {
	Version string
	Ready   bool
}

type Backends struct {
	CallbackKey types.NamespacedName
	Key         types.NamespacedName
	Version     string
	URLs        map[Visibility]URLSet
	HTTPOption  v1alpha1.HTTPOption
}

func (b *Backends) AddURL(v Visibility, u url.URL) {
	if b.URLs == nil {
		b.URLs = make(map[Visibility]URLSet)
	}
	urls, ok := b.URLs[v]
	if !ok {
		urls = make(URLSet)
		b.URLs[v] = urls
	}
	urls.Insert(u)
}

type (
	Visibility = v1alpha1.IngressVisibility
	URLSet     = sets.Set[url.URL]
)

// ProbeTargetLister lists all the targets that requires probing.
type ProbeTargetLister interface {
	// BackendsToProbeTargets produces list of targets for the given backends
	BackendsToProbeTargets(ctx context.Context, backends Backends) ([]ProbeTarget, error)
}

// Manager provides a way to check if an Ingress is ready
type Manager interface {
	DoProbes(ctx context.Context, backends Backends) (ProbeState, error)
	IsProbeActive(key types.NamespacedName) (ProbeState, bool)
}

// Prober provides a way to check if a VirtualService is ready by probing the Envoy pods
// handling that VirtualService.
type Prober struct {
	logger *zap.SugaredLogger

	// mu guards routeStates and podContexts
	mu          sync.RWMutex
	routeStates map[types.NamespacedName]*routeState
	podContexts map[string]cancelContext

	workQueue workqueue.TypedRateLimitingInterface[any]

	targetLister ProbeTargetLister

	readyCallback func(types.NamespacedName)

	probeConcurrency int
}

// NewProber creates a new instance of Prober
func NewProber(
	logger *zap.SugaredLogger,
	targetLister ProbeTargetLister,
	readyCallback func(types.NamespacedName),
) *Prober {
	return &Prober{
		logger:      logger,
		routeStates: make(map[types.NamespacedName]*routeState),
		podContexts: make(map[string]cancelContext),
		workQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.NewTypedMaxOfRateLimiter(
				// Per item exponential backoff
				workqueue.NewTypedItemExponentialFailureRateLimiter[any](50*time.Millisecond, 30*time.Second),
				// Global rate limiter
				&workqueue.TypedBucketRateLimiter[any]{Limiter: rate.NewLimiter(rate.Limit(50), 100)},
			),
			workqueue.TypedRateLimitingQueueConfig[any]{Name: "ProbingQueue"}),
		targetLister:     targetLister,
		readyCallback:    readyCallback,
		probeConcurrency: probeConcurrency,
	}
}

// IsProbeActive will return the state of the probes for the given key
func (m *Prober) IsProbeActive(key types.NamespacedName) (ProbeState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ingState, ok := m.routeStates[key]; ok {
		return ProbeState{Version: ingState.version, Ready: ingState.pendingCount.Load() == 0}, true
	}
	return ProbeState{}, false
}

// DoProbes will start probing the desired backends. If probing is already active with the
// correct backend versions it will return the current state.
func (m *Prober) DoProbes(ctx context.Context, backends Backends) (ProbeState, error) {
	if state, ok := func() (ProbeState, bool) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if ingState, ok := m.routeStates[backends.Key]; ok {
			pstate := ProbeState{Version: ingState.version}
			if ingState.version == backends.Version {
				ingState.lastAccessed = time.Now()
				pstate.Ready = ingState.pendingCount.Load() == 0
				return pstate, true
			}

			// Cancel the polling for the outdated version
			ingState.cancel()
			delete(m.routeStates, backends.Key)
		}
		return ProbeState{}, false
	}(); ok {
		return state, nil
	}

	targets, err := m.targetLister.BackendsToProbeTargets(ctx, backends)
	if err != nil {
		return ProbeState{}, err
	}

	logger := logging.FromContext(ctx)
	ready := m.probeRequest(logger,
		backends.Version,
		backends.Key,
		backends.CallbackKey,
		targets,
	)

	return ProbeState{
		Version: backends.Version,
		Ready:   ready,
	}, nil
}

func (m *Prober) probeRequest(
	logger *zap.SugaredLogger,
	version string,
	key types.NamespacedName,
	callbackKey types.NamespacedName,
	targets []ProbeTarget,
) bool {
	ingCtx, cancel := context.WithCancel(context.Background())
	routeState := &routeState{
		version:      version,
		key:          key,
		callbackKey:  callbackKey,
		lastAccessed: time.Now(),
		cancel:       cancel,
	}

	workItems := make(map[string][]*workItem)
	for _, target := range targets {
		for ip := range target.PodIPs {
			for _, url := range target.URLs {
				workItems[ip] = append(workItems[ip], &workItem{
					routeState: routeState,
					url:        url,
					podIP:      ip,
					podPort:    target.PodPort,
					logger:     logger,
				})
			}
		}
	}

	routeState.pendingCount.Store(int64(len(workItems)))

	for ip, ipWorkItems := range workItems {
		// Get or create the context for that IP
		ipCtx := func() context.Context {
			m.mu.Lock()
			defer m.mu.Unlock()
			cancelCtx, ok := m.podContexts[ip]
			if !ok {
				ctx, cancel := context.WithCancel(context.Background())
				cancelCtx = cancelContext{
					context: ctx,
					cancel:  cancel,
				}
				m.podContexts[ip] = cancelCtx
			}
			return cancelCtx.context
		}()

		podCtx, cancel := context.WithCancel(ingCtx)
		podState := &podState{
			cancel: cancel,
		}
		podState.pendingCount.Store(int64(len(ipWorkItems)))

		// Quick and dirty way to join two contexts (i.e. podCtx is cancelled when either ingCtx or ipCtx are cancelled)
		go func() {
			select {
			case <-podCtx.Done():
				// This is the actual context, there is nothing to do except
				// break to avoid leaking this goroutine.
				break
			case <-ipCtx.Done():
				// Cancel podCtx
				cancel()
			}
		}()

		// Update the states when probing is cancelled
		go func() {
			<-podCtx.Done()
			m.onProbingCancellation(routeState, podState)
		}()

		for _, wi := range ipWorkItems {
			wi.podState = podState
			wi.context = podCtx //nolint:fatcontext
			m.workQueue.AddAfter(wi, initialDelay)
			logger.Infof("Queuing probe for %s, IP: %s:%s (version: %s)(depth: %d)",
				wi.url, wi.podIP, wi.podPort, wi.routeState.version, m.workQueue.Len())
		}
	}

	func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.routeStates[key] = routeState
	}()
	return len(workItems) == 0
}

// Start starts the Manager background operations
func (m *Prober) Start(done <-chan struct{}) chan struct{} {
	var wg sync.WaitGroup

	// Start the worker goroutines
	for range m.probeConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for m.processWorkItem() {
			}
		}()
	}

	// Stop processing the queue when cancelled
	go func() {
		<-done
		m.workQueue.ShutDown()
	}()

	// Return a channel closed when all work is done
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

// CancelIngressProbing cancels probing of the provided Ingress
func (m *Prober) CancelIngressProbing(obj interface{}) {
	acc, err := kmeta.DeletionHandlingAccessor(obj)
	if err != nil {
		return
	}

	key := types.NamespacedName{Namespace: acc.GetNamespace(), Name: acc.GetName()}
	m.CancelIngressProbingByKey(key)
}

// CancelIngressProbingByKey cancels probing of the Ingress identified by the provided key.
func (m *Prober) CancelIngressProbingByKey(key types.NamespacedName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.routeStates {
		if v.callbackKey == key {
			v.cancel()
			delete(m.routeStates, key)
		}
	}
}

// CancelPodProbing cancels probing of the provided Pod IP.
//
// TODO(#6269): make this cancellation based on Pod x port instead of just Pod.
func (m *Prober) CancelPodProbing(obj interface{}) {
	if pod, ok := obj.(*corev1.Pod); ok {
		m.mu.Lock()
		defer m.mu.Unlock()

		if ctx, ok := m.podContexts[pod.Status.PodIP]; ok {
			ctx.cancel()
			delete(m.podContexts, pod.Status.PodIP)
		}
	}
}

// processWorkItem processes a single work item from workQueue.
// It returns false when there is no more items to process, true otherwise.
func (m *Prober) processWorkItem() bool {
	obj, shutdown := m.workQueue.Get()
	if shutdown {
		return false
	}

	defer m.workQueue.Done(obj)

	// Crash if the item is not of the expected type
	item, ok := obj.(*workItem)
	if !ok {
		m.logger.Fatalf("Unexpected work item type: want: %s, got: %s\n",
			reflect.TypeOf(&workItem{}).Name(), reflect.TypeOf(obj).Name())
	}
	item.logger.Infof("Processing probe for %s, IP: %s:%s (depth: %d)",
		item.url, item.podIP, item.podPort, m.workQueue.Len())

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		//nolint:gosec
		// We only want to know that the Gateway is configured, not that the configuration is valid.
		// Therefore, we can safely ignore any TLS certificate validation.
		InsecureSkipVerify: true,
	}
	transport.DialContext = func(ctx context.Context, network, _ string) (conn net.Conn, e error) {
		// Requests with the IP as hostname and the Host header set do no pass client-side validation
		// because the HTTP client validates that the hostname (not the Host header) matches the server
		// TLS certificate Common Name or Alternative Names. Therefore, http.Request.URL is set to the
		// hostname and it is substituted it here with the target IP.
		return dialContext(ctx, network, net.JoinHostPort(item.podIP, item.podPort))
	}

	probeURL := deepCopy(item.url)

	if probeURL.Path == "" {
		probeURL.Path = nethttp.HealthCheckPath
	}

	ctx, cancel := context.WithTimeout(item.context, probeTimeout)
	defer cancel()
	ok, err := prober.Do(
		ctx,
		transport,
		probeURL.String(),
		prober.WithHeader(header.UserAgentKey, header.IngressReadinessUserAgent),
		prober.WithHeader(header.ProbeKey, header.ProbeValue),
		prober.WithHeader(header.HashKey, header.HashValueOverride),
		m.probeVerifier(item))

	// In case of cancellation, drop the work item
	select {
	case <-item.context.Done():
		m.workQueue.Forget(obj)
		return true
	default:
	}

	if err != nil || !ok {
		// In case of error, enqueue for retry
		m.workQueue.AddRateLimited(obj)
		item.logger.Errorf("Probing of %s failed, IP: %s:%s, ready: %t, error: %v (depth: %d)",
			item.url, item.podIP, item.podPort, ok, err, m.workQueue.Len())
	} else {
		m.onProbingSuccess(item.routeState, item.podState)
	}
	return true
}

func (m *Prober) onProbingSuccess(routeState *routeState, podState *podState) {
	// The last probe call for the Pod succeeded, the Pod is ready
	if podState.pendingCount.Add(-1) == 0 {
		// Unlock the goroutine blocked on <-podCtx.Done()
		podState.cancel()

		// This is the last pod being successfully probed, the Ingress is ready
		if routeState.pendingCount.Add(-1) == 0 {
			m.readyCallback(routeState.callbackKey)
		}
	}
}

func (m *Prober) onProbingCancellation(routeState *routeState, podState *podState) {
	for {
		pendingCount := podState.pendingCount.Load()
		if pendingCount <= 0 {
			// Probing succeeded, nothing to do
			return
		}

		// Attempt to set pendingCount to 0.
		if podState.pendingCount.CompareAndSwap(pendingCount, 0) {
			// This is the last pod being successfully probed, the Ingress is ready
			if routeState.pendingCount.Add(-1) == 0 {
				m.readyCallback(routeState.callbackKey)
			}
			return
		}
	}
}

func (m *Prober) probeVerifier(item *workItem) prober.Verifier {
	return func(r *http.Response, _ []byte) (bool, error) {
		// In the happy path, the probe request is forwarded to Activator or Queue-Proxy and the response (HTTP 200)
		// contains the "K-Network-Hash" header that can be compared with the expected hash. If the hashes match,
		// probing is successful, if they don't match, a new probe will be sent later.
		// An HTTP 404/503 is expected in the case of the creation of a new Knative service because the rules will
		// not be present in the Envoy config until the new VirtualService is applied.
		// No information can be extracted from any other scenario (e.g. HTTP 302), therefore in that case,
		// probing is assumed to be successful because it is better to say that an Ingress is Ready before it
		// actually is Ready than never marking it as Ready. It is best effort.
		switch r.StatusCode {
		case http.StatusOK:
			hash := r.Header.Get(header.HashKey)
			switch hash {
			case "":
				item.logger.Errorf("Probing of %s abandoned, IP: %s:%s: the response doesn't contain the %q header",
					item.url, item.podIP, item.podPort, header.HashKey)
				return true, nil
			case item.routeState.version:
				return true, nil
			default:
				return false, fmt.Errorf("unexpected version: want %q, got %q", item.routeState.version, hash)
			}

		case http.StatusNotFound, http.StatusServiceUnavailable:
			return false, fmt.Errorf("unexpected status code: want %v, got %v", http.StatusOK, r.StatusCode)

		default:
			item.logger.Errorf("Probing of %s abandoned, IP: %s:%s: the response status is %v, expected one of: %v",
				item.url, item.podIP, item.podPort, r.StatusCode,
				[]int{http.StatusOK, http.StatusNotFound, http.StatusServiceUnavailable})
			return true, nil
		}
	}
}

// deepCopy copies a URL into a new one
func deepCopy(in *url.URL) *url.URL {
	// Safe to ignore the error since this is a deep copy
	newURL, _ := url.Parse(in.String())
	return newURL
}
