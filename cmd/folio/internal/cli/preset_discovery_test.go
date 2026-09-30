package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/cmd/folio/internal/cli"
	"github.com/hollis-labs/folio/service"
)

// isolateHome points the user preset dir (~/.folio/presets/local) at a temp
// dir so no test reads or writes the real one. It returns that user dir.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	userDir := filepath.Join(home, ".folio", "presets", "local")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return userDir
}

func addUserPreset(t *testing.T, userDir, id, version, desc string) {
	t.Helper()
	dir := filepath.Join(userDir, id+"@"+version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "folio_version: \"0.1\"\nid: " + id + "\nversion: " + version + "\ndescription: " + desc + "\nauthor: me\nfiles:\n  source: ./files\n"
	if err := os.WriteFile(filepath.Join(dir, "preset.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := cli.NewRootCmd(loadBundledFS(t), "0.0.0-test")
	root.SetArgs(args)
	var o, e bytes.Buffer
	root.SetOut(&o)
	root.SetErr(&e)
	root.SilenceErrors = true
	root.SilenceUsage = true
	err = root.Execute()
	return o.String(), e.String(), err
}

func TestPresetList_TableOutput(t *testing.T) {
	userDir := isolateHome(t)
	addUserPreset(t, userDir, "mine", "1.0.0", "my local preset")

	out, _, err := runCLI(t, "preset", "list")
	if err != nil {
		t.Fatalf("preset list: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if f := strings.Fields(lines[0]); strings.Join(f, " ") != "ID VERSION SOURCE DESCRIPTION" {
		t.Errorf("header = %q", lines[0])
	}
	var mine, base, baseline string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "mine "):
			mine = l
		case strings.HasPrefix(l, "base "):
			base = l
		case strings.HasPrefix(l, "go-baseline "):
			baseline = l
		}
	}
	if !strings.Contains(mine, "local") || !strings.Contains(mine, "my local preset") {
		t.Errorf("user preset row = %q, want local source and description", mine)
	}
	if !strings.Contains(base, "bundled") {
		t.Errorf("base row = %q, want bundled", base)
	}
	if !strings.Contains(baseline, "[layer]") {
		t.Errorf("go-baseline row = %q, want [layer] marker", baseline)
	}
	if strings.Contains(base, "[layer]") {
		t.Errorf("base row = %q must not be marked as a layer", base)
	}
}

func TestPresetList_JSONOutput(t *testing.T) {
	userDir := isolateHome(t)
	addUserPreset(t, userDir, "mine", "1.0.0", "my local preset")

	out, _, err := runCLI(t, "preset", "list", "--json")
	if err != nil {
		t.Fatalf("preset list --json: %v", err)
	}
	var got []service.PresetSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	byID := map[string]service.PresetSummary{}
	for _, p := range got {
		byID[p.ID] = p
	}
	if p := byID["mine"]; p.Source != "local" || p.Version != "1.0.0" || p.Description != "my local preset" || p.Author != "me" {
		t.Errorf("mine = %+v", p)
	}
	if p := byID["base"]; p.Source != "bundled" || p.LayerOnly {
		t.Errorf("base = %+v", p)
	}
	if !byID["go-baseline"].LayerOnly {
		t.Error("go-baseline layer_only = false, want true")
	}
	// Round trip: re-encoding the decoded value must reproduce the key set.
	var raw []map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "version", "description", "author", "source", "layer_only"} {
		if _, ok := raw[0][k]; !ok {
			t.Errorf("JSON row missing key %q", k)
		}
	}
}

func TestPresetList_JSONEmptyIsArray(t *testing.T) {
	isolateHome(t)
	root := cli.NewRootCmd(nil, "0.0.0-test")
	root.SetArgs([]string{"preset", "list", "--json"})
	var o bytes.Buffer
	root.SetOut(&o)
	root.SilenceErrors = true
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(o.String()) != "[]" {
		t.Errorf("empty list JSON = %q, want []", o.String())
	}
}

func TestPresetShow_KnownIDWithComposes(t *testing.T) {
	isolateHome(t)
	out, _, err := runCLI(t, "preset", "show", "go-lib")
	if err != nil {
		t.Fatalf("preset show: %v", err)
	}
	for _, want := range []string{
		"folio: go-lib", "version:", "0.3.0", "author:", "hollis-labs", "source:", "bundled",
		"composes:     go-baseline (>=0.1,<1.0)", "files.source:", "inputs:", "repo_name", "required",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Only the declared entry: go-baseline's own (empty) composes is not resolved in.
	if strings.Count(out, "composes:") != 1 {
		t.Errorf("expected exactly one composes line:\n%s", out)
	}
}

func TestPresetShow_NoComposes(t *testing.T) {
	isolateHome(t)
	out, _, err := runCLI(t, "preset", "show", "base")
	if err != nil {
		t.Fatalf("preset show base: %v", err)
	}
	if !strings.Contains(out, "composes:     (none)") {
		t.Errorf("want '(none)' composes line:\n%s", out)
	}
	if !strings.Contains(out, "layer_only:   false") {
		t.Errorf("want layer_only false:\n%s", out)
	}
}

func TestPresetShow_LayerOnlyDisplayed(t *testing.T) {
	isolateHome(t)
	out, _, err := runCLI(t, "preset", "show", "go-baseline")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "layer_only:   true") {
		t.Errorf("want layer_only true:\n%s", out)
	}
}

func TestPresetShow_JSONOutput(t *testing.T) {
	isolateHome(t)
	out, _, err := runCLI(t, "preset", "show", "go-lib", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		ID       string `json:"id"`
		Version  string `json:"version"`
		Source   string `json:"source"`
		Layer    bool   `json:"layer_only"`
		Composes []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"composes"`
		Inputs []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"inputs"`
	}
	if err = json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if d.ID != "go-lib" || d.Source != "bundled" || d.Layer {
		t.Errorf("detail = %+v", d)
	}
	if len(d.Composes) != 1 || d.Composes[0].ID != "go-baseline" || d.Composes[0].Version != ">=0.1,<1.0" {
		t.Errorf("composes = %+v", d.Composes)
	}
	if len(d.Inputs) == 0 || d.Inputs[0].Name != "repo_name" || !d.Inputs[0].Required {
		t.Errorf("inputs = %+v", d.Inputs)
	}

	out, _, err = runCLI(t, "preset", "show", "go-baseline", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(out), &d); err != nil || !d.Layer {
		t.Errorf("go-baseline --json layer_only = %v (err %v)", d.Layer, err)
	}
}

func TestPresetShow_UserDirPreset(t *testing.T) {
	userDir := isolateHome(t)
	addUserPreset(t, userDir, "mine", "1.0.0", "my local preset")
	out, _, err := runCLI(t, "preset", "show", "mine")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "source:       local") {
		t.Errorf("want local source:\n%s", out)
	}
}

func TestPresetShow_UnknownID_ExitsNonZero(t *testing.T) {
	isolateHome(t)
	_, _, err := runCLI(t, "preset", "show", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
	var se *service.Error
	if !asServiceErr(err, &se) || se.Code != service.ErrPresetNotFound {
		t.Errorf("err = %v, want preset_not_found", err)
	}
	if code := cli.Run([]string{"preset", "show", "does-not-exist"}, loadBundledFS(t), "0.0.0-test"); code != cli.ExitGeneric {
		t.Errorf("exit code = %d, want %d", code, cli.ExitGeneric)
	}
}

func TestPresetShow_RequiresOneArg(t *testing.T) {
	isolateHome(t)
	if _, _, err := runCLI(t, "preset", "show"); err == nil {
		t.Error("expected a usage error with no preset id")
	}
}

func TestPresetListShow_NoLongerStubs(t *testing.T) {
	isolateHome(t)
	for _, args := range [][]string{{"preset", "list"}, {"preset", "show", "base"}} {
		if _, _, err := runCLI(t, args...); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	// sync and inspect stay reserved.
	for _, name := range []string{"sync", "inspect"} {
		if _, _, err := runCLI(t, name); err == nil || !strings.Contains(err.Error(), "not yet implemented") {
			t.Errorf("%s should remain a stub, got %v", name, err)
		}
	}
}

func asServiceErr(err error, target **service.Error) bool {
	return errors.As(err, target)
}
