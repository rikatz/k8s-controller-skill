// Without SetLogger: lines are dropped; a notice with a stack trace appears on stderr only when a log
// call happens after 30 s. Run takes ~31 s.
package main

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	l := ctrl.Log.WithName("setup")
	l.Info("Line at t=0")
	time.Sleep(31 * time.Second)
	l.Info("Line at t=31s")
	l.Info("Line at t=31s, second")
}
