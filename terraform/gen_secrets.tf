resource "kubernetes_job" "gen_secrets" {
  metadata {
    name      = var.job_name
    namespace = var.namespace_name
    labels    = var.deployment_labels
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

        node_selector = {
          "kubernetes.io/os" = "linux"
        }

        container {
          name  = "gen-secrets"
          image = "${var.image_repository}:${var.image_tag}"
          image_pull_policy = "IfNotPresent"

          args = [
            "gen-secrets",
            "--secrets=$(POD_NAMESPACE)/bootstrap",
          ]

          env {
            name = "POD_NAMESPACE"
            value_from {
              field_ref {
                field_path = "metadata.namespace"
              }
            }
          }

          security_context {
            allow_privilege_escalation = false
          }
        }
      }
    }
  }
}
