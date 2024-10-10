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

    external_traffic_policy = var.proxy_service_type == "LoadBalancer" ? "Local" : null

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

    type = var.proxy_service_type
  }
}

resource "kubernetes_service" "databroker" {
  count = var.enable_databroker ? 1 : 0

  metadata {
    name      = "pomerium-databroker"
    namespace = kubernetes_namespace.pomerium.metadata[0].name
    labels    = var.service_labels
  }

  spec {
    selector = {
      "app.kubernetes.io/name" = "pomerium-ingress-controller"
    }

    port {
      name        = "databroker"
      port        = 443
      target_port = "databroker"
      protocol    = "TCP"
    }

    type = "ClusterIP"
  }
}
