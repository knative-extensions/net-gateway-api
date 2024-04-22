/*
Copyright 2024 The Knative Authors

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

package main

import (
	"context"

	gatewayapiconfig "knative.dev/net-gateway-api/pkg/reconciler/ingress/config"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/webhook"
	"knative.dev/pkg/webhook/certificates"
	"knative.dev/pkg/webhook/configmaps"
)

func NewConfigValidationController(ctx context.Context, _ configmap.Watcher) *controller.Impl {
	return configmaps.NewAdmissionController(ctx,

		// Name of the resource webhook.
		"config.webhook.gateway-api.networking.internal.knative.dev",

		// The path on which to serve the webhook.
		"/config-validation",

		// The configmaps to validate.
		configmap.Constructors{
			gatewayapiconfig.GatewayConfigName: gatewayapiconfig.FromConfigMap,
		},
	)
}

func main() {
	ctx := webhook.WithOptions(signals.NewContext(), webhook.Options{
		ServiceName: "net-gateway-api-webhook",
		SecretName:  "net-gateway-api-webhook-certs",
		Port:        webhook.PortFromEnv(8443),
	})

	ctx = sharedmain.WithHealthProbesDisabled(ctx)
	sharedmain.WebhookMainWithContext(
		ctx, "net-gateway-api-webhook",
		certificates.NewController,
		NewConfigValidationController,
	)
}
