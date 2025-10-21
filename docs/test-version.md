<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v1.3.0 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.27.1     | retry,httpoption   |
| Contour | v1.33.0    | httpoption |
| Envoy Gateway | v1.5.1    | httpoption,host-rewrite |
