# Knative net-gateway-api

**[This component is ALPHA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

[![GoDoc](https://godoc.org/knative-sandbox.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative-sandbox/net-gateway-api)](https://goreportcard.com/report/knative-sandbox/net-gateway-api)

net-gateway-api repository contains a KIngress implementation and testing for Knative
integration with the
[Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/).

This work is still in early development, which means it's _not ready for production_, but also that your feedback can have a big impact.
You can also find the tested Ingress and unavailable features [here](docs/test-version.md).

## Getting started

- Load tested environment versions

```
source ./hack/test-env.sh
```

- Install Knative Serving

```bash
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-core.yaml
```

- Install net-gateway-api

```bash
ko apply -f config/
```

- Then install Istio and its gateway resources:

__NOTE__ You can find the Istio version to be installed in `./hack/test-env.sh`.

```bash
curl -sL https://istio.io/downloadIstioctl | sh -
$HOME/.istioctl/bin/istioctl install -y

kubectl apply -f third_party/istio/gateway/
```

- Configure Knative Serving to use the proper "ingress.class":

```bash
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"gateway-api.ingress.networking.knative.dev"}}'
```

- Configure Knative Serving to use nip.io for DNS. For `kind` the loadbalancer IP is `127.0.0.1`:

```bash
kubectl patch configmap/config-domain \
  -n knative-serving \
  --type merge \
  -p '{"data":{"127.0.0.1.nip.io":""}}'
```

- (OPTIONAL) Deploy a sample hello world app:

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

- (OPTIONAL) For testing purposes, you can use port-forwarding to make requests
  to ingress from your machine:

```bash
kubectl port-forward  -n istio-system $(kubectl get pod -n istio-system -l "app=istio-ingressgateway" --output=jsonpath="{.items[0].metadata.name}") 8080:8080

curl -v -H "Host: helloworld-go.default.127.0.0.1.nip.io" http://localhost:8080
```

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).
