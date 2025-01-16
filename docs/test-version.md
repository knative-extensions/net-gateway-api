<!--
  This documentation is inserted in release note for each release.
  All variables are defined in .
-->

The following Gateway API version and Ingress were tested as part of the release.

### Tested Gateway API version

| Tested Gateway API       |
| ------------------------ |
| v1.2.1 |

### Tested Ingress

| Ingress | Tested version          | Unavailable features           |
| ------- | ----------------------- | ------------------------------ |
| Istio   | v1.24.2     | retry,httpoption   |
| Contour | v1.30.2    | httpoption |
| Envoy Gateway | v1.2.5    | httpoption,host-rewrite |
