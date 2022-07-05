# Development
This doc explains how to setup a development environment so you can get started
contributing to Knative `net-gateway-api`.

Before submitting a PR, see also:
- [CONTRIBUTING.md](./CONTRIBUTING.md).
- [The pull request workflow](https://knative.dev/community/contributing/reviewing/)

## Notes
If you use the konk script to setup your cluster, your cluster will be named `knative`. However, most of the scripts expect it to be the default `kind` name. Set the kind cluster name env `export KIND_CLUSTER_NAME=knative` to point to `knative` cluster. KO requires a registry which if you are developing locally you could use `export KO_DOCKER_REPO=kind.local` to use the local one on kind. Please see official [KO documentation](https://github.com/google/ko#local-publishing-options) for more information.

Versions to be installed are listed in [`hack/test-env.sh`](hack/test-env.sh).

Tests are currently wip. Please see [README#tests](README.md#tests)

## Requirements
1. A running cluster
2. [Knative serving installed](README.md#install-knative-serving)
3. [`ko`](https://github.com/google/ko) (for development and testing)
4. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (for managing development environments)
5. [`bash`](https://www.gnu.org/software/bash/) v4 or later. On macOS the default bash is too old, you can use [Homebrew](https://brew.sh) to install a later version.

## Environment
To start your environment you'll need to set `KO_DOCKER_REPO`: The repository to which developer/test images should be pushed. Ex:

```shell
export KO_DOCKER_REPO='gcr.io/my-gcloud-project-id'
```

### Notes
- If you are using Docker Hub to store your images your `KO_DOCKER_REPO` variable should be `docker.io/<username>`.
- Currently Docker Hub doesn't let you create subdirs under your username.
- You'll need to be authenticated with your `KO_DOCKER_REPO` before pushing images.
  - Google Container Registry: `gcloud auth configure-docker`
  - Docker Hub: `docker login`

## Building the test images
NOTE: this is only required when you run conformance/e2e tests locally with `go test` commands, and may be required periodically.

The [`upload-test-images.sh`](test/upload-test-images.sh) script can be used to build and push the test images used by the conformance and e2e tests.

To run the script for all end to end test images:

```bash
./test/upload-test-images.sh
```

## Tests
### Conformance
#### Istio
`./test/kind-conformance-istio.sh`

#### Contour
`./test/kind-conformance-contour.sh`

### e2e
Calling a script without arguments will create a new cluster in your current GCP project (assuming you have one) and run the tests against it.

Calling a script with `--run-tests` and the variable `KO_DOCKER_REPO` set will immediately start the tests against the cluster currently configured for `kubectl`.

#### Istio
`./test/kind-e2e-istio.sh`

#### Contour
`./test/kind-e2e-contour.sh`

## Adding new tests
1) To add a new test for a new vendor/implementation, add the corresponding bash script file(s) with the following header:

```shell
set -euo pipefail

source "$(dirname $0)"/setup-and-deploy.sh

deploy_new_vendor
```

2) Add a new function to the file  [`setup-and-deploy.sh`](test/setup-and-deploy.sh) and name it `function deploy_new_vendor()`.

3) Add the configuration specific to this vendor inside the `deploy_new_vendor()` function.

4) Add a call to the new vendor configuration to the `test_setup()` function in the [`e2e-tests.sh`](test/e2e-tests.sh) file.
