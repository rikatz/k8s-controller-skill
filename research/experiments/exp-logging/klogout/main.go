// Quirk: klog.SetOutput(w) (with logtostderr=false) writes each Error line to w once per severity level (3x here).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

func main() {
	var fs flag.FlagSet
	klog.InitFlags(&fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("stderrthreshold", "FATAL")
	var buf bytes.Buffer
	klog.SetOutput(&buf)
	klog.NewKlogr().Error(errors.New("boom"), "Could not update Pod")
	klog.NewKlogr().Info("Created Pod")
	klog.Flush()
	fmt.Printf("Error lines=%d Info lines=%d\n", strings.Count(buf.String(), "Could not update Pod"), strings.Count(buf.String(), "Created Pod"))
}
