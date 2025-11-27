<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v1.4.0 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.28.0     | retry,httpoption   |
| Contour | v1.33.0    | httpoption |
| Envoy Gateway | v1.6.0    | httpoption,host-rewrite |
