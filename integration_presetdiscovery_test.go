package folio_test

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// allPresetInputs holds a valid input set for rendering each bundled preset
// on its own. TestPresetDiscovery_EveryBundledPresetHasInputs fails when a
// preset is added without one, so the render-invariance test below cannot
// silently skip it.
var allPresetInputs = map[string]map[string]any{
	"base":           {"project_name": "smoke", "github_owner": "chrispian", "description": "x"},
	"go-package":     {"project_name": "smoke", "github_owner": "chrispian", "package_name": "greeter"},
	"go-baseline":    {"repo_name": "go-smoke"},
	"go-lib":         {"repo_name": "go-smoke-lib", "package_name": "smoke", "description": "d"},
	"go-service-app": {"repo_name": "svc-app", "store_pkg_name": "store", "store_type": "Store"},
	"ts-plugin":      {"plugin_name": "minimal", "description": "d"},
	"nanite-plugin":  {"plugin_name": "minimal", "github_owner": "hollis-labs", "description": "d"},
	"chat-app":       {"project_name": "acme_chat", "github_owner": "hollis-labs", "description": "d"},
	"app-dashboard":  {"project_name": "acme_sysop", "github_owner": "hollis-labs", "description": "d"},
}

func bundledIDs(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(folio.BundledPresets, "presets")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

// TestPresetDiscovery_ListMatchesEmbeddedPresets is the drift guard the README
// table never had: ListPresets over the real compiled embed.FS must name
// exactly the preset directories that ship, and mark only go-baseline as a
// layer.
func TestPresetDiscovery_ListMatchesEmbeddedPresets(t *testing.T) {
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir()})
	got, warnings, err := svc.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
		if p.Source != "bundled" || p.Version == "" || p.Description == "" || p.Author == "" {
			t.Errorf("incomplete summary: %+v", p)
		}
		if p.LayerOnly != (p.ID == "go-baseline") {
			t.Errorf("%s LayerOnly = %v", p.ID, p.LayerOnly)
		}
	}
	if want := bundledIDs(t); strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("listed %v, embedded dirs %v", ids, want)
	}
}

func TestPresetDiscovery_EveryBundledPresetHasInputs(t *testing.T) {
	for _, id := range bundledIDs(t) {
		if _, ok := allPresetInputs[id]; !ok {
			t.Errorf("bundled preset %q has no entry in allPresetInputs", id)
		}
	}
}

// renderDigests renders one preset from bundled and returns path -> digest of
// every file, the same digests recorded in .folio.yaml.
func renderDigests(t *testing.T, bundled fs.FS, id string) map[string]string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "out")
	svc := service.New(service.Options{
		BundledFS:    bundled,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	})
	res, err := svc.New(service.NewOptions{PresetID: id, TargetDir: target, Inputs: allPresetInputs[id]})
	if err != nil {
		t.Fatalf("render %s: %v", id, err)
	}
	out := map[string]string{}
	for _, f := range res.Files {
		out[f.Path] = f.Digest
	}
	// The manifest on disk must record the same digests.
	m, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for p, rec := range m.Files {
		if out[p] != rec.DigestAtGen {
			t.Errorf("%s %s: result digest %q != manifest %q", id, p, out[p], rec.DigestAtGen)
		}
	}
	return out
}

var layerOnlyLine = regexp.MustCompile(`(?m)^layer_only:.*\n`)

// TestPresetDiscovery_LayerOnlyDoesNotChangeRenderedOutput proves the schema
// addition is inert: every bundled preset is rendered as shipped and again
// with its layer_only line removed from preset.yaml, and every rendered file's
// digest (the value recorded in .folio.yaml) must be identical.
func TestPresetDiscovery_LayerOnlyDoesNotChangeRenderedOutput(t *testing.T) {
	stripped := fstest.MapFS{}
	sawLayerOnly := false
	err := fs.WalkDir(folio.BundledPresets, "presets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(folio.BundledPresets, p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, "/preset.yaml") && strings.Count(p, "/") == 2 {
			without := layerOnlyLine.ReplaceAll(data, nil)
			if len(without) != len(data) {
				sawLayerOnly = true
			}
			data = without
		}
		stripped[p] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawLayerOnly {
		t.Fatal("no bundled preset declares layer_only; the test would prove nothing")
	}
	for _, id := range bundledIDs(t) {
		t.Run(id, func(t *testing.T) {
			with := renderDigests(t, folio.BundledPresets, id)
			without := renderDigests(t, stripped, id)
			if len(with) == 0 {
				t.Fatal("rendered no files")
			}
			if len(with) != len(without) {
				t.Fatalf("file count differs: %d vs %d", len(with), len(without))
			}
			for p, d := range with {
				if without[p] != d {
					t.Errorf("%s digest changed: %s vs %s", p, d, without[p])
				}
			}
		})
	}
}
