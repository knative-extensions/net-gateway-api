# Knative net-gateway-api
**[This component is ALPHA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

[![GoDoc](https://godoc.org/knative-sandbox.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative-sandbox/net-gateway-api)](https://goreportcard.com/report/knative-sandbox/net-gateway-api)

net-gateway-api repository contains a KIngress implementation and testing for Knative integration with the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/).

This work is still in early development, which means it's _not ready for production_, but also that your feedback can have a big impact. You can find the tested Ingress and unavailable features [here](docs/test-version.md).

This work is still in early development, which means it's _not ready for production_, but also that your feedback can have a big impact.
You can also find the tested Ingress and unavailable features [here](docs/test-version.md).

## KIngress Conformance Tests

We run our Knative Ingress Conformance tests and are tracking support by different implementations here:

- [Contour Epic · Issue #384](https://github.com/knative-sandbox/net-gateway-api/issues/384)
- [Istio EPIC · Issue #383](https://github.com/knative-sandbox/net-gateway-api/issues/383)

Versions to be installed are listed in [`hack/test-env.sh`](hack/test-env.sh).
---
## Requirements
1. A Kind cluster
1. Knative serving installed
2. [`ko`](https://github.com/ko-build/ko) (for installing the net-gateway-api)
3. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
4. `export KO_DOCKER_REPO=kind.local`

## Getting started
### Install Knative serving
```bash
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-core.yaml
```

#### Configure Knative
##### Ingress
Configuration so Knative serving uses the proper "ingress.class":

```bash
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"gateway-api.ingress.networking.knative.dev"}}'
```

##### (OPTIONAL) Deploy a sample hello world app:
```bash
cat <<-EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: helloworld-go
spec:
  template:
    spec:
      containers:
      - image: gcr.io/knative-samples/helloworld-go
        env:
        - name: TARGET
          value: Go Sample v1
EOF
```

### Install net-gateway-api
```bash
ko apply -f config/
```

### Load tested environment versions
```
source ./hack/test-env.sh
```

### Install a supported implementation
#### Istio
```bash
# gateway-api CRD must be installed before Istio.
echo ">> Installing Gateway API CRDs"
kubectl apply -f config/100-gateway-api.yaml

echo ">> Bringing up Istio"
curl -sL https://istio.io/downloadIstioctl | sh -
"$HOME"/.istioctl/bin/istioctl install -y --set values.global.proxy.clusterDomain="${CLUSTER_SUFFIX}"

echo ">> Deploy Gateway API resources"
kubectl apply -f ./third_party/istio
```

#### Contour
```bash
echo ">> Bringing up Contour"
kubectl apply -f "https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VERSION}/examples/render/contour-gateway-provisioner.yaml"

# wait for operator deployment to be Available
kubectl wait deploy --for=condition=Available --timeout=60s -n "projectcontour"

echo ">> Deploy Gateway API resources"
kubectl apply -f ./third_party/contour
```

### (OPTIONAL) For testing purpose (Istio)

Use Kind with MetalLB - https://kind.sigs.k8s.io/docs/user/loadbalancer

For Mac setup a SOCK5 Proxy in the Docker KinD network and use the `ALL_PROXY`
environment variable

```bash
docker run --name kind-proxy -d --network kind -p 1080:1080 serjs/go-socks5-proxy
export ALL_PROXY=socks5://localhost:1080
curl 172.18.255.200 -v -H 'Host: helloworld-test-image.default.example.com'
```

---

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).
