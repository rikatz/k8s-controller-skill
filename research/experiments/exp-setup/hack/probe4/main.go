package main

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	ctrl.SetupSignalHandler()
	defer func() { fmt.Println("recovered:", recover()) }()
	ctrl.SetupSignalHandler()
}
