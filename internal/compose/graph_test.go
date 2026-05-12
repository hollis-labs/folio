package compose_test

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hollis-labs/folio/internal/compose"
	"github.com/hollis-labs/folio/internal/preset"
)

// fakeLoader satisfies compose.Loader from canned in-memory presets.
type fakeLoader struct {
	presets map[string]string // id → preset.yaml body
}

func (l *fakeLoader) Load(entry preset.ComposeEntry, _ string) (*compose.LoadResult, error) {
	body, ok := l.presets[entry.ID]
	if !ok {
		return nil, fmt.Errorf("fake loader: %s not registered", entry.ID)
	}
	p, err := preset.ParseBytes([]byte(body))
	if err != nil {
		return nil, fmt.Errorf("fake loader parse %s: %w", entry.ID, err)
	}
	return &compose.LoadResult{
		Preset:       p,
		FS:           fstest.MapFS{},
		Source:       "test",
		ResolvedPath: "test:" + entry.ID,
		ParentDir:    "test/" + entry.ID,
	}, nil
}

// fixturePreset builds a minimal preset.yaml body with the given id and
// composes entries.
func fixturePreset(id string, composes ...string) string {
	var sb strings.Builder
	sb.WriteString("folio_version: \"0.1\"\n")
	fmt.Fprintf(&sb, "id: %s\n", id)
	sb.WriteString("version: 1.0.0\n")
	sb.WriteString("files:\n  source: ./files\n")
	if len(composes) > 0 {
		sb.WriteString("composes:\n")
		for _, c := range composes {
			fmt.Fprintf(&sb, "  - id: %s\n    version: \">=1.0,<2.0\"\n    source: local\n    path: ../%s\n", c, c)
		}
	}
	return sb.String()
}

func layerIDs(layers []compose.LayerRef) []string {
	out := make([]string, 0, len(layers))
	for _, l := range layers {
		out = append(out, l.Preset.ID)
	}
	return out
}

func loadRoot(t *testing.T, id string, composes ...string) *compose.LoadResult {
	t.Helper()
	p, err := preset.ParseBytes([]byte(fixturePreset(id, composes...)))
	if err != nil {
		t.Fatalf("parse root %s: %v", id, err)
	}
	return &compose.LoadResult{
		Preset:       p,
		FS:           fstest.MapFS{},
		Source:       "test",
		ResolvedPath: "test:" + id,
		ParentDir:    "test/" + id,
	}
}

func TestBuildGraph_TwoPresetLinear(t *testing.T) {
	loader := &fakeLoader{presets: map[string]string{
		"base": fixturePreset("base"),
	}}
	root := loadRoot(t, "go-package", "base")

	g, err := compose.BuildGraph(root, loader)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	got := layerIDs(g.LayerOrder())
	want := []string{"base", "go-package"}
	if !equalStrings(got, want) {
		t.Errorf("LayerOrder = %v, want %v", got, want)
	}
}

func TestBuildGraph_Diamond(t *testing.T) {
	loader := &fakeLoader{presets: map[string]string{
		"base":  fixturePreset("base"),
		"mid_a": fixturePreset("mid_a", "base"),
		"mid_b": fixturePreset("mid_b", "base"),
	}}
	root := loadRoot(t, "top", "mid_a", "mid_b")

	g, err := compose.BuildGraph(root, loader)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	got := layerIDs(g.LayerOrder())
	// Declared-order tiebreak: mid_a's subtree fully emits before mid_b's
	// visit; base is deduped on mid_b's path.
	want := []string{"base", "mid_a", "mid_b", "top"}
	if !equalStrings(got, want) {
		t.Errorf("LayerOrder = %v, want %v", got, want)
	}
}

func TestBuildGraph_SelfCycle(t *testing.T) {
	loader := &fakeLoader{presets: map[string]string{
		"go-package": fixturePreset("go-package", "go-package"),
	}}
	root := loadRoot(t, "go-package", "go-package")

	_, err := compose.BuildGraph(root, loader)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q should mention cycle", err.Error())
	}
	if !strings.Contains(err.Error(), "go-package → go-package") {
		t.Errorf("error should show cycle path, got: %s", err.Error())
	}
}

func TestBuildGraph_TransitiveCycle(t *testing.T) {
	loader := &fakeLoader{presets: map[string]string{
		"a": fixturePreset("a", "b"),
		"b": fixturePreset("b", "c"),
		"c": fixturePreset("c", "a"),
	}}
	root := loadRoot(t, "a", "b")

	_, err := compose.BuildGraph(root, loader)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "a → b → c → a") {
		t.Errorf("error should show full cycle path, got: %s", err.Error())
	}
}

func TestBuildGraph_DepthCap(t *testing.T) {
	// Build a 10-deep chain: root → l1 → l2 → ... → l9
	loader := &fakeLoader{presets: map[string]string{}}
	for i := 1; i < 10; i++ {
		next := ""
		if i < 9 {
			next = fmt.Sprintf("l%d", i+1)
		}
		if next == "" {
			loader.presets[fmt.Sprintf("l%d", i)] = fixturePreset(fmt.Sprintf("l%d", i))
		} else {
			loader.presets[fmt.Sprintf("l%d", i)] = fixturePreset(fmt.Sprintf("l%d", i), next)
		}
	}
	root := loadRoot(t, "root", "l1")

	_, err := compose.BuildGraph(root, loader)
	if err == nil {
		t.Fatal("expected depth error, got nil")
	}
	if !strings.Contains(err.Error(), "depth exceeded") {
		t.Errorf("error should mention depth, got: %s", err.Error())
	}
}

func TestBuildGraph_LoaderError(t *testing.T) {
	loader := &fakeLoader{presets: map[string]string{}} // empty — load fails
	root := loadRoot(t, "go-package", "base")

	_, err := compose.BuildGraph(root, loader)
	if err == nil {
		t.Fatal("expected loader error, got nil")
	}
	if !strings.Contains(err.Error(), "base") {
		t.Errorf("error should mention missing id, got: %s", err.Error())
	}
}

func TestBuildGraph_NoComposes(t *testing.T) {
	loader := &fakeLoader{}
	root := loadRoot(t, "atom")

	g, err := compose.BuildGraph(root, loader)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	got := layerIDs(g.LayerOrder())
	want := []string{"atom"}
	if !equalStrings(got, want) {
		t.Errorf("LayerOrder = %v, want %v", got, want)
	}
}

func TestResolveComposePath(t *testing.T) {
	cases := []struct {
		name      string
		parent    string
		entry     string
		root      string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "parent_relative_dotdot",
			parent: "presets/go-package",
			entry:  "../base",
			root:   "presets",
			want:   "presets/base",
		},
		{
			name:   "same_dir",
			parent: "presets/go-package",
			entry:  ".",
			root:   "presets",
			want:   "presets/go-package",
		},
		{
			name:   "nested",
			parent: "presets/top",
			entry:  "sub/leaf",
			root:   "presets",
			want:   "presets/top/sub/leaf",
		},
		{
			name:      "escape_dotdot",
			parent:    "presets/go-package",
			entry:     "../../escape",
			root:      "presets",
			wantErr:   true,
			errSubstr: "escapes root",
		},
		{
			name:    "absolute_escape",
			parent:  "presets/x",
			entry:   "/etc/passwd",
			root:    "presets",
			want:    "presets/x/etc/passwd", // path.Join treats the leading "/" as separator
			wantErr: false,
		},
		{
			name:      "empty_entry",
			parent:    "presets/x",
			entry:     "",
			root:      "presets",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:   "root_dot",
			parent: "go-package",
			entry:  "../base",
			root:   ".",
			want:   "base",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compose.ResolveComposePath(tc.parent, tc.entry, tc.root)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q missing substr %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Compile-time check: fstest.MapFS satisfies fs.FS (sanity for the test plumbing).
var _ fs.FS = fstest.MapFS{}
