package render_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio/internal/render"
)

func treeContext() render.Context {
	return render.Context{
		Inputs: map[string]any{
			"project_name": "smoke_test",
			"description":  "folio smoke fixture",
		},
		Computed: map[string]any{
			"module_path": "github.com/chrispian/smoke_test",
		},
		Target: "/tmp/folio-render-test",
		Preset: render.PresetInfo{ID: "fixture", Version: "1.0.0"},
		Folio:  render.FolioInfo{Version: "0.1.0"},
		Now:    time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
	}
}

func TestRenderTree_Basics(t *testing.T) {
	src, err := filepath.Abs(filepath.Join("testdata", "preset", "files"))
	if err != nil {
		t.Fatal(err)
	}
	opts := render.TreeOptions{
		Source:         render.DirFSAt(src),
		TemplateSuffix: ".tmpl",
		Ignore:         []string{"*.example"},
	}
	res, err := render.RenderTree(opts, treeContext())
	if err != nil {
		t.Fatalf("RenderTree: %v", err)
	}

	got := map[string]render.RenderedFile{}
	for _, f := range res.Files {
		got[f.RelPath] = f
	}

	expectations := []struct {
		path       string
		isTemplate bool
		contains   string
	}{
		{"README.md", true, "# SmokeTest"},
		{"go.mod", true, "module github.com/chrispian/smoke_test"},
		{".gitignore", false, "/{{.inputs.project_name}}"},
		{"cmd/smoke_test/main.go", true, "Hello, smoke_test"},
		{".github/workflows/ci.yml", true, "working-directory: smoke_test"},
	}

	for _, e := range expectations {
		t.Run(e.path, func(t *testing.T) {
			f, ok := got[e.path]
			if !ok {
				t.Fatalf("expected file %q not produced; files = %v", e.path, keys(got))
			}
			if f.IsTemplate != e.isTemplate {
				t.Errorf("IsTemplate = %v, want %v", f.IsTemplate, e.isTemplate)
			}
			if !strings.Contains(string(f.Content), e.contains) {
				t.Errorf("content does not contain %q; got:\n%s", e.contains, f.Content)
			}
		})
	}

	if _, ok := got["example.txt.example"]; ok {
		t.Error("*.example ignore glob did not filter the file")
	}
}

func TestRenderTree_Sorted(t *testing.T) {
	src, _ := filepath.Abs(filepath.Join("testdata", "preset", "files"))
	res, err := render.RenderTree(render.TreeOptions{
		Source: render.DirFSAt(src),
		Ignore: []string{"*.example"},
	}, treeContext())
	if err != nil {
		t.Fatalf("RenderTree: %v", err)
	}
	for i := 1; i < len(res.Files); i++ {
		if res.Files[i-1].RelPath >= res.Files[i].RelPath {
			t.Errorf("files not sorted: %q >= %q", res.Files[i-1].RelPath, res.Files[i].RelPath)
		}
	}
}

func TestRenderTree_FrozenNow(t *testing.T) {
	// Two renders with the same ctx produce identical timestamps.
	src, _ := filepath.Abs(filepath.Join("testdata", "preset", "files"))
	ctx := treeContext()
	a, err := render.RenderTree(render.TreeOptions{Source: render.DirFSAt(src), Ignore: []string{"*.example"}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := render.RenderTree(render.TreeOptions{Source: render.DirFSAt(src), Ignore: []string{"*.example"}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Files) != len(b.Files) {
		t.Fatalf("file count differs: %d vs %d", len(a.Files), len(b.Files))
	}
	for i := range a.Files {
		if string(a.Files[i].Content) != string(b.Files[i].Content) {
			t.Errorf("content differs for %q on re-render", a.Files[i].RelPath)
		}
	}
}

func TestRenderTree_BadSource(t *testing.T) {
	if _, err := render.RenderTree(render.TreeOptions{Source: nil}, treeContext()); err == nil {
		t.Fatal("expected error for nil Source")
	}
	if _, err := render.RenderTree(render.TreeOptions{Source: render.DirFSAt("/path/does/not/exist")}, treeContext()); err == nil {
		t.Fatal("expected error for missing Source")
	}
}

func keys(m map[string]render.RenderedFile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
