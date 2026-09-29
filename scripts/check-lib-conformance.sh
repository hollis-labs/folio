#!/bin/sh
# check-lib-conformance.sh — report a Go lib tree against the hollis-labs lib
# checklist (libs/docs/LIB-STANDARD-CHECKLIST.md §B, §C4, §E, §F).
#
#   check-lib-conformance.sh <dir> [--release] [--report]
#
# One `PASS|FAIL|N/A <id> <text>` line per item (FAIL detail is indented under
# it). Items are PROPERTIES derived from the tree — never counts.
#
# Exit: 0 all pass · 1 any FAIL · 2 nothing examined / usage · 3 a required
# tool is missing. --report never exits 1 (for sweeps over existing libs, which
# are report-only). --release additionally fails on an unresolved
# `TODO(author)` and requires a CHANGELOG heading for each tag on HEAD.
#
# DELETION CONDITION: this script is a bridge between a DRAFT checklist and the
# `go-lib` preset. Remove it when the checklist is promoted and folio owns the
# conformance definition, or when `folio` grows a conformance command.

usage() {
	echo "usage: $0 <dir> [--release] [--report]" >&2
	exit 2
}

dir=""
release=0
report=0
for a in "$@"; do
	case "$a" in
	--release) release=1 ;;
	--report) report=1 ;;
	-*) usage ;;
	*) if [ -n "$dir" ]; then usage; fi; dir="$a" ;;
	esac
done
[ -n "$dir" ] || usage
[ -d "$dir" ] || { echo "not a directory: $dir" >&2; exit 2; }
[ -f "$dir/go.mod" ] || { echo "no go.mod in $dir: nothing to examine" >&2; exit 2; }
command -v go >/dev/null 2>&1 || { echo "required tool missing: go" >&2; exit 3; }
GREP=/usr/bin/grep
[ -x "$GREP" ] || GREP=grep

cd "$dir" || exit 2
GOWORK=off
export GOWORK

failed=0
tmp=$(mktemp) || exit 3
trap 'rm -f "$tmp"' EXIT INT TERM

res() { # status id text
	printf '%s %s %s\n' "$1" "$2" "$3"
	[ "$1" = FAIL ] && failed=1
	return 0
}
detail() { head -n 5 "$tmp" | sed 's/^/    /'; }
# ok <id> <text> — PASS if the previous command list succeeded
run_item() { # id text cmd...
	id=$1; text=$2; shift 2
	if "$@" >"$tmp" 2>&1; then res PASS "$id" "$text"; else res FAIL "$id" "$text"; detail; fi
}

modpath=$(sed -n 's/^module[[:space:]][[:space:]]*//p' go.mod | head -n 1)
gover=$(sed -n 's/^go[[:space:]][[:space:]]*\([0-9][0-9.]*\).*/\1/p' go.mod | head -n 1)

# --- B1 no replace ---------------------------------------------------------
if $GREP -Eq '^[[:space:]]*replace([[:space:]]|\()' go.mod; then
	res FAIL B1 "go.mod has a replace directive"
else
	res PASS B1 "go.mod has no replace directive"
fi

# --- B2 go.work ignored and untracked --------------------------------------
if $GREP -Fxq go.work .gitignore 2>/dev/null && $GREP -Fxq go.work.sum .gitignore 2>/dev/null; then
	tracked=""
	if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		tracked=$(git ls-files go.work go.work.sum 2>/dev/null)
	fi
	if [ -n "$tracked" ]; then res FAIL B2 "go.work / go.work.sum are tracked"; else res PASS B2 ".gitignore lists go.work and go.work.sum; none tracked"; fi
else
	res FAIL B2 ".gitignore does not list go.work and go.work.sum"
fi

# --- B3 modules ------------------------------------------------------------
run_item B3 "go mod verify and go mod tidy -diff clean" sh -c 'go mod verify && go mod tidy -diff'

# --- B4 / B5 go pin --------------------------------------------------------
wf=""
[ -d .github/workflows ] && wf=$(ls .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null)
if [ -n "$wf" ] && $GREP -q 'go-version-file:[[:space:]]*go\.mod' $wf && ! $GREP -Eiq 'go-version:[^#]*stable|check-latest' $wf; then
	res PASS B4 "workflows use go-version-file: go.mod, no 'stable'"
else
	res FAIL B4 "workflows must use go-version-file: go.mod and not 'stable'"
fi
floor_ok=0
if [ -n "$gover" ]; then
	major=${gover%%.*}
	rest=${gover#*.}
	minor=${rest%%.*}
	patch=0
	case "$rest" in *.*) patch=${rest#*.} ;; esac
	if [ "$major" -gt 1 ] || { [ "$major" -eq 1 ] && { [ "$minor" -gt 26 ] || { [ "$minor" -eq 26 ] && [ "$patch" -ge 6 ]; }; }; }; then floor_ok=1; fi
fi
if [ "$floor_ok" = 1 ]; then res PASS B5 "go directive $gover is at or above the 1.26.6 floor"; else res FAIL B5 "go directive '${gover:-missing}' is below the 1.26.6 floor"; fi

# --- B6-B8 gates -----------------------------------------------------------
run_item B6 "gofmt -l . is empty" sh -c 'out=$(gofmt -l .); [ -z "$out" ] || { echo "$out"; exit 1; }'
# -o /dev/null: a lone `main` package would otherwise leave a binary in the tree
# being examined. This script never writes into it.
run_item B7 "go build ./... and go vet ./..." sh -c 'go build -o /dev/null ./... && go vet ./...'
run_item B8 "go test -count=1 ./..." go test -count=1 ./...

# --- B10 golangci config ---------------------------------------------------
if [ -f .golangci.yml ] || [ -f .golangci.yaml ]; then res PASS B10 ".golangci config present"; else res FAIL B10 "no .golangci.yml"; fi

# --- B15-B17 README --------------------------------------------------------
if [ -f README.md ]; then
	if $GREP -Fq "go get $modpath" README.md; then res PASS B15 "README has 'go get $modpath'"; else res FAIL B15 "README lacks 'go get $modpath'"; fi
	if $GREP -q '^```go' README.md; then
		if [ -d examples ] && ! go build -o /dev/null ./examples/... >"$tmp" 2>&1; then
			res FAIL B16 "examples/ does not build"; detail
		else
			res PASS B16 "README has a go fence; examples/ builds"
		fi
	else
		res FAIL B16 "README has no \`\`\`go fence"
	fi
	if $GREP -q '^## License' README.md; then res PASS B17 "README has ## License"; else res FAIL B17 "README lacks ## License"; fi
	if $GREP -q '^## Out of scope' README.md; then res PASS F3 "README has ## Out of scope"; else res FAIL F3 "README lacks ## Out of scope"; fi
	if $GREP -q '^## Compatibility' README.md; then res PASS F4 "README has ## Compatibility"; else res FAIL F4 "README lacks ## Compatibility"; fi
else
	for i in B15 B16 B17 F3 F4; do res FAIL "$i" "no README.md"; done
fi

# --- B18 release workflow --------------------------------------------------
if [ -n "$wf" ] && $GREP -Eq '^[[:space:]]*tags:' $wf && $GREP -q 'gh release' $wf; then
	res PASS B18 "a workflow releases on tags"
else
	res FAIL B18 "no workflow publishes a GitHub Release on a tag"
fi

# --- C4 workflow hygiene ---------------------------------------------------
# Comment lines are not commands: strip them before looking for anti-patterns.
wfcode=""
[ -n "$wf" ] && wfcode=$(cat $wf | $GREP -v '^[[:space:]]*#')
if printf '%s\n' "$wfcode" | $GREP -q '@latest'; then res FAIL C4 "a workflow uses @latest"; else res PASS C4 "no @latest in workflows"; fi
if printf '%s\n' "$wfcode" | $GREP -Eq '\|\|[[:space:]]*(true|:)([[:space:]]|$)|command -v .*\|\|'; then
	res FAIL C4b "a workflow has a skip idiom (|| true / command -v ... ||)"
else
	res PASS C4b "no skip idioms in workflows"
fi

# --- F module path, doc.go --------------------------------------------------
if $GREP -Eq '^module github\.com/hollis-labs/[a-z0-9-]+([[:space:]]|$)' go.mod; then res PASS F1 "module path is github.com/hollis-labs/<name>"; else res FAIL F1 "module path is not github.com/hollis-labs/<name>"; fi
missing=""
for d in $(find . \( -name .git -o -name node_modules -o -name vendor -o -name testdata -o -name examples \) -prune -o -type f -name '*.go' ! -name '*_test.go' -exec dirname {} \; | sort -u); do
	[ -f "$d/doc.go" ] || missing="$missing $d"
done
if [ -z "$missing" ]; then res PASS F2 "doc.go in every package directory"; else res FAIL F2 "doc.go missing in:$missing"; fi

# --- --release only --------------------------------------------------------
if [ "$release" = 1 ]; then
	if $GREP -rIl 'TODO(author)' --exclude-dir=.git --exclude=.folio.yaml . >"$tmp" 2>/dev/null && [ -s "$tmp" ]; then
		res FAIL R1 "unresolved TODO(author)"; detail
	else
		res PASS R1 "no unresolved TODO(author)"
	fi
	tags=$(git tag --points-at HEAD 2>/dev/null)
	if [ -z "$tags" ]; then
		res N/A R2 "no tag on HEAD"
	else
		bad=""
		for t in $tags; do
			v=${t#v}
			esc=$(printf '%s' "$v" | sed 's/\./\\./g')
			$GREP -Eq "^## \[?v?${esc}\]?([[:space:]]|\$)" CHANGELOG.md 2>/dev/null || bad="$bad $t"
		done
		if [ -z "$bad" ]; then res PASS R2 "CHANGELOG has a heading for every tag on HEAD"; else res FAIL R2 "CHANGELOG has no heading for:$bad"; fi
	fi
fi

if [ "$failed" = 1 ] && [ "$report" = 0 ]; then exit 1; fi
exit 0
