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

# This script includes common functions for testing setup and teardown.
source "$(dirname "${BASH_SOURCE[0]}")/../hack/test-env.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../vendor/knative.dev/hack/e2e-tests.sh"

export SYSTEM_NAMESPACE=${SYSTEM_NAMESPACE:-knative-serving}
export CLUSTER_DOMAIN=${CLUSTER_DOMAIN:-cluster.local}
export INGRESS=${INGRESS:-istio}
export GATEWAY_OVERRIDE=${GATEWAY_OVERRIDE:-istio-ingressgateway}
export GATEWAY_NAMESPACE_OVERRIDE=${GATEWAY_NAMESPACE_OVERRIDE:-istio-system}
export UNSUPPORTED_E2E_TESTS=${UNSUPPORTED_E2E_TESTS:-$ISTIO_UNSUPPORTED_E2E_TESTS}
export KIND=${KIND:-0}

function parse_flags() {
  case "$1" in
    --istio)
      readonly INGRESS=istio
      readonly GATEWAY_OVERRIDE=istio-ingressgateway
      readonly GATEWAY_NAMESPACE_OVERRIDE=istio-system
      readonly UNSUPPORTED_E2E_TESTS="${ISTIO_UNSUPPORTED_E2E_TESTS}"
      return 1
      ;;
    --contour)
      readonly INGRESS=contour
      readonly GATEWAY_OVERRIDE=envoy-knative-gateway
      readonly GATEWAY_NAMESPACE_OVERRIDE=contour-external
      readonly UNSUPPORTED_E2E_TESTS="${CONTOUR_UNSUPPORTED_E2E_TESTS}"
      return 1
      ;;
    --kind)
      readonly KIND=1
      return 1
      ;;
  esac
  return 0
}

function test_setup() {
  header "Uploading test images"

  # Build and Publish the test images
  "${REPO_ROOT_DIR}/test/upload-test-images.sh" || return 1
}


function knative_setup() {
  header "Installing networking layer and net-gateway-api controller"

  ko apply -f "${REPO_ROOT_DIR}/test/config/" || failed_test "Fail to setup test env"

  (
    ko apply -f "${REPO_ROOT_DIR}/config/" && \
    kubectl -n $SYSTEM_NAMESPACE scale deployment net-gateway-api-controller --replicas=2 && \
    kubectl -n $SYSTEM_NAMESPACE rollout status deployment net-gateway-api-controller
  ) || failed_test "failed to install net-gateway-api controller "


  # We deploy this last since we have ingress specific config-gateway configs that
  # we don't want to override with the default empty config
  setup_networking || fail_test "failed to setup networking layer"

  wait_until_service_has_external_ip \
    $GATEWAY_NAMESPACE_OVERRIDE $GATEWAY_OVERRIDE || fail_test "Service did not get an IP address"
}

function knative_teardown() {
  teardown_networking
  ko delete \
    --ignore-not-found=true \
    -f "${REPO_ROOT_DIR}/test/config" \
    -f "${REPO_ROOT_DIR}/config"
}

function setup_networking() {
  echo ">> Installing Gateway API CRDs"
  kubectl apply -f "${REPO_ROOT_DIR}/third_party/gateway-api/gateway-api.yaml" || return $?
  kubectl wait --for=condition=complete --timeout=60s -n "gateway-system" job/gateway-api-admission
  kubectl wait --for=condition=complete --timeout=60s -n "gateway-system" job/gateway-api-admission-patch

  # make sure webhook is ready.
  kubectl wait -n gateway-system --for=condition=Available --timeout=60s deploy/gateway-api-admission-server

  if [[ "${INGRESS}" == "contour" ]]; then
    setup_contour
  else
    setup_istio
  fi
}

function teardown_networking() {
  kubectl delete -f "${REPO_ROOT_DIR}/third_party/${INGRESS}"
  kubectl delete -f "${REPO_ROOT_DIR}/third_party/gateway-api/gateway-api.yaml" 

  if [[ "$INGRESS" == "contour" ]]; then
    kubectl delete -f "https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VERSION}/examples/render/contour-gateway-provisioner.yaml"
  else
    istioctl uninstall -y --purge
    kubectl delete namespace istio-system
  fi
}

function setup_contour() {
  # Version is selected is in $REPO_ROOT/hack/test-env.sh
  kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VERSION}/examples/render/contour-gateway-provisioner.yaml" && \
  kubectl wait deploy --for=condition=Available --timeout=60s -n projectcontour contour-gateway-provisioner && \
  kubectl wait deploy --for=condition=Available --timeout=60s -n gateway-system gateway-api-admission-server && \
  kubectl apply -f "${REPO_ROOT_DIR}/third_party/contour"

  local ret=$?
  if [ $ret -ne 0 ]; then
    echo "failed to setup contour" >&2
    return $ret
  fi
}

function setup_istio() {
  # Version is selected by ISTIO_VERSION that's source in $REPO_ROOT/hack/test-env.sh
  curl -L https://istio.io/downloadIstio | sh - && \
  export PATH="${PWD}/istio-${ISTIO_VERSION}/bin:${PATH}" && \
  istioctl install -y --set values.global.proxy.clusterDomain="${CLUSTER_DOMAIN}" && \
  kubectl apply -f "${REPO_ROOT_DIR}/third_party/istio"

  local ret=$?
  if [ $ret -ne 0 ]; then
    echo "failed to setup istio" >&2
    return $ret
  fi
}

function test_conformance() {
  local parallel_count="12"
  if (( KIND )); then
    parallel_count="1"
  fi

  go_test_e2e -timeout=20m -tags=e2e -parallel="${parallel_count}" ./test/conformance \
    -enable-alpha \
    -enable-beta \
    -skip-tests="${UNSUPPORTED_E2E_TESTS}" \
    -ingressClass=gateway-api.ingress.networking.knative.dev

  return $?
}

function test_ha() {
  go_test_e2e -timeout=15m -failfast -parallel=1 ./test/ha \
    -spoofinterval="10ms" \
    -ingressClass=gateway-api.ingress.networking.knative.dev

  return $?
}
