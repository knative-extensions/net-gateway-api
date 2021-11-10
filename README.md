# Knative net-gateway-api

**[This component is ALPHA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

[![GoDoc](https://godoc.org/knative-sandbox.dev/net-gateway-api?status.svg)](https://godoc.org/knative.dev/net-gateway-api)
[![Go Report Card](https://goreportcard.com/badge/knative-sandbox/net-gateway-api)](https://goreportcard.com/report/knative-sandbox/net-gateway-api)

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
