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

set -o errexit
set -o nounset
set -o pipefail

source $(dirname "$0")/../vendor/knative.dev/hack/codegen-library.sh

boilerplate="${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt"

echo "=== Update Codegen for $MODULE_NAME"

group "Gateway API Codegen"

## Gateway API
OUTPUT_PKG="knative.dev/net-gateway-api/pkg/client/injection" \
VERSIONED_CLIENTSET_PKG="sigs.k8s.io/gateway-api/pkg/client/clientset/versioned" \
EXTERNAL_INFORMER_PKG="sigs.k8s.io/gateway-api/pkg/client/informers/externalversions" \
"${KNATIVE_CODEGEN_PKG}"/hack/generate-knative.sh "injection" \
  sigs.k8s.io/gateway-api/pkg/client \
  sigs.k8s.io/gateway-api \
  "apis:v1beta1,v1" \
  --go-header-file "${boilerplate}"

# Deepcopy is broken for fields that use generics - so we generate the code
# ignore failures and then clean it up ourselves with sed until k8s upstream
# fixes the issue
group "Deepcopy Gen"
go run k8s.io/code-generator/cmd/deepcopy-gen \
  -O zz_generated.deepcopy \
  --go-header-file "${boilerplate}" \
  --input-dirs knative.dev/net-gateway-api/pkg/reconciler/ingress/config

# group "Update deps post-codegen"
# Make sure our dependencies are up-to-date
"${REPO_ROOT_DIR}"/hack/update-deps.sh
group "Update tested version docs"

source "${REPO_ROOT_DIR}"/hack/test-env.sh
template=$(cat "${REPO_ROOT_DIR}"/docs/.test-version.template)
eval "echo \"${template}\"" > "${REPO_ROOT_DIR}"/docs/test-version.md
