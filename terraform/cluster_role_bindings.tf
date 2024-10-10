resource "kubernetes_cluster_role_binding" "controller" {
  metadata {
    name   = var.controller_cluster_role_name
    labels = var.cluster_role_labels
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.controller.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.controller.metadata[0].name
    namespace = kubernetes_namespace.pomerium.metadata[0].name
  }
}

resource "kubernetes_cluster_role_binding" "gen_secrets" {
  metadata {
    name   = var.gen_secrets_cluster_role_name
    labels = var.cluster_role_labels
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.gen_secrets.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.gen_secrets.metadata[0].name
    namespace = kubernetes_namespace.pomerium.metadata[0].name
  }
}
