<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v0.8.1 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.19.3     | retry,httpoption   |
| Contour | v1.28.1    | httpoption,basics/http2,grpc,grpc/split,update |
