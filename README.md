# Pomerium Kubernetes Ingress Controller

See [docs for usage details](https://www.pomerium.com/docs/k8s/ingress.html).

## How it works

- Deployment
  - containers
    - ingress-controller
      - Ingress controller: updates `Ingress` resources
      - Settings controller
        - set configuration to the databroker
        - set configuration to the `pomerium-bootstrap`
    - pomerium all-in-one
      - watches `config.yaml`, mounted as a `Secret` `pomerium-bootstrap`

## Quick start

### Checklist

[ ] Kubernetes version 1.19 or higher
[ ] Can configure access to one of the supported [Identity Providers](https://www.pomerium.com/docs/identity-providers/)
[ ] Can provision TLS certificates i.e. using `cert-manager`.

### Install

The below command would install Pomerium, along with the Pomerium Ingress Controller,
and create an `settings.ingress.pomerium.io` CRD that may be used to dynamically configure Pomerium.

```
kubectl apply -f https://raw.githubusercontent.com/pomerium/ingress-controller/main/deploy
```

### Configure IdP

Once applied, you need complete the Pomerium configuration by creating `Settings` CRD:

```yaml
apiVersion: ingress.pomerium.io/v1
kind: Settings
metadata:
  name: global
spec:
  authenticate:
    url: https://login.localhost.pomerium.io
  identityProvider:
    provider: see https://www.pomerium.com/reference/#identity-provider-name
    secret: pomerium-idp
  certificates:
    - login-localhost-pomerium-io
---
apiVersion: v1
stringData:
  client_id:
  client_secret:
kind: Secret
metadata:
  name: pomerium-idp
type: Opaque
```

### Session Persistence

By default, Pomerium stores its identity and session data in-memory.
There are two supported storage backends that you may use for data persistence:

1. Redis
2. PostgreSQL

## System Requirements

- [Pomerium](https://github.com/pomerium/pomerium) v0.15.0+
- Kubernetes v1.19.0+
- `networking.k8s.io/v1` Ingress versions supported

## Command Line Options

## Namespaces

Ingress Controller may either monitor all namespaces (default), or only selected few, provided as a comma separated list to `--namespaces` command line option.

## HTTPS endpoints

`Ingress` spec defines that all communications to the service should happen in cleartext. Pomerium supports HTTPS endpoints, including mTLS.

Annotate your `Ingress` with

```yaml
ingress.pomerium.io/secure_upstream: true
```

Additional TLS may be supplied by creating a Kubernetes secret(s) in the same namespaces as `Ingress` resource. Note we do not support file paths or embedded secret references.

- [`tls_client_secret`](https://pomerium.io/reference/#tls-client-certificate)
- [`tls_custom_ca_secret`](https://pomerium.io/reference/#tls-custom-certificate-authority)
- [`tls_downstream_client_ca_secret`](https://pomerium.io/reference/#tls-downstream-client-certificate-authority)

Note the referenced `tls_client_secret` must be a [TLS Kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets). `tls_custom_ca_secret` and `tls_downstream_client_ca_secret` must contain `ca.crt` containing a .PEM encoded (Base64-encoded DER format) public certificate.

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
