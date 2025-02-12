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

source $(dirname $0)/e2e-common.sh

# Script entry point.
initialize "$@" --cluster-version=1.30

# Run the tests
header "Running e2e tests"

failed=0

if [[ "$GATEWAY_TESTS_ONLY" -eq "0" ]]; then
  knative_conformance || failed=1
  test_ha || failed=1
  test_e2e || failed=1
fi

if [[ "${JOB_NAME:-unknown}" == *"continuous"* ]] || (( GATEWAY_TESTS_ONLY )); then
  gateway_conformance || true # this is informational
fi



(( failed )) && fail_test

success
