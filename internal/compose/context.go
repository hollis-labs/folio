package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/internal/render"
)

// ScopeVarsForLayer produces the input map a composed preset sees during
// render.
//
// Default: inherit the caller's `.inputs.*` verbatim — including original
// Go types (number, bool, list[string]), not just strings.
//
// Per-entry overrides: each key in entryVars is rendered as a Go template
// against the *caller's* render.Context (so `{{.inputs.foo}}` resolves to
// the caller's input named foo, NOT the composed preset's), and the
// resulting string replaces the inherited value. Override values are
// always strings — type coercion against composed.Inputs[*].Type runs in
// the service layer's resolveInputs after this call.
//
// Validation: each key in entryVars must match an input name declared on
// the composed preset; an unknown key produces an error listing the
// declared inputs. This is the cross-preset rule deferred from
// preset.Validate (see plan §4 D8).
func ScopeVarsForLayer(
	callerCtx render.Context,
	entryVars map[string]string,
	composed *preset.Preset,
) (map[string]any, error) {
	out := make(map[string]any, len(callerCtx.Inputs)+len(entryVars))
	for k, v := range callerCtx.Inputs {
		out[k] = v
	}

	if len(entryVars) == 0 {
		return out, nil
	}

	declared := make(map[string]struct{}, len(composed.Inputs))
	for _, in := range composed.Inputs {
		declared[in.Name] = struct{}{}
	}

	for _, key := range sortedKeys(entryVars) {
		if _, ok := declared[key]; !ok {
			return nil, fmt.Errorf(
				"compose vars key %q is not a declared input of preset %q (declared: %s)",
				key, composed.ID, declaredList(composed),
			)
		}
		tpl := entryVars[key]
		rendered, err := render.RenderString(tpl, callerCtx)
		if err != nil {
			return nil, fmt.Errorf("compose vars[%q] template: %w", key, err)
		}
		out[key] = rendered
	}
	return out, nil
}

// MergeComputed combines per-layer computed maps in apply order. On key
// collision, the later (higher-indexed) layer wins — parallels the
// same-path file overwrite policy. Within each layer, keys are iterated in
// sorted order so map-iteration nondeterminism never affects the output
// (per-layer computed values are themselves produced in sorted-key order
// by service.resolveComputed). This function only orders ACROSS layers.
func MergeComputed(layers []map[string]any) map[string]any {
	out := map[string]any{}
	for _, layer := range layers {
		for _, k := range sortedAnyKeys(layer) {
			out[k] = layer[k]
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func declaredList(p *preset.Preset) string {
	names := make([]string, 0, len(p.Inputs))
	for _, in := range p.Inputs {
		names = append(names, in.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
