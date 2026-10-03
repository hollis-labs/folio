package folio_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/service"
)

func renderTSPlugin(t *testing.T, ui bool, sdk string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "plugin")
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version, Now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)})
	inputs := map[string]any{"plugin_name": "echo-plugin", "description": "Echo \"quoted\" text\nwith a newline", "include_ui": ui}
	if sdk != "" {
		inputs["sdk_spec"] = sdk
	}
	_, err := svc.New(service.NewOptions{PresetID: "ts-plugin", TargetDir: target, Inputs: inputs})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

// Rendering executes only dependency-free schema generation and validation.
// npm installation and bundled SDK acceptance are explicit, opt-in gates.
func TestIntegration_TSPluginSchema(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required for generated schema checks")
	}
	for _, ui := range []bool{false, true} {
		t.Run(map[bool]string{false: "core", true: "ui"}[ui], func(t *testing.T) {
			target := renderTSPlugin(t, ui, "")
			runTSCommand(t, target, "node", "scripts/schema.mjs")
			if modules := os.Getenv("FOLIO_TS_FAST_MODULES"); modules != "" {
				if err := os.Symlink(modules, filepath.Join(target, "node_modules")); err != nil {
					t.Fatal(err)
				}
				runTSCommand(t, target, "node", filepath.Join(modules, "typescript/bin/tsc"), "--noEmit")
			}
			runTSCommand(t, target, "node", "--test", "test/schema.test.mjs")
			if _, err := os.Stat(filepath.Join(target, "ui/index.ts")); ui && err != nil {
				t.Fatal(err)
			} else if !ui && !os.IsNotExist(err) {
				t.Fatalf("optional UI rendered: %v", err)
			}
		})
	}
}

func runTSCommand(t *testing.T, dir, command string, args ...string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", command, args, err, output)
	}
}

// Set FOLIO_TS_SDK_SPEC to a local protocol-2 SDK tarball. This test invokes
// dependency installation and belongs in the serialized end gate only.
func TestIntegration_TSPluginAcceptance(t *testing.T) {
	sdk := os.Getenv("FOLIO_TS_SDK_SPEC")
	if sdk == "" {
		t.Skip("set FOLIO_TS_SDK_SPEC for rendered protocol-2 SDK acceptance")
	}
	for _, ui := range []bool{false, true} {
		t.Run(map[bool]string{false: "core", true: "ui"}[ui], func(t *testing.T) {
			target := renderTSPlugin(t, ui, sdk)
			runTSCommand(t, target, "npm", "install", "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund")
			runTSCommand(t, target, "npm", "ci", "--no-audit", "--no-fund")
			runTSCommand(t, target, "npm", "run", "typecheck")
			if os.Getenv("FOLIO_TS_SERVE_V2_HELD") == "1" {
				runTSCommand(t, target, "npm", "run", "build")
				runTSCommand(t, target, "node", "--test", "test/schema.test.mjs")
				runTSCommand(t, target, "node", "--test", "test/reload.test.mjs")
				t.Log("HOLD: protocol-2 harness and stdio execution await Serve v2; no fallback executed")
			} else {
				runTSCommand(t, target, "npm", "test")
			}
			runTSCommand(t, target, "npm", "run", "verify")
		})
	}
}
