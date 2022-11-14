# Pomerium Kubernetes Ingress Controller

See [docs for usage details](https://www.pomerium.com/docs/k8s/ingress) for end-user details.

# Operation Modes

- _All in one_ launches Pomerium and Ingress Controller in-process. This is easiest to use, and is recommended for most users.
- _Controller only_ only runs ingress controller that communicates to a remote Pomerium cluster. Running Pomerium in split mode is only required to satisfy some very specific deployment requirements, and successful operation requires deep understanding of inter-component interaction. Please reach out to us first if you believe you absolutely need deploy in that mode.

# Installation

See [Quick Start](https://www.pomerium.com/docs/k8s/quickstart) for a step-by-step guide.

```shell
kubectl apply -f https://raw.githubusercontent.com/pomerium/ingress-controller/v0.19.0/deployment.yaml
```

The manifests-based installation:

- Creates `pomerium` namespace.
- Creates `pomerium.ingress.pomerium.io` cluster-scoped CRD.
- Creates `pomerium` `IngressClass`. Assign that `IngressClass` to the `Ingress` objects that should be managed by Pomerium.
- All-in-one Pomerium deployment with a single replica is created.
- Pomerium expects a `pomerium` CRD named `global` to be created.
- A one time `Job` to generate `pomerium/bootstrap` secrets, that have to be referenced from the CRD via `secrets` parameter.

Pomerium requires further configuration to become operational (see below).

# Configuration

Default Pomerium deployment is configured to watch `global` CRD.
[Pomerium should be configured via the CRD](https://www.pomerium.com/docs/k8s/reference).

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

# Customizing your deployment

`deployment.yaml` deploys a single Pomerium replica into `pomerium` namespace.

That deployment file is built via `kubectl kustomize config/default > deployment.yaml`.
