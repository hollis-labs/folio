.PHONY: build install test test-race vet lint vuln pins clean tidy all

# Default: build the folio binary into the repo root.
build:
	go build -o folio ./cmd/folio

# Install folio to $(go env GOBIN) (typically ~/go/bin/).
install:
	go install ./cmd/folio

# Run the test suite. Use `make test-race` for the race detector pass.
test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Govulncheck — gate per portfolio Go baseline (feedback_go_ecosystem_baseline).
vuln:
	govulncheck ./...

# Every version pin this repo carries must agree with platform-pins.yaml.
pins:
	sh scripts/check-platform-pins-drift.sh .
	sh scripts/check-platform-pins-drift.sh . --self-test

tidy:
	go mod tidy

clean:
	rm -f folio
	rm -f cmd/folio/folio

# Full pre-commit pipeline matching CI.
all: tidy vet lint test-race
