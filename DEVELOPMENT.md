# Development

This doc explains how to setup a development environment so you can get started
[contributing](https://www.knative.dev/contributing/) to Knative
`net-ingressv2`. Also take a look at:

- [The pull request workflow](https://knative.dev/community/contributing/reviewing/)

## Getting started

1. Create [a GitHub account](https://github.com/join)
1. Setup
   [GitHub access via SSH](https://help.github.com/articles/connecting-to-github-with-ssh/)
1. Install [requirements](#requirements)
1. Set up your [shell environment](#environment-setup)
1. [Create and checkout a repo fork](#checkout-your-fork)

Before submitting a PR, see also [CONTRIBUTING.md](./CONTRIBUTING.md).

### Requirements

You must install these tools:

1. [`go`](https://golang.org/doc/install): The language Knative
   `net-ingressv2` is built in
1. [`git`](https://help.github.com/articles/set-up-git/): For source control
1. [`dep`](https://github.com/golang/dep): For managing external dependencies.

### Environment setup

To get started you'll need to set these environment variables (we recommend
adding them to your `.bashrc`):

1. `GOPATH`: If you don't have one, simply pick a directory and add
   `export GOPATH=...`

1. `$GOPATH/bin` on `PATH`: This is so that tooling installed via `go get` will
   work properly.

`.bashrc` example:

```shell
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
```

### Checkout your fork

The Go tools require that you clone the repository to the
`src/knative.dev/net-ingressv2` directory in your
[`GOPATH`](https://github.com/golang/go/wiki/SettingGOPATH).

To check out this repository:

1. Create your own
   [fork of this repo](https://help.github.com/articles/fork-a-repo/)

1. Clone it to your machine:

```shell
mkdir -p ${GOPATH}/src/knative.dev
cd ${GOPATH}/src/knative.dev
git clone git@github.com:${YOUR_GITHUB_USERNAME}/net-ingressv2.git
cd net-ingressv2
git remote add upstream https://github.com/knative-sandbox/net-ingressv2.git
git remote set-url --push upstream no_push
```

_Adding the `upstream` remote sets you up nicely for regularly
[syncing your fork](https://help.github.com/articles/syncing-a-fork/)._

Once you reach this point you are ready to do a full build and deploy as
described below.

### Install service-apis

```
kubectl apply -k 'github.com/kubernetes-sigs/service-apis/config/crd?ref=v0.1.0-rc2'
```

### Install Istio

Run the following command to install Istio for development purpose:

```shell
third_party/istio-latest/install-istio.sh istio-ci-no-mesh.yaml
```

### Install Knative Serving

Run the following command to install Knative

```shell
kubectl apply --filename https://storage.googleapis.com/knative-nightly/serving/latest/serving-crds.yaml
kubectl apply --filename https://storage.googleapis.com/knative-nightly/serving/latest/serving-core.yaml
```

### Install Knative net-ingressv2

```
KNATIVE_NAMESPACE="knative-serving"
kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"ingress.class":"ingressv2.ingress.networking.knative.dev"}}'
ko apply -f test/config/ -f config/
```
