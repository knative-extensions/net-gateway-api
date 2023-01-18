<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v0.6.0 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.16.1     | retry,httpoption,host-rewrite   |
| Contour | v1.23.0    | httpoption,basics/http2,websocket,websocket/split,grpc,grpc/split,update,host-rewrite |
