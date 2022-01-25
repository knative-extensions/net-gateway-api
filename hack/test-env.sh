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

export GATEWAY_API_VERSION="v0.4.0"
export ISTIO_VERSION="1.12.2"
export ISTIO_UNSUPPORTED_TESTS="tls,retry,httpoption,host-rewrite"
export CONTOUR_VERSION="v1.20.0-beta.1"
export CONTOUR_UNSUPPORTED_TESTS="tls,retry,httpoption,basics/http2,websocket,websocket/split,grpc,grpc/split,visibility/path,visibility,update,host-rewrite"
