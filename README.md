# Knative net-gateway-api

[![GoDoc](https://godoc.org/knative.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative/net-gateway-api)](https://goreportcard.com/report/knative/net-ingressv2)

This repository contains a KIngress implementation and testing for Knative
integration with the 
[Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/).
The tests are same with
[Knative networking](https://github.com/knative/networking/tree/main/test/conformance)
but against Gateway API resources without Knative Ingress.

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).
