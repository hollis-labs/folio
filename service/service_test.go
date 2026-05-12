package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	absPresets, err := filepath.Abs(filepath.Join("testdata", "presets"))
	if err != nil {
		t.Fatal(err)
	}
	return service.New(service.Options{
		BundledFS:    os.DirFS(absPresets),
		BundledRoot:  ".",
		UserDir:      t.TempDir(),
		FolioVersion: "0.1.0-test",
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})
}

func validInputs() map[string]any {
	return map[string]any{
		"project_name": "smoke_test",
		"github_owner": "chrispian",
		"description":  "folio v0 smoke",
	}
}

func TestNew_WritesProjectTree(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")
	res, err := svc.New(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    validInputs(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(res.Files) == 0 {
		t.Fatal("expected at least one file in NewResult")
	}

	expected := []string{
		".gitignore",
		"README.md",
		"cmd/smoke_test/main.go",
		"go.mod",
	}
	for _, p := range expected {
		full := filepath.Join(target, filepath.FromSlash(p))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected file not written: %s (%v)", p, err)
		}
	}

	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if mf.FolioVersion != "0.1" {
		t.Errorf("manifest folio_version = %q, want 0.1", mf.FolioVersion)
	}
	if mf.Generator != "folio/0.1.0-test" {
		t.Errorf("manifest generator = %q", mf.Generator)
	}
	if !mf.GeneratedAt.Equal(time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC)) {
		t.Errorf("manifest generated_at = %v", mf.GeneratedAt)
	}
	if len(mf.Presets) != 1 || mf.Presets[0].ID != "sample" {
		t.Errorf("manifest presets = %+v", mf.Presets)
	}
	if mf.Computed["module_path"] != "github.com/chrispian/smoke_test" {
		t.Errorf("manifest computed.module_path = %v", mf.Computed["module_path"])
	}
	if _, ok := mf.Files["README.md"]; !ok {
		t.Errorf("manifest.files missing README.md; got %v", mf.Files)
	}
}

func TestNew_MissingRequiredInput(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")
	_, err := svc.New(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    map[string]any{"project_name": "smoke_test"}, // missing github_owner
	})
	if err == nil {
		t.Fatal("expected ErrInputMissing")
	}
	var se *service.Error
	if !errors.As(err, &se) {
		t.Fatalf("expected *service.Error, got %T", err)
	}
	if se.Code != service.ErrInputMissing {
		t.Errorf("code = %q, want %q", se.Code, service.ErrInputMissing)
	}
}

func TestNew_PatternViolation(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")
	_, err := svc.New(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    map[string]any{"project_name": "NotLowerCase", "github_owner": "chrispian"},
	})
	if err == nil {
		t.Fatal("expected pattern violation")
	}
	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.ErrInputInvalid {
		t.Errorf("expected ErrInputInvalid, got %v", err)
	}
}

func TestNew_PresetNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.New(service.NewOptions{
		PresetID:  "does-not-exist",
		TargetDir: filepath.Join(t.TempDir(), "out"),
		Inputs:    validInputs(),
	})
	if err == nil {
		t.Fatal("expected ErrPresetNotFound")
	}
	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.ErrPresetNotFound {
		t.Errorf("code = %v, want %v", se, service.ErrPresetNotFound)
	}
}

func TestNew_TargetExists(t *testing.T) {
	svc := newTestService(t)
	target := t.TempDir()
	// Make target non-empty.
	if err := os.WriteFile(filepath.Join(target, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.New(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    validInputs(),
	})
	if err == nil {
		t.Fatal("expected ErrTargetExists")
	}
	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.ErrTargetExists {
		t.Errorf("code = %v, want %v", se, service.ErrTargetExists)
	}
}

func TestPlan_NoWrites(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "plan-only")
	res, err := svc.Plan(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    validInputs(),
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(res.Files) == 0 {
		t.Fatal("Plan returned no files")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("Plan should not create target dir; stat returned %v", err)
	}
	// Preview content for the README should be present + readable.
	var sawReadme bool
	for _, f := range res.Files {
		if f.Path == "README.md" {
			sawReadme = true
			if !strings.Contains(f.Preview, "SmokeTest") {
				t.Errorf("README preview missing PascalCase title: %q", f.Preview)
			}
		}
	}
	if !sawReadme {
		t.Error("Plan did not surface README.md")
	}
}

func TestValidatePreset_FromPath(t *testing.T) {
	svc := newTestService(t)
	abs, _ := filepath.Abs(filepath.Join("testdata", "presets", "sample", "preset.yaml"))
	res, p, err := svc.ValidatePreset(abs)
	if err != nil {
		t.Fatalf("ValidatePreset: %v", err)
	}
	if !res.OK() {
		t.Errorf("expected clean validation, got errors: %v", res.Errors)
	}
	if p == nil || p.ID != "sample" {
		t.Errorf("ValidatePreset returned wrong preset: %+v", p)
	}
}

func TestNew_FilesDigestStable(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "stable")
	res, err := svc.New(service.NewOptions{
		PresetID:  "sample",
		TargetDir: target,
		Inputs:    validInputs(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Re-read written files and recompute digest — must match the digest
	// recorded in the manifest.
	for _, fr := range res.Files {
		raw, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(fr.Path)))
		if err != nil {
			t.Fatalf("read written file %s: %v", fr.Path, err)
		}
		got := manifest.Digest(raw)
		if got != fr.Digest {
			t.Errorf("digest drift for %s: recorded %q, recomputed %q", fr.Path, fr.Digest, got)
		}
	}
}

func TestNew_PostRenderEmitsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "preset.yaml"), []byte(`folio_version: "0.1"
id: warn-postrender
version: 1.0.0
files:
  source: ./files
post_render:
  blueprint: ./hooks/bootstrap.hadron.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files", "README.md"), []byte("# warn\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := service.New(service.Options{
		BundledFS:    os.DirFS(filepath.Dir(dir)),
		BundledRoot:  ".",
		UserDir:      t.TempDir(),
		FolioVersion: "0.1.0-test",
		Now:          time.Now(),
	})
	target := filepath.Join(t.TempDir(), "out")
	res, err := svc.New(service.NewOptions{
		PresetID:  filepath.Base(dir),
		TargetDir: target,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected post_render warning, got none")
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "post_render") {
		t.Errorf("warnings should mention post_render: %v", res.Warnings)
	}
}
