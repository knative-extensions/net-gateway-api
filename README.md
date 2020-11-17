# Knative Sample Controller

[![GoDoc](https://godoc.org/knative.dev/net-ingressv2?status.svg)](https://godoc.org/knative.dev/net-ingressv2)
[![Go Report Card](https://goreportcard.com/badge/knative/net-ingressv2)](https://goreportcard.com/report/knative/net-ingressv2)

Knative `net-ingressv2` defines a few simple resources that are validated by
webhook and managed by a controller to demonstrate the canonical style in which
Knative writes controllers.

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).

# Install service-apis

kubectl apply -k 'github.com/kubernetes-sigs/service-apis/config/crd?ref=v0.1.0-rc2'

KNATIVE_NAMESPACE="knative-serving"
kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"ingress.class":"ingressv2.ingress.networking.knative.dev"}}'
