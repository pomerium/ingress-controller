resource "kubernetes_job" "gen_secrets" {
  metadata {
    name      = var.job_name
    namespace = var.namespace_name
    labels    = var.deployment_labels
  }

  lifecycle {
    ignore_changes = [
      metadata[0].annotations
    ]
  }

  spec {
    template {
      metadata {
        name   = var.job_name
        labels = var.deployment_labels
      }

      spec {
        service_account_name = kubernetes_service_account.gen_secrets.metadata[0].name
        restart_policy       = "OnFailure"

        security_context {
          fs_group        = 1000
          run_as_non_root = true
          run_as_user     = 1000
        }

        node_selector = merge(local.default_node_selector, var.node_selector)

        container {
          name              = "gen-secrets"
          image             = "${var.image_repository}:${var.image_tag}"
          image_pull_policy = "IfNotPresent"

          args = [
            "gen-secrets",
            "--secrets=${var.namespace_name}/bootstrap",
          ]

          security_context {
            allow_privilege_escalation = false
          }
        }

        dynamic "toleration" {
          for_each = var.tolerations
          content {
            key                = lookup(toleration.value, "key", null)
            operator           = lookup(toleration.value, "operator", null)
            value              = lookup(toleration.value, "value", null)
            effect             = lookup(toleration.value, "effect", null)
            toleration_seconds = lookup(toleration.value, "toleration_seconds", null)
          }
        }
      }
    }
  }
}
