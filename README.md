# folio

Preset-driven project scaffolding for the hollis-labs portfolio.

`folio new <preset> <target-dir>` renders a typed preset (YAML manifest + template tree)
into a new project directory, with a `.folio.yaml` breadcrumb recording what was
generated so the project can be re-rendered and (in a later version) synced when the
preset evolves.

## Status

v0 vertical slice. Single bundled `base` preset proving the format end-to-end.
Composition, sync, MCP, and post-render Hadron integration are deliberately deferred.

See `docs/user/` for the user guide and `docs/agent/` for agent-facing references
(MCP tool schemas, headless invocation patterns).

## Quickstart

```sh
go install github.com/hollis-labs/folio/cmd/folio@latest

folio new base /tmp/folio-smoke \
  --input project_name=foo \
  --input github_owner=chrispian \
  --non-interactive

cd /tmp/folio-smoke
go vet ./...
go test ./...
```

## Development

```sh
make test       # go test -race ./...
make vet        # go vet ./...
make lint       # golangci-lint run
make build      # build the folio binary
make install    # go install ./cmd/folio
```

## License

MIT
