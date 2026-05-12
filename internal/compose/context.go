package compose

// ScopeVarsForLayer produces the input map a composed preset sees during
// render. The caller's .inputs.* are inherited by default; per-key
// overrides in the compose entry's vars: block are rendered against the
// caller's context and replace the inherited value.
//
// Implementation lands in P3.
func ScopeVarsForLayer( //nolint:unused // P3
	callerInputs map[string]any,
	entryVars map[string]string,
) (map[string]any, error) {
	return callerInputs, nil
}

// MergeComputed combines per-layer computed maps in apply order. On key
// collision, the later (higher-indexed) layer wins — parallels the
// same-path file overwrite policy.
//
// Implementation lands in P3.
func MergeComputed(layers []map[string]any) map[string]any { //nolint:unused // P3
	out := map[string]any{}
	for _, l := range layers {
		for k, v := range l {
			out[k] = v
		}
	}
	return out
}
