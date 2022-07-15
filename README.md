# Pomerium Kubernetes Ingress Controller

See [docs for usage details](https://www.pomerium.com/docs/k8s/ingress) for end-user details.

# Operation Modes

- _All in one_ launches Pomerium and Ingress Controller in-process. This is easiest to use, and is recommended for most users.
- _Controller only_ only runs ingress controller that communicates to a remote Pomerium cluster. Running Pomerium in split mode is only required to satisfy some very specific deployment requirements, and successful operation requires deep understanding of inter-component interaction. Please reach out to us first if you believe you absolutely need deploy in that mode.

# Installation

```
kubectl apply -f https://raw.githubusercontent.com/pomerium/ingress-controller/main/deployment.yaml
```

- `pomerium` namespace is created that would contain an installation.
- `pomerium.ingress.pomerium.io` cluster-scoped CRD is created.
- `pomerium` `IngressClass`. Assign that `IngressClass` to the `Ingress` objects that should be managed by Pomerium.
- All-in-one Pomerium deployment with a single replica is created.
- Pomerium expects a `pomerium` CRD named `global` to be created.
- A one time `Job` to generate `pomerium/bootstrap` secrets, that have to be referenced from the CRD via `secrets` parameter.

# Configuration

Default Pomerium deployment is configured to watch `global` CRD.
Most Pomerium configuration could be set via CRD.

```yaml
apiVersion: ingress.pomerium.io/v1
kind: Pomerium
metadata:
  name: global
spec:
  authenticate:
    url: https://authenticate.localhost.pomerium.io
  certificates:
    - pomerium/wildcard-localhost-pomerium-io
  identityProvider:
    provider: xxxxxxx
    secret: pomerium/idp
  secrets: pomerium/bootstrap
```

_Note:_: the configuration must be complete. i.e. if you're missing a referenced secret, it would not be accepted.

# Inspecting the state

Use `kubectl describe pomerium` to assess the status of your Pomerium installation(s).
In case Ingress or Pomerium configuration resources were not successfully reconciled, the errors would bubble up here.

```
Status:
  Ingress:
    pomerium/envoy:
      Observed At:          2022-07-15T15:41:43Z
      Observed Generation:  5
      Reconciled:           true
    pomerium/httpbin:
      Observed At:          2022-07-15T15:41:43Z
      Observed Generation:  1
      Reconciled:           true
  Settings Status:
    Observed At:          2022-07-15T15:41:44Z
    Observed Generation:  5
    Reconciled:           true
Events:
  Type    Reason   Age   From                                 Message
  ----    ------   ----  ----                                 -------
  Normal  Updated  13m   bootstrap-pomerium-584b89f6c8-tdbgh  config updated
  Normal  Updated  13m   bootstrap-pomerium-584b89f6c8-g2gxk  config updated
  Normal  Updated  13m   pomerium-crd                         config updated
```

# Session Persistence

Pomerium requires a storage backend for user sessions. An in-memory backend is used by default.
You should use a storage backend for production multi-user deployments and/or multiple replicas.

PostgreSQL is a recommended persistence backend for new deployments.

## Ingress annotations

Pomerium supports `Ingress` `v1` resource.

- Only `Ingress` resources with `pomerium` `IngressClass` would be managed, unless the `pomerium` `IngressClass` is marked as default controller.
- Pomerium-specific options are supplied via `Ingress` annotations. See https://www.pomerium.com/docs/k8s/ingress#supported-annotations

# TLS

- only TLS `Ingress` resources are supported. Pomerium is not designed for cleartext HTTP.
- Pomerium-managed `Ingress` resources may have TLS certificates provisioned by `cert-manager`.
- Pomerium may be used as `HTTP01` ACME challenge solver for `cert-manager`.
- You may also provide certificates via `Secrets`, referenced by `Pomerium` CRD `certificates` parameter.
