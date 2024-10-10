
First install Pomerium Ingress Controller

```terraform
provider "kubernetes" {

}

module "pomerium_ingress_controller" {
    source = "git:https://github.com/pomerium/ingress-controller//terraform?ref=v0.28.0"
}
```

Once Pomerium Ingress Controller is installed, you may reference additional configurations via the `Pomerium` CRD.
See https://www.pomerium.com/docs/k8s/configure

```terraform
resource "kubernetes_manifest" "pomerium_config" {
    manifest = {
        apiVersion = "ingress.pomerium.io/v1"
        kind = "Pomerium"
        metadata = {
            name = "global"
        }
        spec = {
            secrets = "pomerium-ingress-controller/bootstrap"
        }
    }
}
```
