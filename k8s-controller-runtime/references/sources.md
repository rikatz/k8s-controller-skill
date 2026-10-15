# Sources

Read this when you need to see where a rule's tag comes from, or to re-check a claim against its source. Tags appear in square brackets after rules.
A tag may carry a detail suffix (`[CR FAQ]`, `[CR server.go]`, `[CT README]`): it names a file or page inside that source. Two markers are not sources:
`run` means the behaviour was reproduced in `research/experiments/` (outside the shipped skill), and `[unverified]` means it was not confirmed.
URLs below were taken from the research notes (`research/notes/`), where each claim is tied to its page or code. Version facts live in `versions.md`.

| Tag | Source | Read at | Where |
|---|---|---|---|
| `CR` | controller-runtime: code, godoc, README, FAQ, VERSIONING.md, TMP-LOGGING.md, designs/ | tag v0.25.2 (2026-10-01) | https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/ (FAQ.md, VERSIONING.md, TMP-LOGGING.md, pkg/reconcile/reconcile.go, pkg/controller/controller.go, pkg/cache/cache.go, pkg/envtest/server.go, pkg/manager/manager.go, designs/warmreplicas.md); release notes: https://github.com/kubernetes-sigs/controller-runtime/releases |
| `API` | Kubernetes API conventions and API change rules | master | https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md , https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api_changes.md |
| `K8S` | Kubernetes docs and community devel docs; apimachinery and client-go behaviour (client-go v0.37.0, module cache) | master / v0.37 | https://kubernetes.io/docs/concepts/extend-kubernetes/operator/ , https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/ , https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/ , https://kubernetes.io/docs/concepts/architecture/leases/ , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/reference/using-api/server-side-apply.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/overview/working-with-objects/owners-dependents.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/architecture/garbage-collection.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/security/rbac-good-practices.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/security/pod-security-standards.md , https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/reference/access-authn-authz/validating-admission-policy.md , https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/controllers.md , https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-instrumentation/logging.md , https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-instrumentation/metric-instrumentation.md , https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-testing/testing-strategy.md , https://raw.githubusercontent.com/kubernetes/apimachinery/master/pkg/apis/meta/v1/types.go , https://github.com/kubernetes/sample-controller |
| `KB` | Kubebuilder book, used for concepts only (nothing scaffolding-specific is carried over) | 2026-10 | https://book.kubebuilder.io ; metrics pages: https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/reference/metrics.md |
| `CT` | controller-tools / controller-gen: markers, generators, README, behaviour | tag v0.23.0 (released 2026-10-15) | https://github.com/kubernetes-sigs/controller-tools ; envtest release index: https://raw.githubusercontent.com/kubernetes-sigs/controller-tools/HEAD/envtest-releases.yaml |
| `KAL` | kube-api-linter: linters, configuration, behaviour (no tags; pseudo-version `v0.0.0-20261012124120-79b557aa8c8d`) | commit 79b557a | https://github.com/kubernetes-sigs/kube-api-linter/blob/main/docs/linters.md |
| `CAPI` | Cluster API: manager setup, patch helper, predicates, indexes, conditions, tests, docs | commit c8f8936 (main, 2026-10-15), controller-runtime v0.25.2 | https://github.com/kubernetes-sigs/cluster-api ; background: https://github.com/kubernetes-sigs/cluster-api/pull/9347 |
| `CAPV` | Cluster API provider vSphere: provider-style controllers, webhooks, API types, KAL config | commit 9d3e283 (main, 2026-10-14) | https://github.com/kubernetes-sigs/cluster-api-provider-vsphere |
| `TALK` | Conference talks, see the table below | transcripts read | table below |
| `BLOG` | "How the controller-runtime Cache Actually Works, and Why Your Controller Does Not Crash the API Server" (Kvapil, Larkin; 2026-07-29, revised). Two of 27 claims were wrong and are corrected in the references | 2026-10 | https://kubernetes.io/blog/2026/07/29/controller-runtime-cache-explained/ |
| `OCP` | OpenShift API and operator conventions (openshift/api commit e07c1fd9) | master | https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md , https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md , https://raw.githubusercontent.com/openshift/enhancements/master/dev-guide/operators.md |
| `GW` | Gateway API conventions (gateway-api commit aabc423c) | main | https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/main/geps/gep-1364/index.md , https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/main/apis/v1/shared_types.go |
| `PROM` | Prometheus instrumentation and naming practices | 2026-10 | https://prometheus.io/docs/practices/naming/ , https://prometheus.io/docs/practices/instrumentation/ |
| `KSM` | kube-state-metrics custom resource state metrics (feature-frozen in favour of `resource-state-metrics`) | main | https://raw.githubusercontent.com/kubernetes/kube-state-metrics/main/docs/metrics/extend/customresourcestate-metrics.md |
| `OSS` | Metrics code of other controllers: cert-manager, Flux, Kong ingress controller (Argo CD was surveyed in `research/notes/11-metrics.md`, with no URL recorded) | master / main | https://raw.githubusercontent.com/cert-manager/cert-manager/master/pkg/metrics/metrics.go , https://raw.githubusercontent.com/fluxcd/pkg/main/runtime/metrics/recorder.go , https://fluxcd.io/flux/monitoring/metrics/ , https://raw.githubusercontent.com/Kong/kubernetes-ingress-controller/main/internal/metrics/prometheus.go |

Other background read but not used as a tag: the CNCF operator white paper
(https://raw.githubusercontent.com/cncf/tag-app-delivery/main/operator-wg/whitepaper/Operator-WhitePaper_v1-0.md), operator-sdk best practices
(https://sdk.operatorframework.io/docs/best-practices/best-practices/) and a 2018 Google blog post
(https://cloud.google.com/blog/products/containers-kubernetes/best-practices-for-building-kubernetes-operators-and-stateful-apps), which is dated and recommends kubebuilder.

## Talks (`TALK`)

Transcripts were obtained with yt-dlp. Event names and dates are inferred from upload dates and descriptions. Spellings of names were taken from the CNCF titles.

| # | Talk | Speaker(s) | URL | Read |
|---|---|---|---|---|
| T1 | Don't Write Controllers Like Charlie Don't Does | Nick Young | https://www.youtube.com/watch?v=tnSraS9JqZ8 | full |
| T2 | Cluster API Deep Dive: Improving Performance up to 2k Clusters | Fabrizio Pandini, Stefan Büringer | https://www.youtube.com/watch?v=bRPfmviTi3s | full |
| T3 | Kubernetes CRD Design for the Long Haul | Christian Schlotter, Fabrizio Pandini | https://www.youtube.com/watch?v=7IA-Vw1K7eg | full |
| T4 | Building Better Controllers | John Howard | https://www.youtube.com/watch?v=GKPBQDJ2Hjk | full |
| T5 | Scaling Kubernetes Controllers Horizontally | Tim Ebert | https://www.youtube.com/watch?v=OTzd9eTtLRA | full |
| T6 | Writing a Kubernetes Operator: the Hard Parts | Sebastien Guilloux | https://www.youtube.com/watch?v=wMqzAOp15wo | full |
| T7 | Why Are We Copying and Pasting So Much? | Solly Ross, Phil Wittrock | https://www.youtube.com/watch?v=0tX3q_WAEdE | full |
| T8 | Beyond Kubebuilder | Amine Hilaly, Jay Pipes | https://www.youtube.com/watch?v=rfXk6svglrA | full |
| T9 | Automated, Distributed Systems Testing for Kubernetes Controllers (Sieve) | Lalith Suresh, Xudong Sun | https://www.youtube.com/watch?v=6JnhjgOaZVk | full |
| T10 | Stop Reinventing Operator Wheels | Evan Cordell | https://www.youtube.com/watch?v=yHJR8d9oHFI | full |
| T11 | Lightning Talk: Oh Sh*t! The Config Changed! | Joel Speed | https://www.youtube.com/watch?v=8P7-C44Gjj8 | full (tangential) |
| T12 | In-place Updates with Cluster API | Fabrizio Pandini, Stefan Büringer | https://www.youtube.com/watch?v=CMf6rOPo9Z0 | skimmed only; no authoring guidance taken |
| T13 | Kubernetes is Your Platform: Design Patterns For Extensibility | Rafael Fernandez Lopez, Fabrizio Pandini | https://www.youtube.com/watch?v=I1-s7AxD1Ls | **unread** (no subtitles) |
| T14 | Tutorial: How To Write a Reconciler Using K8s Controller-Runtime (1h27m) | not identified | https://www.youtube.com/watch?v=Npvz84HpO3o | **unread** (no subtitles; probably valuable) |

The 2018 to 2019 talks (T6, T7) describe APIs that have since changed; the references only use their design advice.
