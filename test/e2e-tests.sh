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
initialize "$@"

# Run the tests
header "Running e2e tests"

failed=0

test_conformance || failed=1
test_ha || failed=1

(( failed )) && fail_test

# Remove the kail log file if the test flow passes.
# This is for preventing too many large log files to be uploaded to GCS in CI.
rm "${ARTIFACTS}/k8s.log-$(basename "${E2E_SCRIPT}").txt"
