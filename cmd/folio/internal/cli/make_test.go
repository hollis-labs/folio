package cli

import (
	"testing"
)

func TestDeriveTargetAndInputs(t *testing.T) {
	tests := []struct {
		name          string
		presetID      string
		nameArg       string
		wantTarget    string
		wantInputs    []string
		wantErrSubstr string
	}{
		{
			name:       "plugin preset injects plugin_name and prefixes dir",
			presetID:   "nanite-plugin",
			nameArg:    "giphy",
			wantTarget: "./nanite-plugin-giphy",
			wantInputs: []string{"plugin_name=giphy"},
		},
		{
			name:       "non-plugin preset uses bare name as dir",
			presetID:   "go-package",
			nameArg:    "mylib",
			wantTarget: "./mylib",
			wantInputs: nil,
		},
		{
			name:       "hyphenated plugin name preserved verbatim",
			presetID:   "nanite-plugin",
			nameArg:    "agent-mux",
			wantTarget: "./nanite-plugin-agent-mux",
			wantInputs: []string{"plugin_name=agent-mux"},
		},
		{
			name:          "empty name is rejected",
			presetID:      "nanite-plugin",
			nameArg:       "",
			wantErrSubstr: "name argument is required",
		},
		{
			name:          "name containing slash is rejected",
			presetID:      "nanite-plugin",
			nameArg:       "evil/name",
			wantErrSubstr: "path separators",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target, autoInputs, err := deriveTargetAndInputs(tc.presetID, tc.nameArg)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSubstr)
				}
				if !contains(err.Error(), tc.wantErrSubstr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tc.wantTarget {
				t.Errorf("target = %q, want %q", target, tc.wantTarget)
			}
			if !slicesEqual(autoInputs, tc.wantInputs) {
				t.Errorf("autoInputs = %v, want %v", autoInputs, tc.wantInputs)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
