package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/folio/service"
)

func inspectCmd(bundledFS fs.FS, version string) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "inspect <target-dir>",
		Short: "Report how a project has drifted from its preset (read-only)",
		Long: `inspect re-renders the preset recorded in <target-dir>/.folio.yaml with the
inputs recorded there and compares it, file by file, with the project on disk:

  unchanged         neither the file nor the preset changed
  preset_updated    the preset now renders something different; the file is as generated
  locally_modified  the file was edited; the preset renders what it did
  conflict          both changed
  added_upstream    the preset now renders a file the project does not track
  removed_upstream  the preset no longer renders a tracked file
  missing_locally   a tracked file is gone from the project

inspect never writes and exits 0 whenever it can produce a report. A file marked
with * renders different bytes on every run, so only local edits are reported
for it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := service.New(service.Options{
				BundledFS:    bundledFS,
				BundledRoot:  "presets",
				FolioVersion: version,
			})
			res, err := svc.Inspect(service.InspectOptions{TargetDir: args[0]})
			if err != nil {
				return err
			}
			if asJSON {
				return writeInspectJSON(cmd, args[0], res)
			}
			writeInspectTable(cmd, args[0], res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the report as JSON")
	return cmd
}

type inspectJSON struct {
	Target   string            `json:"target"`
	Preset   string            `json:"preset"`
	Version  string            `json:"version"`
	Files    []inspectFileJSON `json:"files"`
	Warnings []string          `json:"warnings"`
}

type inspectFileJSON struct {
	Path     string `json:"path"`
	Preset   string `json:"preset"`
	Status   string `json:"status"`
	Unstable bool   `json:"unstable,omitempty"`
}

func writeInspectJSON(cmd *cobra.Command, target string, res service.InspectResult) error {
	out := inspectJSON{
		Target:   target,
		Preset:   res.RootPreset,
		Version:  res.RootVersion,
		Files:    make([]inspectFileJSON, 0, len(res.Files)),
		Warnings: res.Warnings,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	for _, f := range res.Files {
		out.Files = append(out.Files, inspectFileJSON{Path: f.Path, Preset: f.Preset, Status: string(f.Status), Unstable: f.Unstable})
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeInspectTable(cmd *cobra.Command, target string, res service.InspectResult) {
	w := cmd.OutOrStdout()
	name := target
	if abs, err := filepath.Abs(target); err == nil {
		name = filepath.Base(abs)
	}
	fmt.Fprintf(w, "folio inspect: %s (preset %s@%s)\n\n", name, res.RootPreset, res.RootVersion)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  PATH\tPRESET\tSTATUS")
	for _, f := range res.Files {
		status := string(f.Status)
		if f.Unstable {
			status += "*"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Path, f.Preset, status)
	}
	_ = tw.Flush()
	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "\nwarning: %s", warn)
	}
	if len(res.Warnings) > 0 {
		fmt.Fprintln(w)
	}
}
