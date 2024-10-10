resource "kubernetes_namespace" "pomerium" {
  metadata {
    name   = var.namespace_name
    labels = var.labels
  }
}
