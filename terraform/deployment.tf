resource "kubernetes_deployment" "pomerium" {
  metadata {
    name      = var.deployment_name
    namespace = var.namespace_name
    labels    = var.deployment_labels
  }

  spec {
    replicas = var.deployment_replicas

    selector {
      match_labels = {
        "app.kubernetes.io/name" = "pomerium-ingress-controller"
      }
    }

    template {
      metadata {
        labels = {
          "app.kubernetes.io/name" = "pomerium-ingress-controller"
        }
      }

      spec {
        service_account_name             = kubernetes_service_account.controller.metadata[0].name
        termination_grace_period_seconds = 10

        security_context {
          run_as_non_root = true
        }

        node_selector = {
          "kubernetes.io/os" = "linux"
        }

        container {
          name              = "pomerium-ingress-controller"
          image             = "${var.image_repository}:${var.image_tag}"
          image_pull_policy = var.image_pull_policy

          args = compact([
            "all-in-one",
            "--pomerium-config=${var.pomerium_config_name}",
            "--update-status-from-service=${var.namespace_name}/pomerium-proxy",
            "--metrics-bind-address=$(POD_IP):9090",
            var.enable_databroker ? "--databroker-auto-tls=pomerium-databroker.${var.namespace_name}.svc" : null,
          ])

          env {
            name  = "TMPDIR"
            value = "/tmp"
          }

          env {
            name  = "XDG_CACHE_HOME"
            value = "/tmp"
          }

          env {
            name = "POD_IP"
            value_from {
              field_ref {
                field_path = "status.podIP"
              }
            }
          }

          port {
            container_port = 8443
            name           = "https"
            protocol       = "TCP"
          }

          port {
            container_port = 8080
            name           = "http"
            protocol       = "TCP"
          }

          port {
            container_port = 9090
            name           = "metrics"
            protocol       = "TCP"
          }

          dynamic "port" {
            for_each = var.enable_databroker ? [1] : []
            content {
              container_port = 5443
              name           = "databroker"
              protocol       = "TCP"
            }
          }

          resources {
            limits = {
              cpu    = var.resources_limits_cpu
              memory = var.resources_limits_memory
            }

            requests = {
              cpu    = var.resources_requests_cpu
              memory = var.resources_requests_memory
            }
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true
            run_as_group               = 65532
            run_as_non_root            = true
            run_as_user                = 65532
          }

          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
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

        volume {
          name = "tmp"

          empty_dir {}
        }
      }
    }
  }
}
