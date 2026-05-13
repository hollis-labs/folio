package folio_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/service"
)

// TestIntegration_NanitePluginPreset_MinimalCore exercises the
// nanite-plugin preset with every capability flag off. The result
// should be the minimal Init/Load/Unload skeleton — no command.go,
// event.go, mcp.go, http.go, envelopes/, or ui/ files.
func TestIntegration_NanitePluginPreset_MinimalCore(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nanite-plugin-minimal")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "nanite-plugin",
		TargetDir: target,
		Inputs: map[string]any{
			"plugin_name":  "minimal",
			"github_owner": "hollis-labs",
			"description":  "minimal smoke plugin",
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	want := []string{
		".folio.yaml",
		".gitignore",
		"CHANGELOG.md",
		"LICENSE",
		"Makefile",
		"README.md",
		"cmd/sign/main.go",
		"go.mod",
		"main.go",
		"plugin.yaml",
	}
	for _, p := range want {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(p))); err != nil {
			t.Errorf("expected %s, missing: %v", p, err)
		}
	}

	unwanted := []string{
		"command.go",
		"event.go",
		"http.go",
		"mcp.go",
		"envelopes",
		"ui",
	}
	for _, p := range unwanted {
		if _, err := os.Stat(filepath.Join(target, p)); err == nil {
			t.Errorf("did not expect %s in minimal scaffold", p)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(target, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"commands:", "events:", "mcp_servers:", "http_routes:", "envelopes:", "ui:"} {
		if strings.Contains(string(manifest), missing) {
			t.Errorf("manifest unexpectedly contains %q (handler should be gated off):\n%s", missing, manifest)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gomod), "module github.com/hollis-labs/nanite-plugin-minimal") {
		t.Errorf("go.mod missing module declaration:\n%s", gomod)
	}
	if !strings.Contains(string(gomod), "github.com/hollis-labs/plugin-sdk v0.3.0") {
		t.Errorf("go.mod missing default SDK pin:\n%s", gomod)
	}
}

// TestIntegration_NanitePluginPreset_AllHandlers turns on every
// capability flag and asserts the corresponding files and manifest
// blocks materialize.
func TestIntegration_NanitePluginPreset_AllHandlers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nanite-plugin-kitchen")

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	})

	_, err := svc.New(service.NewOptions{
		PresetID:  "nanite-plugin",
		TargetDir: target,
		Inputs: map[string]any{
			"plugin_name":       "kitchen",
			"github_owner":      "hollis-labs",
			"description":       "kitchen-sink smoke plugin",
			"license":           "Apache-2.0",
			"handler_command":   true,
			"handler_event":     true,
			"handler_mcp":       true,
			"handler_http":      true,
			"include_envelopes": true,
			"include_ui":        true,
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	want := []string{
		"command.go",
		"event.go",
		"http.go",
		"mcp.go",
		"envelopes/kitchen-card.schema.json",
		"ui/package.json",
		"ui/tsconfig.json",
		"ui/vite.config.ts",
		"ui/src/index.tsx",
		"ui/src/components/KitchenCard.tsx",
	}
	for _, p := range want {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(p))); err != nil {
			t.Errorf("expected %s, missing: %v", p, err)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(target, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{"commands:", "events:", "mcp_servers:", "http_routes:", "envelopes:", "ui:"} {
		if !strings.Contains(string(manifest), present) {
			t.Errorf("manifest missing expected block %q:\n%s", present, manifest)
		}
	}

	license, err := os.ReadFile(filepath.Join(target, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(license), "Apache License") {
		t.Errorf("LICENSE did not select Apache-2.0 branch:\n%s", license)
	}

	makefile, err := os.ReadFile(filepath.Join(target, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "KITCHEN_SIGNING_KEY") {
		t.Errorf("Makefile missing computed signing-key env var:\n%s", makefile)
	}
	if !strings.Contains(string(makefile), "build-ui") {
		t.Errorf("Makefile missing build-ui target when UI included:\n%s", makefile)
	}
}
