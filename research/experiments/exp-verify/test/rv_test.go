package test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	demov1 "example.com/demo/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type bodyRT struct{ rt http.RoundTripper; t *testing.T }

func (b *bodyRT) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	b.t.Logf("HTTP %s %s ct=%s body=%s", r.Method, r.URL.Path, r.Header.Get("Content-Type"), body)
	return b.rt.RoundTrip(r)
}

func TestRV(t *testing.T) {
	ctx := context.Background()
	env := &envtest.Environment{DownloadBinaryAssets: true, BinaryAssetsDirectory: "/tmp/envtest-bin-783541524", CRDDirectoryPaths: []string{"../config/crd/bases"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil { t.Fatal(err) }
	defer env.Stop()
	sch := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(sch)
	_ = demov1.AddToScheme(sch)
	cfg2 := rest.CopyConfig(cfg)
	cfg2.Wrap(func(rt http.RoundTripper) http.RoundTripper { return &bodyRT{rt, t} })
	c, _ := client.New(cfg2, client.Options{Scheme: sch})
	w := &demov1.Widget{ObjectMeta: metav1.ObjectMeta{Name: "rv", Namespace: "default"}, Spec: demov1.WidgetSpec{Size: "Small"}}
	must(t, c.Create(ctx, w))
	rv0 := w.ResourceVersion
	orig := w.DeepCopy()
	must(t, c.Patch(ctx, w, client.MergeFrom(orig)))
	t.Logf("rv before=%s after empty Patch=%s gen=%d", rv0, w.ResourceVersion, w.Generation)
	orig = w.DeepCopy()
	must(t, c.Status().Patch(ctx, w, client.MergeFrom(orig)))
	t.Logf("rv after empty Status().Patch=%s", w.ResourceVersion)
	orig = w.DeepCopy()
	must(t, c.Patch(ctx, w, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})))
	t.Logf("rv after empty optimistic-lock Patch=%s", w.ResourceVersion)
}
