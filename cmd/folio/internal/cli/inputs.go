package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"gopkg.in/yaml.v3"

	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/service"
)

// inputResolver materializes the final inputs map for service.New/Plan from
// flag values + --inputs-file + environment variables, following the
// resolution order in cli-prompt-flow-v0.md §2.
type inputResolver struct {
	flagPairs []string
	inputFile string
}

func newInputResolver(cmd *cobra.Command) (*inputResolver, error) {
	pairs, err := cmd.Flags().GetStringArray("input")
	if err != nil {
		return nil, err
	}
	file, err := cmd.Flags().GetString("inputs-file")
	if err != nil {
		return nil, err
	}
	return &inputResolver{flagPairs: pairs, inputFile: file}, nil
}

// resolve walks the preset's declared inputs and produces a map of typed
// values. Order: --input flags > --inputs-file > FOLIO_INPUT_<UPPER_NAME>
// env vars > preset default > prompt (when interactive) > error.
func (r *inputResolver) resolve(p *preset.Preset, interactive bool, prompter Prompter) (map[string]any, error) {
	out := map[string]any{}

	flagVals, err := parseFlagPairs(r.flagPairs)
	if err != nil {
		return nil, &service.Error{Code: service.ErrInputInvalid, Message: err.Error()}
	}

	fileVals := map[string]any{}
	if r.inputFile != "" {
		fileVals, err = readInputsFile(r.inputFile)
		if err != nil {
			return nil, &service.Error{Code: service.ErrInputInvalid, Message: err.Error()}
		}
	}

	var missing []string

	for _, in := range p.Inputs {
		if v, ok := flagVals[in.Name]; ok {
			out[in.Name] = coerceForCLI(in, v)
			continue
		}
		if v, ok := fileVals[in.Name]; ok {
			out[in.Name] = v
			continue
		}
		if v, ok := envVal(in.Name); ok {
			out[in.Name] = coerceForCLI(in, v)
			continue
		}
		if in.Default != nil {
			// Service layer will apply the default; we don't pre-fill so
			// "ignored: not declared by preset" warnings aren't triggered.
			continue
		}
		if interactive {
			val, err := prompter.Prompt(in)
			if err != nil {
				return nil, err
			}
			if val != nil {
				out[in.Name] = val
				continue
			}
		}
		if in.Required {
			missing = append(missing, in.Name)
		}
	}

	if len(missing) > 0 {
		return nil, &service.Error{Code: service.ErrInputMissing, Message: fmt.Sprintf("missing required inputs: %s\n  hint: pass --input %s=... (or supply via --inputs-file)", strings.Join(missing, ", "), missing[0])}
	}
	return out, nil
}

// parseFlagPairs splits "key=value" strings into a name → string-value map.
// Repeated keys take the last value (CLI convention).
func parseFlagPairs(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		idx := strings.IndexByte(p, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --input %q (expected key=value)", p)
		}
		out[p[:idx]] = p[idx+1:]
	}
	return out, nil
}

// readInputsFile decodes a YAML or JSON inputs file. Format detected from
// extension; .yaml/.yml default to YAML, .json is JSON, anything else
// tries YAML first.
func readInputsFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read inputs file %s: %w", path, err)
	}
	out := map[string]any{}
	switch ext := strings.ToLower(extOf(path)); ext {
	case ".json":
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("parse inputs file %s as JSON: %w", path, err)
		}
	default:
		if err := yaml.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("parse inputs file %s as YAML: %w", path, err)
		}
	}
	return out, nil
}

func extOf(p string) string {
	idx := strings.LastIndexByte(p, '.')
	if idx < 0 {
		return ""
	}
	return p[idx:]
}

// envVal looks up FOLIO_INPUT_<UPPER> with hyphens normalised to
// underscores per cli-prompt-flow-v0.md §2.
func envVal(name string) (string, bool) {
	key := "FOLIO_INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// coerceForCLI parses a string flag value into the declared input type.
// Failures here surface to the user as ErrInputInvalid via the service
// layer's coercion (which we still rely on for the canonical check); this
// function only handles the obvious string → typed-value conversion for
// bool / number flags.
func coerceForCLI(in preset.Input, raw string) any {
	switch in.Type {
	case "bool":
		if b, err := strconv.ParseBool(raw); err == nil {
			return b
		}
		return raw // service will surface the error
	case "number":
		if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
		return raw
	case "list[string]":
		if raw == "" {
			return []any{}
		}
		parts := strings.Split(raw, ",")
		out := make([]any, len(parts))
		for i, p := range parts {
			out[i] = strings.TrimSpace(p)
		}
		return out
	default:
		return raw
	}
}
