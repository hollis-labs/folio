package service_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

func TestNew_ComposerOverSample(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")

	res, err := svc.New(service.NewOptions{
		PresetID:  "composer",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "smoke_compose",
			"github_owner": "chrispian",
			"description":  "compose smoke",
			"package_name": "greeter",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Files from base preset (sample) plus composer's overlay.
	mustExist := []string{
		".gitignore",                  // from sample
		"go.mod",                      // from sample
		"cmd/smoke_compose/main.go",   // from sample
		"README.md",                   // overwritten by composer
		"internal/greeter/greeter.go", // added by composer
	}
	for _, p := range mustExist {
		full := filepath.Join(target, filepath.FromSlash(p))
		if _, statErr := os.Stat(full); statErr != nil {
			t.Errorf("expected file not written: %s (%v)", p, statErr)
		}
	}

	// README.md should be the COMPOSER's version (overwrite).
	data, err := os.ReadFile(filepath.Join(target, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if !strings.Contains(string(data), "(composer)") {
		t.Errorf("README.md should be composer's overlay, got: %s", string(data))
	}

	// Manifest shape: presets array has both entries in apply order.
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(mf.Presets) != 2 {
		t.Fatalf("expected 2 entries in presets, got %d: %+v", len(mf.Presets), mf.Presets)
	}
	wantOrder := []string{"sample", "composer"}
	for i, want := range wantOrder {
		if mf.Presets[i].ID != want {
			t.Errorf("presets[%d].id = %q, want %q (full: %+v)", i, mf.Presets[i].ID, want, mf.Presets)
		}
	}

	// Per-file preset reflects the last layer that produced it.
	if mf.Files["README.md"].Preset != "composer" {
		t.Errorf("README.md preset = %q, want composer", mf.Files["README.md"].Preset)
	}
	if mf.Files[".gitignore"].Preset != "sample" {
		t.Errorf(".gitignore preset = %q, want sample", mf.Files[".gitignore"].Preset)
	}
	if mf.Files["internal/greeter/greeter.go"].Preset != "composer" {
		t.Errorf("internal/greeter/greeter.go preset = %q, want composer", mf.Files["internal/greeter/greeter.go"].Preset)
	}

	// Computed merge: pkg_module_path references the prior layer's
	// computed.module_path — proving cross-layer computed visibility.
	wantPkgPath := "github.com/chrispian/smoke_compose/internal/greeter"
	if mf.Computed["pkg_module_path"] != wantPkgPath {
		t.Errorf("computed.pkg_module_path = %v, want %s", mf.Computed["pkg_module_path"], wantPkgPath)
	}
	// module_path from the base layer should be preserved (no collision).
	if mf.Computed["module_path"] != "github.com/chrispian/smoke_compose" {
		t.Errorf("computed.module_path = %v, want preserved from base", mf.Computed["module_path"])
	}

	// SyncHistory init records both layers.
	if len(mf.SyncHistory) != 1 || mf.SyncHistory[0].Operation != "init" {
		t.Fatalf("syncHistory = %+v", mf.SyncHistory)
	}
	if len(mf.SyncHistory[0].Presets) != 2 {
		t.Errorf("syncHistory[0].Presets should have 2 entries, got: %+v", mf.SyncHistory[0].Presets)
	}

	// NewResult.Files count matches manifest.Files.
	if len(res.Files) != len(mf.Files) {
		t.Errorf("NewResult.Files (%d) != manifest.Files (%d)", len(res.Files), len(mf.Files))
	}
}

// TestNew_ComposerDoesNotLeakUndeclaredInputs is the regression for the
// review comment on layerInputs: a `--input` for a key that no layer in
// the compose chain declares MUST NOT appear in the rendered manifest's
// inputs section. The undeclared key flows through resolveInputs's
// "ignored" warning path (which composedLayers suppresses for keys
// declared elsewhere — see ignoredInputDeclaredElsewhere) and is dropped
// from layerInputs.
func TestNew_ComposerDoesNotLeakUndeclaredInputs(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")

	_, err := svc.New(service.NewOptions{
		PresetID:  "composer",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name":         "smoke_compose",
			"github_owner":         "chrispian",
			"description":          "compose smoke",
			"package_name":         "greeter",
			"bogus_undeclared_key": "leak-canary",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, present := mf.Inputs["bogus_undeclared_key"]; present {
		t.Errorf("manifest.inputs leaked undeclared key: %+v", mf.Inputs)
	}
}

func TestPlan_ComposerOverSample(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")
	res, err := svc.Plan(service.NewOptions{
		PresetID:  "composer",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "smoke_compose",
			"github_owner": "chrispian",
			"description":  "compose smoke",
			"package_name": "greeter",
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Plan should not write the target directory.
	if _, err := os.Stat(target); err == nil {
		t.Errorf("Plan wrote target dir; should be dry-run")
	}
	if len(res.Files) == 0 {
		t.Fatal("Plan produced no files")
	}
	if res.Computed["pkg_module_path"] == nil {
		t.Errorf("Plan computed missing pkg_module_path: %+v", res.Computed)
	}
}
