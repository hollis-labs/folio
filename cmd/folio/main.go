// Command folio is the CLI surface for preset-driven project scaffolding.
//
// The CLI is a thin shell around service/ — every command resolves to a
// single service call (New, Plan, ValidatePreset). Interactive prompting,
// input parsing, output formatting, and exit codes are the CLI's only
// responsibilities; rendering and validation live in service+internal.
package main

import (
	"os"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/cmd/folio/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], folio.BundledPresets, folio.Version))
}
