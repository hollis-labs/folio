package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	gh "github.com/hollis-labs/folio/internal/github"
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
	addGitHubFlags(cmd)
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

// addGitHubFlags registers the GitHub-automation flag set. Only `folio new`
// wires these — `folio plan` is render-only and never publishes.
func addGitHubFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("create-github-repo", false, "After render, create a GitHub repo and push the initial commit (requires gh CLI)")
	cmd.Flags().String("github-owner", "", "GitHub owner (org or user). Defaults to inputs.github_owner when present.")
	cmd.Flags().String("github-repo", "", "GitHub repo name. Defaults to basename(<target-dir>).")
	cmd.Flags().String("github-visibility", "private", "GitHub repo visibility: public|private|internal")
	cmd.Flags().String("github-description", "", "GitHub repo description. Defaults to inputs.description when present.")
	cmd.Flags().String("github-branch", "main", "Default branch name")
	cmd.Flags().Bool("github-no-push", false, "Create the repo and remote, but do not push the initial commit")
}

// githubOptions is the resolved per-invocation snapshot pulled from CLI
// flags + the resolved inputs map. Empty when --create-github-repo was
// not set.
type githubOptions struct {
	Enabled     bool
	Owner       string
	Repo        string
	Visibility  string
	Description string
	Branch      string
	Push        bool
}

func resolveGitHubOptions(cmd *cobra.Command, inputs map[string]any, targetDir string) (githubOptions, error) {
	if cmd.Flags().Lookup("create-github-repo") == nil {
		return githubOptions{}, nil
	}
	enabled, _ := cmd.Flags().GetBool("create-github-repo")
	if !enabled {
		return githubOptions{}, nil
	}
	owner, _ := cmd.Flags().GetString("github-owner")
	if owner == "" {
		if v, ok := inputs["github_owner"].(string); ok {
			owner = v
		}
	}
	if owner == "" {
		return githubOptions{}, fmt.Errorf("--github-owner not supplied and inputs.github_owner is empty")
	}
	repo, _ := cmd.Flags().GetString("github-repo")
	if repo == "" {
		repo = filepath.Base(targetDir)
	}
	visibility, _ := cmd.Flags().GetString("github-visibility")
	description, _ := cmd.Flags().GetString("github-description")
	if description == "" {
		if v, ok := inputs["description"].(string); ok {
			description = v
		}
	}
	branch, _ := cmd.Flags().GetString("github-branch")
	noPush, _ := cmd.Flags().GetBool("github-no-push")
	return githubOptions{
		Enabled:     true,
		Owner:       owner,
		Repo:        repo,
		Visibility:  visibility,
		Description: description,
		Branch:      branch,
		Push:        !noPush,
	}, nil
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
		res, planErr := svc.Plan(opts)
		if planErr != nil {
			return planErr
		}
		printPlan(cmd, res, loaded.Preset.ID, targetDir, verbose)
		return nil
	}

	if !yes && !nonInteractive {
		// Run Plan first so the summary reflects exactly what New would write.
		plan, planErr := svc.Plan(opts)
		if planErr != nil {
			return planErr
		}
		printSummary(cmd, plan, loaded.Preset.ID, loaded.Preset.Version, targetDir)
		if !confirmYes(cmd) {
			fprintln(cmd, "canceled")
			return nil
		}
	}

	ghOpts, err := resolveGitHubOptions(cmd, inputs, targetDir)
	if err != nil {
		return err
	}
	if ghOpts.Enabled {
		if preErr := gh.Preflight(cmd.Context(), gh.ExecRunner{}); preErr != nil {
			return preErr
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

	if ghOpts.Enabled {
		if err := publishToGitHub(cmd, svc, targetDir, ghOpts, quiet); err != nil {
			return err
		}
	}

	return nil
}

// publishToGitHub runs Service.PublishToGitHub against the just-rendered
// project. On failure, it prints a clear retry hint (the local tree is
// already on disk) and returns the error so the CLI exits non-zero.
func publishToGitHub(cmd *cobra.Command, svc *service.Service, targetDir string, opts githubOptions, quiet bool) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := svc.PublishToGitHub(ctx, service.PublishOptions{
		TargetDir:   targetDir,
		Owner:       opts.Owner,
		Repo:        opts.Repo,
		Visibility:  opts.Visibility,
		Description: opts.Description,
		Branch:      opts.Branch,
		Push:        opts.Push,
	})
	if err != nil {
		var sErr *service.Error
		errors.As(err, &sErr)
		printPublishRetryHint(cmd, targetDir, opts, sErr)
		return err
	}
	if !quiet {
		if res.Pushed {
			fmt.Fprintf(cmd.OutOrStdout(), "folio: published to %s (branch %s)\n", res.URL, res.Branch)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "folio: created %s (no push — run `git push -u origin %s` when ready)\n", res.URL, res.Branch)
		}
	}
	return nil
}

// printPublishRetryHint emits an actionable retry command. The render is
// preserved on disk; the user just has to re-run the gh step from the
// target dir. Keeps stderr/stdout disciplined: hint goes to stderr so it
// doesn't get mixed into normal stdout consumers.
func printPublishRetryHint(cmd *cobra.Command, targetDir string, opts githubOptions, sErr *service.Error) {
	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "\nfolio: render succeeded at %s, but GitHub publish failed.\n", targetDir)
	if sErr != nil && sErr.Code != "" {
		fmt.Fprintf(w, "folio: code=%s\n", sErr.Code)
	}
	pushFlag := ""
	if opts.Push {
		pushFlag = " --push"
	}
	descFlag := ""
	if opts.Description != "" {
		descFlag = fmt.Sprintf(" --description %q", opts.Description)
	}
	fmt.Fprintf(w, "folio: retry manually:\n  cd %s\n  gh repo create %s/%s --%s --source=. --remote=origin%s%s\n",
		targetDir, opts.Owner, opts.Repo, opts.Visibility, descFlag, pushFlag)
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

// confirmYes prompts for the post-summary confirmation. Returns true when
// the user accepts (default on Enter), false when they decline. Non-TTY
// invocations always return true — they reach this path only when the
// caller passed --yes was false but stdin/stdout aren't terminals.
func confirmYes(cmd *cobra.Command) bool {
	if !isTTY(cmd.OutOrStdout()) {
		return true
	}
	fmt.Fprint(cmd.OutOrStdout(), "\nProceed? [Y/n] ")
	var resp string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &resp); err != nil {
		// EOF on empty Enter is fine — treat as yes.
		if err.Error() == "unexpected newline" || err.Error() == "EOF" {
			return true
		}
		return false
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	return resp == "" || resp == "y" || resp == "yes"
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
