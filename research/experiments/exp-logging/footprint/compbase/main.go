package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	opts := logs.NewOptions()
	logsv1.AddFlags(opts, pflag.CommandLine)
	pflag.Parse()
	if err := logsv1.ValidateAndApply(opts, nil); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ctrl.SetLogger(klog.Background())
	klog.Background().Info("Started")
}
