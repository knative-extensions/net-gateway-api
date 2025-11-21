#!/usr/bin/env bash

# Copyright 2024 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "$0")/../vendor/knative.dev/hack/library.sh"
source "${REPO_ROOT_DIR}/hack/test-env.sh"
readonly OUTPUT="${REPO_ROOT_DIR}/third_party/envoy-gateway/install.yaml"

group "Update Envoy Gateway rendered manifest to version ${ENVOY_GATEWAY_VERSION}"

cat > "${OUTPUT}" << 'EOF'
---
apiVersion: v1
kind: Namespace
metadata:
  name: envoy-gateway-system
EOF

# Render the Envoy Gateway chart with the same settings we use in tests,
# but commit the rendered YAML so that users and CI do not need Helm installed.
helm template eg oci://docker.io/envoyproxy/gateway-helm \
  --version "${ENVOY_GATEWAY_VERSION}" \
  --namespace envoy-gateway-system \
  --include-crds \
  -f "${REPO_ROOT_DIR}/third_party/envoy-gateway/helm/values-eg.yaml" \
  >> "${OUTPUT}"

echo "Wrote rendered manifest to ${OUTPUT}"
