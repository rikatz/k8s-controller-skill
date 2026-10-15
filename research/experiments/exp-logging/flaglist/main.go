// Lists the flags each option registers on a fresh FlagSet.
package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func names(fs *flag.FlagSet) string {
	var n []string
	fs.VisitAll(func(f *flag.Flag) { n = append(n, f.Name) })
	sort.Strings(n)
	return strings.Join(n, " ")
}

func main() {
	fs := flag.NewFlagSet("zap", flag.ContinueOnError)
	(&zap.Options{}).BindFlags(fs)
	fmt.Println("zap Options.BindFlags:", names(fs))
	fs = flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	fmt.Println("klog.InitFlags:       ", names(fs))
	fs = flag.NewFlagSet("text", flag.ContinueOnError)
	textlogger.NewConfig().AddFlags(fs)
	fmt.Println("textlogger AddFlags:  ", names(fs))
}
