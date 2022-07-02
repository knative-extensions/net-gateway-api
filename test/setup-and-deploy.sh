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

# Deploy artifacts specific for a suported vendor implementation.
# Parameters: $1 - name of vendor implementation.
function deploy() {
    # echo "${1-default}"
    # local vendor=$1
    echo ">> Deploy Gateway API resources"
    # kubectl apply -f ./third_party/"$vendor"/gateway/
}


# echo "${1-default}"
# if [ -z "${1-default}" ]; then
#     echo "empty"
#     EXIT
# fi

if [ $# -eq 0 ]; then
   echo "what"
   exit
fi

vendor=$1


# while $vendor; do
   case $vendor in
    "istio" )
      echo "istio"
        ;;
     *) # Invalid option
         echo "Error: Invalid option"
         exit;;
   esac
# done


echo "hello $vendor!"

# gateway-api CRD must be installed before Istio.
#    kubectl apply  -f config/100-gateway-api.yaml

#    echo ">> Bringing up Istio"
#    curl -sL https://istio.io/downloadIstioctl | sh -
#    $HOME/.istioctl/bin/istioctl install -y --set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

# deploy istio

#    wait_until_service_has_external_http_address istio-system istio-ingressgateway

#     echo Waiting for Pods to become ready.
#     kubectl wait pod --for=condition=Ready -n knative-serving -l '!job-name'

#     # For debugging.
#     kubectl get pods --all-namespaces



