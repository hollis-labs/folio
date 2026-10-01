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

// TestIntegration_AppDashboardPreset_Defaults renders the app-dashboard preset with
// default inputs and asserts the scaffold's shape: the Go module + serving
// package, the frontend tree, and the wiring that ties them together.
//
// Structural rendering remains offline; the opt-in frontend integration below
// installs the published packages and exercises their actual export/build surface.
func TestIntegration_AppDashboardPreset_Defaults(t *testing.T) {
	target := filepath.Join(t.TempDir(), "dashboard-app")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "app-dashboard",
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
		"frontend/eslint.config.js",
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
	if !strings.Contains(gomod, "\ngo 1.26.6\n") {
		t.Errorf("go.mod missing default go directive 1.26.6:\n%s", gomod)
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

	// package.json — published design-kit packages rather than a git-tag kit.
	pkg := readFile(t, target, "frontend/package.json")
	for _, name := range []string{"design-tokens", "design-components", "design-app-runtime", "kit-dashboard", "eslint-config-design"} {
		if !strings.Contains(pkg, `"@hollis-labs/`+name+`": "^0.1.0"`) {
			t.Errorf("frontend/package.json missing published %s dependency:\n%s", name, pkg)
		}
	}

	// vite.config.ts — base path + the build output aimed at the Go embed dir.
	vite := readFile(t, target, "frontend/vite.config.ts")
	if !strings.Contains(vite, `base: "/sysop/"`) {
		t.Errorf("vite.config.ts missing base path:\n%s", vite)
	}
	if !strings.Contains(vite, `outDir: "../internal/webui/dist"`) {
		t.Errorf("vite.config.ts does not target the Go embed dir:\n%s", vite)
	}

	// Makefile — the ui-build / ui-dev targets are the headline feature.
	makefile := readFile(t, target, "Makefile")
	for _, tgt := range []string{"ui-build:", "ui-dev:", "build:", "install:"} {
		if !strings.Contains(makefile, tgt) {
			t.Errorf("Makefile missing %q target:\n%s", tgt, makefile)
		}
	}

	// Manifest records the app-dashboard preset.
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(mf.Presets) != 1 || mf.Presets[0].ID != "app-dashboard" {
		t.Errorf("manifest presets = %+v, want single app-dashboard entry", mf.Presets)
	}
	if mf.Computed["app_title"] != "AcmeSysop" {
		t.Errorf("manifest computed.app_title = %v, want AcmeSysop", mf.Computed["app_title"])
	}
}

// TestIntegration_AppDashboardPreset_CustomBasePath verifies a non-default
// base_path threads through every place it is consumed — the Go serving
// constant and the Vite base.
func TestIntegration_AppDashboardPreset_CustomBasePath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ops-app")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "app-dashboard",
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
	if vite := readFile(t, target, "frontend/vite.config.ts"); !strings.Contains(vite, `base: "/ops/"`) {
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

// TestIntegration_AppDashboardPreset_Frontend checks a real consumer of the
// published packages, without making the offline Go suite require npm/network.
func TestIntegration_AppDashboardPreset_Frontend(t *testing.T) {
	if os.Getenv("FOLIO_FRONTEND_E2E") != "1" {
		t.Skip("set FOLIO_FRONTEND_E2E=1 to build and lint the generated frontend")
	}
	target := filepath.Join(t.TempDir(), "dashboard")
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
	if _, err := svc.New(service.NewOptions{
		PresetID: "app-dashboard", TargetDir: target,
		Inputs: map[string]any{"project_name": "smoke_dashboard", "github_owner": "hollis-labs"},
	}); err != nil {
		t.Fatal(err)
	}
	probeHome := t.TempDir()
	for _, args := range [][]string{{"install"}, {"run", "typecheck"}, {"run", "lint"}, {"run", "build"}} {
		cmd := exec.Command("npm", args...)
		cmd.Dir = filepath.Join(target, "frontend")
		cmd.Env = append(os.Environ(), "HOME="+probeHome)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("npm %v: %v\n%s", args, err, output)
		} else {
			t.Logf("npm %v:\n%s", args, output)
		}
	}
	for _, args := range [][]string{{"mod", "tidy"}, {"build", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = target
		cmd.Env = append(os.Environ(), "HOME="+probeHome, "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -modcacherw"))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, output)
		}
	}
}

func TestIntegration_AppDashboardPreset_LegacyAlias(t *testing.T) {
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
	target := filepath.Join(t.TempDir(), "legacy")
	if _, err := svc.New(service.NewOptions{
		PresetID: "sysop-ui", TargetDir: target,
		Inputs: map[string]any{"project_name": "legacy_dashboard", "github_owner": "hollis-labs", "sysop_ui_version": "v0.4.0"},
	}); err != nil {
		t.Fatal(err)
	}
	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatal(err)
	}
	if mf.Presets[0].ID != "app-dashboard" {
		t.Fatalf("alias did not record canonical preset: %+v", mf.Presets)
	}
	// Simulate an existing breadcrumb, whose id predates the preset rename.
	mf.Presets[0].ID = "sysop-ui"
	mf.Presets[0].Version = "1.2.0"
	mf.Inputs["sysop_ui_version"] = "v0.4.0"
	if err := manifest.Write(target, mf); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Inspect(service.InspectOptions{TargetDir: target}); err != nil {
		t.Fatalf("inspect of old sysop-ui breadcrumb: %v", err)
	}
}
