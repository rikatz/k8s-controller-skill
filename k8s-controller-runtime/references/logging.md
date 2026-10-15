# Logging

Read this when you pick a logging backend, wire `main`, write log lines in a reconciler, or decide which level a line
belongs at. This file owns all logging guidance; other references only point here. Snippets compile against
controller-runtime v0.25.2, logr v1.4.3, klog v2.140.0, client-go v0.37.1 (imports omitted). Tags are defined in
`sources.md`; `run` = reproduced in `research/experiments/exp-logging`. No performance claim is made: nothing was benchmarked.

## 1. Recommended default

- Application code logs through exactly one abstraction: the logr `Logger` from `ctrl.LoggerFrom(ctx)`. controller-runtime
  only speaks logr, so the backend is chosen once in `main` and never appears in reconciler code. [CR]
- **Default for a new controller: klog as the backend.** Two lines, no new module, one verbosity flag (`-v`), and one output format for
  controller-runtime, client-go and your own lines with nothing to route. [CR, K8S, run]

```go
func main() {
	klog.InitFlags(nil) // -v, -vmodule (plus klog's file/stderr flags) on flag.CommandLine
	flag.Parse()
	ctrl.SetLogger(klog.Background()) // same form as CAPI and CAPV main.go
	// ...
}
```

- Why klog: it is already in the dependency graph (client-go, and controller-runtime imports it) so go.mod gains no module (klog/v2 moves from
  indirect to direct); CAPI, CAPV and sample-controller's `main.go` all log through klog; client-go logs through klog, so no second format appears. [CAPI, CAPV, run]
- **Deviate to controller-runtime's zap** (section 4 wiring) when you need JSON output on stdout, stack traces on errors, or
  the name/namespace trimming of logged objects (section 3). Same logr API in your code, still no new dependency, but
  one more wiring line and two flag families. [CR, run]
- Other routes: `k8s.io/component-base/logs/api/v1` gives `--logging-format=json` plus klog flags (what CAPI and CAPV use) at
  the cost of +39 packages (section 3); `log/slog` works cleanly through logr but CAPI and CAPV have no non-test `log/slog` import, and it has the rough edges in
  the table. [CAPI, CAPV, run]
- Whatever you choose, record the reason: a different backend is a deviation, not a default.

## 2. What controller-runtime does with the logger

- `ctrl.SetLogger(l)` must run first. Without it every line is dropped; the stderr notice with a stack trace appears only when
  a log call happens after 30 s (a line at t=0 and one at t=31 s: both lost, one notice printed). [CR, run]
- Reconcile context logger carries `controller`, `controllerGroup`, `controllerKind`, `<Kind>` (a `klog.KRef`), `namespace`,
  `name` and `reconcileID`. Fetch it with `ctrl.LoggerFrom(ctx)` (falls back to the root logger if the context has none);
  put an enriched one back with `ctrl.LoggerInto(ctx, l)`. [CR, run]
- `WithName` adds a `logger` key (names joined with `.` on zap, `/` on slog); `WithValues` adds pairs to every later line. Do `WithValues` once per
  reconcile, then pass `ctx` down; helpers and client-go calls that receive that `ctx` inherit the pairs (a client-go
  "Waited before sending request" line printed inside a reconcile carried `controller`, `reconcileID` and the Widget ref). [CR, run]
- controller-runtime's own per-reconcile lines ("Reconciling", "Reconcile successful", "Reconcile done, requeueing") are `V(5)`,
  so they are invisible unless you run at `-v=5`; do not add your own per-reconcile Info. [CR, run]

## 3. Backends compared

Same calls through each backend (`exp-logging/backends`); size numbers are a trivial `main` importing `ctrl` (`exp-logging/footprint`).

| | klog (`klog.Background()`) | zap (`pkg/log/zap`) | `log/slog` via `logr.FromSlogHandler` |
|---|---|---|---|
| go.mod additions | none | none | none (stdlib; logr already required) |
| binary / packages vs `ctrl` only | +0.0 MB / +0 | +1.4 MB / +13 | -0.01 MB / +0 |
| Format | klog text only | JSON (production), console (dev or `--zap-encoder=console`) | JSON or text handler |
| Verbosity | `-v N`, `-vmodule` | `--zap-log-level=N\|debug\|info` (V(1) is `debug`, V(N>=2) prints as `Level(-N)`) | handler `Level`; V(n) = slog level -n, so V(1) prints `DEBUG+3`, V(4) is `DEBUG` |
| Change at runtime | `flag.Set("v", ...)` | `zap.AtomicLevel.SetLevel` | `slog.LevelVar.Set` |
| Caller | `main.go:58` in header | none by default; `zap.RawZapOpts(uzap.AddCaller())` is correct through logr | `AddSource: true`, correct line |
| `Error(err, ...)` | `err="boom"`, no stack | `"error":"boom"` plus `stacktrace` (production: Error and up, dev: Warn and up) | `err=boom`, no stack |
| `klog.KObj(pod)` | `pod="ns1/p1"` | `{"name":"p1","namespace":"ns1"}` | text `pod.name=p1 pod.namespace=ns1`, JSON object |
| Whole `*corev1.Secret` as a value | full dump incl. `Data` bytes | name/namespace only; **dev mode: full dump incl. `Data`** | JSON incl. base64 `data` |
| Odd key/value list | `onlykey="(MISSING)"` | production: extra `dpanic` line, pair dropped; **dev mode: panic** | `!BADKEY=onlykey` |
| `url.URL` value | string | string | JSON struct `{"Scheme":...}` (no Stringer use) |
| 300 identical Info calls | 300 kept | production **samples**: 102 kept | 300 kept |

- klog.KRef/KObj render sensibly on all three backends and keep the key stable, so the CAPI rule "MUST use klog.KObj or
  klog.KRef for objects" holds regardless of backend. [CAPI, run]
- component-base `logsv1` route: JSON line is `{"ts":...,"caller":"main.go:24","msg":"Started","v":0}`; adds three indirect modules
  (semver, otel, otel/trace), `pflag` becomes direct, +1.8 MB / +39 packages. CAPI and CAPV use it with `klog.Background()`. [CAPI, CAPV, run]
- textlogger (`k8s.io/klog/v2/textlogger`): `textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(0), textlogger.Output(w)))` gives the same text
  format as klog without klog's global flags; `cfg.AddFlags(fs)` registers `v` and `vmodule`, `cfg.Verbosity().Set("2")` changes the level at runtime. Use it when you
  want a klog-format logger without the global klog state; you still route client-go to it with `SetLoggerWithOptions`. `-vmodule` (file-name pattern) also works
  through logr on the klog backend. [K8S, run]
- slog needs nothing but `logr.FromSlogHandler(handler)` (and `logr.ToSlogHandler(logger)` for the other direction), both in logr
  v1.4.3. `WithName` becomes a `logger` attribute, `Error(err, ...)` an `err` attribute at `slog.LevelError`. Lost versus zap: stack traces,
  object trimming, V(n) readable level names (fixable only with `ReplaceAttr`; not tried). [run]

## 4. One output for controller-runtime, client-go and your code

client-go logs through klog: in non-test code of v0.37.1 there are 142 call sites of `klog.FromContext`/`Background`/`TODO`/`LoggerWithValues`,
13 `klog.V(n)` and 4 other legacy calls. With a non-klog backend, those lines come out in klog text format unless klog is pointed at your logger. [K8S, run]

```go
opts := zap.Options{}
opts.BindFlags(flag.CommandLine) // --zap-devel, --zap-encoder, --zap-log-level, --zap-stacktrace-level, --zap-time-encoding
flag.Parse()
logger := zap.New(zap.UseFlagOptions(&opts)) // slog: logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: ...}))
ctrl.SetLogger(logger)
klog.SetLoggerWithOptions(logger, klog.ContextualLogger(true)) // klog.Background() and FromContext now return logger
```

Proof (`exp-logging/unify`, envtest, manager with leader election, client limited to QPS=1, `-level 3`):

```
not wired (zap):  {"level":"info",...,"msg":"Annotating ConfigMap","controller":"configmap",...,"reconcileID":"0dd5..."}
                  I1015 15:59:41.037991 1348099 request.go:754] "Waited before sending request" ... URL="https://127.0.0.1:32931/apis"
wired (zap):      {"level":"Level(-3)",...,"msg":"Waited before sending request","verb":"GET","URL":"https://127.0.0.1:36925/apis"}
wired (slog):     {"time":...,"level":"INFO","msg":"Annotating ConfigMap","controller":"configmap",...,"reconcileID":"258f..."}
klog (no wiring): I1015 16:00:34.782573 1348651 request.go:685] "Waited before sending request" controller="configmap" ... reconcileID="9c88..." verb="PATCH" ...
```

In the wired runs and the klog run no line was in a second format (zap 47 lines, slog 48, klog 48). [run]

- `klog.SetLogger(l)` alone is not enough: client-go lines that have no logger in their context still go through klog's own `-v` check (default 0).
  Run with zap level 3: plain `SetLogger` kept 1 of 3 context-less client-go lines at klog `-v=0`, `ContextualLogger(true)` kept all 3. Use `SetLoggerWithOptions(..., ContextualLogger(true))`. [run]
- Legacy `klog.V(n)` calls (13 in client-go) are still gated by klog's `-v` even then, so they stay hidden unless you also register klog's flags and set `-v`. [K8S, run]
- Flags that remain: klog backend: klog's own (only `-v`, `-vmodule` matter once a logger is set). zap backend: `--zap-*` for the logger,
  optionally `klog.InitFlags` for legacy calls. `klog.InitFlags(nil)` plus your own `-v` flag panics with `flag redefined: v`;
  `textlogger.Config.AddFlags` registers only `v` and `vmodule`. [run]
- To send klog to a writer, use a logger with its own output (`textlogger.Output`), not `klog.SetOutput(w)` with `logtostderr=false`: that wrote each Error line
  three times to `w`. [run]

## 5. Message and key conventions

- Message: capitalised, no trailing period, constant text; variable data goes in key/value pairs, never `fmt.Sprintf`. The user convention and
  CAPI docs require the capital and no period; controller-runtime's `TMP-LOGGING.md` requires constant messages and pairs but is silent on
  capitals and periods, so those two rules go beyond it. [CAPI, CR]
- Say what happened and to what: "Created Deployment", not "Done"; past tense for outcomes ("Could not delete Pod"), active voice. CAPI prefers logging
  before the action ("Creating MachineSet") so an error line next to it is easy to correlate; use that form only where a failure would otherwise be hard to place. [K8S, CAPI]
- Keys: lowerCamelCase (`resourceVersion`, `oldReplicas`), Kinds as UpperCamel (`Widget`), `reason` for why and `diff` for a change (CAPI). `TMP-LOGGING.md` says lower-case
  space-separated keys, and controller-runtime itself emits both `reconcileID` and `worker count`: pick camelCase and stay consistent. [CAPI, CR, run]
- Objects: `klog.KObj(obj)` or `klog.KRef(ns, name)`, never the object (section 3 shows the Secret dump). Reference types by name in a key named after the Kind.
- Context, not globals: take the logger from `ctx`; do not keep a package-level logger or a logger field copied once at construction. [CR]
- Never log Secret values or whole objects that may reference credentials; log `klog.KObj(secret)` or its key names. [API, run]

```go
log := ctrl.LoggerFrom(ctx).WithValues("generation", cm.Generation) // once per reconcile
ctx = ctrl.LoggerInto(ctx, log)
log.V(1).Info("Using Secret", "secret", klog.KObj(sec)) // never "secret", sec
```

## 6. Which level to use

logr has two calls: `Info(msg, kv...)` with a `V(n)` level and `Error(err, msg, kv...)`. There is no `Warn` and no `Debug` method:
debug is `V(1).Info(...)`. `Error` has no verbosity and is always emitted; negative V levels behave as V(0). [run, logr godoc]

Scale: controller-runtime says use `V(0)` and `V(1)` only, because other numbers lose meaning. Kubernetes uses V(0)-V(5) with V(2) as the
steady-state default (state changes at V(2)); CAPI runs core controllers at `-v=2`, calls <=3 "what happened" and >=4 "how it happened", and forbids >5.
This skill follows controller-runtime: V(0) for what an operator must see, V(1) for debug, run at `-v=0` normally. Adopt the CAPI scale only when your
project already does (then default `-v=2`). [CR, K8S, CAPI]

| Situation | Level | Example (capitalised, no period, pairs) |
|---|---|---|
| State change an operator needs: created, updated, deleted, finalizer added/removed | Info `V(0)` | `log.Info("Created Deployment", "deployment", klog.KRef(ns, name))` |
| Expected but notable condition (the K8s guide logs these as Info) | Info `V(0)` | `log.Info("Skipped deletion, owner already gone", "reason", "ownerNotFound")` |
| Reconcile decision: no-op write skipped, requeue reason, waiting for a dependency, predicate dropped an event | Debug `V(1)` | `log.V(1).Info("Waiting for Secret", "secret", klog.KRef(ns, name))` |
| Background goroutine or handler where the error cannot be returned | Error | `log.Error(err, "Could not refresh inventory", "interval", "5m")` |
| Failure you handle by returning it from `Reconcile` | none (controller-runtime logs it) | `return ctrl.Result{}, fmt.Errorf("getting Secret: %w", err)` |
| Invalid state with no error value | Error with nil err | `log.Error(nil, "Unsupported phase", "phase", w.Status.Phase)` |
| NotFound on the object being reconciled, conflict you will retry, dependency not ready | none or `V(1)` | `return ctrl.Result{}, client.IgnoreNotFound(err)` |

**Error** is for something an administrator may need to act on and that nobody else will log. Do not use it for expected conditions (NotFound, a conflict
you retry, a dependency that is not ready yet), and do not log an error you also return: the K8s guide says a caller may handle it gracefully, so an error
log would wrongly alert; if you want it for debugging, use `V(4).Info` with an `err` key. [K8S] Run (`exp-logging/errorlog`, zap, level V(5)):

| `Reconcile` returns | controller-runtime logs | Requeue |
|---|---|---|
| `err` | Error `Reconciler error` with `err` and a stack trace that points into controller-runtime (`controller.go:497`), not your code | backoff |
| `reconcile.TerminalError(err)` | the same Error line (`terminal error: ...`) | none (1 call) |
| `err` and `RequeueAfter` | Info `Warning: Reconciler returned both a result with either RequeueAfter or Requeue set and a non-nil error...` then the Error line | backoff; `RequeueAfter` ignored |
| `RequeueAfter` or success, nil error | `V(5)` only | as asked |
| you `log.Error` then return the same `err` | two Error lines with the same `err` | backoff |

`TMP-LOGGING.md` allows an extra error log in `Reconcile` to get a more specific file and line; the run shows the cost (two Error lines per failure and per retry), so wrap with `fmt.Errorf("...: %w", err)` instead. An expected wait is a nil-error `RequeueAfter` plus a `V(1)` line: it logs nothing at
Info or Error, but it also does not count in `controller_runtime_reconcile_errors_total`, which is correct for an expected wait. [CR, K8S, run]

**Info** is for transitions: one line per real change, with `name`/`namespace` pairs, written after the change succeeds. **Debug** is for the reasoning behind
a reconcile that changed nothing. Do not log at Info: "Reconciling"/"Reconciled" per loop (controller-runtime has them at V(5)), cache hits, every skipped
write, loops over children, whole objects, anything that fires on every resync. Do not log at Debug: Secret or credential data, full object dumps, request bodies.

Set verbosity: klog `-v=1` (also `-vmodule=widget_controller=1`); zap `--zap-log-level=1` (`debug` enables V(1) only, `2` enables V(2)); slog handler `Level: slog.Level(-1)` (default `Info`
hides V(1); `slog.LevelDebug` shows V(0)-V(4)). All four change at runtime in code (section 3 row), but no endpoint or signal for it exists
unless you add one. [run]

## 7. Hazards

- No `SetLogger`: every line dropped, and the notice only appears on a log call after 30 s. [CR, run]
- Logging a whole object (`"secret", sec`) dumps `Data`/`StringData` on klog, textlogger, slog and zap in dev mode; only zap outside dev mode trims to name/namespace. [run]
- zap dev mode (`--zap-devel`, `UseDevMode(true)`): an odd key/value list panics the process; production only logs a `dpanic` line. [run]
- zap production samples identical messages (first 100 per second per level+message, then every 100th), so a burst of the same error line is thinned. [CR, run]
- Log-and-return doubles every error line and the stack trace in `Reconciler error` never shows your code. [CR, run]
- `klog.SetLogger` without `ContextualLogger(true)` leaves context-less client-go `V(n)` lines gated by klog's `-v` (default 0). [run]
- slog bridge: a handler left at `Info` hides all `V(1)` debug lines because V(n) is slog level -n. [run]
