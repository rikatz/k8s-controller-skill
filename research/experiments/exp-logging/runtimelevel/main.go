// Shows that V(2) can be switched on at runtime for each backend, and that zap's
// production mode samples repeated identical messages.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-logr/logr"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func probe(name string, l logr.Logger, buf *bytes.Buffer, enable func()) {
	l.V(2).Info("Before")
	enable()
	l.V(2).Info("After")
	out := buf.String()
	fmt.Printf("%-11s before-visible=%v after-visible=%v\n", name, strings.Contains(out, "Before"), strings.Contains(out, "After"))
}

func main() {
	// zap: pass an AtomicLevel and change it later.
	{
		var buf bytes.Buffer
		lvl := uzap.NewAtomicLevelAt(zapcore.InfoLevel)
		l := zap.New(zap.WriteTo(&buf), zap.Level(&lvl))
		probe("zap", l, &buf, func() { lvl.SetLevel(zapcore.Level(-2)) })
	}
	// textlogger: Config.Verbosity() is documented as settable at runtime.
	{
		var buf bytes.Buffer
		cfg := textlogger.NewConfig(textlogger.Output(&buf))
		probe("textlogger", textlogger.NewLogger(cfg), &buf, func() { _ = cfg.Verbosity().Set("2") })
	}
	// klog global: flag value set at runtime.
	{
		var fs flag.FlagSet
		klog.InitFlags(&fs)
		_ = fs.Set("logtostderr", "false")
		_ = fs.Set("stderrthreshold", "FATAL")
		var buf bytes.Buffer
		klog.SetOutput(&buf)
		_ = fs.Set("v", "0")
		probe("klog", klog.NewKlogr(), &buf, func() { _ = fs.Set("v", "2") })
		klog.Flush()
	}
	// slog: LevelVar, V(n) is slog level -n.
	{
		var buf bytes.Buffer
		var lv slog.LevelVar // default Info
		l := logr.FromSlogHandler(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: &lv}))
		probe("slog", l, &buf, func() { lv.Set(slog.Level(-2)) })
	}
	// zap flags: --zap-log-level=N enables V(N); "debug" enables V(1) only.
	for _, arg := range []string{"debug", "2", "info"} {
		var buf bytes.Buffer
		var fs flag.FlagSet
		opts := zap.Options{DestWriter: &buf}
		opts.BindFlags(&fs)
		_ = fs.Parse([]string{"--zap-log-level=" + arg})
		l := zap.New(zap.UseFlagOptions(&opts))
		l.V(1).Info("V1")
		l.V(2).Info("V2")
		out := buf.String()
		fmt.Printf("--zap-log-level=%-5s V(1) visible=%v V(2) visible=%v\n", arg, strings.Contains(out, "V1"), strings.Contains(out, "V2"))
	}
	// klog -vmodule: enable V(1) for this file only, through the logr wrapper.
	{
		var fs flag.FlagSet
		klog.InitFlags(&fs)
		_ = fs.Set("logtostderr", "false")
		_ = fs.Set("stderrthreshold", "FATAL")
		var buf bytes.Buffer
		klog.SetOutput(&buf)
		_ = fs.Set("v", "0")
		_ = fs.Set("vmodule", "main=1")
		klog.NewKlogr().V(1).Info("Vmodule line")
		klog.Flush()
		fmt.Println("klog -vmodule=main=1 enables V(1) via logr:", strings.Contains(buf.String(), "Vmodule line"))
		_ = fs.Set("vmodule", "")
		_ = fs.Set("v", "0")
	}
	// klog and slog do not sample identical lines.
	{
		var buf bytes.Buffer
		l := logr.FromSlogHandler(slog.NewJSONHandler(&buf, nil))
		for i := 0; i < 300; i++ {
			l.Info("Same message", "i", i)
		}
		fmt.Println("slog: 300 identical Info calls, lines kept =", strings.Count(buf.String(), "\n"))
		buf.Reset()
		var fs flag.FlagSet
		klog.InitFlags(&fs)
		_ = fs.Set("logtostderr", "false")
		_ = fs.Set("stderrthreshold", "FATAL")
		klog.SetOutput(&buf)
		for i := 0; i < 300; i++ {
			klog.NewKlogr().Info("Same message", "i", i)
		}
		klog.Flush()
		fmt.Println("klog: 300 identical Info calls, lines kept =", strings.Count(buf.String(), "Same message"))
	}
	// zap production sampling: 300 identical messages within one second.
	{
		var buf bytes.Buffer
		l := zap.New(zap.WriteTo(&buf))
		for i := 0; i < 300; i++ {
			l.Info("Same message", "i", i)
		}
		fmt.Println("zap production: wrote 300 identical Info calls, lines kept =", strings.Count(buf.String(), "\n"))
		buf.Reset()
		l = zap.New(zap.WriteTo(&buf), zap.Level(uzap.NewAtomicLevelAt(zapcore.Level(-2))))
		for i := 0; i < 300; i++ {
			l.Info("Same message", "i", i)
		}
		fmt.Println("zap level>=V(2) (sampling off): lines kept =", strings.Count(buf.String(), "\n"))
	}
}
