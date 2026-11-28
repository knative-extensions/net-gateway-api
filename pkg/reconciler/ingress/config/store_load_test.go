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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networkcfg "knative.dev/networking/pkg/config"
	. "knative.dev/pkg/configmap/testing"
)

func networkConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: networkcfg.ConfigMapName},
	}
}

func TestStoreLoad(t *testing.T) {
	store := NewStore(context.Background())

	gatewayCM, _ := ConfigMapsFromTestFile(t, GatewayConfigName)
	store.OnConfigChanged(gatewayCM)
	store.OnConfigChanged(networkConfigMap())

	want, err := FromConfigMap(gatewayCM)
	if err != nil {
		t.Fatal("FromConfigMap() =", err)
	}

	cfg := store.Load()
	if got, want := cfg.GatewayPlugin.ExternalGateway().NamespacedName, want.ExternalGateway().NamespacedName; got != want {
		t.Errorf("Load().GatewayPlugin.ExternalGateway() = %v, want %v", got, want)
	}
	if got, want := cfg.GatewayPlugin.LocalGateway().NamespacedName, want.LocalGateway().NamespacedName; got != want {
		t.Errorf("Load().GatewayPlugin.LocalGateway() = %v, want %v", got, want)
	}
	if cfg.Network == nil {
		t.Error("Load().Network = nil, want a populated Network config")
	}
}

func TestStoreLoadReturnsACopy(t *testing.T) {
	store := NewStore(context.Background())

	gatewayCM, _ := ConfigMapsFromTestFile(t, GatewayConfigName)
	store.OnConfigChanged(gatewayCM)
	store.OnConfigChanged(networkConfigMap())

	first := store.Load()
	first.GatewayPlugin.ExternalGateways[0].Name = "mutated"

	second := store.Load()
	if second.GatewayPlugin.ExternalGateways[0].Name == "mutated" {
		t.Error("Load() returned a Config sharing state with a previous Load() call")
	}
}

func TestStoreToContext(t *testing.T) {
	store := NewStore(context.Background())
	store.OnConfigChanged(networkConfigMap())

	gatewayCM, _ := ConfigMapsFromTestFile(t, GatewayConfigName)
	store.OnConfigChanged(gatewayCM)

	ctx := store.ToContext(context.Background())

	got := FromContext(ctx)
	want := store.Load()
	if got.GatewayPlugin.ExternalGateway().NamespacedName != want.GatewayPlugin.ExternalGateway().NamespacedName {
		t.Errorf("FromContext(ctx).GatewayPlugin = %v, want %v", got.GatewayPlugin, want.GatewayPlugin)
	}
}
