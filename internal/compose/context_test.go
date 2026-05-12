package compose_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/folio/internal/compose"
	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/internal/render"
)

func TestScopeVarsForLayer_DefaultInherit(t *testing.T) {
	caller := render.Context{
		Inputs: map[string]any{"license": "MIT", "owner": "alice"},
	}
	composed := &preset.Preset{
		Inputs: []preset.Input{
			{Name: "license", Type: "string"},
			{Name: "owner", Type: "string"},
		},
	}
	got, err := compose.ScopeVarsForLayer(caller, nil, composed)
	if err != nil {
		t.Fatalf("ScopeVarsForLayer: %v", err)
	}
	if got["license"] != "MIT" {
		t.Errorf("license = %v, want MIT", got["license"])
	}
	if got["owner"] != "alice" {
		t.Errorf("owner = %v, want alice", got["owner"])
	}
}

func TestScopeVarsForLayer_PerKeyOverride(t *testing.T) {
	caller := render.Context{
		Inputs: map[string]any{"license": "MIT"},
	}
	composed := &preset.Preset{
		Inputs: []preset.Input{{Name: "license", Type: "string"}},
	}
	got, err := compose.ScopeVarsForLayer(
		caller,
		map[string]string{"license": "Apache-2.0"},
		composed,
	)
	if err != nil {
		t.Fatalf("ScopeVarsForLayer: %v", err)
	}
	if got["license"] != "Apache-2.0" {
		t.Errorf("license = %v, want Apache-2.0 (override)", got["license"])
	}
}

func TestScopeVarsForLayer_TemplateInVarsAgainstCallerCtx(t *testing.T) {
	caller := render.Context{
		Inputs: map[string]any{"owner": "alice", "project_name": "thing"},
	}
	composed := &preset.Preset{
		Inputs: []preset.Input{
			{Name: "owner", Type: "string"},
			{Name: "project_name", Type: "string"},
			{Name: "module_path", Type: "string"},
		},
	}
	got, err := compose.ScopeVarsForLayer(
		caller,
		map[string]string{"module_path": "github.com/{{.inputs.owner}}/{{.inputs.project_name}}"},
		composed,
	)
	if err != nil {
		t.Fatalf("ScopeVarsForLayer: %v", err)
	}
	if got["module_path"] != "github.com/alice/thing" {
		t.Errorf("module_path = %v, want github.com/alice/thing", got["module_path"])
	}
	// Inherited keys remain untouched.
	if got["owner"] != "alice" {
		t.Errorf("owner = %v, want alice", got["owner"])
	}
}

func TestScopeVarsForLayer_UndeclaredInputKey(t *testing.T) {
	caller := render.Context{Inputs: map[string]any{}}
	composed := &preset.Preset{
		Inputs: []preset.Input{{Name: "license", Type: "string"}},
	}
	_, err := compose.ScopeVarsForLayer(
		caller,
		map[string]string{"not_an_input": "value"},
		composed,
	)
	if err == nil {
		t.Fatal("expected error for undeclared input key, got nil")
	}
	if !strings.Contains(err.Error(), "not_an_input") {
		t.Errorf("error should name the bad key, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "license") {
		t.Errorf("error should list declared inputs, got: %s", err.Error())
	}
}

func TestScopeVarsForLayer_TemplateError(t *testing.T) {
	caller := render.Context{Inputs: map[string]any{}}
	composed := &preset.Preset{
		Inputs: []preset.Input{{Name: "broken", Type: "string"}},
	}
	_, err := compose.ScopeVarsForLayer(
		caller,
		map[string]string{"broken": "{{ .inputs.missing | bogus }}"},
		composed,
	)
	if err == nil {
		t.Fatal("expected template parse error")
	}
}

func TestMergeComputed(t *testing.T) {
	cases := []struct {
		name   string
		layers []map[string]any
		want   map[string]any
	}{
		{
			name:   "empty",
			layers: nil,
			want:   map[string]any{},
		},
		{
			name: "single_layer",
			layers: []map[string]any{
				{"x": 1, "y": 2},
			},
			want: map[string]any{"x": 1, "y": 2},
		},
		{
			name: "last_wins_on_collision",
			layers: []map[string]any{
				{"x": 1, "shared": "first"},
				{"y": 2, "shared": "second"},
			},
			want: map[string]any{"x": 1, "y": 2, "shared": "second"},
		},
		{
			name: "three_layer_chain",
			layers: []map[string]any{
				{"v": "a"},
				{"v": "b"},
				{"v": "c"},
			},
			want: map[string]any{"v": "c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compose.MergeComputed(tc.layers)
			if len(got) != len(tc.want) {
				t.Fatalf("MergeComputed = %v (len %d), want %v (len %d)", got, len(got), tc.want, len(tc.want))
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("key %q = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
