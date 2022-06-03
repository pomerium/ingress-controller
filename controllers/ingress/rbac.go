package ingress

// https://book.kubebuilder.io/reference/markers/rbac.html

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/secrets,verbs=update

//+kubebuilder:rbac:groups=core.k8s.io,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=core.k8s.io,resources=secrets/status,verbs=get
//+kubebuilder:rbac:groups=core.k8s.io,resources=secrets/secrets,verbs=update

//+kubebuilder:rbac:groups=core.k8s.io,resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=core.k8s.io,resources=services/status,verbs=get
//+kubebuilder:rbac:groups=core.k8s.io,resources=services/secrets,verbs=update
