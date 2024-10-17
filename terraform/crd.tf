resource "kubernetes_manifest" "pomerium_crd" {
  manifest = yamldecode(file("${path.module}/crd.yaml"))
}
