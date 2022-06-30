# Development
This doc explains how to setup a development environment so you can get started
contributing to Knative `net-gateway-api`.

Before submitting a PR, see also:
- [CONTRIBUTING.md](./CONTRIBUTING.md).
- [The pull request workflow](https://knative.dev/community/contributing/reviewing/)

__NOTE__ If you use the konk script to setup your cluster, your cluster will be named `knative`. However, most of the scripts expect it to be the default `kind` name. Set the kind cluster name env `export KIND_CLUSTER_NAME=knative` to point to `knative` cluster. KO requires a registry which if you are developing locally you could use `export KO_DOCKER_REPO=kind.local` to use the local one on kind. Please see official [KO documentation](https://github.com/google/ko#local-publishing-options) for more information.

__NOTE__ Versions to be installed are listed in `./hack/test-env.sh`.

__NOTE__ Tests are currently wip. Please see [README#tests](README.md#tests)

### Requirements
1. A Kind cluster
1. [Knative serving installed](README.md#install-knative-serving)
1. [`ko`](https://github.com/google/ko): For development.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/): For managing development environments.

## Testing
Currently this repo contains Knative conformance tests that run with Istio and Contour.

### Conformance
#### Istio

`./test/kind-conformance-istio.sh`

#### Contour

`./test/kind-conformance-contour.sh`

### e2e
#### Istio

`./test/kind-e2e-istio.sh`

#### Contour

`./test/kind-e2e-contour.sh`