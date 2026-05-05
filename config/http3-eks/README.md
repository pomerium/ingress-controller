# HTTP/3 Support for EKS

By default EKS will complain about a multi-protocol (TCP+UDP) load balancer. This kustomization will make the necessary changes to the pomerium proxy service to support HTTP/3.

These changes were taken from Amazon's [documentation](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/use_cases/quic/).
