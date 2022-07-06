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

# This script includes common functions for setting up different Gateway API implementations.
# source "$(dirname $0)"/e2e-common.sh
source "$(dirname $0)"/../hack/test-env.sh

function setup() {
    export CONTROL_NAMEsSPACE=knative-serving
    export CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}
    export IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )
}

function wait() {
    echo ">>Waiting for Pods to become ready"
    kubectl wait pod --for=condition=Ready -n knative-serving -l '!job-name'

    # For debugging.
    kubectl get pods --all-namespaces
}

function deploy_istio() {
    setup
    # gateway-api CRD must be installed before Istio.
    echo ">> Installing Gateway API CRDs"
    kubectl apply -f config/100-gateway-api.yaml

    echo ">> Bringing up Istio"
    curl -sL https://istio.io/downloadIstioctl | sh -
    "$HOME"/.istioctl/bin/istioctl install -y --set values.gateways.istio-ingressgateway.type=NodePort --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

    echo ">> Deploy Gateway API resources"
    kubectl apply -f ./third_party/istio/gateway/

    wait
}

function deploy_contour() {
    setup
    export GATEWAY_OVERRIDE=envoy
    export GATEWAY_NAMESPACE_OVERRIDE=contour-external

    echo ">> Bringing up Contour"
    kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour-operator/${CONTOUR_VERSION}/examples/operator/operator.yaml"

    # wait for operator deployment to be Available
    kubectl wait deploy --for=condition=Available --timeout=120s -n "contour-operator" -l '!job-name'

    echo ">> Deploy Gateway API resources"
    ko resolve -f ./third_party/contour/gateway/ | \
        sed 's/LoadBalancerService/NodePortService/g' | \
        kubectl apply -f -

    wait
}
