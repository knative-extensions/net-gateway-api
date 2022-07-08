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

set -euo pipefail

source "$(dirname $0)"/../hack/test-env.sh

CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}

function deploy_istio() {
  local KIND="$1"
  # gateway-api CRD must be installed before Istio.
  echo ">> Installing Gateway API CRDs"
  kubectl apply -f config/100-gateway-api.yaml

  # TODO(@carlisia): remove configs that only work on Kind
  echo ">> Bringing up Istio"
  flag=""
  if [[ "$KIND" == "kind" ]]; then
    flag="--set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain=${CLUSTER_SUFFIX}"
  fi
  curl -sL https://istio.io/downloadIstioctl | sh -
  "$HOME"/.istioctl/bin/istioctl install -y $flag

  echo ">> Deploy Gateway API resources"
  kubectl apply -f ./third_party/istio/gateway/
}

function deploy_contour() {
  kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour-operator/${CONTOUR_VERSION}/examples/operator/operator.yaml"

  echo ">> Waiting for Contour operator deployment to be available"
  kubectl wait deploy --for=condition=Available --timeout=120s -n "contour-operator" -l '!job-name'

  ko resolve -f ./third_party/contour/gateway/ | \
      sed 's/LoadBalancerService/NodePortService/g' | \
      kubectl apply -f -
}

function deploy_gateway_for() {
  local VENDOR="$1"
  shift

  echo ">> Deploying Gateway resources for ${VENDOR}"

  case $VENDOR in
    "contour")
      deploy_contour "$@"
      ;;
    "istio")
      deploy_istio "$@"
        ;;
    *)
        echo "Error: Invalid option, no deployment for ${VENDOR} exists."
        exit;;
  esac
}
