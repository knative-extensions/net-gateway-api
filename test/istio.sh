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

# This script runs e2e tests on a local kind environment.

set -euo pipefail

source $(dirname $0)/../hack/test-env.sh
source $(dirname $0)/e2e-common.sh

function setup_and_deploy() {
   # gateway-api CRD must be installed before Istio.
   kubectl apply  -f config/100-gateway-api.yaml

   echo ">> Bringing up Istio"
   curl -sL https://istio.io/downloadIstioctl | sh -
   $HOME/.istioctl/bin/istioctl install -y --set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

   deploy istio

   wait_until_service_has_external_http_address istio-system istio-ingressgateway

    echo Waiting for Pods to become ready.
    kubectl wait pod --for=condition=Ready -n knative-serving -l '!job-name'

    # For debugging.
    kubectl get pods --all-namespaces
}