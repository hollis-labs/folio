package folio_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// The go-lib preset must render a module that passes the lib checklist by
// construction. These tests render it into t.TempDir() and run the real
// tools against it — the previous suite only ran `go vet`, which is how a
// golangci-lint pin that cannot analyze a go 1.26 module shipped in five libs.
//
// A missing tool is a visible SKIP, never a silent pass; set
// FOLIO_REQUIRE_TOOLS=1 (folio's CI should) to make it a failure instead.
// FOLIO_NET=1 additionally runs govulncheck, which needs the network.

const conformanceScript = "scripts/check-lib-conformance.sh"

func newGoLibService(t *testing.T) *service.Service {
	t.Helper()
	return service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		UserDir:      t.TempDir(),
		FolioVersion: folio.Version,
		Now:          time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	})
}

// renderGoLib renders go-lib into a fresh temp dir and returns it.
func renderGoLib(t *testing.T, extra map[string]any) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "go-demo")
	inputs := map[string]any{
		"repo_name":    "go-demo",
		"package_name": "demo",
		"description":  "go-lib conformance sample",
	}
	for k, v := range extra {
		inputs[k] = v
	}
	if _, err := newGoLibService(t).New(service.NewOptions{
		PresetID:  "go-lib",
		TargetDir: target,
		Inputs:    inputs,
	}); err != nil {
		t.Fatalf("service.New go-lib: %v", err)
	}
	return target
}

// needTool reports whether name is on PATH; when it is not, the test skips
// (or fails under FOLIO_REQUIRE_TOOLS=1).
func needTool(t *testing.T, name string) bool {
	t.Helper()
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	if os.Getenv("FOLIO_REQUIRE_TOOLS") == "1" {
		t.Fatalf("required tool %q not on PATH (FOLIO_REQUIRE_TOOLS=1)", name)
	}
	t.Skipf("SKIPPED, not passed: tool %q not on PATH", name)
	return false
}

// run executes a command in dir with GOWORK=off (the repo may sit under a
// go.work) and returns combined output.
func run(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}

func mustRun(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	out, err := run(t, dir, name, args...)
	if err != nil {
		t.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func goDirectiveMinor(t *testing.T, gomod string) int {
	t.Helper()
	m := regexp.MustCompile(`(?m)^go 1\.(\d+)`).FindStringSubmatch(gomod)
	if m == nil {
		t.Fatalf("no go directive in go.mod:\n%s", gomod)
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestGoLib_RendersConformingModule(t *testing.T) {
	needTool(t, "go")
	cases := []struct {
		name   string
		inputs map[string]any
		pkgDir string // directory holding the package, relative to the module
	}{
		{"default", nil, "demo"},
		{"root", map[string]any{"package_layout": "root"}, "."},
		{"stable", map[string]any{"stability": "stable"}, "demo"},
		{"toolchain", map[string]any{"go_toolchain": "1.26.8"}, "demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := renderGoLib(t, tc.inputs)
			gomod := readText(t, filepath.Join(dir, "go.mod"))

			if tc.inputs["go_toolchain"] == nil {
				// Default inputs render exactly the decided floor, no toolchain.
				if !strings.Contains(gomod, "\ngo 1.26.6\n") || strings.Contains(gomod, "toolchain") {
					t.Errorf("go.mod should be exactly go 1.26.6 with no toolchain:\n%s", gomod)
				}
			} else if !strings.Contains(gomod, "\ntoolchain go1.26.8\n") {
				t.Errorf("go.mod missing requested toolchain:\n%s", gomod)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.pkgDir, "doc.go")); err != nil {
				t.Errorf("package doc.go missing: %v", err)
			}

			if out, err := run(t, dir, "gofmt", "-l", "."); err != nil || strings.TrimSpace(out) != "" {
				t.Errorf("gofmt -l . not empty (err=%v):\n%s", err, out)
			}
			mustRun(t, dir, "go", "build", "./...")
			mustRun(t, dir, "go", "vet", "./...")
			mustRun(t, dir, "go", "test", "-race", "-count=1", "./...")
			mustRun(t, dir, "go", "mod", "verify")
			mustRun(t, dir, "go", "mod", "tidy", "-diff")

			checkGolangciLint(t, dir, gomod)

			if os.Getenv("FOLIO_NET") == "1" {
				checkGovulncheck(t, dir)
			}

			if out, err := run(t, ".", "sh", conformanceScript, dir); err != nil {
				t.Errorf("conformance script failed on a fresh render: %v\n%s", err, out)
			}
		})
	}
}

// checkGolangciLint runs the linter at the version PINNED in the rendered
// workflow, and asserts that binary was built with a Go at least as new as
// the module's — the exact defect (v2.1.6, go1.24, against go 1.26.x) that
// made every previously rendered lib red in CI.
func checkGolangciLint(t *testing.T, dir, gomod string) {
	t.Helper()
	if !needTool(t, "golangci-lint") {
		return
	}
	wf := readText(t, filepath.Join(dir, ".github", "workflows", "check.yml"))
	pin := regexp.MustCompile(`(?m)^\s*version:\s*(v\d+\.\d+\.\d+)\s*$`).FindStringSubmatch(wf)
	if pin == nil {
		t.Fatalf("no pinned golangci-lint version in check.yml:\n%s", wf)
	}
	out := mustRun(t, dir, "golangci-lint", "--version")
	m := regexp.MustCompile(`version (\S+) built with go1\.(\d+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("cannot parse golangci-lint --version: %s", out)
	}
	if "v"+strings.TrimPrefix(m[1], "v") != pin[1] {
		t.Skipf("SKIPPED, not passed: installed golangci-lint %s, workflow pins %s", m[1], pin[1])
	}
	built := 0
	for _, c := range m[2] {
		built = built*10 + int(c-'0')
	}
	if want := goDirectiveMinor(t, gomod); built < want {
		t.Fatalf("pinned golangci-lint %s is built with go1.%d, below the module's go 1.%d", pin[1], built, want)
	}
	if out, err := run(t, dir, "golangci-lint", "run", "--allow-parallel-runners"); err != nil {
		t.Errorf("golangci-lint run: %v\n%s", err, out)
	}
}

func checkGovulncheck(t *testing.T, dir string) {
	t.Helper()
	wf := readText(t, filepath.Join(dir, ".github", "workflows", "check.yml"))
	pin := regexp.MustCompile(`govulncheck@(v\d+\.\d+\.\d+)`).FindStringSubmatch(wf)
	if pin == nil {
		t.Fatalf("govulncheck is not pinned in check.yml:\n%s", wf)
	}
	mustRun(t, dir, "go", "run", "golang.org/x/vuln/cmd/govulncheck@"+pin[1], "./...")
}

// scriptResult runs the conformance script with --report and returns the
// item -> status map plus the exit code.
func scriptResult(t *testing.T, dir string, extra ...string) (map[string]string, int) {
	t.Helper()
	args := append([]string{conformanceScript, dir}, extra...)
	out, err := run(t, ".", "sh", args...)
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !errors.As(err, &ee) {
			t.Fatalf("run script: %v", err)
		}
		code = ee.ExitCode()
	}
	items := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && (f[0] == "PASS" || f[0] == "FAIL" || f[0] == "N/A") && !strings.HasPrefix(line, " ") {
			items[f[1]] = f[0]
		}
	}
	return items, code
}

// treeSnapshot maps every file under dir to its content hash, so two snapshots
// differ if anything was added, removed or changed.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		snap[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// The script reports on a tree; it must never change it. A stray `hello`
// binary from a bare `go build ./examples/...` once appeared in the examined
// tree and, in a real lib, would be committable.
func TestConformanceScript_LeavesTreeUnchanged(t *testing.T) {
	needTool(t, "go")
	dir := renderGoLib(t, nil)
	before := treeSnapshot(t, dir)
	if _, code := scriptResult(t, dir, "--report"); code != 0 {
		t.Fatalf("script exit %d on a fresh render", code)
	}
	if after := treeSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("the conformance script modified the examined tree:\nbefore: %v\nafter:  %v", before, after)
	}

	// Control: prove the snapshot notices the defect it guards against.
	mustRun(t, dir, "go", "build", "./examples/...")
	if reflect.DeepEqual(before, treeSnapshot(t, dir)) {
		t.Error("control failed: a bare go build of examples/ left the snapshot unchanged")
	}
}

func TestConformanceScript_PositiveControls(t *testing.T) {
	needTool(t, "go")
	needTool(t, "git")

	edit := func(rel string, f func(string) string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) {
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.WriteFile(p, []byte(f(readText(t, p))), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write := func(rel, content string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) {
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	remove := func(rel string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
		}
	}
	repl := func(old, replacement string) func(string) string {
		return func(s string) string {
			if !strings.Contains(s, old) {
				panic("mutation anchor not found: " + old)
			}
			return strings.Replace(s, old, replacement, 1)
		}
	}

	// One mutation per checklist item: each must turn exactly that item FAIL.
	controls := map[string]func(t *testing.T, dir string){
		"B1": edit("go.mod", func(s string) string { return s + "\nreplace example.com/x => ../x\n" }),
		"B2": write(".gitignore", "*.test\n"),
		// An orphan go.sum line: `go mod tidy -diff` wants it gone, no network needed.
		"B3":  write("go.sum", "example.com/x v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"),
		"B4":  edit(".github/workflows/check.yml", repl("go-version-file: go.mod", "go-version: 'stable'")),
		"B5":  edit("go.mod", repl("go 1.26.6", "go 1.26.1")),
		"B6":  write("demo/bad.go", "package demo\nfunc  Bad( ) {}\n"),
		"B7":  write("demo/bad.go", "package demo\n\nfunc Bad() int { return \"x\" }\n"),
		"B8":  write("demo/bad_test.go", "package demo\n\nimport \"testing\"\n\nfunc TestBad(t *testing.T) { t.Fatal(\"x\") }\n"),
		"B10": remove(".golangci.yml"),
		"B15": edit("README.md", repl("go get ", "get ")),
		"B16": edit("README.md", repl("```go", "```text")),
		"B17": edit("README.md", repl("## License", "## Licnse")),
		"B18": remove(".github/workflows/release.yml"),
		"C4":  edit(".github/workflows/check.yml", repl("govulncheck@v1.8.0", "govulncheck@latest")),
		"C4b": edit(".github/workflows/check.yml", repl("go vet ./...", "go vet ./... || true")),
		"F1":  edit("go.mod", repl("hollis-labs", "chrispian")),
		"F2":  remove("demo/doc.go"),
		"F3":  edit("README.md", repl("## Out of scope", "## Non-goals")),
		"F4":  edit("README.md", repl("## Compatibility", "## Policy")),
	}

	// Clean tree: everything passes, exit 0, and every item that ran has a
	// control (so a newly added item cannot ship without one).
	clean := renderGoLib(t, nil)
	items, code := scriptResult(t, clean)
	if code != 0 {
		out, _ := run(t, ".", "sh", conformanceScript, clean)
		t.Fatalf("fresh render should pass, exit %d:\n%s", code, out)
	}
	var uncovered []string
	for id, st := range items {
		if st != "PASS" {
			t.Errorf("clean render: %s = %s", id, st)
		}
		if _, ok := controls[id]; !ok {
			uncovered = append(uncovered, id)
		}
	}
	sort.Strings(uncovered)
	if len(uncovered) > 0 {
		t.Errorf("items without a positive control: %v", uncovered)
	}

	var wg sync.WaitGroup
	for id, mutate := range controls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t.Run(id, func(t *testing.T) {
				dir := renderGoLib(t, nil)
				mutate(t, dir)
				got, code := scriptResult(t, dir)
				if got[id] != "FAIL" {
					t.Errorf("mutation for %s did not make it FAIL (got %q)", id, got[id])
				}
				if code != 1 {
					t.Errorf("exit = %d, want 1", code)
				}
				if _, reportCode := scriptResult(t, dir, "--report"); reportCode != 0 {
					t.Errorf("--report exit = %d, want 0", reportCode)
				}
			})
		}()
	}
	wg.Wait()

	t.Run("release", func(t *testing.T) {
		dir := renderGoLib(t, nil)
		// TODO(author) placeholders are expected in a fresh render but must
		// block a release.
		if got, _ := scriptResult(t, dir, "--release"); got["R1"] != "FAIL" {
			t.Errorf("R1 = %q, want FAIL while TODO(author) remains", got["R1"])
		}
		git := func(args ...string) {
			t.Helper()
			mustRun(t, dir, "git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
		}
		git("init", "-q")
		git("add", "-A")
		git("commit", "-q", "-m", "init")
		git("tag", "v9.9.9")
		if got, _ := scriptResult(t, dir, "--release"); got["R2"] != "FAIL" {
			t.Errorf("R2 = %q, want FAIL: tag v9.9.9 has no CHANGELOG heading", got["R2"])
		}
		edit("CHANGELOG.md", repl("## Unreleased", "## v9.9.9 — 2026-09-25\n\n## Unreleased"))(t, dir)
		git("commit", "-q", "-am", "changelog")
		git("tag", "-f", "v9.9.9")
		if got, _ := scriptResult(t, dir, "--release"); got["R2"] != "PASS" {
			t.Errorf("R2 = %q, want PASS once the heading exists", got["R2"])
		}
	})

	t.Run("usage", func(t *testing.T) {
		if _, code := scriptResult(t, filepath.Join(t.TempDir(), "absent")); code != 2 {
			t.Errorf("missing dir exit = %d, want 2", code)
		}
		if _, code := scriptResult(t, t.TempDir()); code != 2 {
			t.Errorf("no go.mod exit = %d, want 2 (nothing examined)", code)
		}
	})
}

// TestGoLib_ComposeAttribution asserts the layering contract: shared files
// come from go-baseline, library files from go-lib, baseline applied first.
func TestGoLib_ComposeAttribution(t *testing.T) {
	dir := renderGoLib(t, nil)
	mf, err := manifest.Read(dir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(mf.Presets) != 2 || mf.Presets[0].ID != "go-baseline" || mf.Presets[1].ID != "go-lib" {
		t.Fatalf("presets = %+v, want [go-baseline go-lib] in apply order", mf.Presets)
	}
	want := map[string]string{
		".github/workflows/check.yml":   "go-baseline",
		".gitignore":                    "go-baseline",
		".golangci.yml":                 "go-baseline",
		"lefthook.yml":                  "go-baseline",
		"LICENSE":                       "go-baseline",
		"CHANGELOG.md":                  "go-baseline",
		"AGENTS.md":                     "go-baseline",
		"CLAUDE.md":                     "go-baseline",
		"README.md":                     "go-lib",
		"go.mod":                        "go-lib",
		"demo/doc.go":                   "go-lib",
		"demo/demo.go":                  "go-lib",
		"demo/demo_test.go":             "go-lib",
		"demo/example_test.go":          "go-lib",
		"examples/hello/main.go":        "go-lib",
		".github/workflows/release.yml": "go-lib",
	}
	for path, preset := range want {
		rec, ok := mf.Files[path]
		if !ok {
			t.Errorf("manifest.files missing %q", path)
			continue
		}
		if rec.Preset != preset {
			t.Errorf("manifest.files[%q].preset = %q, want %q", path, rec.Preset, preset)
		}
	}
	if got := strings.TrimSpace(readText(t, filepath.Join(dir, "CLAUDE.md"))); got != "@AGENTS.md" {
		t.Errorf("CLAUDE.md = %q, want exactly @AGENTS.md", got)
	}

	svc := newGoLibService(t)
	for _, id := range []string{"go-baseline", "go-lib"} {
		res, _, err := svc.ValidatePreset(filepath.Join("presets", id, "preset.yaml"))
		if err != nil {
			t.Fatalf("ValidatePreset %s: %v", id, err)
		}
		if !res.OK() {
			t.Errorf("ValidatePreset %s: %+v", id, res.Errors)
		}
	}
}

func TestGoLib_RejectsBadInputs(t *testing.T) {
	svc := newGoLibService(t)
	for name, inputs := range map[string]map[string]any{
		"wrong org":         {"repo_name": "go-demo", "package_name": "demo", "github_owner": "chrispian"},
		"uppercase repo":    {"repo_name": "Go-Demo", "package_name": "demo"},
		"bad stability":     {"repo_name": "go-demo", "package_name": "demo", "stability": "beta"},
		"bad layout":        {"repo_name": "go-demo", "package_name": "demo", "package_layout": "nested"},
		"malformed toolchn": {"repo_name": "go-demo", "package_name": "demo", "go_toolchain": "latest"},
	} {
		_, err := svc.New(service.NewOptions{
			PresetID:  "go-lib",
			TargetDir: filepath.Join(t.TempDir(), "x"),
			Inputs:    inputs,
		})
		if err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}
