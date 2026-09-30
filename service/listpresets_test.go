package service_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hollis-labs/folio/service"
)

func presetYAML(id, version, desc string, layer bool) string {
	s := "folio_version: \"0.1\"\nid: " + id + "\nversion: " + version + "\ndescription: " + desc + "\nauthor: tester\n"
	if layer {
		s += "layer_only: true\n"
	}
	return s + "files:\n  source: ./files\n"
}

func bundledFixture() fstest.MapFS {
	return fstest.MapFS{
		"presets/zeta/preset.yaml":  {Data: []byte(presetYAML("zeta", "1.0.0", "last", false))},
		"presets/alpha/preset.yaml": {Data: []byte(presetYAML("alpha", "2.0.0", "first", false))},
		"presets/lay/preset.yaml":   {Data: []byte(presetYAML("lay", "0.1.0", "a layer", true))},
		"presets/stray.txt":         {Data: []byte("not a preset")},
	}
}

func writeUserPreset(t *testing.T, userDir, dirName, body string) {
	t.Helper()
	dir := filepath.Join(userDir, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preset.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListPresets_ReturnsAllBundled(t *testing.T) {
	svc := service.New(service.Options{BundledFS: bundledFixture(), UserDir: t.TempDir()})
	got, warnings, err := svc.ListPresets()
	if err != nil {
		t.Fatalf("ListPresets: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
		if p.Source != "bundled" {
			t.Errorf("%s source = %q, want bundled", p.ID, p.Source)
		}
	}
	if want := "alpha,lay,zeta"; strings.Join(ids, ",") != want {
		t.Errorf("ids = %v, want %s (sorted, files ignored)", ids, want)
	}
	if got[0].Version != "2.0.0" || got[0].Description != "first" || got[0].Author != "tester" {
		t.Errorf("alpha metadata wrong: %+v", got[0])
	}
}

func TestListPresets_LayerOnlyFlag(t *testing.T) {
	svc := service.New(service.Options{BundledFS: bundledFixture(), UserDir: t.TempDir()})
	got, _, err := svc.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if want := p.ID == "lay"; p.LayerOnly != want {
			t.Errorf("%s LayerOnly = %v, want %v", p.ID, p.LayerOnly, want)
		}
	}
}

func TestListPresets_IncludesUserDir(t *testing.T) {
	userDir := t.TempDir()
	writeUserPreset(t, userDir, "mine@1.0.0", presetYAML("mine", "1.0.0", "old", false))
	writeUserPreset(t, userDir, "mine@1.2.0", presetYAML("mine", "1.2.0", "new", false))
	svc := service.New(service.Options{BundledFS: bundledFixture(), UserDir: userDir})
	got, warnings, err := svc.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (3 bundled + 1 user): %+v", len(got), got)
	}
	last := got[3]
	if last.ID != "mine" || last.Source != "local" || last.Version != "1.2.0" || last.Description != "new" {
		t.Errorf("user preset = %+v, want highest version 1.2.0 from local", last)
	}
}

func TestListPresets_SkipsInvalidUserPresetWithWarning(t *testing.T) {
	userDir := t.TempDir()
	writeUserPreset(t, userDir, "broken@1.0.0", "id: [unterminated\n")
	writeUserPreset(t, userDir, "fine@1.0.0", presetYAML("fine", "1.0.0", "ok", false))
	svc := service.New(service.Options{BundledFS: bundledFixture(), UserDir: userDir})
	got, warnings, err := svc.ListPresets()
	if err != nil {
		t.Fatalf("a broken user preset must not be fatal: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken@1.0.0") {
		t.Errorf("warnings = %v, want one naming broken@1.0.0", warnings)
	}
	found := false
	for _, p := range got {
		if p.ID == "broken" {
			t.Error("broken preset should not be listed")
		}
		found = found || p.ID == "fine"
	}
	if !found {
		t.Error("valid user preset was hidden by the broken one")
	}
}

func TestListPresets_BundledShadowsUserID(t *testing.T) {
	userDir := t.TempDir()
	writeUserPreset(t, userDir, "alpha@9.0.0", presetYAML("alpha", "9.0.0", "shadowed", false))
	svc := service.New(service.Options{BundledFS: bundledFixture(), UserDir: userDir})
	got, warnings, err := svc.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3 (shadowed user preset omitted)", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "shadowed") {
		t.Errorf("warnings = %v, want a shadow warning", warnings)
	}
}

func TestListPresets_MissingUserDirAndNilBundled(t *testing.T) {
	svc := service.New(service.Options{UserDir: filepath.Join(t.TempDir(), "absent")})
	got, warnings, err := svc.ListPresets()
	if err != nil || len(got) != 0 || len(warnings) != 0 {
		t.Errorf("got %v, %v, %v; want empty and nil", got, warnings, err)
	}
}
