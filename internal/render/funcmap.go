package render

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// FuncMap returns the folio template funcmap bound to ctx. Helpers that
// need run-scoped state (gitUser cache, .now alias) close over ctx so each
// render call is independent.
//
// The funcmap deliberately omits env, readFile, getHostByName, httpGet, and
// exec/shell — see package doc for the rationale.
func FuncMap(ctx Context) template.FuncMap {
	gitCache := &gitUserCache{}
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}

	return template.FuncMap{
		// --- string ---
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"title":      titleCase,
		"trim":       strings.TrimSpace,
		"trimPrefix": func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
		"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
		"replace":    func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"contains":   func(substr, s string) bool { return strings.Contains(s, substr) },
		"hasPrefix":  func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"hasSuffix":  func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },
		"split":      func(sep, s string) []string { return strings.Split(s, sep) },
		"join":       joinAny,
		"repeat":     func(count int, s string) string { return strings.Repeat(s, count) },
		"indent":     indent,
		"nindent":    func(spaces int, s string) string { return "\n" + indent(spaces, s) },

		// --- case conversion ---
		"kebabCase":  kebabCase,
		"snakeCase":  snakeCase,
		"camelCase":  camelCase,
		"pascalCase": pascalCase,

		// --- path (Hadron-aligned names) ---
		"basename":  filepath.Base,
		"dirname":   filepath.Dir,
		"ext":       filepath.Ext,
		"pathJoin":  func(elems ...string) string { return filepath.Join(elems...) },
		"pathClean": filepath.Clean,

		// --- default / fallback ---
		"default":  defaultFn,
		"coalesce": coalesce,
		"ternary": func(yes, no any, cond bool) any {
			if cond {
				return yes
			}
			return no
		},

		// --- quoting / escaping ---
		"quote":      strconv.Quote,
		"squote":     squote,
		"shellQuote": shellQuote,
		"jsonEscape": jsonEscape,

		// --- encoding ---
		"json":       jsonMarshal,
		"jsonIndent": jsonMarshalIndent,
		"toYAML":     toYAML,
		"b64encode":  func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"b64decode":  b64decode,

		// --- date / time ---
		"now":     func() time.Time { return now },
		"date":    func(layout string, t time.Time) string { return t.Format(layout) },
		"dateISO": func(t time.Time) string { return t.Format("2006-01-02") },

		// --- random / uuid ---
		"uuid":         uuidV4,
		"randAlphaNum": randAlphaNum,

		// --- lists / dicts ---
		"list":   func(items ...any) []any { return items },
		"first":  first,
		"last":   last,
		"slice":  sliceFn,
		"dict":   dict,
		"get":    getFn,
		"hasKey": hasKey,

		// --- folio-specific ---
		"licenseHeader": licenseHeader,
		"gomodPath":     func() string { return computedString(ctx, "module_path") },
		"gitUser":       gitCache.lookup,
		"spdxId":        spdxID,
	}
}

// titleCase is a Unicode-safe replacement for the deprecated strings.Title.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(prev) {
			prev = r
			return unicode.ToTitle(r)
		}
		prev = r
		return unicode.ToLower(r)
	}, s)
}

// joinAny joins any slice (typically []any from the list helper) with sep.
// Each element is rendered with fmt.Sprint so callers don't need to know the
// concrete element type — common ergonomic for template authors mixing
// strings and other primitives.
func joinAny(sep string, items any) string {
	switch v := items.(type) {
	case []string:
		return strings.Join(v, sep)
	case []any:
		parts := make([]string, len(v))
		for i, it := range v {
			parts[i] = fmt.Sprint(it)
		}
		return strings.Join(parts, sep)
	default:
		return fmt.Sprint(items)
	}
}

func indent(spaces int, s string) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// kebabCase: "MyPackageName" / "my_package_name" / "my-package-name" → "my-package-name".
func kebabCase(s string) string { return splitWords(s, "-", false) }

// snakeCase: ... → "my_package_name".
func snakeCase(s string) string { return splitWords(s, "_", false) }

// camelCase: ... → "myPackageName".
func camelCase(s string) string { return camelOrPascal(s, false) }

// pascalCase: ... → "MyPackageName".
func pascalCase(s string) string { return camelOrPascal(s, true) }

// splitWords tokenises an identifier on word boundaries (camel humps,
// hyphens, underscores, whitespace) and rejoins with sep. Optionally
// uppercases tokens.
func splitWords(s, sep string, upper bool) string {
	tokens := tokenise(s)
	for i, t := range tokens {
		if upper {
			tokens[i] = strings.ToUpper(t)
		} else {
			tokens[i] = strings.ToLower(t)
		}
	}
	return strings.Join(tokens, sep)
}

func camelOrPascal(s string, pascal bool) string {
	tokens := tokenise(s)
	var sb strings.Builder
	for i, t := range tokens {
		t = strings.ToLower(t)
		if i == 0 && !pascal {
			sb.WriteString(t)
			continue
		}
		if t == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(t[:1]))
		if len(t) > 1 {
			sb.WriteString(t[1:])
		}
	}
	return sb.String()
}

func tokenise(s string) []string {
	if s == "" {
		return nil
	}
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == '-' || r == '_' || unicode.IsSpace(r):
			flush()
		case unicode.IsUpper(r):
			// Hump boundary: lowercase→upper or runup of uppers ending in lower.
			if cur.Len() > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) {
					flush()
				} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					// e.g. URLPath → URL, Path
					flush()
				}
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func defaultFn(def, val any) any {
	if val == nil {
		return def
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return def
	}
	return val
}

func coalesce(values ...any) any {
	for _, v := range values {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		return v
	}
	return nil
}

// squote wraps s in single quotes. Embedded single quotes are doubled per
// POSIX-shell convention so the result is safe inside single-quoted shell
// contexts.
func squote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuote is the POSIX-safe single-quote wrapper. Identical to squote,
// kept as a separate name for clarity at template call sites.
func shellQuote(s string) string { return squote(s) }

// jsonEscape returns the JSON-escaped form of s without the outer quotes,
// suitable for embedding inside a JSON string literal.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		return string(b[1 : len(b)-1])
	}
	return string(b)
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func jsonMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func toYAML(v any) (string, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

func b64decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// uuidV4 returns a freshly-generated RFC 4122 v4 UUID. Non-deterministic;
// templates using this helper produce different output per render.
func uuidV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // v4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

const alphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randAlphaNum(n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphaNum[int(buf[i])%len(alphaNum)]
	}
	return string(buf), nil
}

func first(seq any) any {
	switch v := seq.(type) {
	case []any:
		if len(v) == 0 {
			return nil
		}
		return v[0]
	case []string:
		if len(v) == 0 {
			return nil
		}
		return v[0]
	}
	return nil
}

func last(seq any) any {
	switch v := seq.(type) {
	case []any:
		if len(v) == 0 {
			return nil
		}
		return v[len(v)-1]
	case []string:
		if len(v) == 0 {
			return nil
		}
		return v[len(v)-1]
	}
	return nil
}

func sliceFn(seq any, start, end int) any {
	switch v := seq.(type) {
	case []any:
		if start < 0 {
			start = 0
		}
		if end > len(v) {
			end = len(v)
		}
		if start > end {
			return []any{}
		}
		return v[start:end]
	case []string:
		if start < 0 {
			start = 0
		}
		if end > len(v) {
			end = len(v)
		}
		if start > end {
			return []string{}
		}
		return v[start:end]
	}
	return nil
}

func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict requires an even number of arguments")
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key at position %d is not a string: %v", i, pairs[i])
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}

func getFn(m any, key string) any {
	switch v := m.(type) {
	case map[string]any:
		return v[key]
	case map[any]any:
		return v[key]
	}
	return nil
}

func hasKey(m any, key string) bool {
	switch v := m.(type) {
	case map[string]any:
		_, ok := v[key]
		return ok
	case map[any]any:
		_, ok := v[key]
		return ok
	}
	return false
}

// licenseHeader returns a license comment block. v0 supports only MIT —
// any other SPDX value yields a render error suggesting raw template text.
func licenseHeader(license string) (string, error) {
	switch license {
	case "MIT", "":
		return mitHeader, nil
	default:
		return "", fmt.Errorf("licenseHeader: %q is not supported in v0 (only MIT); embed the header as raw template text or wait for v0.x", license)
	}
}

const mitHeader = `// Licensed under the MIT License. See LICENSE in the project root for license information.`

// spdxID normalises common license-name forms to their SPDX identifier.
// "MIT License" → "MIT", "Apache License 2.0" → "Apache-2.0", etc.
func spdxID(name string) string {
	n := strings.TrimSpace(name)
	switch strings.ToLower(n) {
	case "mit", "mit license":
		return "MIT"
	case "apache", "apache 2.0", "apache license 2.0", "apache-2.0":
		return "Apache-2.0"
	case "bsd-3-clause", "bsd 3-clause", "bsd-3":
		return "BSD-3-Clause"
	case "bsd-2-clause", "bsd 2-clause", "bsd-2":
		return "BSD-2-Clause"
	case "isc":
		return "ISC"
	}
	return n
}

func computedString(ctx Context, key string) string {
	if ctx.Computed == nil {
		return ""
	}
	if v, ok := ctx.Computed[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// gitUserCache reads `git config user.name` / `git config user.email` once
// per render and serves the cached pair to every gitUser call.
type gitUserCache struct {
	once  sync.Once
	name  string
	email string
	err   error
}

func (c *gitUserCache) lookup() (map[string]string, error) {
	c.once.Do(c.do)
	if c.err != nil {
		return nil, c.err
	}
	return map[string]string{"name": c.name, "email": c.email}, nil
}

func (c *gitUserCache) do() {
	nameOut, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		c.err = fmt.Errorf("gitUser: git config user.name failed: %w (hint: pass --input git_user_name=... and --input git_user_email=... instead of relying on gitUser)", err)
		return
	}
	emailOut, err := exec.Command("git", "config", "user.email").Output()
	if err != nil {
		c.err = fmt.Errorf("gitUser: git config user.email failed: %w (hint: pass --input git_user_name=... and --input git_user_email=... instead of relying on gitUser)", err)
		return
	}
	c.name = strings.TrimSpace(string(nameOut))
	c.email = strings.TrimSpace(string(emailOut))
	if c.name == "" || c.email == "" {
		c.err = errors.New("gitUser: git user.name/user.email is empty (hint: pass --input git_user_name=... and --input git_user_email=... instead of relying on gitUser)")
	}
}
