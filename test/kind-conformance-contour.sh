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
UNSUPPORTED_TESTS="basics/http2,websocket,websocket/split,grpc,grpc/split,host-rewrite"

export GATEWAY_OVERRIDE=envoy
export GATEWAY_NAMESPACE_OVERRIDE=contour-external
export LOCAL_GATEWAY_OVERRIDE=envoy
export LOCAL_GATEWAY_NAMESPACE_OVERRIDE=contour-internal

echo ">> Bringing up Contour"
./third_party/contour-head/install-operator.sh

# wait for operator deployment to be Available
kubectl wait deploy --for=condition=Available --timeout=120s -n "contour-operator" -l '!job-name'

echo ">> Deploy Gateway API resources"
ko resolve -f ./third_party/contour-head/gateway/ | \
  sed 's/LoadBalancerService/NodePortService/g' | \
  kubectl apply -f -

echo ">> Running conformance tests"
go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance/ingressv2 \
   --enable-alpha --enable-beta \
   --skip-tests="${UNSUPPORTED_TESTS}" \
   --ingressendpoint="${IPS[0]}" \
   --cluster-suffix=$CLUSTER_SUFFIX
