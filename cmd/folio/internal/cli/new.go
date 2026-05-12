package cli

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/folio/service"
)

func newCmd(bundledFS fs.FS, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <preset-id> <target-dir>",
		Short: "Render a preset into a new project directory",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd, args, bundledFS, version, false)
		},
	}
	addGenerateFlags(cmd)
	cmd.Flags().Bool("plan", false, "Plan only — show what would be rendered and exit")
	cmd.Flags().Bool("yes", false, "Skip the summary-confirmation prompt")
	return cmd
}

func planCmd(bundledFS fs.FS, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan <preset-id> <target-dir>",
		Short: "Preview what `folio new` would render — no writes",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd, args, bundledFS, version, true)
		},
	}
	addGenerateFlags(cmd)
	return cmd
}

func addGenerateFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("input", nil, "Supply an input value (repeatable): --input key=value")
	cmd.Flags().String("inputs-file", "", "Path to a YAML or JSON file of inputs")
	cmd.Flags().Bool("non-interactive", false, "Fail on missing required inputs instead of prompting")
	cmd.Flags().BoolP("quiet", "q", false, "Suppress informational output")
	cmd.Flags().BoolP("verbose", "v", false, "Print verbose progress")
}

func runGenerate(cmd *cobra.Command, args []string, bundledFS fs.FS, version string, planOnly bool) error {
	presetID := args[0]
	targetDir := args[1]

	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	quiet, _ := cmd.Flags().GetBool("quiet")
	verbose, _ := cmd.Flags().GetBool("verbose")
	yes := false
	if cmd.Flags().Lookup("yes") != nil {
		yes, _ = cmd.Flags().GetBool("yes")
	}
	if cmd.Flags().Lookup("plan") != nil {
		v, _ := cmd.Flags().GetBool("plan")
		planOnly = planOnly || v
	}

	if !isTTY(cmd.OutOrStdout()) {
		nonInteractive = true
	}

	svc := service.New(service.Options{
		BundledFS:    bundledFS,
		BundledRoot:  "presets",
		UserDir:      "",
		FolioVersion: version,
	})

	loaded, err := svc.LoadPreset(presetID)
	if err != nil {
		return err
	}

	resolver, err := newInputResolver(cmd)
	if err != nil {
		return err
	}

	var prompter Prompter = noopPrompter{}
	if !nonInteractive {
		prompter = huhPrompter{}
	}

	inputs, err := resolver.resolve(loaded.Preset, !nonInteractive, prompter)
	if err != nil {
		return err
	}

	opts := service.NewOptions{
		PresetID:  presetID,
		TargetDir: targetDir,
		Inputs:    inputs,
	}

	if planOnly {
		res, err := svc.Plan(opts)
		if err != nil {
			return err
		}
		printPlan(cmd, res, loaded.Preset.ID, targetDir, verbose)
		return nil
	}

	if !yes && !nonInteractive {
		// Run Plan first so the summary reflects exactly what New would write.
		plan, err := svc.Plan(opts)
		if err != nil {
			return err
		}
		printSummary(cmd, plan, loaded.Preset.ID, loaded.Preset.Version, targetDir)
		confirmed, err := confirmYes(cmd)
		if err != nil {
			return err
		}
		if !confirmed {
			fprintln(cmd, "cancelled")
			return nil
		}
	}

	res, err := svc.New(opts)
	if err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "folio: wrote %d files to %s\n", len(res.Files), targetDir)
		if verbose {
			for _, f := range res.Files {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s (%d bytes)\n", f.Path, f.Bytes)
			}
		}
		for _, w := range res.Warnings {
			fprintln(cmd, "warning: "+w)
		}
	}

	return nil
}

func printPlan(cmd *cobra.Command, res service.PlanResult, presetID, targetDir string, verbose bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "folio plan")
	fmt.Fprintf(out, "  Preset:  %s\n", presetID)
	fmt.Fprintf(out, "  Target:  %s\n\n", targetDir)

	if len(res.Inputs) > 0 {
		fmt.Fprintln(out, "  Inputs:")
		for _, k := range sortedKeys(res.Inputs) {
			fmt.Fprintf(out, "    %-20s %v\n", k, res.Inputs[k])
		}
		fmt.Fprintln(out)
	}
	if len(res.Computed) > 0 {
		fmt.Fprintln(out, "  Computed:")
		for _, k := range sortedKeys(res.Computed) {
			fmt.Fprintf(out, "    %-20s %v\n", k, res.Computed[k])
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Files (%d):\n", len(res.Files))
	for _, f := range res.Files {
		marker := "  "
		if f.IsTemplate {
			marker = "T "
		}
		fmt.Fprintf(out, "    %s%-40s %6d bytes\n", marker, f.Path, f.Bytes)
	}
	if verbose {
		fmt.Fprintln(out, "\n  Preview (first 2 KiB per file):")
		for _, f := range res.Files {
			fmt.Fprintf(out, "\n--- %s ---\n%s\n", f.Path, f.Preview)
		}
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+w)
	}
}

func printSummary(cmd *cobra.Command, res service.PlanResult, presetID, version, targetDir string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "folio new — summary")
	fmt.Fprintf(out, "  Preset:  %s@%s\n", presetID, version)
	fmt.Fprintf(out, "  Target:  %s\n", targetDir)

	if len(res.Inputs) > 0 {
		fmt.Fprintln(out, "  Inputs:")
		for _, k := range sortedKeys(res.Inputs) {
			fmt.Fprintf(out, "    %-20s %v\n", k, res.Inputs[k])
		}
	}
	if len(res.Computed) > 0 {
		fmt.Fprintln(out, "  Computed:")
		for _, k := range sortedKeys(res.Computed) {
			fmt.Fprintf(out, "    %-20s %v\n", k, res.Computed[k])
		}
	}
	fmt.Fprintf(out, "  Files to write: %d\n", len(res.Files))
}

func confirmYes(cmd *cobra.Command) (bool, error) {
	if !isTTY(cmd.OutOrStdout()) {
		return true, nil
	}
	fmt.Fprint(cmd.OutOrStdout(), "\nProceed? [Y/n] ")
	var resp string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &resp); err != nil {
		// EOF on empty Enter is fine — treat as yes.
		if err.Error() == "unexpected newline" || err.Error() == "EOF" {
			return true, nil
		}
		return false, nil
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	return resp == "" || resp == "y" || resp == "yes", nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isTTY reports whether w is a terminal. We accept anything that has a
// Fd() method returning a file descriptor matching a tty; for everything
// else assume non-interactive (CI, pipes, captured-output tests).
type fdWriter interface {
	Fd() uintptr
}

func isTTY(w any) bool {
	if w == nil {
		return false
	}
	fw, ok := w.(fdWriter)
	if !ok {
		return false
	}
	// Avoid importing golang.org/x/term — for our minimal needs, check
	// against os.Stdout.Fd() so test buffers (which lack Fd()) return false
	// while real terminals return true.
	if fw.Fd() == os.Stdout.Fd() {
		return termIsTTY(os.Stdout)
	}
	return false
}
