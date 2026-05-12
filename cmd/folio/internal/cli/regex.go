package cli

import "regexp"

// regexpCompile is exported as a var so tests can swap it for a stub that
// doesn't actually compile (useful when the test would otherwise need to
// validate a real regex string).
var regexpCompile = regexp.Compile
