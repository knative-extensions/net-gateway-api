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

source $(dirname $0)/../vendor/knative.dev/hack/codegen-library.sh

boilerplate="${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt"

echo "=== Update Codegen for $MODULE_NAME"

group "Gateway API Codegen"

# Gateway API
${CODEGEN_PKG}/generate-groups.sh "client,informer,lister" \
  knative.dev/net-gateway-api/pkg/client/gatewayapi sigs.k8s.io/gateway-api \
  "apis:v1alpha2" \
  --go-header-file ${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt

## Gateway API
${KNATIVE_CODEGEN_PKG}/hack/generate-knative.sh "injection" \
  knative.dev/net-gateway-api/pkg/client/gatewayapi sigs.k8s.io/gateway-api \
  "apis:v1alpha2" \
  --go-header-file ${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt

# Remove deprecated type
# Note: our injection script generates the code incorrectly and was causing the build to fail. Since this is deprecated as of v0.5.0, we are removing it.
rm -rf pkg/client/gatewayapi/injection/informers/apis/v1alpha2/referencepolicy

group "Deepcopy Gen"

# Depends on generate-groups.sh to install bin/deepcopy-gen
${GOPATH}/bin/deepcopy-gen \
  -O zz_generated.deepcopy \
  --go-header-file "${boilerplate}" \
  -i knative.dev/net-gateway-api/pkg/reconciler/ingress/config

group "Update deps post-codegen"

# Make sure our dependencies are up-to-date
${REPO_ROOT_DIR}/hack/update-deps.sh

group "Update tested version docs"

source ${REPO_ROOT_DIR}/hack/test-env.sh
template=$(cat ${REPO_ROOT_DIR}/docs/.test-version.template)
eval "echo \"${template}\"" > ${REPO_ROOT_DIR}/docs/test-version.md

group "Update gateway API CRDs"

if command -v kubectl &> /dev/null
then
  echo "# Generated with \"kubectl kustomize github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=${GATEWAY_API_VERSION}" > "${REPO_ROOT_DIR}/third_party/gateway-api/00-crds.yaml"
	kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=${GATEWAY_API_VERSION}" >> "${REPO_ROOT_DIR}/third_party/gateway-api/00-crds.yaml"
  # TODO(carlisia): remove the below two lines in a future release.
  # Reason: although the `gateway.networking.k8s.io_referencepolicies.yaml` file is included in the `v0.5.0-rc1` version, it is not referenced in the corresponding kustomize file. I'm assuming this will remain the same for the actual release.
  # The file reference is included in the kustomize file in the `main` branch. With that, if these lines are not removed, there would be a duplicate of this resource.
	echo "---" >> "${REPO_ROOT_DIR}/third_party/gateway-api/00-crds.yaml"
	curl -s "https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/crd/experimental/gateway.networking.k8s.io_referencepolicies.yaml" >> "${REPO_ROOT_DIR}/third_party/gateway-api/00-crds.yaml"
else
  echo "Skipping: kubectl command does not exist."
fi
