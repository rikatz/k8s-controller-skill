// Caller info per backend: where does the reported file:line point?
package main

import (
	"os"

	"github.com/go-logr/logr"
	uzap "go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	var l logr.Logger = zap.New(zap.WriteTo(os.Stdout), zap.RawZapOpts(uzap.AddCaller()))
	l.Info("Caller here")
	l.WithName("sub").WithValues("a", "b").Info("Caller via WithName")
}
