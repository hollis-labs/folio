// Package folio is the root of the folio binary. Its only job today is to
// expose the bundled presets via an embed.FS — the runtime code lives under
// cmd/folio (CLI), service/ (canonical API), and internal/* (parser, render,
// manifest). Future MCP / HTTP surfaces will live in additional cmd/* or
// internal/* directories importing the same service layer.
package folio

import "embed"

// Version is the human-readable folio version. Bumped per release; reads
// into the .folio.yaml generator field via service.Options.FolioVersion.
const Version = "0.1.0"

// BundledPresets contains the read-only filesystem of presets shipped with
// the folio binary. cmd/folio/main.go passes this directly to
// service.Options. The first directory level under "presets/" is the
// preset id (matches design doc §9 layout).
//
//go:embed all:presets
var BundledPresets embed.FS
