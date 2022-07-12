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

source $(dirname $0)/e2e-common.sh
source $(dirname $0)/../vendor/knative.dev/hack/e2e-tests.sh
source "$(dirname $0)"/e2e-library-deployments.sh
source "$(dirname $0)"/e2e-library.sh

set +x

# Script entry point.
initialize "$@" --skip-istio-addon

deploy_gateway_for istio

# Run the tests
header "Running e2e tests with all available Gateway API vendors installed"
e2e_istio

# TODO(@carlisia): add a call to deploy and run e2e tests for contour
