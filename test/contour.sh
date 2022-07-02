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

function setup_and_deploy() {
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