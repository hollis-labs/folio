package compose_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/folio/internal/compose"
)

func TestParseConstraint(t *testing.T) {
	cases := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"empty", "", true},
		{"star", "*", false},
		{"exact", "1.0.0", false},
		{"exact_partial", "1.0", false},
		{"gte", ">=1.0", false},
		{"gte_full", ">=1.0.0", false},
		{"lte", "<=2.0.0", false},
		{"gt", ">1.0.0", false},
		{"lt", "<2.0.0", false},
		{"tilde", "~1.2.3", false},
		{"caret", "^1.2.3", false},
		{"caret_zero_major", "^0.2.3", false},
		{"and", ">=1.0,<2.0", false},
		{"and_spaced", ">=1.0, <2.0", false},
		{"or", "^1.0 || ^2.0", false},
		{"or_and_mixed", ">=1.0,<2.0 || ^3.0", false},
		{"garbage", "not-a-version", true},
		{"gte_garbage", ">=garbage", true},
		{"empty_or_clause", "^1.0 ||", true},
		{"empty_and_term", ">=1.0,,<2.0", true},
		{"unknown_op", "@1.0.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compose.ParseConstraint(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseConstraint(%q): expected error, got nil", tc.expr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConstraint(%q): unexpected error: %v", tc.expr, err)
			}
		})
	}
}

func TestConstraintMatches(t *testing.T) {
	cases := []struct {
		expr    string
		version string
		want    bool
	}{
		// Star
		{"*", "1.0.0", true},
		{"*", "99.99.99-rc.1", true},

		// Exact
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"1.0.0", "1.0", true}, // 1.0 canonicalizes to 1.0.0

		// >=
		{">=1.0", "1.0.0", true},
		{">=1.0", "0.9.0", false},
		{">=1.0", "2.0.0", true},

		// <
		{"<2.0", "1.9.9", true},
		{"<2.0", "2.0.0", false},

		// AND (comma)
		{">=1.0,<2.0", "1.0.0", true},
		{">=1.0,<2.0", "1.5.0", true},
		{">=1.0,<2.0", "2.0.0", false},
		{">=1.0,<2.0", "0.9.0", false},

		// Caret (left-most non-zero)
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.5.0", true},
		{"^1.2.3", "1.2.2", false},
		{"^1.2.3", "2.0.0", false},
		{"^0.2.3", "0.2.3", true},
		{"^0.2.3", "0.2.99", true},
		{"^0.2.3", "0.3.0", false},
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},

		// Tilde (patch updates within minor)
		{"~1.2.3", "1.2.3", true},
		{"~1.2.3", "1.2.99", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "1.2.2", false},

		// OR
		{"^1.0 || ^2.0", "1.5.0", true},
		{"^1.0 || ^2.0", "2.3.0", true},
		{"^1.0 || ^2.0", "3.0.0", false},

		// Invalid version → never matches
		{">=1.0", "garbage", false},
	}
	for _, tc := range cases {
		t.Run(tc.expr+"_vs_"+tc.version, func(t *testing.T) {
			c, err := compose.ParseConstraint(tc.expr)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.expr, err)
			}
			if got := c.Matches(tc.version); got != tc.want {
				t.Errorf("Matches(%q against %q) = %v, want %v", tc.version, tc.expr, got, tc.want)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name      string
		expr      string
		available []string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "pick_highest_under_and",
			expr:      ">=1.0,<2.0",
			available: []string{"1.0.0", "1.5.0", "2.0.0"},
			want:      "1.5.0",
		},
		{
			name:      "bundled_single_version_matches",
			expr:      ">=1.0,<2.0",
			available: []string{"1.0.0"},
			want:      "1.0.0",
		},
		{
			name:      "no_match_lists_available",
			expr:      ">=2.0",
			available: []string{"1.0.0", "1.5.0"},
			wantErr:   true,
			errSubstr: "no version satisfies",
		},
		{
			name:      "empty_available",
			expr:      ">=1.0",
			available: []string{},
			wantErr:   true,
			errSubstr: "no versions",
		},
		{
			name:      "star_picks_highest",
			expr:      "*",
			available: []string{"1.0.0", "2.5.0", "1.99.0"},
			want:      "2.5.0",
		},
		{
			name:      "or_clause",
			expr:      "^1.0 || ^2.0",
			available: []string{"1.0.0", "1.5.0", "2.0.0", "2.3.0", "3.0.0"},
			want:      "2.3.0",
		},
		{
			name:      "invalid_versions_skipped",
			expr:      ">=1.0",
			available: []string{"garbage", "1.5.0", "1.0.0"},
			want:      "1.5.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := compose.ParseConstraint(tc.expr)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.expr, err)
			}
			got, err := compose.ResolveVersion(c, tc.available)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveVersion: expected error, got %q", got)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveVersion: unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	if !compose.MatchAny().Matches("1.0.0") {
		t.Error("MatchAny should match 1.0.0")
	}
	if !compose.MatchAny().Matches("99.99.99") {
		t.Error("MatchAny should match 99.99.99")
	}
}
