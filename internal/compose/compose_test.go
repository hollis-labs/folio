package compose_test

import (
	"testing"

	"github.com/hollis-labs/folio/internal/compose"
)

// TestSkeleton is a smoke test that the package compiles. Real coverage
// lands phase-by-phase: P1 resolver, P2 graph, P3 context.
func TestSkeleton(t *testing.T) {
	if compose.MaxComposeDepth != 8 {
		t.Errorf("MaxComposeDepth = %d, want 8", compose.MaxComposeDepth)
	}
	if compose.DefaultSource != "local" {
		t.Errorf("DefaultSource = %q, want %q", compose.DefaultSource, "local")
	}
}
