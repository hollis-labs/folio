package cli_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/cmd/folio/internal/cli"
)

func bundledFS(t *testing.T) fs.FS {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "presets"))
	if err != nil {
		t.Fatal(err)
	}
	return &subFS{root: abs}
}

// subFS exposes the on-disk presets/ dir under the path layout the service
// expects (root contains "<id>/preset.yaml"). The real bundled FS exposes
// "presets/<id>/preset.yaml" with the same key.
type subFS struct{ root string }

func (s *subFS) Open(name string) (fs.File, error) {
	const prefix = "presets"
	if name == prefix {
		return os.Open(s.root)
	}
	if strings.HasPrefix(name, prefix+"/") {
		return os.Open(filepath.Join(s.root, strings.TrimPrefix(name, prefix+"/")))
	}
	if name == "." {
		// fs.WalkDir starts from "." — return a one-entry virtual root.
		return os.Open(s.root)
	}
	return nil, fs.ErrNotExist
}

func TestRun_PresetValidate(t *testing.T) {
	dir, _ := filepath.Abs(filepath.Join("..", "..", "..", "..", "presets", "base"))
	args := []string{"preset", "validate", dir}

	stdout, stderr := captureStreams(t, args)
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("expected PASS in stdout, got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestRun_NewNonInteractive(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	args := []string{
		"new", "base", target,
		"--input", "project_name=clitest",
		"--input", "github_owner=chrispian",
		"--non-interactive",
		"--quiet",
	}
	stdout, stderr := captureStreams(t, args)
	_ = stdout
	if _, err := os.Stat(filepath.Join(target, ".folio.yaml")); err != nil {
		t.Errorf("manifest not written: %v\nstderr: %s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(target, "go.mod")); err != nil {
		t.Errorf("go.mod not written: %v", err)
	}
}

func TestRun_NewMissingInput_ExitsUsage(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	args := []string{"new", "base", target, "--non-interactive"}

	bfs := loadBundledFS(t)
	root := cli.NewRootCmd(bfs, "0.1.0-test")
	root.SetArgs(args)
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing required inputs")
	}
	if !strings.Contains(err.Error(), "input_missing") {
		t.Errorf("expected input_missing error, got: %v", err)
	}
}

func TestRun_Plan_NoWrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "would-not-exist")
	args := []string{
		"plan", "base", target,
		"--input", "project_name=plantest",
		"--input", "github_owner=chrispian",
		"--non-interactive",
	}
	stdout, _ := captureStreams(t, args)
	if !strings.Contains(stdout, "folio plan") {
		t.Errorf("expected 'folio plan' in stdout, got: %s", stdout)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("plan should not create target dir; stat: %v", err)
	}
}

func TestRun_StubCommand(t *testing.T) {
	bfs := loadBundledFS(t)
	root := cli.NewRootCmd(bfs, "0.1.0-test")
	root.SetArgs([]string{"sync"})
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected not-yet-implemented error, got: %v", err)
	}
}

// loadBundledFS rebuilds the bundled FS by reading on-disk presets/.
// We can't use folio.BundledPresets because the embed only triggers in
// the real binary build path.
func loadBundledFS(t *testing.T) fs.FS {
	t.Helper()
	abs, _ := filepath.Abs(filepath.Join("..", "..", "..", "..", "presets"))
	return os.DirFS(filepath.Dir(abs))
}

// captureStreams runs cli.NewRootCmd against args with stdin/stdout/stderr
// piped into buffers, returns (stdout, stderr).
func captureStreams(t *testing.T, args []string) (string, string) {
	t.Helper()
	bfs := loadBundledFS(t)
	root := cli.NewRootCmd(bfs, "0.1.0-test")
	root.SetArgs(args)
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true
	_ = root.Execute()
	return outBuf.String(), errBuf.String()
}
