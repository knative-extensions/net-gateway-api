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

# This script runs e2e tests on a local kind environment.

set -euo pipefail

source $(dirname $0)/../hack/test-env.sh
source $(dirname $0)/e2e-common.sh

function implementation_setup() {
   export GATEWAY_OVERRIDE=envoy
   export GATEWAY_NAMESPACE_OVERRIDE=contour-external

   echo ">> Bringing up Contour"
   kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour-operator/${CONTOUR_VERSION}/examples/operator/operator.yaml"

   # wait for operator deployment to be Available
   kubectl wait deploy --for=condition=Available --timeout=120s -n "contour-operator" -l '!job-name'

   # TODO(carlisia) consider moving configuration override to inside `./third_party/contour/gateway/`
   # and usiong maybe ytt so we can just call `deploy contour` here.`
   echo ">> Deploy Gateway API resources"
   ko resolve -f ./third_party/contour/gateway/gateway-external.yaml | \
      sed 's/LoadBalancerService/NodePortService/g' | \
      kubectl apply -f -
}

function exectute_test() {
   echo Waiting for Pods to become ready.
   kubectl wait pod --for=condition=Ready -n knative-serving -l '!job-name'

   # For debugging.
   kubectl get pods --all-namespaces

   echo ">> Running e2e tests"
   go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance \
      --enable-alpha --enable-beta \
      --skip-tests="${CONTOUR_UNSUPPORTED_E2E_TESTS}" \
      --ingressendpoint="${IPS[0]}" \
      --ingressClass=gateway-api.ingress.networking.knative.dev \
      --cluster-suffix=${CLUSTER_SUFFIX}

   # Give the controller time to sync with the rest of the system components.
   sleep 30

   echo ">> Scale up controller for HA tests"
   kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=2

   go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
      --enable-alpha --enable-beta \
      --ingressendpoint="${IPS[0]}" \
      --ingressClass=gateway-api.ingress.networking.knative.dev \
      --cluster-suffix=${CLUSTER_SUFFIX}

   echo ">> Scale down after HA tests"
   kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=1
}

if [ "${1-default}" != "ci" ]; then
   log_setup
fi
test_setup
implementation_setup
exectute_test
