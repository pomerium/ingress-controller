resource "kubernetes_service_account" "controller" {
  metadata {
    name      = var.controller_service_account_name
    namespace = kubernetes_namespace.pomerium.metadata[0].name
    labels    = var.service_account_labels
  }
}

resource "kubernetes_service_account" "gen_secrets" {
  metadata {
    name      = var.gen_secrets_service_account_name
    namespace = kubernetes_namespace.pomerium.metadata[0].name
    labels    = var.service_account_labels
  }
}
