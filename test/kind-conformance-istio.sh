#!/usr/bin/env bash

# Copyright 2021 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script runs conformance tests on a local kind environment.

set -euo pipefail

IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )
CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}
UNSUPPORTED_TESTS=""

# gateway-api CRD must be installed before Istio.
kubectl apply -k 'github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.3.0'

echo ">> Bringing up Istio"
sed -ie "s/cluster\.local/${CLUSTER_SUFFIX}/g" ./third_party/istio-head/istio-kind-no-mesh.yaml
./third_party/istio-head/install-istio.sh istio-kind-no-mesh.yaml

echo ">> Deploy Gateway API resources"
kubectl apply -f ./third_party/istio-head/gateway/

echo ">> Running conformance tests"
go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance/ingressv2 \
   --enable-alpha --enable-beta \
   --skip-tests="${UNSUPPORTED_TESTS}" \
   --ingressendpoint="${IPS[0]}" \
   --cluster-suffix=$CLUSTER_SUFFIX
