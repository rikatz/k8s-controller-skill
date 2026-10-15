// Verification of CRD validation ratcheting (stable since v1.33).
//
// Method: install a loose CRD, create objects that violate future tightenings,
// then swap in tightened CRDs and observe which updates still succeed.
// Stages (each REPLACES the previous schema):
//
//	stage1: no constraints
//	stage2: CEL rule on spec.size        (self <= 10)            -> ratchets
//	stage3: enum on spec.mode            ([Fast, Slow])          -> observe
//	stage4: transition rule on spec.size (oldSelf <= 10)         -> never ratcheted, never runs on create
//	stage5: optionalOldSelf rule on size (grandfathers old < 4)  -> custom ratchet
//	stage6: required: [size]             -> observe
//	stage7: oldSelf under an atomic list          -> CRD update itself rejected
//	stage8: O(n^2) CEL over an unbounded list     -> CRD update itself rejected (cost budget)
//	stage9: minProperties on spec.tls, field-level self == oldSelf on spec.mode
//	stage10: parent-level set-once rule on spec
package ratcheting_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg     *rest.Config
	crds    apiextensionsclientset.Interface
	dyn     dynamic.Interface
	testEnv *envtest.Environment
	gvr     = schema.GroupVersionResource{Group: "demo.example.com", Version: "v1", Resource: "gadgets"}
	ns      = "default"
)

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		DownloadBinaryAssets:        true,
		DownloadBinaryAssetsVersion: "1.37.0",
		BinaryAssetsDirectory:       filepath.Join("bin", "envtest"),
	}
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		os.Exit(1)
	}
	if crds, err = apiextensionsclientset.NewForConfig(cfg); err != nil {
		os.Exit(1)
	}
	if dyn, err = dynamic.NewForConfig(cfg); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

func TestRatcheting(t *testing.T) {
	ctx := context.Background()

	applyStage(t, ctx, "stage1")

	// Objects created under the loose schema, each violating a later tightening.
	mustCreate(t, ctx, "g1", map[string]any{"size": 100})                  // violates stage2 CEL max
	mustCreate(t, ctx, "g2", map[string]any{"size": 1, "mode": "Turbine"}) // violates stage3 enum
	mustCreate(t, ctx, "g3", map[string]any{"size": 100})                  // fails stage4 transition rule
	mustCreate(t, ctx, "g5", map[string]any{"size": 2})                    // below stage5's new minimum
	mustCreate(t, ctx, "g6", map[string]any{"mode": "Fast"})               // lacks stage6's required size
	mustCreate(t, ctx, "g4", map[string]any{"size": 100})                  // for the status-subresource check

	t.Run("stage2_CEL_rule_ratchets", func(t *testing.T) {
		applyStage(t, ctx, "stage2")
		waitRejected(t, ctx, "new violations on create are rejected", func() error {
			return createErr(ctx, map[string]any{"size": 101})
		})
		expectOK(t, "unchanged violating field is ratcheted (annotation-only update)", setAnnotation(ctx, "g1"))
		expectReject(t, "making the violation worse", setSize(ctx, "g1", 101))
		expectOK(t, "fixing the violation", setSize(ctx, "g1", 5))
		expectReject(t, "reintroducing a violation", setSize(ctx, "g1", 100))
		expectOK(t, "spec touched but failing field unchanged (g4: set mode)", setMode(ctx, "g4", "Slow"))
		expectOK(t, "status-only update is ratcheted too (spec.size unchanged)", setStatus(ctx, "g4"))
	})

	t.Run("stage3_enum_ratchets", func(t *testing.T) {
		applyStage(t, ctx, "stage3")
		waitRejected(t, ctx, "new enum violations on create are rejected", func() error {
			return createErr(ctx, map[string]any{"size": 1, "mode": "Turbine"})
		})
		expectOK(t, "unchanged violating enum value is ratcheted (annotation-only update)", setAnnotation(ctx, "g2"))
		expectOK(t, "ratcheted per-field: spec.size changed, failing spec.mode unchanged", setSize(ctx, "g2", 2))
		mustCreate(t, ctx, "g10", map[string]any{"size": 5, "mode": "Fast"})
		expectReject(t, "enum enforced on update from a valid value", setMode(ctx, "g10", "Turbine"))
		expectOK(t, "fixing the enum value", setMode(ctx, "g2", "Fast"))
		expectReject(t, "reintroducing an enum violation", setMode(ctx, "g2", "Turbine"))
	})

	t.Run("stage4_oldSelf_rule_never_ratchets_and_skips_create", func(t *testing.T) {
		applyStage(t, ctx, "stage4")
		// Transition rules never apply to create: a violating object can be created directly.
		mustCreate(t, ctx, "g7", map[string]any{"size": 100})
		// oldSelf rules are never ratcheted: any update of a failing object is rejected...
		waitRejected(t, ctx, "oldSelf rule rejects updates once live", func() error {
			return setAnnotation(ctx, "g7")
		})
		// ...including for objects created under the loose schema...
		expectReject(t, "oldSelf rule not ratcheted on a pre-existing object", setAnnotation(ctx, "g3"))
		// ...and including the repair itself (the rule still sees the bad old value).
		expectReject(t, "an oldSelf rule the old state fails blocks even the fixing update", setSize(ctx, "g7", 5))
	})

	t.Run("stage5_optionalOldSelf_custom_ratchet", func(t *testing.T) {
		applyStage(t, ctx, "stage5")
		waitRejected(t, ctx, "create enforces the new minimum (no oldSelf on create)", func() error {
			return createErr(ctx, map[string]any{"size": 1})
		})
		expectOK(t, "old value below the new minimum is grandfathered", setAnnotation(ctx, "g5"))
		expectOK(t, "staying below the new minimum is grandfathered", setSize(ctx, "g5", 3))
		mustCreate(t, ctx, "g8", map[string]any{"size": 5})
		expectReject(t, "dropping below the new minimum from a valid value", setSize(ctx, "g8", 1))
	})

	t.Run("stage6_required", func(t *testing.T) {
		applyStage(t, ctx, "stage6")
		waitRejected(t, ctx, "create without size is rejected", func() error {
			return createErr(ctx, map[string]any{"mode": "Fast"})
		})
		// Observed on 1.37.0: the pre-existing missing field does not block updates.
		expectOK(t, "pre-existing missing required field does not block an annotation-only update", setAnnotation(ctx, "g6"))
		// Discriminating case: same missing field, but this update touches spec itself.
		expectReject(t, "missing required field blocks an update that changes spec", setMode(ctx, "g6", "Slow"))
		// The repair itself is allowed: the new object is valid, so nothing needs excusing.
		expectOK(t, "repair: adding the missing required field is allowed", setSpec(ctx, "g6", map[string]any{"mode": "Slow", "size": 5}))
		// Required is enforced on update when the field WAS present.
		mustCreate(t, ctx, "g9", map[string]any{"size": 5, "mode": "Fast"})
		expectReject(t, "removing a present required field on update", setSpec(ctx, "g9", map[string]any{"mode": "Fast"}))
	})

	t.Run("stage7_oldSelf_under_atomic_list_rejected_at_crd_admission", func(t *testing.T) {
		applyStageReject(t, ctx, "stage7", "oldSelf cannot be used on the uncorrelatable portion of the schema")
	})

	t.Run("stage8_cel_over_unbounded_list_rejected_at_crd_admission", func(t *testing.T) {
		err := applyStageReject(t, ctx, "stage8", "estimated rule cost exceeds budget by factor of more than 100x")
		// Negative control: the same CRD carries a contains() rule over a single
		// unbounded string, which must NOT appear in the rejection.
		if strings.Contains(err.Error(), "properties[note]") {
			t.Errorf("a rule over a single unbounded string should not trip the cost check: %v", err)
		}
	})

	t.Run("stage9_empty_struct_rejected_and_field_rule_allows_removal", func(t *testing.T) {
		applyStage(t, ctx, "stage9")
		// omitempty on a struct field still serializes {}; with minProperties: 1 the
		// create is rejected by the API server.
		err := waitRejected(t, ctx, "empty tls object create rejected by minProperties", func() error {
			return createErr(ctx, map[string]any{"tls": map[string]any{}})
		})
		if !strings.Contains(err.Error(), "at least 1 properties") {
			t.Errorf("empty tls create: expected an 'at least 1 properties' rejection, got: %v", err)
		}
		mustCreate(t, ctx, "g11", map[string]any{"tls": map[string]any{"minVersion": "TLS12"}, "mode": "Fast"})
		// A field-level self == oldSelf rule is not evaluated when the field is absent
		// in the new object, so removing the field is allowed.
		expectOK(t, "field-level self == oldSelf does not stop removing the field",
			setSpec(ctx, "g11", map[string]any{"tls": map[string]any{"minVersion": "TLS12"}}))
	})

	t.Run("stage10_parent_set_once_rule_blocks_removal", func(t *testing.T) {
		applyStage(t, ctx, "stage10")
		// Gate on the new rule being live before asserting anything (probe object:
		// removal must be rejected once stage10 is served).
		mustCreate(t, ctx, "g12", map[string]any{"mode": "Fast"})
		waitRejected(t, ctx, "parent set-once rule rejects removal once live", func() error {
			return setSpec(ctx, "g12", map[string]any{})
		})
		expectOK(t, "set-once: first set is allowed (old object lacks the field)",
			setSpec(ctx, "g11", map[string]any{"tls": map[string]any{"minVersion": "TLS12"}, "mode": "Slow"}))
		expectReject(t, "set-once on the parent blocks removing the field",
			setSpec(ctx, "g11", map[string]any{"tls": map[string]any{"minVersion": "TLS12"}}))
	})
}

// --- helpers ---

func expectOK(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: expected success, got: %v", what, err)
	}
}

func expectReject(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: expected rejection, got success", what)
		return
	}
	t.Logf("%s: rejected with: %v", what, err)
}

func applyStage(t *testing.T, ctx context.Context, stage string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("config", "crd", "stages", stage+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := kyaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	_, err = crds.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crd, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, gerr := crds.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if gerr != nil {
			t.Fatal(gerr)
		}
		crd.ResourceVersion = existing.ResourceVersion
		_, err = crds.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crd, metav1.UpdateOptions{})
	}
	if err != nil {
		t.Fatalf("apply %s: %v", stage, err)
	}
	waitFor(t, stage+" established", func() bool {
		c, gerr := crds.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if gerr != nil {
			return false
		}
		for _, cond := range c.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true
			}
		}
		return false
	})
}

func gadget(name string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("demo.example.com/v1")
	u.SetKind("Gadget")
	u.SetNamespace(ns)
	if name != "" {
		u.SetName(name)
	} else {
		u.SetGenerateName("probe-")
	}
	u.Object["spec"] = spec
	return u
}

func createErr(ctx context.Context, spec map[string]any) error {
	_, err := dyn.Resource(gvr).Namespace(ns).Create(ctx, gadget("", spec), metav1.CreateOptions{})
	return err
}

func mustCreate(t *testing.T, ctx context.Context, name string, spec map[string]any) {
	t.Helper()
	if _, err := dyn.Resource(gvr).Namespace(ns).Create(ctx, gadget(name, spec), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func get(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
}

func setAnnotation(ctx context.Context, name string) error {
	u, err := get(ctx, name)
	if err != nil {
		return err
	}
	u.SetAnnotations(map[string]string{"probe": "1"})
	_, err = dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func setSpec(ctx context.Context, name string, spec map[string]any) error {
	u, err := get(ctx, name)
	if err != nil {
		return err
	}
	u.Object["spec"] = spec
	_, err = dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func setSize(ctx context.Context, name string, size int64) error {
	u, err := get(ctx, name)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(u.Object, size, "spec", "size"); err != nil {
		return err
	}
	_, err = dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func setMode(ctx context.Context, name string, mode string) error {
	u, err := get(ctx, name)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(u.Object, mode, "spec", "mode"); err != nil {
		return err
	}
	_, err = dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func setStatus(ctx context.Context, name string) error {
	u, err := get(ctx, name)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(u.Object, true, "status", "ready"); err != nil {
		return err
	}
	_, err = dyn.Resource(gvr).Namespace(ns).UpdateStatus(ctx, u, metav1.UpdateOptions{})
	return err
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// waitRejected polls fn (which may create probe objects with GenerateName) until it
// returns an error: the newly applied validation is provably live before assertions run.
// Returns the observed rejection for message assertions.
func waitRejected(t *testing.T, ctx context.Context, what string, fn func() error) error {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := fn(); err != nil {
			t.Logf("%s: live (probe rejected with: %v)", what, err)
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
	return nil
}

// applyStageReject asserts that a CRD stage is rejected while the CRD object itself is
// admitted (structural-schema validation of the CRD), leaving the previous schema live.
// Returns the observed rejection for message assertions.
func applyStageReject(t *testing.T, ctx context.Context, stage, substr string) error {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("config", "crd", "stages", stage+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := kyaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	existing, err := crds.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	crd.ResourceVersion = existing.ResourceVersion
	_, err = crds.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crd, metav1.UpdateOptions{})
	if err == nil {
		t.Fatalf("apply %s: expected the CRD update to be rejected, got success", stage)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("apply %s: rejection does not contain %q: %v", stage, substr, err)
	}
	t.Logf("apply %s rejected with: %v", stage, err)
	return err
}
