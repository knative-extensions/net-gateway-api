# Knative net-gateway-api

**[This component is ALPHA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

[![GoDoc](https://godoc.org/knative-sandbox.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative-sandbox/net-gateway-api)](https://goreportcard.com/report/knative-sandbox/net-gateway-api)

**[This component is ALPHA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

[![GoDoc](https://godoc.org/knative-sandbox.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative-sandbox/net-gateway-api)](https://goreportcard.com/report/knative-sandbox/net-gateway-api)

net-gateway-api repository contains a KIngress implementation and testing for Knative
integration with the
[Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/).

## Getting started

- Install Knative Serving

```bash
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-core.yaml
```

- Install net-gateway-api

```
ko resolve -f config/ | kubectl apply -f -
```

- Then install Istio:

```bash
./third_party/istio-head/install-istio.sh istio-kind-no-mesh.yaml
kubectl apply -f third_party/istio-head/gateway/
```

- Configure Knative Serving to use the proper "ingress.class":

```bash
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"gateway-api.ingress.networking.knative.dev"}}'
```

- Configure Knative Serving to use the proper "ingress.class":

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
  to Kourier from your machine:

```bash
kubectl port-forward  -n istio-system $(kubectl get pod -n istio-system -l "app=istio-ingressgateway" --output=jsonpath="{.items[0].metadata.name}") 8080:8080

curl -v -H "Host: helloworld-go.default.127.0.0.1.nip.io" http://localhost:8080
```

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).
