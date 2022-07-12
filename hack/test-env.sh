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

export GATEWAY_API_VERSION="v0.5.0-rc1"
export ISTIO_VERSION="1.13.2"
export ISTIO_UNSUPPORTED_E2E_TESTS="retry,httpoption,host-rewrite"
export CONTOUR_VERSION="485238e"
export CONTOUR_UNSUPPORTED_E2E_TESTS="retry,httpoption,basics/http2,websocket,websocket/split,grpc,grpc/split,visibility/path,visibility,update,host-rewrite"
