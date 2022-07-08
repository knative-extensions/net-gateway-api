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

source "$(dirname $0)"/e2e-common.sh

# Script entry point.
initialize "$@" --skip-istio-addon

# Run the tests
header "Running Istio e2e tests"
source "$(dirname $0)"/kind-e2e-istio.sh

(( failed )) && dump_cluster_state
(( failed )) && fail_test

success

# TODO(carlisia): add a call to run the Contour conformance tests