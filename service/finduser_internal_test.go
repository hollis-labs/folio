package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hollis-labs/folio/internal/compose"
	"github.com/hollis-labs/folio/internal/preset"
)

// TestCoerceInput_ListStringFromString covers the v0.2 path where the CLI
// passes through undeclared --input pairs verbatim. For inputs declared as
// list[string] on a composed layer, the value reaches coerceInput as a
// comma-separated string (the CLI's coerceForCLI only runs for top-level
// declared inputs). coerceInput must accept and split it.
func TestCoerceInput_ListStringFromString(t *testing.T) {
	in := preset.Input{Name: "tags", Type: "list[string]"}

	cases := []struct {
		name string
		val  any
		want []string
	}{
		{"comma_separated", "a,b,c", []string{"a", "b", "c"}},
		{"trimmed", " a , b , c ", []string{"a", "b", "c"}},
		{"single", "solo", []string{"solo"}},
		{"empty", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coerceInput(in, tc.val)
			if err != nil {
				t.Fatalf("coerceInput: %v", err)
			}
			gotSlice, ok := got.([]string)
			if !ok {
				t.Fatalf("coerceInput returned %T, want []string", got)
			}
			if !reflect.DeepEqual(gotSlice, tc.want) {
				t.Errorf("coerceInput(%q) = %v, want %v", tc.val, gotSlice, tc.want)
			}
		})
	}
}

// TestFindUserPreset_ConstraintResolution exercises findUserPreset against a
// multi-version user dir, covering both the no-constraint (highest overall)
// and constrained paths. The bundled-FS single-version case is exercised by
// the compose loader integration in P2.
func TestFindUserPreset_ConstraintResolution(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"base@1.0.0", "base@1.5.0", "base@2.0.0", "other@9.9.9"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("seed user dir: %v", err)
		}
	}

	svc := &Service{userDir: dir}

	tests := []struct {
		name       string
		id         string
		constraint string // empty = nil constraint
		want       string
		wantErr    bool
	}{
		{name: "no_constraint_picks_highest", id: "base", want: "base@2.0.0"},
		{name: "constraint_within_range", id: "base", constraint: ">=1.0,<2.0", want: "base@1.5.0"},
		{name: "constraint_no_match_returns_error", id: "base", constraint: ">=3.0", wantErr: true},
		{name: "caret_one_dot_x", id: "base", constraint: "^1.0", want: "base@1.5.0"},
		{name: "unknown_id_returns_empty", id: "missing", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cp *compose.Constraint
			if tc.constraint != "" {
				c, err := compose.ParseConstraint(tc.constraint)
				if err != nil {
					t.Fatalf("parse constraint: %v", err)
				}
				cp = &c
			}
			got, err := svc.findUserPreset(tc.id, cp)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("findUserPreset(%q, %q) = %q, want error", tc.id, tc.constraint, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("findUserPreset: %v", err)
			}
			if got != tc.want {
				t.Errorf("findUserPreset(%q, %q) = %q, want %q", tc.id, tc.constraint, got, tc.want)
			}
		})
	}
}
