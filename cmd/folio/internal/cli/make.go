package cli

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// makeCmd implements `folio make <preset> <name>` — a name-first
// ergonomic on top of `folio new`. The verb derives the target
// directory from <name> and, for preset ids that follow the
// `<host>-plugin` convention (e.g. `nanite-plugin`), auto-supplies the
// `plugin_name=<name>` input so the user doesn't have to repeat the
// name. All other flags pass through to runGenerate verbatim.
//
// Examples:
//
//	folio make nanite-plugin giphy
//	  → target: ./nanite-plugin-giphy, --input plugin_name=giphy
//
//	folio make go-package mylib
//	  → target: ./mylib
//
// `folio new` remains available for power users who want to specify
// the target directory explicitly.
func makeCmd(bundledFS fs.FS, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "make <preset-id> <name>",
		Short: "Scaffold a project by name (derives target dir from preset + name)",
		Long: `make is a name-first wrapper around 'new'.

The target directory is derived from <name>:
  - For presets whose id ends in '-plugin' (e.g. nanite-plugin), the
    target is ./<preset-id>-<name> and --input plugin_name=<name> is
    auto-supplied.
  - For any other preset, the target is ./<name>.

All other 'new' flags (--input, --inputs-file, --non-interactive, etc.)
are accepted and forwarded. An explicit --input plugin_name=... still
wins over the auto-supplied value.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			presetID := args[0]
			name := args[1]
			targetDir, autoInputs, err := deriveTargetAndInputs(presetID, name)
			if err != nil {
				return err
			}
			if err := prependInputs(cmd.Flags(), autoInputs); err != nil {
				return err
			}
			return runGenerate(cmd, []string{presetID, targetDir}, bundledFS, version, false)
		},
	}
	addGenerateFlags(cmd)
	cmd.Flags().Bool("plan", false, "Plan only — show what would be rendered and exit")
	cmd.Flags().Bool("yes", false, "Skip the summary-confirmation prompt")
	return cmd
}

// deriveTargetAndInputs computes the target directory and any auto-
// supplied --input pairs for `folio make`. Split out so tests can
// exercise the derivation without spinning up the full Cobra tree.
func deriveTargetAndInputs(presetID, name string) (target string, autoInputs []string, err error) {
	if name == "" {
		return "", nil, fmt.Errorf("name argument is required")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", nil, fmt.Errorf("name %q must not contain path separators", name)
	}
	if isPluginPreset(presetID) {
		return "./" + presetID + "-" + name, []string{"plugin_name=" + name}, nil
	}
	return "./" + name, nil, nil
}

// isPluginPreset returns true when presetID follows the `<host>-plugin`
// convention. Any preset whose id ends in "-plugin" gets the auto
// plugin_name injection — matches the preset.yaml authoring
// convention used by nanite-plugin and future host-plugin presets.
func isPluginPreset(presetID string) bool {
	return strings.HasSuffix(presetID, "-plugin")
}

// prependInputs inserts pairs at the front of the --input StringArray
// flag. parseFlagPairs in inputs.go takes the LAST value on key
// collision, so prepending makes any user-supplied --input override
// the auto-injected default.
func prependInputs(flags *pflag.FlagSet, pairs []string) error {
	if len(pairs) == 0 {
		return nil
	}
	f := flags.Lookup("input")
	if f == nil {
		return fmt.Errorf("internal: --input flag not registered")
	}
	sv, ok := f.Value.(pflag.SliceValue)
	if !ok {
		return fmt.Errorf("internal: --input flag is not a slice value (%T)", f.Value)
	}
	combined := append([]string(nil), pairs...)
	combined = append(combined, sv.GetSlice()...)
	return sv.Replace(combined)
}
