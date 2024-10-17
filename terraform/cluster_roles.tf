resource "kubernetes_cluster_role" "controller" {
  metadata {
    name   = var.controller_cluster_role_name
    labels = var.cluster_role_labels
  }

  rule {
    api_groups = [""]
    resources  = ["services", "endpoints", "secrets"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["services/status", "secrets/status", "endpoints/status"]
    verbs      = ["get"]
  }

  rule {
    api_groups = ["networking.k8s.io"]
    resources  = ["ingresses", "ingressclasses"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["networking.k8s.io"]
    resources  = ["ingresses/status"]
    verbs      = ["get", "patch", "update"]
  }

  rule {
    api_groups = ["ingress.pomerium.io"]
    resources  = ["pomerium"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["ingress.pomerium.io"]
    resources  = ["pomerium/status"]
    verbs      = ["get", "update", "patch"]
  }

  rule {
    api_groups = [""]
    resources  = ["events"]
    verbs      = ["create", "patch"]
  }
}

resource "kubernetes_cluster_role" "gen_secrets" {
  metadata {
    name   = var.gen_secrets_cluster_role_name
    labels = var.cluster_role_labels
  }

  rule {
    api_groups = [""]
    resources  = ["secrets"]
    verbs      = ["create"]
  }
}
