package compose

// Constraint matches a semver string against a parsed expression. The zero
// value matches nothing; use ParseConstraint to obtain a usable value.
//
// Implementation lands in P1.
type Constraint struct{}

// Matches reports whether the given version satisfies the constraint.
//
// Implementation lands in P1.
func (Constraint) Matches(version string) bool { return false }

// ParseConstraint parses an npm-style semver constraint expression.
// Accepted operators: >=, <=, >, <, ~, ^, *, exact (e.g. "1.2.3"),
// AND-via-comma (">=1.0,<2.0"), OR-via-pipe ("^1.0 || ^2.0").
//
// Implementation lands in P1.
func ParseConstraint(s string) (Constraint, error) { //nolint:unused // P1
	return Constraint{}, nil
}

// ResolveVersion picks the highest available version that satisfies the
// constraint. Returns an error listing the available versions when no match
// is found.
//
// Implementation lands in P1.
func ResolveVersion(c Constraint, available []string) (string, error) { //nolint:unused // P1
	return "", nil
}
