package folio_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

func TestIntegration_ChimeraHostChoice(t *testing.T) {
	for _, preset := range []string{"app-dashboard", "chat-app"} {
		for _, host := range []string{"go-webui", "chimera"} {
			for _, base := range []string{"/", "/review"} {
				t.Run(preset+host+base, func(t *testing.T) {
					target := filepath.Join(t.TempDir(), "consumer")
					svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
					_, err := svc.New(service.NewOptions{PresetID: preset, TargetDir: target, Inputs: map[string]any{"project_name": "fresh_consumer", "github_owner": "hollis-labs", "gui_host": host, "base_path": base}})
					if err != nil {
						t.Fatal(err)
					}
					report, err := svc.Inspect(service.InspectOptions{TargetDir: target})
					if err != nil {
						t.Fatal(err)
					}
					for _, row := range report.Files {
						if row.Status != service.DriftUnchanged {
							t.Fatalf("initial drift: %+v", row)
						}
					}
					breadcrumb, err := manifest.Read(target)
					if err != nil {
						t.Fatal(err)
					}
					if breadcrumb.Inputs["gui_host"] != host {
						t.Fatalf("host breadcrumb: %v", breadcrumb.Inputs)
					}
					for path, record := range breadcrumb.Files {
						bytes, readErr := os.ReadFile(filepath.Join(target, filepath.FromSlash(path)))
						if readErr != nil {
							t.Fatal(readErr)
						}
						if got := manifest.Digest(bytes); got != record.DigestAtGen {
							t.Fatalf("LF digest mismatch %s", path)
						}
					}
					if breadcrumb.Inputs["chimera_version"] != "v0.0.0-20261008115211-389155313ee5" || breadcrumb.Inputs["chimera_gui_recipe"] != "design-0.4.0-react-19.3.0" {
						t.Fatal("selected contract pin metadata missing")
					}
					edited := filepath.Join(target, "README.md")
					changed := readFile(t, target, "README.md") + "\nLocal note\n"
					if writeErr := os.WriteFile(edited, []byte(changed), 0600); writeErr != nil {
						t.Fatal(writeErr)
					}
					drift, err := svc.Inspect(service.InspectOptions{TargetDir: target})
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, row := range drift.Files {
						if row.Path == "README.md" && row.Status == service.DriftLocallyModified {
							found = true
						}
					}
					if !found || readFile(t, target, "README.md") != changed {
						t.Fatal("inspect failed readonly local drift reporting")
					}
					for _, optional := range []string{"third_party/hollis-labs-plugin-host-ui-0.1.0.tgz", "plugin-provenance.json", "frontend/src/Plugin.tsx", "frontend/src/vendor/chimera/composition.ts"} {
						if _, statErr := os.Stat(filepath.Join(target, optional)); !os.IsNotExist(statErr) {
							t.Fatalf("non-plugin preset acquired %s: %v", optional, statErr)
						}
					}
					lockpath := filepath.Join(target, "frontend/package-lock.json")
					if host == "go-webui" {
						if _, err := os.Stat(lockpath); !os.IsNotExist(err) {
							t.Fatalf("legacy branch acquired lock: %v", err)
						}
						if _, err := os.Stat(filepath.Join(target, "chimera-provenance.json")); !os.IsNotExist(err) {
							t.Fatalf("legacy branch acquired provenance: %v", err)
						}
						if !strings.Contains(readFile(t, target, "go.mod"), "go-webui v0.1.0") {
							t.Fatal("legacy module changed")
						}
						return
					}
					var lock struct {
						Name     string
						Packages map[string]struct{ Name string }
					}
					if err := json.Unmarshal([]byte(readFile(t, target, "frontend/package-lock.json")), &lock); err != nil {
						t.Fatal(err)
					}
					if lock.Name != "fresh_consumer-frontend" || lock.Packages[""].Name != lock.Name {
						t.Fatal("root lock names disagree")
					}
					if !strings.Contains(readFile(t, target, "go.mod"), "chimera v0.0.0-20261008115211-389155313ee5") {
						t.Fatal("public host pin missing")
					}
					if strings.Contains(readFile(t, target, "frontend/vite.config.ts"), `base: "//"`) {
						t.Fatal("root base became protocol-relative")
					}
					if !strings.Contains(readFile(t, target, "cmd/fresh_consumer/main.go"), "signal.NotifyContext") {
						t.Fatal("maintained lifecycle missing")
					}
				})
			}
		}
	}
}

func TestIntegration_ChimeraRejectsUnsupportedChoice(t *testing.T) {
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir()})
	for _, input := range []map[string]any{{"gui_host": "other"}, {"chimera_version": "v99.0.0"}, {"chimera_gui_recipe": "unknown"}, {"base_path": "//review"}, {"base_path": "/review/.."}, {"base_path": "/review?query"}, {"base_path": "/review/"}} {
		input["project_name"] = "invalid_choice"
		if _, err := svc.New(service.NewOptions{PresetID: "app-dashboard", TargetDir: filepath.Join(t.TempDir(), "invalid"), Inputs: input}); err == nil {
			t.Fatalf("accepted unsupported input: %v", input)
		}
	}
}

func TestIntegration_ChimeraPluginDistribution(t *testing.T) {
	for _, preset := range []string{"app-dashboard-chimera-plugin", "chat-app-chimera-plugin"} {
		t.Run(preset, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "plugin")
			svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
			_, err := svc.New(service.NewOptions{PresetID: preset, TargetDir: target, Inputs: map[string]any{"project_name": "licensed_consumer", "github_owner": "hollis-labs", "base_path": "/review"}})
			if err != nil {
				t.Fatal(err)
			}
			archive, err := os.ReadFile(filepath.Join(target, "third_party/hollis-labs-plugin-host-ui-0.1.0.tgz"))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(archive)); got != "360c4bd74df7369d57af74151bba0ff0739b31b5c67499f1c01c15cefb4b24b0" {
				t.Fatalf("raw archive changed: %s", got)
			}
			var provenance struct {
				Baseline string `json:"baseline"`
				Adapters []struct {
					Local  string `json:"local"`
					SHA256 string `json:"sha256"`
				} `json:"adapters"`
			}
			if decodeErr := json.Unmarshal([]byte(readFile(t, target, "plugin-provenance.json")), &provenance); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if provenance.Baseline != "389155313ee5b95f4125e64b61077ddf4257e3d6" || len(provenance.Adapters) != 4 {
				t.Fatal("immutable closure provenance missing")
			}
			for _, adapter := range provenance.Adapters {
				bytes, readErr := os.ReadFile(filepath.Join(target, adapter.Local))
				if readErr != nil {
					t.Fatal(readErr)
				}
				if fmt.Sprintf("%x", sha256.Sum256(bytes)) != adapter.SHA256 {
					t.Fatalf("adapter changed: %s", adapter.Local)
				}
			}
			for _, license := range []string{"third_party/LICENSE-plugin-host-ui.txt", "frontend/src/vendor/chimera/LICENSE", "frontend/public/LICENSE-Chimera.txt"} {
				if !strings.Contains(readFile(t, target, license), "MIT License") {
					t.Fatalf("license missing: %s", license)
				}
			}
			breadcrumb, err := manifest.Read(target)
			if err != nil {
				t.Fatal(err)
			}
			if len(breadcrumb.Presets) != 3 {
				t.Fatalf("expected base, fixture and wrapper: %+v", breadcrumb.Presets)
			}
			for path, record := range breadcrumb.Files {
				bytes, readErr := os.ReadFile(filepath.Join(target, path))
				if readErr != nil {
					t.Fatal(readErr)
				}
				if manifest.Digest(bytes) != record.DigestAtGen {
					t.Fatalf("LF breadcrumb mismatch: %s", path)
				}
			}
			report, err := svc.Inspect(service.InspectOptions{TargetDir: target})
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range report.Files {
				if row.Status != service.DriftUnchanged {
					t.Fatalf("initial plugin drift: %+v", row)
				}
			}
		})
	}
}
