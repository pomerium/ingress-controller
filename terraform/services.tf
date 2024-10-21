resource "kubernetes_service" "proxy" {
  count = var.proxy_service_type == null ? 0 : 1

  metadata {
    name      = "pomerium-proxy"
    namespace = kubernetes_namespace.pomerium.metadata[0].name
    labels    = var.service_labels
  }

  lifecycle {
    ignore_changes = [
      metadata[0].annotations
    ]
  }

  spec {
    selector = {
      "app.kubernetes.io/name" = "pomerium-ingress-controller"
    }

    external_traffic_policy = var.proxy_service_type == "LoadBalancer" ? "Local" : null

    port {
      name        = "https"
      port        = var.proxy_port_https
      node_port   = var.proxy_node_port_https
      target_port = "https"
      protocol    = "TCP"
    }

    dynamic "port" {
      for_each = var.proxy_port_http != null ? [var.proxy_port_http] : []
      content {
        name        = "http"
        port        = port.value
        node_port   = var.proxy_node_port_http
        target_port = "http"
        protocol    = "TCP"
      }
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

  lifecycle {
    ignore_changes = [
      metadata[0].annotations
    ]
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
