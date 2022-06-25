<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v0.5.0-rc1 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.13.2     | tls,retry,httpoption,host-rewrite   |
| Contour | 11d1f703bfd94e567960a402d0fcdd9c36901e6e    | tls,retry,httpoption,basics/http2,websocket,websocket/split,grpc,grpc/split,visibility/path,visibility,update,host-rewrite |
