// Package compose runs the composes: layering protocol described in
// planning/folio/design/preset-format-and-manifest-v0.md §3, §6, §7.
//
// Responsibilities:
//   - Resolve semver constraints declared in composes[].version (resolver.go).
//   - Walk composes: recursively, detect cycles, cap depth, and produce a
//     topologically-ordered apply list (graph.go).
//   - Scope per-layer template inputs and merge computed values across layers
//     (context.go).
//
// The service layer drives rendering against this package's outputs;
// internal/compose does not write files or know about render.TreeOptions.
package compose

import (
	"io/fs"

	"github.com/hollis-labs/folio/internal/preset"
)

// MaxComposeDepth caps the depth of transitive composes: walks. Exceeding the
// cap produces a path-bearing error during BuildGraph.
const MaxComposeDepth = 8

// DefaultSource is the source value applied when a composes[] entry omits
// the source field. v0.2 supports source: local only.
const DefaultSource = "local"

// LayerRef is one preset layer in apply order. Populated by Graph.LayerOrder
// and consumed by the service render loop.
type LayerRef struct {
	// Preset is the loaded preset for this layer.
	Preset *preset.Preset

	// Source is the loader-assigned source label ("bundled" | "local").
	Source string

	// ResolvedPath is the path the loader resolved for this layer, recorded
	// in .folio.yaml for reproducibility.
	ResolvedPath string

	// FS is the layer's render-time sub-FS rooted at the composed preset's
	// directory. The service uses this with render.RenderTree.
	FS fs.FS

	// CallerVars carries the scoped inputs for this layer (caller inputs
	// with per-entry overrides applied). Populated by ScopeVarsForLayer in
	// P3 — left nil by BuildGraph.
	CallerVars map[string]any
}
