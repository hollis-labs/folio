package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/folio/service"
)

func presetCmd(bundledFS fs.FS, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "Inspect and validate presets",
	}
	cmd.AddCommand(presetValidateCmd(bundledFS, version))
	for _, name := range []string{"list", "show"} {
		cmd.AddCommand(stubCmd(name))
	}
	return cmd
}

func presetValidateCmd(bundledFS fs.FS, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <preset-dir>",
		Short: "Validate a preset.yaml against the v0 schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			svc := service.New(service.Options{
				BundledFS:    bundledFS,
				BundledRoot:  "presets",
				FolioVersion: version,
			})
			res, p, err := svc.ValidatePreset(filepath.Join(dir, "preset.yaml"))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "folio: validating %s\n\n", dir)
			if p != nil {
				fmt.Fprintf(out, "  id:      %s\n", p.ID)
				fmt.Fprintf(out, "  version: %s\n", p.Version)
				fmt.Fprintf(out, "  inputs:  %d declared\n", len(p.Inputs))
				fmt.Fprintf(out, "  files.source: %s\n\n", p.Files.Source)
			}
			for _, w := range res.Warnings {
				fmt.Fprintf(out, "  ⚠ %s\n", w.Message)
			}
			if !res.OK() {
				for _, e := range res.Errors {
					fmt.Fprintf(out, "  ✗ %s\n", e.Error())
				}
				return &service.Error{Code: service.ErrPresetInvalid, Message: fmt.Sprintf("%d validation error(s)", len(res.Errors))}
			}
			fmt.Fprintln(out, "  ✓ PASS")
			return nil
		},
	}
}
