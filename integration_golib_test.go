package folio_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/service"
)

// TestIntegration_GoLibPreset exercises the go-lib preset — a standalone
// (non-composed) importable shared-library scaffold. The result must be a
// module-root package with no internal/, no cmd/, and no Makefile, and the
// generated tree must vet clean.
func TestIntegration_GoLibPreset(t *testing.T) {
	target := filepath.Join(t.TempDir(), "go-smoke-lib")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "go-lib",
		TargetDir: target,
		Inputs: map[string]any{
			"repo_name":    "go-smoke-lib",
			"package_name": "smoke",
			"description":  "go-lib preset smoke library",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	want := []string{
		".folio.yaml",
		".github/workflows/check.yml",
		".gitignore",
		"CHANGELOG.md",
		"LICENSE",
		"README.md",
		"examples/README.md",
		"go.mod",
		"smoke/doc.go",
		"smoke/smoke.go",
		"smoke/smoke_test.go",
	}
	for _, p := range want {
		if _, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(p))); statErr != nil {
			t.Errorf("expected %s, missing: %v", p, statErr)
		}
	}

	// go-lib is an importable shared-library layout: the package sits at the
	// module root. internal/ would make it un-importable; cmd/ and Makefile
	// belong to the binary-oriented base/go-package presets, not here.
	for _, p := range []string{"internal", "cmd", "Makefile"} {
		if _, statErr := os.Stat(filepath.Join(target, p)); statErr == nil {
			t.Errorf("go-lib scaffold should not contain %q", p)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gomod), "module github.com/hollis-labs/go-smoke-lib") {
		t.Errorf("go.mod missing expected module declaration:\n%s", gomod)
	}
	if !strings.Contains(string(gomod), "go 1.26.1") {
		t.Errorf("go.mod missing default go directive:\n%s", gomod)
	}

	doc, err := os.ReadFile(filepath.Join(target, "smoke", "doc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "package smoke") {
		t.Errorf("smoke/doc.go missing package clause:\n%s", doc)
	}

	license, err := os.ReadFile(filepath.Join(target, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(license), "Copyright (c) 2026 Hollis Labs") {
		t.Errorf("LICENSE missing expected copyright line:\n%s", license)
	}

	// Generated files must be owner-writable. Bundled presets live in an
	// embed.FS (every file reports mode 0o444); rendering must not propagate
	// that read-only bit, or `go mod tidy` and ordinary edits fail.
	for _, p := range []string{"go.mod", "README.md", "smoke/smoke.go"} {
		info, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(p)))
		if statErr != nil {
			t.Errorf("stat %s: %v", p, statErr)
			continue
		}
		if info.Mode().Perm()&0o200 == 0 {
			t.Errorf("%s rendered read-only (mode %o); generated files must be owner-writable", p, info.Mode().Perm())
		}
	}

	// The generated tree vets clean.
	if _, err := exec.LookPath("go"); err == nil {
		cmd := exec.Command("go", "vet", "./...")
		cmd.Dir = target
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("go vet in %s: %v\n%s", target, err, out)
		}
	}
}
