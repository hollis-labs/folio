package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/service"
)

// layerTag is appended to a layer-only preset's row in `preset list`.
const layerTag = "[layer]"

func presetListCmd(bundledFS fs.FS, version string) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bundled and user-dir presets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := service.New(service.Options{
				BundledFS:    bundledFS,
				BundledRoot:  "presets",
				FolioVersion: version,
			})
			list, warnings, err := svc.ListPresets()
			if err != nil {
				return err
			}
			for _, w := range warnings {
				fprintln(cmd, "folio: warning: "+w)
			}
			out := cmd.OutOrStdout()
			if asJSON {
				if list == nil {
					list = []service.PresetSummary{}
				}
				return writeJSON(out, list)
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tVERSION\tSOURCE\tDESCRIPTION")
			for _, p := range list {
				desc := p.Description
				if p.LayerOnly {
					desc = strings.TrimSpace(desc + "  " + layerTag)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, p.Version, p.Source, desc)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON")
	return cmd
}

// presetDetail is the JSON shape of `preset show --json`.
type presetDetail struct {
	ID          string          `json:"id"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	License     string          `json:"license"`
	Source      string          `json:"source"`
	LayerOnly   bool            `json:"layer_only"`
	Composes    []composeDetail `json:"composes"`
	FilesSource string          `json:"files_source"`
	Inputs      []inputDetail   `json:"inputs"`
}

type composeDetail struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

type inputDetail struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Values      []string `json:"values,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
}

func newPresetDetail(lp *service.LoadedPreset) presetDetail {
	p := lp.Preset
	d := presetDetail{
		ID:          p.ID,
		Version:     p.Version,
		Description: p.Description,
		Author:      p.Author,
		License:     p.License,
		Source:      lp.Source,
		LayerOnly:   p.LayerOnly,
		Composes:    []composeDetail{},
		FilesSource: p.Files.Source,
		Inputs:      []inputDetail{},
	}
	// Only the preset's own declared entries, as authored. Resolving the
	// transitive chain is internal/compose's job at render time.
	for _, c := range p.Composes {
		d.Composes = append(d.Composes, composeDetail{ID: c.ID, Version: c.Version})
	}
	for _, in := range p.Inputs {
		d.Inputs = append(d.Inputs, inputDetailFrom(in))
	}
	return d
}

func inputDetailFrom(in preset.Input) inputDetail {
	return inputDetail{
		Name: in.Name, Type: in.Type, Required: in.Required, Default: in.Default,
		Description: in.Description, Values: in.Values, Pattern: in.Pattern,
		Min: in.Min, Max: in.Max,
	}
}

func presetShowCmd(bundledFS fs.FS, version string) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <preset-id>",
		Short: "Show one preset's metadata, composes entries and inputs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := service.New(service.Options{
				BundledFS:    bundledFS,
				BundledRoot:  "presets",
				FolioVersion: version,
			})
			lp, err := svc.LoadPreset(args[0])
			if err != nil {
				return err
			}
			d := newPresetDetail(lp)
			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, d)
			}
			return writePresetDetail(out, d)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the preset as JSON")
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writePresetDetail(w io.Writer, d presetDetail) error {
	fmt.Fprintf(w, "folio: %s\n\n", d.ID)
	fmt.Fprintf(w, "  version:      %s\n", d.Version)
	fmt.Fprintf(w, "  description:  %s\n", d.Description)
	fmt.Fprintf(w, "  author:       %s\n", d.Author)
	fmt.Fprintf(w, "  source:       %s\n", d.Source)
	fmt.Fprintf(w, "  layer_only:   %t\n", d.LayerOnly)
	if len(d.Composes) == 0 {
		fmt.Fprintf(w, "  composes:     (none)\n")
	}
	for i, c := range d.Composes {
		label := "  composes:     "
		if i > 0 {
			label = "                "
		}
		entry := c.ID
		if c.Version != "" {
			entry += " (" + c.Version + ")"
		}
		fmt.Fprintf(w, "%s%s\n", label, entry)
	}
	fmt.Fprintf(w, "  files.source: %s\n\n", d.FilesSource)

	if len(d.Inputs) == 0 {
		fmt.Fprintln(w, "  inputs: (none)")
		return nil
	}
	fmt.Fprintln(w, "  inputs:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, in := range d.Inputs {
		fmt.Fprintf(tw, "    %s\t%s\t%s\n", in.Name, in.Type, inputNotes(in))
	}
	return tw.Flush()
}

func inputNotes(in inputDetail) string {
	var notes []string
	if in.Required {
		notes = append(notes, "required")
	}
	if in.Default != nil {
		notes = append(notes, fmt.Sprintf("default=%v", in.Default))
	}
	if len(in.Values) > 0 {
		notes = append(notes, "values=["+strings.Join(in.Values, " ")+"]")
	}
	if in.Pattern != "" {
		notes = append(notes, "pattern="+in.Pattern)
	}
	if in.Min != nil {
		notes = append(notes, fmt.Sprintf("min=%v", *in.Min))
	}
	if in.Max != nil {
		notes = append(notes, fmt.Sprintf("max=%v", *in.Max))
	}
	return strings.Join(notes, "  ")
}
