package preset_test

import (
	"path/filepath"
	"testing"

	"github.com/hollis-labs/folio/internal/preset"
)

func TestParse_Minimal(t *testing.T) {
	p, err := preset.Parse(filepath.Join("testdata", "valid", "minimal.yaml"))
	if err != nil {
		t.Fatalf("Parse minimal: %v", err)
	}
	if p.ID != "minimal" {
		t.Errorf("id = %q, want %q", p.ID, "minimal")
	}
	if p.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", p.Version)
	}
	if p.Files.Source != "./files" {
		t.Errorf("files.source = %q, want ./files", p.Files.Source)
	}
	if p.SourceFile() == "" {
		t.Error("SourceFile() should be set after Parse()")
	}
}

func TestParse_FullPreset(t *testing.T) {
	p, err := preset.Parse(filepath.Join("testdata", "valid", "full.yaml"))
	if err != nil {
		t.Fatalf("Parse full: %v", err)
	}
	if len(p.Inputs) != 5 {
		t.Fatalf("inputs = %d, want 5", len(p.Inputs))
	}
	if p.Inputs[0].Name != "project_name" {
		t.Errorf("inputs[0].name = %q, want project_name", p.Inputs[0].Name)
	}
	if p.Inputs[0].Pattern == "" {
		t.Error("inputs[0].pattern should be set")
	}
	if p.Inputs[3].Type != "enum" {
		t.Errorf("inputs[3].type = %q, want enum", p.Inputs[3].Type)
	}
	if len(p.Inputs[3].Values) != 3 {
		t.Errorf("inputs[3].values = %d, want 3", len(p.Inputs[3].Values))
	}
	if got := p.Computed["module_path"]; got == "" {
		t.Error("computed.module_path should be set")
	}
	if p.Sync == nil || p.Sync.Default != "prompt" {
		t.Errorf("sync.default missing or wrong: %+v", p.Sync)
	}
	if len(p.Sync.Rules) != 4 {
		t.Errorf("sync.rules = %d, want 4", len(p.Sync.Rules))
	}
}

func TestParseBytes_RoundTrip(t *testing.T) {
	src := []byte(`
folio_version: "0.1"
id: bytes-test
version: 0.1.0
files:
  source: ./files
`)
	p, err := preset.ParseBytes(src)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if p.SourceFile() != "" {
		t.Errorf("SourceFile() should be empty for ParseBytes, got %q", p.SourceFile())
	}
	if p.ID != "bytes-test" {
		t.Errorf("id = %q, want bytes-test", p.ID)
	}
}

func TestParse_SyntaxError(t *testing.T) {
	src := []byte("folio_version: \"0.1\"\nid: : oops\n")
	if _, err := preset.ParseBytes(src); err == nil {
		t.Fatal("expected yaml parse error")
	}
}

func TestParse_MissingFile(t *testing.T) {
	if _, err := preset.Parse(filepath.Join("testdata", "does-not-exist.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFiles_TemplateSuffixOrDefault(t *testing.T) {
	tests := []struct {
		name string
		in   preset.Files
		want string
	}{
		{"unset uses default", preset.Files{}, ".tmpl"},
		{"explicit override", preset.Files{TemplateSuffix: ".gotmpl"}, ".gotmpl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.TemplateSuffixOrDefault(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
