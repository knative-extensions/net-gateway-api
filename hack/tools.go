//go:build tools
// +build tools

/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

// This package imports things required by this repository, to force `go mod` to see them as dependencies
import (
	_ "k8s.io/code-generator"
	_ "knative.dev/hack"

	_ "k8s.io/code-generator/cmd/deepcopy-gen"

	// codegen: hack/generate-knative.sh
	_ "knative.dev/pkg/hack"

	_ "knative.dev/networking/test/conformance/ingress"
	_ "knative.dev/networking/test/test_images/grpc-ping"
	_ "knative.dev/networking/test/test_images/httpproxy"
	_ "knative.dev/networking/test/test_images/retry"
	_ "knative.dev/networking/test/test_images/runtime"
	_ "knative.dev/networking/test/test_images/timeout"
	_ "knative.dev/networking/test/test_images/wsserver"

	// networking resource
	_ "knative.dev/networking/config"
)
