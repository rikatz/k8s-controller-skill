// Reproductions of the controller-gen behaviour hazards in api-design.md:
// a misspelled marker is silently ignored (generation exits 0, the constraint
// disappears), and MinProperties on a struct-typed field is rejected at
// generation time.
package genbehaviour_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runControllerGen(t *testing.T, paths, outDir string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", "tool", "controller-gen", "crd", "paths="+paths, "output:crd:dir="+outDir)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// The misspelled MaxLenth marker must not fail generation and must not produce
// a maxLength constraint: the constraint silently disappears.
func TestMisspelledMarkerSilentlyIgnored(t *testing.T) {
	out := t.TempDir()
	output, err := runControllerGen(t, "./misspell/...", out)
	if err != nil {
		t.Fatalf("controller-gen exited non-zero on a misspelled marker: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(out, "misspell.example.com_zaps.yaml"))
	if err != nil {
		t.Fatalf("no CRD generated: %v\n%s", err, output)
	}
	if strings.Contains(string(data), "maxLength") {
		t.Errorf("the misspelled MaxLenth marker produced a maxLength constraint anyway")
	}
	t.Logf("controller-gen exited 0; the generated schema carries no maxLength:\n%s", string(data))
}

// MinProperties on a struct-typed FIELD is rejected by controller-gen; the
// marker belongs on the struct type.
func TestMinPropertiesOnFieldRejected(t *testing.T) {
	output, err := runControllerGen(t, "./minprops/...", t.TempDir())
	if err == nil {
		t.Fatalf("controller-gen exited 0 on field-level MinProperties\n%s", output)
	}
	if !strings.Contains(output, "must apply minproperties to an object") {
		t.Errorf("expected the minproperties rejection, got:\n%s", output)
	}
	t.Logf("rejected with: %s", output)
}
