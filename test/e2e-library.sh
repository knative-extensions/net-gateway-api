#!/usr/bin/env bash

# Copyright 2022 The Knative Authors
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

source "$(dirname $0)"/../hack/test-env.sh

# TODO(@carlisia): refactor these tests

function e2e_istio() {
    export GATEWAY_OVERRIDE=""
    export GATEWAY_NAMESPACE_OVERRIDE=""

    failed=0
    go_test_e2e -timeout=20m -tags=e2e -parallel=12 \
        ./test/conformance \
        --enable-alpha --enable-beta \
        --skip-tests="${ISTIO_UNSUPPORTED_E2E_TESTS}" \
        --ingressClass=gateway-api.ingress.networking.knative.dev || failed=1

    # Give the controller time to sync with the rest of the system components.
    sleep 30

    go_test_e2e -timeout=15m -failfast -parallel=1 ./test/ha -spoofinterval="10ms" \
    --ingressClass=gateway-api.ingress.networking.knative.dev || failed=1

    (( failed )) && dump_cluster_state
    (( failed )) && fail_test

    success
}

# The tests below are run in a Kind cluster, therefore they need to have their ingressendpoint and cluster-suffix specified.
function cluster_info() {
    CONTROL_NAMESPACE=knative-serving
    IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )
    CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}
}

function kind_e2e_contour() {
    export GATEWAY_OVERRIDE=envoy
    export GATEWAY_NAMESPACE_OVERRIDE=contour-external

    cluster_info

    echo ">> Running e2e tests"

    go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance \
    --enable-alpha --enable-beta \
    --skip-tests="${CONTOUR_UNSUPPORTED_E2E_TESTS}" \
    --ingressendpoint="${IPS[0]}" \
    --ingressClass=gateway-api.ingress.networking.knative.dev \
    --cluster-suffix="${CLUSTER_SUFFIX}"

    # Give the controller time to sync with the rest of the system components.
    sleep 30

    echo ">> Scale up controller for HA tests"
    kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=2

    go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
    --enable-alpha --enable-beta \
    --ingressendpoint="${IPS[0]}" \
    --ingressClass=gateway-api.ingress.networking.knative.dev \
    --cluster-suffix="${CLUSTER_SUFFIX}"

    echo ">> Scale down after HA tests"
    kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=1
}

function kind_e2e_istio() {
    export GATEWAY_OVERRIDE=""
    export GATEWAY_NAMESPACE_OVERRIDE=""

    cluster_info

    go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance \
        --enable-alpha --enable-beta \
        --skip-tests="${ISTIO_UNSUPPORTED_E2E_TESTS}" \
        --ingressendpoint="${IPS[0]}" \
        --ingressClass=gateway-api.ingress.networking.knative.dev \
        --cluster-suffix="$CLUSTER_SUFFIX"

    # Give the controller time to sync with the rest of the system components.
    sleep 30

    echo ">> Scale up controller for HA tests"
    kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=2

    go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
        --enable-alpha --enable-beta \
        --ingressendpoint="${IPS[0]}" \
        --ingressClass=gateway-api.ingress.networking.knative.dev \
        --cluster-suffix="${CLUSTER_SUFFIX}"

    echo ">> Scale down after HA tests"
    kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=1
}

function kind_conformance_contour() {
    UNSUPPORTED_CONFORMANCE_TESTS="basics/http2,websocket,websocket/split,grpc,grpc/split,host-rewrite,visibility/path,visibility"

    export GATEWAY_OVERRIDE=envoy
    export GATEWAY_NAMESPACE_OVERRIDE=contour-external
    export LOCAL_GATEWAY_OVERRIDE=envoy
    export LOCAL_GATEWAY_NAMESPACE_OVERRIDE=contour-internal

    cluster_info

    conformance_setup
    deploy_contour

    echo ">> Running conformance tests"
    go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance/gateway-api \
    --enable-alpha --enable-beta \
    --skip-tests="${UNSUPPORTED_CONFORMANCE_TESTS}" \
    --ingressendpoint="${IPS[0]}" \
    --cluster-suffix="${CLUSTER_SUFFIX}"
}

function kind_conformance_istio() {
    UNSUPPORTED_CONFORMANCE_TESTS="visibility/split"

    export GATEWAY_OVERRIDE=""
    export GATEWAY_NAMESPACE_OVERRIDE=""
    export LOCAL_GATEWAY_OVERRIDE=""
    export LOCAL_GATEWAY_NAMESPACE_OVERRIDE=""

    cluster_info

    conformance_setup
    deploy_istio

    echo ">> Running conformance tests"
    go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance/gateway-api \
    --enable-alpha --enable-beta \
    --skip-tests="${UNSUPPORTED_CONFORMANCE_TESTS}" \
    --ingressendpoint="${IPS[0]}" \
    --cluster-suffix="${CLUSTER_SUFFIX}"
}
