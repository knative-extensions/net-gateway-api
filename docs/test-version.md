<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v0.3.0 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.11.5     | tls,retry,httpoption   |
| Contour | v1.19.1    | tls,retry,httpoption,basics/http2,websocket,websocket/split,grpc,grpc/split,visibility/path,update,host-rewrite |
