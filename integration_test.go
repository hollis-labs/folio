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

// TestIntegration_BundledBasePreset is the v0 end-to-end gate. It exercises
// the full pipeline — bundled preset → render → write — and asserts the
// generated project tree is a valid Go module.
func TestIntegration_BundledBasePreset(t *testing.T) {
	target := filepath.Join(t.TempDir(), "smoke")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})

	res, err := svc.New(service.NewOptions{
		PresetID:  "base",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "smoke_test",
			"github_owner": "chrispian",
			"description":  "folio v0 smoke",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	expected := []string{
		".folio.yaml",
		".github/workflows/ci.yml",
		".gitignore",
		"LICENSE",
		"Makefile",
		"README.md",
		"cmd/smoke_test/main.go",
		"go.mod",
	}
	for _, p := range expected {
		full := filepath.Join(target, filepath.FromSlash(p))
		if _, statErr := os.Stat(full); statErr != nil {
			t.Errorf("expected %s, missing: %v", p, statErr)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gomod), "module github.com/chrispian/smoke_test") {
		t.Errorf("go.mod missing expected module declaration:\n%s", gomod)
	}

	mainGo, err := os.ReadFile(filepath.Join(target, "cmd", "smoke_test", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), "Hello, smoke_test") {
		t.Errorf("main.go missing expected greeting:\n%s", mainGo)
	}

	readme, err := os.ReadFile(filepath.Join(target, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "# SmokeTest") {
		t.Errorf("README missing PascalCase title:\n%s", readme)
	}
	if !strings.Contains(string(readme), "folio v0 smoke") {
		t.Errorf("README missing description:\n%s", readme)
	}

	makefile, err := os.ReadFile(filepath.Join(target, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"build:", "test:", "vet:", "install:"} {
		if !strings.Contains(string(makefile), target) {
			t.Errorf("Makefile missing %q target:\n%s", target, makefile)
		}
	}

	license, err := os.ReadFile(filepath.Join(target, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(license), "Copyright (c) 2026 chrispian") {
		t.Errorf("LICENSE missing expected copyright line:\n%s", license)
	}

	// Manifest assertions.
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if mf.FolioVersion != "0.1" {
		t.Errorf("manifest folio_version = %q", mf.FolioVersion)
	}
	if mf.Generator != "folio/"+folio.Version {
		t.Errorf("manifest generator = %q, want folio/%s", mf.Generator, folio.Version)
	}
	if len(mf.Presets) != 1 || mf.Presets[0].ID != "base" {
		t.Errorf("manifest presets = %+v", mf.Presets)
	}
	if mf.Inputs["project_name"] != "smoke_test" {
		t.Errorf("manifest inputs.project_name = %v", mf.Inputs["project_name"])
	}
	if mf.Computed["module_path"] != "github.com/chrispian/smoke_test" {
		t.Errorf("manifest computed.module_path = %v", mf.Computed["module_path"])
	}
	if rec, ok := mf.Files["go.mod"]; !ok {
		t.Error("manifest files missing go.mod")
	} else if !strings.HasPrefix(rec.DigestAtGen, "sha256:") {
		t.Errorf("go.mod digest_at_gen not sha256: %q", rec.DigestAtGen)
	}

	// Cross-check: NewResult digests match on-disk content.
	for _, f := range res.Files {
		raw, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatalf("read %s: %v", f.Path, err)
		}
		if manifest.Digest(raw) != f.Digest {
			t.Errorf("digest drift for %s", f.Path)
		}
	}

	// Sanity: generated project should pass go vet + go build if the
	// host has a Go toolchain. Skip when go is unavailable.
	if _, err := exec.LookPath("go"); err == nil {
		runGoCommand(t, target, "vet", "./...")
		runGoCommand(t, target, "build", "./...")
	}
}

func runGoCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("go %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}
