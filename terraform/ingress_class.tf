resource "kubernetes_ingress_class" "pomerium" {
  metadata {
    name   = var.ingress_class_name
    labels = var.labels
  }

  spec {
    controller = "pomerium.io/ingress-controller"
  }
}
