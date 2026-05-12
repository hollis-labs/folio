package render_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio/internal/render"
)

// renderString is a thin test helper that wraps render.RenderString with a
// fixed Context so each table-driven case stays a one-liner.
func renderString(t *testing.T, tpl string) string {
	t.Helper()
	out, err := render.RenderString(tpl, fixture())
	if err != nil {
		t.Fatalf("RenderString(%q): %v", tpl, err)
	}
	return out
}

func fixture() render.Context {
	return render.Context{
		Inputs: map[string]any{
			"project_name":   "my_thing",
			"github_owner":   "chrispian",
			"description":    "  trim me  ",
			"empty":          "",
			"replicas":       3,
			"tags":           []any{"alpha", "beta"},
		},
		Computed: map[string]any{
			"module_path": "github.com/chrispian/my_thing",
		},
		Target: "/tmp/folio-test",
		Preset: render.PresetInfo{ID: "base", Version: "1.0.0"},
		Folio:  render.FolioInfo{Version: "0.1.0"},
		Now:    time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	}
}

func TestRenderString_NoTemplate(t *testing.T) {
	out, err := render.RenderString("literal text, no directives", fixture())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "literal text, no directives" {
		t.Errorf("got %q, want unchanged", out)
	}
}

func TestRenderString_ContextAccess(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"inputs", `{{ .inputs.project_name }}`, "my_thing"},
		{"computed", `{{ .computed.module_path }}`, "github.com/chrispian/my_thing"},
		{"target", `{{ .target }}`, "/tmp/folio-test"},
		{"preset id", `{{ .preset.id }}`, "base"},
		{"preset version", `{{ .preset.version }}`, "1.0.0"},
		{"folio version", `{{ .folio.version }}`, "0.1.0"},
		{"now year", `{{ .now.Year }}`, "2026"},
		{"now format", `{{ .now.Format "2006-01-02" }}`, "2026-05-12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_Strings(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"upper", `{{ upper "abc" }}`, "ABC"},
		{"lower", `{{ lower "ABC" }}`, "abc"},
		{"title", `{{ title "hello world" }}`, "Hello World"},
		{"trim", `{{ trim "  hi  " }}`, "hi"},
		{"trimPrefix", `{{ trimPrefix "go-" "go-folio" }}`, "folio"},
		{"trimSuffix", `{{ trimSuffix ".tmpl" "main.go.tmpl" }}`, "main.go"},
		{"replace", `{{ replace "-" "_" "a-b-c" }}`, "a_b_c"},
		{"contains true", `{{ contains "wor" "hello world" }}`, "true"},
		{"hasPrefix", `{{ hasPrefix "github" "github.com" }}`, "true"},
		{"hasSuffix", `{{ hasSuffix ".go" "main.go" }}`, "true"},
		{"split+index", `{{ index (split "," "a,b,c") 1 }}`, "b"},
		{"join", `{{ join "/" (list "a" "b" "c") }}`, "a/b/c"},
		{"repeat", `{{ repeat 3 "ab" }}`, "ababab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_CaseConversion(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"kebab from snake", `{{ kebabCase "my_thing_name" }}`, "my-thing-name"},
		{"kebab from pascal", `{{ kebabCase "MyThingName" }}`, "my-thing-name"},
		{"snake from kebab", `{{ snakeCase "my-thing-name" }}`, "my_thing_name"},
		{"camel from snake", `{{ camelCase "my_thing_name" }}`, "myThingName"},
		{"pascal from kebab", `{{ pascalCase "my-thing-name" }}`, "MyThingName"},
		{"pascal already pascal", `{{ pascalCase "MyThing" }}`, "MyThing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_Path(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"basename", `{{ basename "a/b/c.go" }}`, "c.go"},
		{"dirname", `{{ dirname "a/b/c.go" }}`, "a/b"},
		{"ext", `{{ ext "main.go" }}`, ".go"},
		{"pathJoin", `{{ pathJoin "a" "b" "c.go" }}`, "a/b/c.go"},
		{"pathClean", `{{ pathClean "a/./b/../c" }}`, "a/c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_DefaultCoalesce(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"default unset", `{{ default "fallback" .inputs.empty }}`, "fallback"},
		{"default set", `{{ default "fallback" .inputs.project_name }}`, "my_thing"},
		{"coalesce", `{{ coalesce .inputs.empty .inputs.project_name }}`, "my_thing"},
		{"ternary true", `{{ ternary "yes" "no" true }}`, "yes"},
		{"ternary false", `{{ ternary "yes" "no" false }}`, "no"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_QuotingEscaping(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"quote", `{{ quote "hello \"world\"" }}`, `"hello \"world\""`},
		{"squote", `{{ squote "it's mine" }}`, `'it'\''s mine'`},
		{"shellQuote", `{{ shellQuote "x y" }}`, `'x y'`},
		{"jsonEscape", `{{ jsonEscape "a\"b" }}`, `a\"b`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_Encoding(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		want string
	}{
		{"json compact", `{{ json (list "a" "b") }}`, `["a","b"]`},
		{"b64encode", `{{ b64encode "hi" }}`, "aGk="},
		{"b64decode", `{{ b64decode "aGk=" }}`, "hi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	indent := renderString(t, `{{ jsonIndent (dict "a" 1 "b" 2) }}`)
	if !strings.Contains(indent, "\n") || !strings.Contains(indent, "  ") {
		t.Errorf("jsonIndent expected to contain newline+2sp indent, got %q", indent)
	}

	yaml := renderString(t, `{{ toYAML (dict "a" 1 "b" "two") }}`)
	if !strings.Contains(yaml, "a: 1") || !strings.Contains(yaml, "b: two") {
		t.Errorf("toYAML missing expected keys, got %q", yaml)
	}
}

func TestFuncmap_DateTime(t *testing.T) {
	cases := []struct {
		tpl  string
		want string
	}{
		{`{{ date "2006-01-02" .now }}`, "2026-05-12"},
		{`{{ dateISO .now }}`, "2026-05-12"},
		{`{{ (now).Year }}`, "2026"},
	}
	for _, tc := range cases {
		t.Run(tc.tpl, func(t *testing.T) {
			if got := renderString(t, tc.tpl); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFuncmap_ListsDicts(t *testing.T) {
	if got := renderString(t, `{{ first (list "a" "b" "c") }}`); got != "a" {
		t.Errorf("first: got %q", got)
	}
	if got := renderString(t, `{{ last (list "a" "b" "c") }}`); got != "c" {
		t.Errorf("last: got %q", got)
	}
	if got := renderString(t, `{{ index (slice (list "a" "b" "c" "d") 1 3) 0 }}`); got != "b" {
		t.Errorf("slice[0]: got %q", got)
	}
	if got := renderString(t, `{{ get (dict "k" "v") "k" }}`); got != "v" {
		t.Errorf("get: got %q", got)
	}
	if got := renderString(t, `{{ hasKey (dict "k" "v") "k" }}`); got != "true" {
		t.Errorf("hasKey true: got %q", got)
	}
	if got := renderString(t, `{{ hasKey (dict "k" "v") "missing" }}`); got != "false" {
		t.Errorf("hasKey false: got %q", got)
	}
}

func TestFuncmap_FolioSpecific(t *testing.T) {
	if got := renderString(t, `{{ licenseHeader "MIT" }}`); !strings.Contains(got, "MIT License") {
		t.Errorf("licenseHeader MIT: got %q", got)
	}
	if got := renderString(t, `{{ gomodPath }}`); got != "github.com/chrispian/my_thing" {
		t.Errorf("gomodPath: got %q", got)
	}
	if got := renderString(t, `{{ spdxId "Apache License 2.0" }}`); got != "Apache-2.0" {
		t.Errorf("spdxId: got %q", got)
	}
}

func TestFuncmap_LicenseHeader_RejectsNonMIT(t *testing.T) {
	_, err := render.RenderString(`{{ licenseHeader "GPL-3.0" }}`, fixture())
	if err == nil {
		t.Fatal("expected error for non-MIT licenseHeader")
	}
	if !strings.Contains(err.Error(), "MIT") {
		t.Errorf("error should mention MIT, got: %v", err)
	}
}

func TestFuncmap_RandomNonDeterministic(t *testing.T) {
	a := renderString(t, `{{ uuid }}`)
	b := renderString(t, `{{ uuid }}`)
	if a == b {
		t.Errorf("uuid should differ between calls: %q == %q", a, b)
	}
	r := renderString(t, `{{ randAlphaNum 12 }}`)
	if len(r) != 12 {
		t.Errorf("randAlphaNum length: %d", len(r))
	}
}

func TestRenderString_ForbiddenHelpers(t *testing.T) {
	// folio diverges from Hadron by excluding env and readFile. Templates
	// invoking them should fail with a parse error citing the missing func.
	for _, tpl := range []string{`{{ env "HOME" }}`, `{{ readFile "/etc/passwd" }}`} {
		t.Run(tpl, func(t *testing.T) {
			_, err := render.RenderString(tpl, fixture())
			if err == nil {
				t.Fatalf("expected error for %q", tpl)
			}
			if !strings.Contains(err.Error(), "function") {
				t.Errorf("error should be a missing-function error, got: %v", err)
			}
		})
	}
}

func TestRenderString_MissingKeyIsError(t *testing.T) {
	_, err := render.RenderString(`{{ .inputs.never_set }}`, fixture())
	if err == nil {
		t.Fatal("expected missingkey=error to surface as an error")
	}
}
