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

set -eo pipefail

# This script includes common functions for testing setup and teardown.
source "$(dirname $0)"/../vendor/knative.dev/hack/e2e-tests.sh
source "$(dirname $0)"/../hack/test-env.sh

export CONTROL_NAMESPACE=knative-serving

function export_cluster_info() {
  export IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )
  export CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}
}

function conformance_setup() {
  echo ">> Setting up conformance"
  # Prepare test namespaces
  kubectl apply -f test/config/
  # Build and Publish the test images to the docker daemon.
  "$(dirname $0)"/upload-test-images.sh || fail_test "Error uploading test images"
  export_cluster_info
}

function e2e_setup() {
  echo ">> Setting up e2e"
  # Setting up test resources.
  echo ">> Publishing test images"
  "$(dirname $0)"/upload-test-images.sh || fail_test "Error uploading test images"
  echo ">> Creating test resources (test/config/)"
  ko apply ${KO_FLAGS} -f test/config/ || return 1
  export_cluster_info

  # Bringing up controllers.
  echo ">> Bringing up controller"
  ko apply -f config/ || return 1
  kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=2

  # Wait for pods to be running.
  echo ">> Waiting for controller components to be running..."
  kubectl -n "${CONTROL_NAMESPACE}" rollout status deployment net-gateway-api-controller || return 1
}

# Setup resources.
function test_setup() {
  echo ">> Setting up logging..."
  # Install kail if needed.
  if ! which kail >/dev/null; then
    bash <(curl -sfL https://raw.githubusercontent.com/boz/kail/master/godownloader.sh) -b "$GOPATH/bin"
  fi

  # Capture all logs.
  kail >${ARTIFACTS}/k8s.log.txt &
  local kail_pid=$!
  # Clean up kail so it doesn't interfere with job shutting down
  add_trap "kill $kail_pid || true" EXIT

  e2e_setup
}

# Add function call to trap
# Parameters: $1 - Function to call
#             $2...$n - Signals for trap
function add_trap() {
  local cmd=$1
  shift
  for trap_signal in $@; do
    local current_trap="$(trap -p $trap_signal | cut -d\' -f2)"
    local new_cmd="($cmd)"
    [[ -n "${current_trap}" ]] && new_cmd="${current_trap};${new_cmd}"
    trap -- "${new_cmd}" $trap_signal
  done
}

function wait() {
  echo ">>Waiting for Pods to become ready"
  kubectl wait pod --for=condition=Ready -n knative-serving -l '!job-name'
  # For debugging.
  kubectl get pods --all-namespaces
}

function deploy_contour() {
  echo ">> Bringing up Contour"
  kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour-operator/${CONTOUR_VERSION}/examples/operator/operator.yaml"

  # # wait for operator deployment to be Available
  kubectl wait deploy --for=condition=Available --timeout=120s -n "contour-operator" -l '!job-name'

  echo ">> Deploy Gateway API resources"
  ko resolve -f ./third_party/contour/gateway/ | \
      sed 's/LoadBalancerService/NodePortService/g' | \
      kubectl apply -f -
}

function deploy_istio() {
  # gateway-api CRD must be installed before Istio.
  echo ">> Installing Gateway API CRDs"
  kubectl apply -f config/100-gateway-api.yaml

  echo ">> Bringing up Istio"
  curl -sL https://istio.io/downloadIstioctl | sh -
  "$HOME"/.istioctl/bin/istioctl install -y --set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

  echo ">> Deploy Gateway API resources"
  kubectl apply -f ./third_party/istio/gateway/
}
