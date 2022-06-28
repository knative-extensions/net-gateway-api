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

source $(dirname $0)/../hack/test-env.sh

IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )
CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}
UNSUPPORTED_CONFORMANCE_TESTS="visibility/split"

# gateway-api CRD must be installed before Istio.
kubectl apply  -f third_party/gateway-api/00-crds.yaml

echo ">> Bringing up Istio"
curl -sL https://istio.io/downloadIstioctl | sh -
$HOME/.istioctl/bin/istioctl install -y --set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

echo ">> Deploy Gateway API resources"
kubectl apply -f ./third_party/istio/gateway/

echo ">> Running conformance tests"
go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance/gateway-api \
   --enable-alpha --enable-beta \
   --skip-tests="${UNSUPPORTED_CONFORMANCE_TESTS}" \
   --ingressendpoint="${IPS[0]}" \
   --cluster-suffix=$CLUSTER_SUFFIX
