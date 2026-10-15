// Prints the same log calls through each backend that yields a logr.Logger.
// Usage: go run ./backends <zap-json|zap-console|zap-dev|klog|textlogger|slog-json|slog-text> [-v N]
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func lvl(v int) zapcore.LevelEnabler { return uzap.NewAtomicLevelAt(zapcore.Level(-v)) }

func build(name string, v int) logr.Logger {
	switch name {
	case "zap-json":
		return zap.New(zap.WriteTo(os.Stdout), zap.Level(lvl(v)))
	case "zap-console":
		return zap.New(zap.WriteTo(os.Stdout), zap.Level(lvl(v)), zap.ConsoleEncoder())
	case "zap-dev":
		return zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true))
	case "klog":
		var fs flag.FlagSet
		klog.InitFlags(&fs)
		_ = fs.Set("v", fmt.Sprint(v))
		return klog.NewKlogr()
	case "textlogger":
		return textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(v), textlogger.Output(os.Stdout)))
	case "slog-json":
		return logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: slog.Level(-v)}))
	case "slog-text":
		return logr.FromSlogHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: slog.Level(-v)}))
	}
	panic("unknown backend " + name)
}

func main() {
	v := flag.Int("v", 1, "verbosity")
	odd := flag.Bool("odd", false, "also log invalid key/value lists (panics with zap dev mode)")
	flag.Parse()
	l := build(flag.Arg(0), *v).WithName("demo")
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}}
	l = l.WithValues("controller", "widget")
	l.Info("Created Pod", "pod", klog.KObj(pod), "replicas", 3, "elapsed", 1500*time.Millisecond)
	l.Info("Ref only", "node", klog.KRef("", "node-a"), "ref", klog.KRef("ns", "x"))
	l.V(1).Info("Skipped no-op patch", "name", "p1")
	l.V(2).Info("Cache hit", "name", "p1")
	l.Error(errors.New("boom"), "Could not update Pod", "pod", klog.KObj(pod))
	l.Error(nil, "Invalid state without error object", "key", "v")
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ns1"}, Data: map[string][]byte{"password": []byte("hunter2")}, StringData: map[string]string{"tok": "sekrit"}}
	l.Info("Logged whole Secret", "secret", sec)
	l.Info("Logged Secret by reference", "secret", klog.KObj(sec))
	if *odd {
		l.Info("Odd key value list", "onlykey")
		l.Info("Non-string key", 42, "v")
	}
	l.Info("Multi\nline message")
}
