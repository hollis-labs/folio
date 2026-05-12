package cli

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/hollis-labs/folio/internal/preset"
)

// Prompter resolves missing inputs interactively. The real implementation
// drives charmbracelet/huh widgets; tests inject a stub returning canned
// values so they don't need a TTY.
type Prompter interface {
	Prompt(in preset.Input) (any, error)
}

// huhPrompter is the production Prompter. It maps each preset.Input.Type
// to a huh widget (Input / Select / Confirm) and gathers a single value.
type huhPrompter struct{}

// noopPrompter is the non-interactive fallback. Returning (nil, nil)
// signals "no value supplied" so the resolver moves on to the missing/
// required handling.
type noopPrompter struct{}

func (noopPrompter) Prompt(_ preset.Input) (any, error) { return nil, nil }

// Prompt runs one interactive widget for in and returns the typed value.
func (huhPrompter) Prompt(in preset.Input) (any, error) {
	title := promptTitle(in)
	desc := in.Description

	switch in.Type {
	case "bool":
		var v bool
		if d, ok := in.Default.(bool); ok {
			v = d
		}
		if err := huh.NewConfirm().
			Title(title).
			Description(desc).
			Value(&v).
			Run(); err != nil {
			return nil, mapHuhErr(err)
		}
		return v, nil

	case "enum":
		opts := make([]huh.Option[string], len(in.Values))
		for i, v := range in.Values {
			opts[i] = huh.NewOption(v, v)
		}
		var v string
		if d, ok := in.Default.(string); ok {
			v = d
		}
		if err := huh.NewSelect[string]().
			Title(title).
			Description(desc).
			Options(opts...).
			Value(&v).
			Run(); err != nil {
			return nil, mapHuhErr(err)
		}
		return v, nil

	default:
		var v string
		if d, ok := in.Default.(string); ok {
			v = d
		}
		input := huh.NewInput().
			Title(title).
			Description(desc).
			Value(&v)
		if in.Pattern != "" {
			pat := in.Pattern
			input = input.Validate(func(s string) error {
				return validatePattern(s, pat)
			})
		}
		if err := input.Run(); err != nil {
			return nil, mapHuhErr(err)
		}
		return v, nil
	}
}

func promptTitle(in preset.Input) string {
	t := titleCaseFromIdentifier(in.Name)
	if in.Required {
		return t + " *"
	}
	return t
}

func titleCaseFromIdentifier(s string) string {
	out := ""
	upper := true
	for _, r := range s {
		switch {
		case r == '_' || r == '-':
			out += " "
			upper = true
		case upper:
			out += string(toUpperRune(r))
			upper = false
		default:
			out += string(r)
		}
	}
	return out
}

func toUpperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

func mapHuhErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return &cancelledError{Err: err}
	}
	return fmt.Errorf("prompt: %w", err)
}

// validatePattern is the inline-validation hook used by huh for string
// inputs that declare a pattern. It is a thin wrapper around the same
// regex compile we run server-side; failure messages mention the pattern
// so users can self-correct.
func validatePattern(s, pattern string) error {
	re, err := regexpCompile(pattern)
	if err != nil {
		return fmt.Errorf("preset pattern %q invalid: %w", pattern, err)
	}
	if !re.MatchString(s) {
		return fmt.Errorf("must match %s", pattern)
	}
	return nil
}

// cancelledError signals that the user dismissed the prompt; Run maps it
// to exit code 130.
type cancelledError struct{ Err error }

func (c *cancelledError) Error() string { return "cancelled by user" }
func (c *cancelledError) Unwrap() error { return c.Err }
