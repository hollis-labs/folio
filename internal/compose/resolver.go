package compose

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Constraint represents a parsed npm-style semver constraint expression.
// The zero value matches nothing; obtain a usable value via ParseConstraint
// or MatchAny.
type Constraint struct {
	expr    string
	clauses []clause
}

// clause is an AND'd group of comparators. A constraint matches if any of
// its clauses matches (clauses are OR'd via "||").
type clause []comparator

type comparator struct {
	op  string // ">=", "<=", ">", "<", "="
	ver string // canonical "vMAJOR.MINOR.PATCH"
}

// String returns the original constraint expression.
func (c Constraint) String() string { return c.expr }

// ParseConstraint accepts npm-style semver constraint expressions:
//   - exact ("1.2.3", "1.2", "1")
//   - operators: ">=", "<=", ">", "<"
//   - tilde ("~1.2.3" → patch updates within 1.2.x)
//   - caret ("^1.2.3" → left-most non-zero updates: ^1.2.3 → <2.0.0;
//     ^0.2.3 → <0.3.0; ^0.0.3 → <0.0.4)
//   - star ("*") matches any version
//   - AND via comma (">=1.0,<2.0")
//   - OR via pipe ("^1.0 || ^2.0")
func ParseConstraint(s string) (Constraint, error) {
	expr := strings.TrimSpace(s)
	if expr == "" {
		return Constraint{}, fmt.Errorf("empty constraint expression")
	}
	parts := strings.Split(expr, "||")
	clauses := make([]clause, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return Constraint{}, fmt.Errorf("empty OR clause in %q", s)
		}
		cl, err := parseClause(p)
		if err != nil {
			return Constraint{}, err
		}
		clauses = append(clauses, cl)
	}
	return Constraint{expr: expr, clauses: clauses}, nil
}

// MatchAny returns a constraint that matches every valid semver string.
// Used by the service for top-level invocations without an explicit
// constraint expression.
func MatchAny() Constraint {
	c, _ := ParseConstraint("*")
	return c
}

func parseClause(s string) (clause, error) {
	parts := strings.Split(s, ",")
	out := clause{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("empty AND term in %q", s)
		}
		cmps, err := parseComparators(p)
		if err != nil {
			return nil, err
		}
		out = append(out, cmps...)
	}
	return out, nil
}

func parseComparators(s string) ([]comparator, error) {
	if s == "*" {
		return nil, nil // empty clause matches anything in matchClause
	}
	switch {
	case strings.HasPrefix(s, ">="):
		return []comparator{cmp(">=", strings.TrimSpace(s[2:]))}, validate(s[2:])
	case strings.HasPrefix(s, "<="):
		return []comparator{cmp("<=", strings.TrimSpace(s[2:]))}, validate(s[2:])
	case strings.HasPrefix(s, ">"):
		return []comparator{cmp(">", strings.TrimSpace(s[1:]))}, validate(s[1:])
	case strings.HasPrefix(s, "<"):
		return []comparator{cmp("<", strings.TrimSpace(s[1:]))}, validate(s[1:])
	case strings.HasPrefix(s, "~"):
		return expandTilde(strings.TrimSpace(s[1:]))
	case strings.HasPrefix(s, "^"):
		return expandCaret(strings.TrimSpace(s[1:]))
	}
	// Bare version literal: must look like a version (digits + dots).
	if _, err := canonicalize(s); err != nil {
		return nil, fmt.Errorf("invalid constraint term %q: %w", s, err)
	}
	return []comparator{cmp("=", s)}, nil
}

func validate(raw string) error {
	_, err := canonicalize(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid version operand %q: %w", strings.TrimSpace(raw), err)
	}
	return nil
}

func cmp(op, raw string) comparator {
	// canonicalize is best-effort here; parseComparators validates upstream.
	canon, _ := canonicalize(raw)
	return comparator{op: op, ver: canon}
}

var versionShape = regexp.MustCompile(`^\d+(?:\.\d+){0,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func canonicalize(v string) (string, error) {
	if v == "" {
		return "", fmt.Errorf("empty version")
	}
	v = strings.TrimPrefix(v, "v")
	if !versionShape.MatchString(v) {
		return "", fmt.Errorf("not a version: %q", v)
	}
	canon := semver.Canonical("v" + v)
	if canon == "" {
		return "", fmt.Errorf("not valid semver: %q", v)
	}
	return canon, nil
}

func expandTilde(v string) ([]comparator, error) {
	canon, err := canonicalize(v)
	if err != nil {
		return nil, fmt.Errorf("invalid tilde operand: %w", err)
	}
	bare := strings.TrimPrefix(v, "v")
	parts := strings.SplitN(bare, ".", 3)
	var upper string
	if len(parts) <= 1 {
		upper = bumpMajor(canon)
	} else {
		upper = bumpMinor(canon)
	}
	return []comparator{
		{op: ">=", ver: canon},
		{op: "<", ver: upper},
	}, nil
}

func expandCaret(v string) ([]comparator, error) {
	canon, err := canonicalize(v)
	if err != nil {
		return nil, fmt.Errorf("invalid caret operand: %w", err)
	}
	major := majorInt(canon)
	minor := minorInt(canon)
	var upper string
	switch {
	case major > 0:
		upper = bumpMajor(canon)
	case minor > 0:
		upper = bumpMinor(canon)
	default:
		upper = bumpPatch(canon)
	}
	return []comparator{
		{op: ">=", ver: canon},
		{op: "<", ver: upper},
	}, nil
}

func majorInt(canon string) int {
	s := strings.TrimPrefix(semver.Major(canon), "v")
	n, _ := strconv.Atoi(s)
	return n
}

func minorInt(canon string) int {
	mm := strings.TrimPrefix(semver.MajorMinor(canon), "v")
	parts := strings.Split(mm, ".")
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

func patchInt(canon string) int {
	bare := strings.TrimPrefix(semver.Canonical(canon), "v")
	parts := strings.Split(bare, ".")
	if len(parts) < 3 {
		return 0
	}
	// trim prerelease/build suffix
	patch := parts[2]
	for i, c := range patch {
		if c == '-' || c == '+' {
			patch = patch[:i]
			break
		}
	}
	n, _ := strconv.Atoi(patch)
	return n
}

func bumpMajor(canon string) string {
	return fmt.Sprintf("v%d.0.0", majorInt(canon)+1)
}

func bumpMinor(canon string) string {
	return fmt.Sprintf("v%d.%d.0", majorInt(canon), minorInt(canon)+1)
}

func bumpPatch(canon string) string {
	return fmt.Sprintf("v%d.%d.%d", majorInt(canon), minorInt(canon), patchInt(canon)+1)
}

// Matches reports whether version satisfies the constraint.
func (c Constraint) Matches(version string) bool {
	canon, err := canonicalize(version)
	if err != nil {
		return false
	}
	if len(c.clauses) == 0 {
		return false
	}
	for _, cl := range c.clauses {
		if matchClause(cl, canon) {
			return true
		}
	}
	return false
}

func matchClause(cl clause, canon string) bool {
	if len(cl) == 0 {
		return true // empty clause from "*"
	}
	for _, c := range cl {
		d := semver.Compare(canon, c.ver)
		switch c.op {
		case ">=":
			if d < 0 {
				return false
			}
		case "<=":
			if d > 0 {
				return false
			}
		case ">":
			if d <= 0 {
				return false
			}
		case "<":
			if d >= 0 {
				return false
			}
		case "=":
			if d != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ResolveVersion picks the highest available version that satisfies the
// constraint. Versions that fail to parse are skipped. Returns an error
// listing the available versions when none match, or when available is
// empty.
func ResolveVersion(c Constraint, available []string) (string, error) {
	if len(available) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	var best string
	var bestCanon string
	for _, v := range available {
		canon, err := canonicalize(v)
		if err != nil {
			continue
		}
		if !c.Matches(v) {
			continue
		}
		if best == "" || semver.Compare(canon, bestCanon) > 0 {
			best = v
			bestCanon = canon
		}
	}
	if best == "" {
		return "", fmt.Errorf("no version satisfies %q (available: %s)", c.expr, strings.Join(available, ", "))
	}
	return best, nil
}
