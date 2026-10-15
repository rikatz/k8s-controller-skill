package x

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func F() {}
