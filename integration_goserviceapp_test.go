package folio_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// go-service-app renders a new service app with the transport-boundary lint
// gate. These tests render it, assert the tree, and then run the RENDERED
// .golangci.transport.yml with the real golangci-lint against a synthetic
// module — the positive control: the rule fires on a store-handle call in a
// transport and stays silent on a type-only use, on the service layer and on a
// path outside the transport regex.
//
// A missing golangci-lint is a visible SKIP (a failure under
// FOLIO_REQUIRE_TOOLS=1), as in the go-lib tests.

func renderGoServiceApp(t *testing.T, extra map[string]any) (string, error) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "my-service")
	inputs := map[string]any{
		"repo_name":      "my-service",
		"store_pkg_name": "store",
		"store_type":     "Store",
	}
	for k, v := range extra {
		inputs[k] = v
	}
	_, err := newGoLibService(t).New(service.NewOptions{
		PresetID:  "go-service-app",
		TargetDir: target,
		Inputs:    inputs,
	})
	return target, err
}

func TestGoServiceApp_RendersTree(t *testing.T) {
	target, err := renderGoServiceApp(t, nil)
	if err != nil {
		t.Fatalf("service.New go-service-app: %v", err)
	}

	// go-baseline's files plus this preset's; nothing from `base` (no Makefile,
	// no README, no ci.yml with a literal go-version).
	want := []string{
		".folio.yaml",
		".github/quality/transport-baseline.txt",
		".github/workflows/check.yml",
		".gitignore",
		".golangci.transport.yml",
		".golangci.yml",
		"AGENTS.md",
		"CHANGELOG.md",
		"CLAUDE.md",
		"LICENSE",
		"cmd/my-service/main.go",
		"docs/transport-boundary.md",
		"go.mod",
		"lefthook.yml",
	}
	for _, p := range want {
		if _, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(p))); statErr != nil {
			t.Errorf("expected %s, missing: %v", p, statErr)
		}
	}
	for _, p := range []string{"Makefile", "README.md", ".github/workflows/ci.yml"} {
		if _, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(p))); statErr == nil {
			t.Errorf("%s must not be rendered (it belongs to `base`, which this preset does not compose)", p)
		}
	}

	// No template residue in any rendered file.
	_ = filepath.WalkDir(target, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read %s: %v", path, readErr)
			return nil
		}
		if strings.Contains(string(b), "{{") || strings.Contains(string(b), "<no value>") {
			rel, _ := filepath.Rel(target, path)
			t.Errorf("%s has unresolved template text", rel)
		}
		return nil
	})

	gomod := readText(t, filepath.Join(target, "go.mod"))
	if !strings.Contains(gomod, "module github.com/hollis-labs/my-service\n") {
		t.Errorf("go.mod module line wrong:\n%s", gomod)
	}
	if !strings.Contains(gomod, "\ngo 1.26.6\n") {
		t.Errorf("go.mod missing default go directive 1.26.6:\n%s", gomod)
	}

	cfg := readText(t, filepath.Join(target, ".golangci.transport.yml"))
	for _, s := range []string{
		`pattern: '^store\.Store\.[A-Z].*$'`,
		`path-except: '^internal/(api|mcp)/'`,
		"relative-path-mode: gomod",
		"analyze-types: true",
		"max-issues-per-linter: 0",
		`pkg: "github.com/hollis-labs/my-service/internal/store"`,
	} {
		if !strings.Contains(cfg, s) {
			t.Errorf(".golangci.transport.yml missing %q:\n%s", s, cfg)
		}
	}

	doc := readText(t, filepath.Join(target, "docs", "transport-boundary.md"))
	for _, s := range []string{
		"go-version-file: go.mod",
		">= v2.11.4",
		"v2.1.6 is NOT acceptable",
		"relative-path-mode: gomod",
		"analyze-types: true",
		"## Ratchet plan",
		"## Transport-parity checklist",
		"## Limits and blind spots",
		"store handle `store.Store`",
	} {
		if !strings.Contains(doc, s) {
			t.Errorf("docs/transport-boundary.md missing %q", s)
		}
	}

	// The CI go-baseline renders satisfies the doc's own requirement.
	check := readText(t, filepath.Join(target, ".github", "workflows", "check.yml"))
	if !strings.Contains(check, "go-version-file: go.mod") {
		t.Errorf("check.yml does not read the Go pin from go.mod")
	}

	// go-baseline's own lint config is untouched by this preset.
	if strings.Contains(readText(t, filepath.Join(target, ".golangci.yml")), "transport-boundary") {
		t.Errorf(".golangci.yml must not carry the transport rule; it is a second file")
	}

	mf, err := manifest.Read(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(mf.Presets) != 2 || mf.Presets[0].ID != "go-baseline" || mf.Presets[1].ID != "go-service-app" {
		t.Fatalf("manifest presets = %+v, want [go-baseline go-service-app]", mf.Presets)
	}
	if got := mf.Files[".golangci.transport.yml"].Preset; got != "go-service-app" {
		t.Errorf("attribution of .golangci.transport.yml = %q", got)
	}

	if _, lookErr := exec.LookPath("go"); lookErr == nil {
		mustRun(t, target, "go", "vet", "./...")
		mustRun(t, target, "go", "build", "./...")
	}
}

func TestGoServiceApp_InputOverridesAndRejections(t *testing.T) {
	target, err := renderGoServiceApp(t, map[string]any{
		"store_pkg_name":       "sqlite",
		"store_type":           "Repository",
		"transport_dirs_regex": "^internal/(httpserver|mcpadapter)/",
		"store_pkg_path":       "internal/store/sqlite",
		"go_version":           "1.26.8",
	})
	if err != nil {
		t.Fatalf("service.New with overrides: %v", err)
	}
	cfg := readText(t, filepath.Join(target, ".golangci.transport.yml"))
	for _, s := range []string{
		`pattern: '^sqlite\.Repository\.[A-Z].*$'`,
		`path-except: '^internal/(httpserver|mcpadapter)/'`,
		`pkg: "github.com/hollis-labs/my-service/internal/store/sqlite"`,
	} {
		if !strings.Contains(cfg, s) {
			t.Errorf("override not rendered: missing %q", s)
		}
	}
	if !strings.Contains(readText(t, filepath.Join(target, "go.mod")), "\ngo 1.26.8\n") {
		t.Errorf("go_version override not rendered")
	}

	// The regex lands inside a single-quoted YAML string: a quote is refused.
	if _, err := renderGoServiceApp(t, map[string]any{"transport_dirs_regex": "^internal/api'/"}); err == nil {
		t.Errorf("transport_dirs_regex containing a single quote must be rejected")
	}
	// A missing required input is refused.
	if _, err := newGoLibService(t).New(service.NewOptions{
		PresetID:  "go-service-app",
		TargetDir: filepath.Join(t.TempDir(), "x"),
		Inputs:    map[string]any{"repo_name": "x"},
	}); err == nil {
		t.Errorf("missing store_pkg_name/store_type must be rejected")
	}
}

func TestGoServiceApp_ConfigVerifies(t *testing.T) {
	needTool(t, "golangci-lint")
	target, err := renderGoServiceApp(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, target, "golangci-lint", "config", "verify", "--config", ".golangci.transport.yml")
}

// syntheticModule writes the positive-control module: internal/{api,service,
// store,mcp}. Only the marked lines may be flagged.
func syntheticModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/syn\n\ngo 1.26.6\n",
		"internal/store/store.go": `package store

type Row struct{ ID int }
type Store struct{}

func (s *Store) Get(id int) Row { return Row{ID: id} }
`,
		"internal/service/service.go": `package service

import "example.com/syn/internal/store"

type Service struct{ st *store.Store }

func New(st *store.Store) *Service { return &Service{st: st} }

// The service layer may call the store: outside the transport regex.
func (s *Service) Get(id int) store.Row { return s.st.Get(id) }
`,
		"internal/api/api.go": `package api

import (
	"context"
	"database/sql"

	"example.com/syn/internal/service"
	"example.com/syn/internal/store"
)

type API struct {
	Store *store.Store
	Svc   *service.Service
	DB    *sql.DB
}

func (a *API) Leak() store.Row { return a.Store.Get(1) } // Rule A: flagged

func (a *API) LeakSQL() { _, _ = a.DB.QueryContext(context.Background(), "select 1") } // Rule C: flagged

func (a *API) Fine() store.Row { return a.Svc.Get(1) } // service layer: not flagged

func rowID(r store.Row) int { return r.ID } // type-only use in a signature: not flagged
`,
		"internal/mcp/mcp.go": `package mcp

import "example.com/syn/internal/store"

func Leak(s *store.Store) store.Row { return s.Get(2) } // flagged only when mcp is in the regex
`,
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type lintIssue struct {
	FromLinter string
	Text       string
	Pos        struct {
		Filename string
		Line     int
	}
}

// lintSynthetic runs the rendered transport config (from outside the module,
// as an adopter would) against mod and returns the issues.
func lintSynthetic(t *testing.T, mod, cfg string) []lintIssue {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out.json")
	cmd := exec.Command("golangci-lint", "run", "--allow-parallel-runners", "--config", cfg,
		"--issues-exit-code=0", "--output.json.path="+outFile, "--output.text.path=/dev/null",
		"--show-stats=false", "./...")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("golangci-lint run: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	var res struct{ Issues []lintIssue }
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("parse lint json: %v\n%s", err, raw)
	}
	return res.Issues
}

func TestGoServiceApp_SyntheticPositiveControl(t *testing.T) {
	needTool(t, "golangci-lint")
	needTool(t, "go")

	t.Run("default regex flags api and mcp", func(t *testing.T) {
		target, err := renderGoServiceApp(t, nil)
		if err != nil {
			t.Fatal(err)
		}
		// The default regex covers api and mcp: exactly the three marked calls.
		issues := lintSynthetic(t, syntheticModule(t), filepath.Join(target, ".golangci.transport.yml"))
		got := map[string]int{}
		for _, is := range issues {
			if is.FromLinter != "forbidigo" || !strings.Contains(is.Text, "transport-boundary") {
				t.Errorf("unexpected issue: %+v", is)
			}
			got[is.Pos.Filename+": "+strings.Fields(is.Text)[2]]++
		}
		want := map[string]int{
			"internal/api/api.go: `a.Store.Get`":       1,
			"internal/api/api.go: `a.DB.QueryContext`": 1,
			"internal/mcp/mcp.go: `s.Get`":             1,
		}
		if len(got) != len(want) {
			t.Errorf("issues = %v, want %v", got, want)
		}
		for k, n := range want {
			if got[k] != n {
				t.Errorf("issue %q count = %d, want %d (all: %v)", k, got[k], n, got)
			}
		}
	})

	t.Run("regex scopes by path", func(t *testing.T) {
		target, err := renderGoServiceApp(t, map[string]any{"transport_dirs_regex": "^internal/mcp/"})
		if err != nil {
			t.Fatal(err)
		}
		issues := lintSynthetic(t, syntheticModule(t), filepath.Join(target, ".golangci.transport.yml"))
		if len(issues) != 1 || issues[0].Pos.Filename != "internal/mcp/mcp.go" {
			t.Errorf("with regex ^internal/mcp/ want exactly the mcp finding, got %+v", issues)
		}
	})

	t.Run("wrong package name matches nothing", func(t *testing.T) {
		// Gotcha: the pattern is <package NAME>.<Type>.<Method>. A name that is
		// not the package's name silently reports 0 — the failure mode the doc
		// warns about, kept here so the control cannot pass vacuously.
		target, err := renderGoServiceApp(t, map[string]any{"store_pkg_name": "storage"})
		if err != nil {
			t.Fatal(err)
		}
		issues := lintSynthetic(t, syntheticModule(t), filepath.Join(target, ".golangci.transport.yml"))
		for _, is := range issues {
			if strings.Contains(is.Text, "store methods") {
				t.Errorf("rule A fired with the wrong package name: %+v", is)
			}
		}
	})
}
