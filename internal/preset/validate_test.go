package preset_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/internal/preset"
)

func TestValidate_Valid(t *testing.T) {
	cases := []string{
		"minimal.yaml",
		"full.yaml",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := preset.Parse(filepath.Join("testdata", "valid", name))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			res := p.Validate()
			if !res.OK() {
				t.Fatalf("expected no errors, got: %v", res.Errors)
			}
			if len(res.Warnings) != 0 {
				t.Errorf("expected no warnings, got: %v", res.Warnings)
			}
		})
	}
}

func TestValidate_PostRenderWarns(t *testing.T) {
	p, err := preset.Parse(filepath.Join("testdata", "valid", "post_render_warns.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := p.Validate()
	if !res.OK() {
		t.Fatalf("post_render should not be a hard error, got: %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected post_render warning, got none")
	}
	if !strings.Contains(res.Warnings[0].Message, "post_render") {
		t.Errorf("warning message should mention post_render, got: %s", res.Warnings[0].Message)
	}
}

func TestValidate_Invalid(t *testing.T) {
	cases := []struct {
		fixture     string
		wantPath    string
		wantMessage string
	}{
		{"missing_required.yaml", "folio_version", "missing required field"},
		{"bad_id.yaml", "id", "invalid id"},
		{"bad_semver.yaml", "version", "invalid semver"},
		{"duplicate_input.yaml", "inputs[1].name", "not unique"},
		{"reserved_input.yaml", "inputs[0].name", "reserved"},
		{"bad_pattern.yaml", "inputs[0].pattern", "invalid regex"},
		{"bad_enum_default.yaml", "inputs[0].default", "not one of values"},
		{"default_type_mismatch.yaml", "inputs[0].default", "not a number"},
		{"computed_collision.yaml", "computed.project_name", "collides"},
		{"composes_non_empty.yaml", "composes", "not yet implemented"},
		{"bad_template_suffix.yaml", "files.template_suffix", "must start with a dot"},
		{"files_source_escapes.yaml", "files.source", "escapes the preset root"},
		{"bad_sync_policy.yaml", "sync.default", "unsupported sync policy"},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			p, err := preset.Parse(filepath.Join("testdata", "invalid", tc.fixture))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			res := p.Validate()
			if res.OK() {
				t.Fatalf("expected validation errors, got none")
			}
			found := false
			for _, e := range res.Errors {
				if e.Path == tc.wantPath && strings.Contains(e.Message, tc.wantMessage) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("did not find error matching path=%q message=%q; got:\n%s",
					tc.wantPath, tc.wantMessage, dumpErrors(res.Errors))
			}
		})
	}
}

func TestValidate_ErrorIncludesLine(t *testing.T) {
	p, err := preset.Parse(filepath.Join("testdata", "invalid", "bad_id.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := p.Validate()
	if res.OK() {
		t.Fatal("expected errors")
	}
	for _, e := range res.Errors {
		if e.Path == "id" {
			if e.Line == 0 {
				t.Errorf("expected line number on id error, got 0; full error: %s", e.Error())
			}
			if !strings.Contains(e.Error(), "bad_id.yaml") {
				t.Errorf("expected file path in error string, got: %s", e.Error())
			}
			return
		}
	}
	t.Fatal("did not find id error")
}

func dumpErrors(es []preset.ValidationError) string {
	var sb strings.Builder
	for _, e := range es {
		sb.WriteString("  ")
		sb.WriteString(e.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}
