package compose

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/hollis-labs/folio/internal/preset"
)

// LoadResult is returned by a Loader when it resolves a composes[] entry.
// It is also the shape BuildGraph expects for its root argument.
type LoadResult struct {
	// Preset is the loaded preset manifest.
	Preset *preset.Preset

	// FS is the sub-FS rooted at the preset's directory (used by the
	// service render loop).
	FS fs.FS

	// Source is the loader-assigned source label ("bundled" | "local").
	Source string

	// ResolvedPath is the human-readable path the loader resolved
	// (e.g., "bundled:presets/base" or "/home/user/.folio/presets/local/base@1.2.0").
	ResolvedPath string

	// ParentDir is the directory this preset lives in, expressed in the
	// loader's path space. Used as the basePath when resolving this
	// preset's own composes[].path entries.
	ParentDir string
}

// Loader resolves a composes[] entry to a loaded preset. Implementations
// translate (entry, parentDir) into a sub-FS, performing constraint
// resolution and path-safety checks. The service satisfies this interface
// from outside internal/compose.
type Loader interface {
	Load(entry preset.ComposeEntry, parentDir string) (*LoadResult, error)
}

// Graph holds the resolved compose DAG rooted at the top-level preset.
type Graph struct {
	layers []LayerRef
}

// LayerOrder returns the topologically-sorted layers in apply order
// (deepest leaves first; root last). Same-path overwrites in the service
// render loop are silent — later layers in this slice win.
func (g *Graph) LayerOrder() []LayerRef { return g.layers }

// BuildGraph walks composes: recursively from root, calling loader.Load
// for each entry, detecting cycles, and capping depth at MaxComposeDepth.
//
// Cycle detection uses DFS three-color marking. A cycle error includes the
// full path through preset ids: "compose cycle detected: a → b → c → a".
// Depth-exceeded errors include the active call chain.
//
// Same preset reached via two paths (legitimate diamond) is deduped: only
// the first visit emits a layer, declared-order in the parent's composes[]
// determines tiebreak between siblings.
func BuildGraph(root *LoadResult, loader Loader) (*Graph, error) {
	g := &Graph{}
	const (
		colorInProgress = 1
		colorDone       = 2
	)
	colors := map[string]int{}
	stack := make([]string, 0, 8)

	var walk func(node *LoadResult, depth int) error
	walk = func(node *LoadResult, depth int) error {
		if depth > MaxComposeDepth {
			chain := append([]string{}, stack...)
			chain = append(chain, node.Preset.ID)
			return fmt.Errorf("compose depth exceeded (max %d): %s",
				MaxComposeDepth, strings.Join(chain, " → "))
		}
		id := node.Preset.ID
		switch colors[id] {
		case colorInProgress:
			chain := append([]string{}, stack...)
			chain = append(chain, id)
			return fmt.Errorf("compose cycle detected: %s", strings.Join(chain, " → "))
		case colorDone:
			return nil
		}
		colors[id] = colorInProgress
		stack = append(stack, id)
		for _, entry := range node.Preset.Composes {
			child, err := loader.Load(entry, node.ParentDir)
			if err != nil {
				return fmt.Errorf("load composes[%s] from %s: %w", entry.ID, id, err)
			}
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		g.layers = append(g.layers, LayerRef{
			Preset:       node.Preset,
			Source:       node.Source,
			ResolvedPath: node.ResolvedPath,
			FS:           node.FS,
		})
		colors[id] = colorDone
		stack = stack[:len(stack)-1]
		return nil
	}

	if err := walk(root, 0); err != nil {
		return nil, err
	}
	return g, nil
}

// ResolveComposePath joins a composes[].path entry against the parent
// preset's directory and verifies the cleaned target stays within the
// supplied root. All arguments use forward-slash path syntax (io/fs); OS
// callers translate before/after with filepath.
//
// An empty entry path is an error. Path traversal that escapes the root
// (e.g., "../../escape" from inside the root) returns an error citing the
// resolved target.
func ResolveComposePath(parentDir, entryPath, root string) (string, error) {
	if entryPath == "" {
		return "", fmt.Errorf("compose path is empty")
	}
	cleaned := path.Clean(path.Join(parentDir, entryPath))
	rootClean := path.Clean(root)
	if rootClean == "." || rootClean == "" {
		return cleaned, nil
	}
	if cleaned != rootClean && !strings.HasPrefix(cleaned, rootClean+"/") {
		return "", fmt.Errorf("compose path %q escapes root %q (resolves to %q)",
			entryPath, root, cleaned)
	}
	return cleaned, nil
}
