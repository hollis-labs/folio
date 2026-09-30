package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/cmd/folio/internal/cli"
)

// scaffoldBase renders the bundled base preset into a fresh directory through
// the CLI and returns it.
func scaffoldBase(t *testing.T) string {
	t.Helper()
	isolateHome(t)
	target := filepath.Join(t.TempDir(), "my-proj")
	if _, _, err := runCLI(t, "new", "base", target, "--non-interactive",
		"--input", "project_name=my_proj", "--input", "github_owner=me", "--input", "description=x"); err != nil {
		t.Fatalf("new: %v", err)
	}
	return target
}

func TestFolioInspect_NoLongerStub(t *testing.T) {
	target := scaffoldBase(t)
	out, _, err := runCLI(t, "inspect", target)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Contains(out, "not yet implemented") {
		t.Fatalf("inspect is still a stub: %s", out)
	}
	// sync stays reserved
	if _, _, err := runCLI(t, "sync"); err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("sync must stay a stub, got %v", err)
	}
}

func TestFolioInspect_TableOutput(t *testing.T) {
	target := scaffoldBase(t)
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "inspect", target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"folio inspect: my-proj (preset base@", "PATH", "PRESET", "STATUS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	readme := lineWith(t, out, "README.md")
	if !strings.Contains(readme, "base") || !strings.Contains(readme, "locally_modified") {
		t.Errorf("README.md row = %q", readme)
	}
	if !strings.Contains(lineWith(t, out, "go.mod"), "unchanged") {
		t.Errorf("go.mod row = %q", lineWith(t, out, "go.mod"))
	}
}

func lineWith(t *testing.T, out, sub string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	t.Fatalf("no line with %q in:\n%s", sub, out)
	return ""
}

func TestFolioInspect_JSONOutput(t *testing.T) {
	target := scaffoldBase(t)
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "inspect", target, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Target  string `json:"target"`
		Preset  string `json:"preset"`
		Version string `json:"version"`
		Files   []struct {
			Path     string `json:"path"`
			Preset   string `json:"preset"`
			Status   string `json:"status"`
			Unstable bool   `json:"unstable"`
		} `json:"files"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Preset != "base" || got.Version == "" || got.Target != target {
		t.Errorf("header = %+v", got)
	}
	if got.Warnings == nil {
		t.Error("warnings must be [] not null")
	}
	status := map[string]string{}
	for _, f := range got.Files {
		status[f.Path] = f.Status
	}
	if status["README.md"] != "locally_modified" || status["go.mod"] != "unchanged" {
		t.Errorf("statuses = %v", status)
	}
}

func TestFolioInspect_DriftStillExitsZero(t *testing.T) {
	target := scaffoldBase(t)
	if err := os.Remove(filepath.Join(target, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := cli.Run([]string{"inspect", target}, loadBundledFS(t), "0.0.0-test")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want 0 for a report with drift", code)
	}
}

func TestFolioInspect_UnknownTargetDir_ExitsNonZero(t *testing.T) {
	isolateHome(t)
	missing := filepath.Join(t.TempDir(), "nope")
	if code := cli.Run([]string{"inspect", missing}, loadBundledFS(t), "0.0.0-test"); code == cli.ExitOK {
		t.Fatal("inspect of a directory with no .folio.yaml must fail")
	}
	_, _, err := runCLI(t, "inspect", missing)
	if err == nil || !strings.Contains(err.Error(), "manifest_not_found") {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := runCLI(t, "inspect"); err == nil {
		t.Error("inspect with no argument must be a usage error")
	}
}
