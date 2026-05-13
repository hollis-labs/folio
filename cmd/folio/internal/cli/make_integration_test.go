package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/cmd/folio/internal/cli"
)

// TestMakeVerb_NanitePlugin_DerivesDirAndInjectsName runs the make
// verb through the full Cobra tree and asserts:
//   - the target dir is ./<preset>-<name>
//   - the auto-injected plugin_name=<name> reaches plugin.yaml as id:
func TestMakeVerb_NanitePlugin_DerivesDirAndInjectsName(t *testing.T) {
	// Resolve bundled FS before chdir — the helper builds the path
	// relative to the test file location via filepath.Abs which
	// depends on the current working directory.
	bfs := loadBundledFS(t)

	tempBase := t.TempDir()
	t.Chdir(tempBase)

	root := cli.NewRootCmd(bfs, "0.1.0-test")
	root.SetArgs([]string{
		"make", "nanite-plugin", "demo",
		"--input", "github_owner=hollis-labs",
		"--input", "description=demo plugin",
		"--non-interactive",
		"--yes",
		"--quiet",
	})
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true

	if err := root.Execute(); err != nil {
		t.Fatalf("make: %v\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	target := filepath.Join(tempBase, "nanite-plugin-demo")
	manifest, err := os.ReadFile(filepath.Join(target, "plugin.yaml"))
	if err != nil {
		t.Fatalf("expected plugin.yaml under %s: %v", target, err)
	}
	if !strings.Contains(string(manifest), "id: demo") {
		t.Errorf("plugin.yaml missing 'id: demo' (auto-injected plugin_name failed):\n%s", manifest)
	}
}

// TestMakeVerb_UserInputOverridesAuto confirms an explicit --input
// plugin_name=X overrides the auto-injected value derived from the
// positional <name> argument. The directory still uses <name>, but
// the manifest reflects the user's override.
func TestMakeVerb_UserInputOverridesAuto(t *testing.T) {
	bfs := loadBundledFS(t)

	tempBase := t.TempDir()
	t.Chdir(tempBase)

	root := cli.NewRootCmd(bfs, "0.1.0-test")
	root.SetArgs([]string{
		"make", "nanite-plugin", "dirname",
		"--input", "plugin_name=actual",
		"--input", "github_owner=hollis-labs",
		"--input", "description=override smoke",
		"--non-interactive",
		"--yes",
		"--quiet",
	})
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true

	if err := root.Execute(); err != nil {
		t.Fatalf("make: %v\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	target := filepath.Join(tempBase, "nanite-plugin-dirname")
	manifest, err := os.ReadFile(filepath.Join(target, "plugin.yaml"))
	if err != nil {
		t.Fatalf("expected plugin.yaml under %s: %v", target, err)
	}
	if !strings.Contains(string(manifest), "id: actual") {
		t.Errorf("user --input plugin_name=actual did not override the auto-injection:\n%s", manifest)
	}
}
