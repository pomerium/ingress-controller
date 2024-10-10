resource "kubernetes_service" "proxy" {
  metadata {
    name      = "pomerium-proxy"
    namespace = kubernetes_namespace.pomerium.metadata[0].name
    labels    = var.service_labels
  }

  spec {
    selector = {
      "app.kubernetes.io/name" = "pomerium-ingress-controller"
    }

    external_traffic_policy = var.service_type == "LoadBalancer" ? "Local" : null

    port {
      name        = "https"
      port        = 443
      target_port = "https"
      protocol    = "TCP"
    }

    port {
      name        = "http"
      port        = 80
      target_port = "http"
      protocol    = "TCP"
    }

    type = var.service_type
  }
}
