// Package demo only carries the go:generate directives for the whole module.
package demo

//go:generate go tool controller-gen object paths=./api/...
//go:generate go tool controller-gen crd paths=./api/... output:crd:artifacts:config=config/crd/bases
//go:generate go tool controller-gen rbac:roleName=manager-role paths=./internal/... output:rbac:artifacts:config=config/rbac
//go:generate go tool controller-gen webhook paths=./internal/... output:webhook:artifacts:config=config/webhook
//go:generate go tool controller-gen applyconfiguration paths=./api/...
