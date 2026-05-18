package folio_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// TestIntegration_SysopUIPreset_Defaults renders the sysop-ui preset with
// default inputs and asserts the scaffold's shape: the Go module + serving
// package, the frontend tree, and the wiring that ties them together.
//
// Unlike the base preset's test it does not run `go build` / `npm build`:
// the scaffold depends on github.com/hollis-labs/go-webui and
// @hollis-labs/sysop-ui, which are not resolvable in an offline test
// sandbox. This mirrors the nanite-plugin preset test, which likewise
// asserts structure for a scaffold carrying an external hollis-labs dep.
func TestIntegration_SysopUIPreset_Defaults(t *testing.T) {
	target := filepath.Join(t.TempDir(), "sysop-app")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "sysop-ui",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "acme_sysop",
			"github_owner": "hollis-labs",
			"description":  "Acme operations console",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	want := []string{
		".folio.yaml",
		".github/workflows/ci.yml",
		".gitignore",
		"LICENSE",
		"Makefile",
		"README.md",
		"go.mod",
		"cmd/acme_sysop/main.go",
		"internal/webui/embed.go",
		"internal/webui/dist/.gitkeep",
		"frontend/index.html",
		"frontend/package.json",
		"frontend/tsconfig.json",
		"frontend/vite.config.ts",
		"frontend/src/main.tsx",
		"frontend/src/App.tsx",
		"frontend/src/index.css",
		"frontend/src/pages/dashboard.tsx",
		"frontend/src/api/client.ts",
		"frontend/src/api/context.tsx",
	}
	for _, p := range want {
		if _, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(p))); statErr != nil {
			t.Errorf("expected %s, missing: %v", p, statErr)
		}
	}

	// go.mod — module path + the go-webui dependency.
	gomod := readFile(t, target, "go.mod")
	if !strings.Contains(gomod, "module github.com/hollis-labs/acme_sysop") {
		t.Errorf("go.mod missing module declaration:\n%s", gomod)
	}
	if !strings.Contains(gomod, "require github.com/hollis-labs/go-webui v0.1.0") {
		t.Errorf("go.mod missing go-webui dependency:\n%s", gomod)
	}

	// embed.go — the //go:embed directive, base path, and go-webui handler.
	embed := readFile(t, target, "internal/webui/embed.go")
	for _, frag := range []string{"//go:embed all:dist", `BasePath = "/sysop"`, "gowebui.Handler"} {
		if !strings.Contains(embed, frag) {
			t.Errorf("embed.go missing %q:\n%s", frag, embed)
		}
	}

	// main.go — mounts the webui package.
	mainGo := readFile(t, target, "cmd/acme_sysop/main.go")
	if !strings.Contains(mainGo, "webui.Mount(mux)") {
		t.Errorf("main.go does not mount the webui handler:\n%s", mainGo)
	}

	// package.json — the sysop-ui kit as a pinned git dependency.
	pkg := readFile(t, target, "frontend/package.json")
	if !strings.Contains(pkg, `"@hollis-labs/sysop-ui": "github:hollis-labs/sysop-ui#v0.4.0"`) {
		t.Errorf("frontend/package.json missing pinned sysop-ui git dependency:\n%s", pkg)
	}

	// vite.config.ts — base path + the build output aimed at the Go embed dir.
	vite := readFile(t, target, "frontend/vite.config.ts")
	if !strings.Contains(vite, "base: '/sysop/'") {
		t.Errorf("vite.config.ts missing base path:\n%s", vite)
	}
	if !strings.Contains(vite, "outDir: '../internal/webui/dist'") {
		t.Errorf("vite.config.ts does not target the Go embed dir:\n%s", vite)
	}

	// Makefile — the ui-build / ui-dev targets are the headline feature.
	makefile := readFile(t, target, "Makefile")
	for _, tgt := range []string{"ui-build:", "ui-dev:", "build:", "install:"} {
		if !strings.Contains(makefile, tgt) {
			t.Errorf("Makefile missing %q target:\n%s", tgt, makefile)
		}
	}

	// Manifest records the sysop-ui preset.
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(mf.Presets) != 1 || mf.Presets[0].ID != "sysop-ui" {
		t.Errorf("manifest presets = %+v, want single sysop-ui entry", mf.Presets)
	}
	if mf.Computed["app_title"] != "AcmeSysop" {
		t.Errorf("manifest computed.app_title = %v, want AcmeSysop", mf.Computed["app_title"])
	}
}

// TestIntegration_SysopUIPreset_CustomBasePath verifies a non-default
// base_path threads through every place it is consumed — the Go serving
// constant and the Vite base.
func TestIntegration_SysopUIPreset_CustomBasePath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ops-app")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "sysop-ui",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "ops_console",
			"github_owner": "hollis-labs",
			"base_path":    "/ops",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	if embed := readFile(t, target, "internal/webui/embed.go"); !strings.Contains(embed, `BasePath = "/ops"`) {
		t.Errorf("embed.go did not pick up custom base_path:\n%s", embed)
	}
	if vite := readFile(t, target, "frontend/vite.config.ts"); !strings.Contains(vite, "base: '/ops/'") {
		t.Errorf("vite.config.ts did not pick up custom base_path:\n%s", vite)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}
