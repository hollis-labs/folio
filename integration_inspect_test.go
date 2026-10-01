package folio_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/service"
)

// The bundled base, go-baseline (through go-lib), app-dashboard and nanite-plugin
// presets all render LICENSE from computed.year = {{ .now.Year }}. Inspecting
// years after generation must not call LICENSE (or anything else) drifted.
func TestIntegration_InspectAfterYearRollover_ReportsNoDrift(t *testing.T) {
	cases := []struct {
		preset string
		inputs map[string]any
	}{
		{"base", map[string]any{"project_name": "yr_base", "github_owner": "me", "description": "x"}},
		{"go-lib", map[string]any{"repo_name": "yr-lib", "package_name": "yrlib", "description": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			mk := func(now time.Time) *service.Service {
				return service.New(service.Options{
					BundledFS:    folio.BundledPresets,
					BundledRoot:  "presets",
					UserDir:      t.TempDir(),
					FolioVersion: folio.Version,
					Now:          now,
				})
			}
			target := filepath.Join(t.TempDir(), "proj")
			if _, err := mk(time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)).New(service.NewOptions{
				PresetID: tc.preset, TargetDir: target, Inputs: tc.inputs,
			}); err != nil {
				t.Fatalf("New: %v", err)
			}
			res, err := mk(time.Date(2031, 8, 9, 10, 11, 12, 0, time.UTC)).Inspect(service.InspectOptions{TargetDir: target})
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if len(res.Files) == 0 {
				t.Fatal("no files reported")
			}
			for _, f := range res.Files {
				if f.Status != service.DriftUnchanged {
					t.Errorf("%s = %s, want unchanged", f.Path, f.Status)
				}
			}
			if len(res.Warnings) != 0 {
				t.Errorf("warnings = %v", res.Warnings)
			}
		})
	}
}
