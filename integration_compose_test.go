package folio_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// TestIntegration_ComposingGoPackagePreset is the v0.2 end-to-end gate. It
// exercises the full layered pipeline — bundled go-package preset composing
// the bundled base preset → render multi-layer → write — and asserts the
// generated project tree is a valid Go module, the .folio.yaml manifest has
// both layers in apply order, and per-file `preset:` attribution reflects
// the last-writer-wins overwrite policy.
func TestIntegration_ComposingGoPackagePreset(t *testing.T) {
	target := filepath.Join(t.TempDir(), "smoke_compose")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})

	res, err := svc.New(service.NewOptions{
		PresetID:  "go-package",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "smoke_compose",
			"github_owner": "chrispian",
			"package_name": "greeter",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	// Union of base files + go-package's additions. README and cmd/<proj>/
	// main.go are overwritten by go-package; everything else is base.
	expected := []string{
		".folio.yaml",
		".github/workflows/ci.yml",         // base
		".gitignore",                       // base
		"LICENSE",                          // base
		"Makefile",                         // base
		"README.md",                        // overwritten by go-package
		"cmd/smoke_compose/main.go",        // overwritten by go-package (stub)
		"go.mod",                           // base
		"internal/greeter/greeter.go",      // go-package
		"internal/greeter/greeter_test.go", // go-package
	}
	for _, p := range expected {
		full := filepath.Join(target, filepath.FromSlash(p))
		if _, statErr := os.Stat(full); statErr != nil {
			t.Errorf("expected %s, missing: %v", p, statErr)
		}
	}

	// README is go-package's overlay (mentions "Layout" + library package).
	readme, err := os.ReadFile(filepath.Join(target, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "## Layout") {
		t.Errorf("README missing go-package's Layout section:\n%s", readme)
	}
	if !strings.Contains(string(readme), "Library package: `github.com/chrispian/smoke_compose/internal/greeter`") {
		t.Errorf("README missing library package reference:\n%s", readme)
	}

	// main.go is go-package's stub that imports the internal library.
	mainGo, err := os.ReadFile(filepath.Join(target, "cmd", "smoke_compose", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), "github.com/chrispian/smoke_compose/internal/greeter") {
		t.Errorf("main.go should import composed library path:\n%s", mainGo)
	}
	if !strings.Contains(string(mainGo), "greeter.Hello()") {
		t.Errorf("main.go should call greeter.Hello():\n%s", mainGo)
	}

	// Library package compiles a Hello() func.
	libGo, err := os.ReadFile(filepath.Join(target, "internal", "greeter", "greeter.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(libGo), "package greeter") {
		t.Errorf("greeter.go missing package declaration:\n%s", libGo)
	}
	if !strings.Contains(string(libGo), "func Hello()") {
		t.Errorf("greeter.go missing Hello() func:\n%s", libGo)
	}

	// Base preset's files survive untouched.
	makefile, err := os.ReadFile(filepath.Join(target, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mtarget := range []string{"build:", "test:", "vet:", "install:"} {
		if !strings.Contains(string(makefile), mtarget) {
			t.Errorf("Makefile (from base) missing %q target:\n%s", mtarget, makefile)
		}
	}

	// Manifest assertions: multi-entry presets array in apply order.
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if mf.Generator != "folio/"+folio.Version {
		t.Errorf("manifest generator = %q", mf.Generator)
	}
	if len(mf.Presets) != 2 {
		t.Fatalf("manifest presets length = %d, want 2: %+v", len(mf.Presets), mf.Presets)
	}
	wantPresets := []struct {
		ID, Version string
	}{
		{"base", "0.1.0"},
		{"go-package", "1.0.0"},
	}
	for i, want := range wantPresets {
		if mf.Presets[i].ID != want.ID {
			t.Errorf("presets[%d].id = %q, want %q", i, mf.Presets[i].ID, want.ID)
		}
		if mf.Presets[i].Version != want.Version {
			t.Errorf("presets[%d].version = %q, want %q", i, mf.Presets[i].Version, want.Version)
		}
		if mf.Presets[i].Source != "bundled" {
			t.Errorf("presets[%d].source = %q, want bundled", i, mf.Presets[i].Source)
		}
	}

	// Per-file preset attribution: last-writer wins.
	wantPresetFor := map[string]string{
		".github/workflows/ci.yml":         "base",
		".gitignore":                       "base",
		"LICENSE":                          "base",
		"Makefile":                         "base",
		"go.mod":                           "base",
		"README.md":                        "go-package",
		"cmd/smoke_compose/main.go":        "go-package",
		"internal/greeter/greeter.go":      "go-package",
		"internal/greeter/greeter_test.go": "go-package",
	}
	for path, wantPreset := range wantPresetFor {
		rec, ok := mf.Files[path]
		if !ok {
			t.Errorf("manifest.files missing %q", path)
			continue
		}
		if rec.Preset != wantPreset {
			t.Errorf("manifest.files[%q].preset = %q, want %q", path, rec.Preset, wantPreset)
		}
	}

	// Cross-layer computed values both visible: module_path from base,
	// pkg_module_path from go-package referencing module_path.
	if mf.Computed["module_path"] != "github.com/chrispian/smoke_compose" {
		t.Errorf("manifest computed.module_path = %v", mf.Computed["module_path"])
	}
	if mf.Computed["pkg_module_path"] != "github.com/chrispian/smoke_compose/internal/greeter" {
		t.Errorf("manifest computed.pkg_module_path = %v (expected cross-layer derivation)",
			mf.Computed["pkg_module_path"])
	}

	// SyncHistory init records both layers in apply order.
	if len(mf.SyncHistory) != 1 || mf.SyncHistory[0].Operation != "init" {
		t.Fatalf("sync_history = %+v", mf.SyncHistory)
	}
	if len(mf.SyncHistory[0].Presets) != 2 {
		t.Errorf("sync_history[0].presets length = %d, want 2", len(mf.SyncHistory[0].Presets))
	}

	// NewResult digests align with on-disk content.
	for _, f := range res.Files {
		raw, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatalf("read %s: %v", f.Path, err)
		}
		if manifest.Digest(raw) != f.Digest {
			t.Errorf("digest drift for %s", f.Path)
		}
	}

	// Generated project must be a valid Go module: go vet + go build + go
	// test all clean. Skip when go is unavailable on the host.
	if _, err := exec.LookPath("go"); err == nil {
		runComposeGoCommand(t, target, "vet", "./...")
		runComposeGoCommand(t, target, "build", "./...")
		runComposeGoCommand(t, target, "test", "./...")
	}
}

func runComposeGoCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("go %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}
