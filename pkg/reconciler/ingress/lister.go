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
	"strconv"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/net-gateway-api/pkg/status"
)

func NewProbeTargetLister(logger *zap.SugaredLogger, endpointsLister corev1listers.EndpointsLister) status.ProbeTargetLister {
	return &gatewayPodTargetLister{
		logger:          logger,
		endpointsLister: endpointsLister,
	}
}

type gatewayPodTargetLister struct {
	logger          *zap.SugaredLogger
	endpointsLister corev1listers.EndpointsLister
}

func (l *gatewayPodTargetLister) BackendsToProbeTargets(ctx context.Context, backends status.Backends) ([]status.ProbeTarget, error) {
	gatewayConfig := config.FromContext(ctx).Gateway

	foundTargets := 0
	targets := make([]status.ProbeTarget, 0, len(backends.URLs))

	for visibility, urls := range backends.URLs {
		service := gatewayConfig.Gateways[visibility].Service
		eps, err := l.endpointsLister.Endpoints(service.Namespace).Get(service.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get endpoints: %w", err)
		}
		for _, sub := range eps.Subsets {
			scheme := "http"
			// Istio uses "http2" for the http port
			// Contour uses "http-80" for the http port
			matchSchemes := sets.New("http", "http2", "http-80")
			if visibility == v1alpha1.IngressVisibilityExternalIP && backends.HTTPOption == v1alpha1.HTTPOptionRedirected {
				scheme = "https"
				matchSchemes = sets.New("https", "https-443")
			}
			pt := status.ProbeTarget{PodIPs: sets.New[string]()}

			portNumber := sub.Ports[0].Port
			for _, port := range sub.Ports {
				if matchSchemes.Has(port.Name) {
					// Prefer to match the name exactly
					portNumber = port.Port
					break
				}
				if port.AppProtocol != nil && matchSchemes.Has(*port.AppProtocol) {
					portNumber = port.Port
				}
			}
			pt.PodPort = strconv.Itoa(int(portNumber))

			for _, address := range sub.Addresses {
				pt.PodIPs.Insert(address.IP)
			}

			for url := range urls {
				url := url
				url.Scheme = scheme
				pt.URLs = append(pt.URLs, &url)
			}

			if len(pt.URLs) > 0 {
				foundTargets += len(pt.PodIPs)
				targets = append(targets, pt)
			}
		}
	}
	if foundTargets == 0 {
		return nil, fmt.Errorf("no gateway pods available")
	}
	return targets, nil
}
