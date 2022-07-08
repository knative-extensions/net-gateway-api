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

# This script runs the end-to-end tests against net-gateway-api built from source.
# It is started by prow for each PR. For convenience, it can also be executed manually:
  # If you already have a Knative cluster setup and kubectl pointing
  # to it, call this script with the --run-tests arguments and it will use
  # the cluster and run the tests.

  # Calling this script without arguments will create a new cluster in
  # project $PROJECT_ID, start knative in it, run the tests and delete the
  # cluster.

set -eo pipefail

source "$(dirname $0)"/e2e-common.sh

deploy_contour
wait

export GATEWAY_OVERRIDE=envoy
export GATEWAY_NAMESPACE_OVERRIDE=contour-external

echo ">> Running e2e tests"
failed=0
go test -race -count=1 -short -timeout=20m -tags=e2e ./test/conformance \
   --enable-alpha \
   --skip-tests="${CONTOUR_UNSUPPORTED_E2E_TESTS}" \
   --ingressendpoint="${IPS[0]}" \
   --ingressClass=gateway-api.ingress.networking.knative.dev \
   --cluster-suffix="$CLUSTER_SUFFIX" || failed=1

# Give the controller time to sync with the rest of the system components.
sleep 30

echo ">> Scale up controller for HA tests"
kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=2

failed=0
go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
   --enable-alpha \
   --ingressendpoint="${IPS[0]}" \
   --ingressClass=gateway-api.ingress.networking.knative.dev \
   --cluster-suffix="$CLUSTER_SUFFIX" || failed=1

echo ">> Scale down after HA tests"
kubectl -n "${CONTROL_NAMESPACE}" scale deployment net-gateway-api-controller --replicas=1
