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

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"

	"knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
)

const (
	// HTTPPortExternal is the port for external availability.
	HTTPPortExternal = "8080"
	// HTTPPortInternal is the port for internal availability.
	HTTPPortInternal = "8081"
	// HTTPSPortExternal is the port for external HTTPS availability.
	HTTPSPortExternal = "8443"
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

func (l *gatewayPodTargetLister) ListProbeTargets(ctx context.Context, ing *v1alpha1.Ingress) ([]status.ProbeTarget, error) {
	gatewayConfig := config.FromContext(ctx).Gateway
	namespacedNameLocalService := gatewayConfig.Gateways[v1alpha1.IngressVisibilityClusterLocal].Service

	eps, err := l.endpointsLister.Endpoints(namespacedNameLocalService.Namespace).Get(namespacedNameLocalService.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get internal service: %w", err)
	}

	readyIPs := sets.NewString()
	for _, sub := range eps.Subsets {
		for _, address := range sub.Addresses {
			readyIPs.Insert(address.IP)
		}
	}
	if len(readyIPs) == 0 {
		return nil, fmt.Errorf("no gateway pods available")
	}
	return l.getIngressUrls(ing, readyIPs)
}

func (l *gatewayPodTargetLister) getIngressUrls(ing *v1alpha1.Ingress, ips sets.String) ([]status.ProbeTarget, error) {

	targets := make([]status.ProbeTarget, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		var target status.ProbeTarget

		domains := rule.Hosts
		scheme := "http"

		if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			target = status.ProbeTarget{
				PodIPs: ips,
			}
			if ing.Spec.HTTPOption == v1alpha1.HTTPOptionRedirected {
				target.PodPort = HTTPSPortExternal
				target.URLs = domainsToURL(domains, "https")
			} else {
				target.PodPort = HTTPPortExternal
				target.URLs = domainsToURL(domains, scheme)
			}
		} else {
			target = status.ProbeTarget{
				PodIPs:  ips,
				PodPort: HTTPPortInternal,
				URLs:    domainsToURL(domains, scheme),
			}
		}

		targets = append(targets, target)

	}
	return targets, nil
}

func domainsToURL(domains []string, scheme string) []*url.URL {
	urls := make([]*url.URL, 0, len(domains))
	for _, domain := range domains {
		url := &url.URL{
			Scheme: scheme,
			Host:   domain,
			Path:   "/",
		}
		urls = append(urls, url)
	}
	return urls
}
