/*
Copyright 2026 The Knative Authors

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

package config

import (
	"context"
	"testing"
)

func TestFromContextOrDefaultsNoConfigInContext(t *testing.T) {
	cfg := FromContextOrDefaults(context.Background())
	if cfg == nil {
		t.Fatal("FromContextOrDefaults() = nil, want a non-nil default Config")
	}
	if cfg.Network != nil {
		t.Errorf("Network = %v, want nil default", cfg.Network)
	}
	if cfg.GatewayPlugin != nil {
		t.Errorf("GatewayPlugin = %v, want nil default", cfg.GatewayPlugin)
	}
}

func TestFromContextOrDefaultsWithConfigInContext(t *testing.T) {
	want := &Config{GatewayPlugin: &GatewayPlugin{}}
	ctx := ToContext(context.Background(), want)

	if got := FromContextOrDefaults(ctx); got != want {
		t.Errorf("FromContextOrDefaults() = %v, want %v", got, want)
	}
}

func TestFromContextRoundTrip(t *testing.T) {
	want := &Config{GatewayPlugin: &GatewayPlugin{}}
	ctx := ToContext(context.Background(), want)

	if got := FromContext(ctx); got != want {
		t.Errorf("FromContext() = %v, want %v", got, want)
	}
}
