package compose

// Graph holds the resolved compose DAG rooted at the top-level preset.
// Implementation lands in P2.
type Graph struct{}

// Loader loads a composed preset given the caller-supplied entry shape.
// The service implements this interface; internal/compose does not import
// service to avoid a cycle.
//
// Implementation lands in P2.
type Loader interface{}

// BuildGraph walks composes: recursively from root, applying constraint
// resolution per entry, detecting cycles, and capping depth at
// MaxComposeDepth. Returns a Graph whose LayerOrder produces the apply
// sequence used by the service render loop.
//
// Implementation lands in P2.
func BuildGraph(rootID string, loader Loader) (*Graph, error) { //nolint:unused // P2
	return &Graph{}, nil
}

// LayerOrder returns the topologically-sorted layers in apply order
// (deepest leaves first; root last). Same-path overwrites in the service
// render loop are silent — later layers in this list win.
//
// Implementation lands in P2.
func (g *Graph) LayerOrder() []LayerRef { return nil }
