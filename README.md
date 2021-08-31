# Pomerium Kubernetes Ingress Controller

## System Requirements

- [Pomerium](https://github.com/pomerium/pomerium) v0.15.0+
- Kubernetes v0.19.0+
- `networking.k8s.io/v1` Ingress versions supported

## Command Line Options

## IngressClass

Create [`IngressClass`](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-class)
for Pomerium Ingress Controller.

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: pomerium
  annotations:
    ingressclass.kubernetes.io/is-default-class: "false"
spec:
  controller: pomerium.io/ingress-controller
```

Use `ingressclass.kubernetes.io/is-default-class: "true"` to mark Pomerium as default controller for your cluster
and manage `Ingress` resources that do not specify an ingress controller in `ingressClassName`.

# HTTP-01 solvers

In order to use [`http-01`](https://cert-manager.io/docs/configuration/acme/http01/#configuring-the-http01-ingress-solver) ACME challenge solver, the following Pomerium configuration parameters must be set:

- [`AUTOCERT: false`](https://www.pomerium.io/reference/#autocert) (default)
- [`HTTP_REDIRECT_ADDR: ':80'`](https://www.pomerium.io/reference/#http-redirect-address)
