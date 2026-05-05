# HTTP/3 Support for GKE

By default GKE will complain about a multi-protocol (TCP+UDP) load balancer. This kustomization will make the necessary changes to the pomerium proxy service to support HTTP/3.

These changes were taken from Google's [documentation](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/mixed-protocol-lb).
