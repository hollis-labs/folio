#!/bin/sh
# check-platform-pins-drift.sh — fail when a file that carries a version pin
# disagrees with platform-pins.yaml.
#
#   check-platform-pins-drift.sh [<folio-dir>] [--checklist FILE] [--self-test]
#
# <folio-dir> defaults to the directory above this script. One
# `PASS|FAIL|N/A <id> <text>` line per pin per file (FAIL detail is indented
# under it), the same idiom as check-lib-conformance.sh. Items are PROPERTIES of
# the files ("this template pins checkout at the manifest's tag"), never counts.
#
# Checked:
#   go-default    each presets/*/preset.yaml go_version input default == go.version
#   go-mod        <folio-dir>/go.mod `go` line == go.version
#   go-floor-script  scripts/check-lib-conformance.sh floor literals == go.version
#   checkout, setup-go, golangci-action, golangci-version, govulncheck
#                 every preset workflow template and folio's own
#                 .github/workflows/*: every tag/version found == the manifest's
#   no-latest     no workflow uses @latest or `version: latest`
#   checklist-govulncheck   (with --checklist) the C4 bullet's pin == the manifest's
#                 and the checklist names platform-pins.yaml
#
# --self-test is the positive control: it copies the tree to a scratch dir,
# breaks one value at a time, and requires exactly the expected check(s) to FAIL
# (and the unbroken copy to PASS). A checker that passes everything is worthless.
#
# Exit: 0 all pass · 1 any FAIL · 2 nothing examined / usage · 3 tool missing.

usage() {
	echo "usage: $0 [<folio-dir>] [--checklist FILE] [--self-test]" >&2
	exit 2
}

dir=""
checklist=""
selftest=0
while [ $# -gt 0 ]; do
	case "$1" in
	--self-test) selftest=1 ;;
	--checklist) shift; [ $# -gt 0 ] || usage; checklist=$1 ;;
	-*) usage ;;
	*) if [ -n "$dir" ]; then usage; fi; dir=$1 ;;
	esac
	shift
done
if [ -z "$dir" ]; then
	dir=$(cd "$(dirname "$0")/.." && pwd) || exit 2
fi
[ -d "$dir" ] || { echo "not a directory: $dir" >&2; exit 2; }
dir=$(cd "$dir" && pwd)
[ -f "$dir/platform-pins.yaml" ] || { echo "no platform-pins.yaml in $dir" >&2; exit 2; }
for t in awk sed grep find sort tr; do
	command -v "$t" >/dev/null 2>&1 || { echo "required tool missing: $t" >&2; exit 3; }
done

if [ "$selftest" = 1 ]; then
	for t in tar cmp mktemp cp; do
		command -v "$t" >/dev/null 2>&1 || { echo "required tool missing: $t" >&2; exit 3; }
	done
	self="$0"
	scratch=$(mktemp -d) || exit 3
	trap 'rm -rf "$scratch"' EXIT INT TERM
	work="$scratch/tree"
	mkdir "$work" || exit 3
	(cd "$dir" && tar --exclude=.git --exclude=.scratch -cf - .) | (cd "$work" && tar -xf -) || exit 3
	cl_args=""
	if [ -n "$checklist" ]; then
		cp "$checklist" "$scratch/checklist.md" || exit 3
		cl_args="--checklist $scratch/checklist.md"
	fi
	bad=0
	say() { printf '%s\n' "$*"; }

	# shellcheck disable=SC2086
	out=$(sh "$self" "$work" $cl_args 2>&1); rc=$?
	if [ "$rc" -eq 0 ]; then say "PASS control unbroken copy passes"; else say "FAIL control unbroken copy must pass"; printf '%s\n' "$out" | sed 's/^/    /'; bad=1; fi

	# case <name> <file> <sed-script> <expected FAIL ids, space separated>
	# The mutated file is restored afterwards so cases are independent.
	mutate() {
		name=$1; file=$2; script=$3; want=$4
		target="$work/$file"
		if [ "$file" = "@checklist" ]; then target="$scratch/checklist.md"; fi
		cp "$target" "$scratch/orig" || exit 3
		sed -e "$script" "$scratch/orig" >"$scratch/mut" || exit 3
		if cmp -s "$scratch/orig" "$scratch/mut"; then
			say "FAIL $name mutation did not change $file (self-test is stale)"; bad=1; return
		fi
		cp "$scratch/mut" "$target"
		# shellcheck disable=SC2086
		out=$(sh "$self" "$work" $cl_args 2>&1); rc=$?
		got=$(printf '%s\n' "$out" | awk '$1=="FAIL"{print $2}' | sort | tr '\n' ' ')
		wantn=$(printf '%s\n' $want | sort | tr '\n' ' ')
		cp "$scratch/orig" "$target"
		if [ "$rc" -ne 0 ] && [ "$got" = "$wantn" ]; then
			say "PASS $name fails exactly: $wantn"
		else
			say "FAIL $name expected only [$wantn] to fail, got [$got] (exit $rc)"; bad=1
		fi
	}
	CHK=presets/go-baseline/files/.github/workflows/check.yml.tmpl
	mutate "go-default-wrong" presets/base/preset.yaml 's/default: "1.26.6"/default: "1.26.5"/' "go-default:presets/base/preset.yaml"
	mutate "go-mod-wrong" go.mod 's/^go 1\.26\.6$/go 1.26.5/' "go-mod:go.mod"
	mutate "chimera-wrong" presets/chat-app/preset.yaml 's/default: v0.0.0-20261008115211-389155313ee5/default: v0.0.0-20261008115211-000000000000/' "chimera_version:presets/chat-app/preset.yaml"
	mutate "chimera-wrapper-wrong" presets/app-dashboard-chimera-plugin/preset.yaml 's/default: design-0.4.0-react-19.3.0/default: design-0.4.0-react-0.0.0/' "chimera_gui_recipe:presets/app-dashboard-chimera-plugin/preset.yaml"
	mutate "floor-script-wrong" scripts/check-lib-conformance.sh 's/"\$patch" -ge 6 \]/"$patch" -ge 5 ]/' "go-floor-script:scripts/check-lib-conformance.sh"
	# first occurrence only: a file with SOME wrong tags must fail too
	mutate "checkout-one-of-many-wrong" .github/workflows/ci.yml '1,/actions\/checkout@v5/s/actions\/checkout@v5/actions\/checkout@v4/' "checkout:.github/workflows/ci.yml"
	mutate "setup-go-wrong" "$CHK" 's/actions\/setup-go@v6/actions\/setup-go@v5/' "setup-go:$CHK"
	mutate "golangci-action-wrong" "$CHK" 's/golangci-lint-action@v7/golangci-lint-action@v8/' "golangci-action:$CHK"
	mutate "golangci-version-wrong" "$CHK" 's/version: v2\.11\.4/version: v2.11.3/' "golangci-version:$CHK"
	mutate "govulncheck-wrong" "$CHK" 's/govulncheck@v1\.8\.0/govulncheck@v1.7.0/' "govulncheck:$CHK"
	mutate "govulncheck-latest" "$CHK" 's/govulncheck@v1\.8\.0/govulncheck@latest/' "govulncheck:$CHK no-latest:$CHK"
	mutate "lint-version-latest" .github/workflows/ci.yml 's/version: v2\.11\.4/version: latest/' "golangci-version:.github/workflows/ci.yml no-latest:.github/workflows/ci.yml"
	if [ -n "$checklist" ]; then
		mutate "checklist-stale-pin" @checklist 's/(`@v1\.8\.0`)/(`@v1.2.0`)/' "checklist-govulncheck"
		mutate "checklist-no-manifest-ref" @checklist 's/platform-pins\.yaml/pins/g' "checklist-govulncheck"
	else
		say "N/A checklist cases (no --checklist given)"
	fi
	[ "$bad" = 0 ] && say "self-test OK" || say "self-test FAILED"
	exit "$bad"
fi

# --- the check ---------------------------------------------------------------
manifest="$dir/platform-pins.yaml"
pin() { # section key
	awk -v sec="$1" -v key="$2" '
		/^[^ #]/ { cur = $1; sub(/:$/, "", cur) }
		cur == sec && $1 == key ":" { v = $2; gsub(/"/, "", v); print v; exit }
	' "$manifest"
}
go_v=$(pin go version)
chimera_v=$(pin chimera version)
chimera_gui=$(pin chimera gui_recipe)
checkout_v=$(pin ci actions_checkout)
setupgo_v=$(pin ci actions_setup_go)
lintact_v=$(pin ci golangci_lint_action)
lintver_v=$(pin ci golangci_lint_version)
vuln_v=$(pin ci govulncheck_version)
for v in "$go_v" "$checkout_v" "$setupgo_v" "$lintact_v" "$lintver_v" "$vuln_v"; do
	[ -n "$v" ] || { echo "platform-pins.yaml is missing a pin (parsed: go=$go_v checkout=$checkout_v setup-go=$setupgo_v lint-action=$lintact_v lint=$lintver_v vuln=$vuln_v)" >&2; exit 2; }
done

failed=0
examined=0
res() { # status id text
	printf '%s %s %s\n' "$1" "$2" "$3"
	[ "$1" = FAIL ] && failed=1
	[ "$1" != "N/A" ] && examined=$((examined + 1))
	return 0
}
rel() { printf '%s' "${1#"$dir"/}"; }

# compare <id> <text> <want> <found...> — PASS iff found is non-empty and every
# value equals want.
compare() {
	id=$1; text=$2; want=$3; shift 3
	found=$*
	if [ -z "$found" ]; then res FAIL "$id" "$text: no value found (want $want)"; return; fi
	uniq=$(printf '%s\n' $found | sort -u | tr '\n' ' ')
	if [ "$uniq" = "$want " ]; then res PASS "$id" "$text = $want"; else res FAIL "$id" "$text: found [${uniq% }], want $want"; fi
}

# go_version defaults in presets that declare the input
for f in "$dir"/presets/*/preset.yaml; do
	got=$(awk '
		/^ *- name: go_version[ \t]*$/ { f = 1; next }
		f && /^ *default:/ { v = $0; sub(/^ *default:[ \t]*/, "", v); sub(/[ \t]*#.*/, "", v); gsub(/"/, "", v); print v; exit }
		f && /^ *- name:/ { exit }
	' "$f")
	[ -n "$got" ] || continue # preset composes go-baseline instead of declaring go_version
	compare "go-default:$(rel "$f")" "go_version default" "$go_v" "$got"
done
for f in "$dir"/presets/app-dashboard/preset.yaml "$dir"/presets/chat-app/preset.yaml "$dir"/presets/app-dashboard-chimera-plugin/preset.yaml "$dir"/presets/chat-app-chimera-plugin/preset.yaml; do
 for key in chimera_version chimera_gui_recipe; do
  got=$(awk -v key="$key" '$0 ~ "^ *- name: "key"[ \t]*$" { f=1; next } f && /^ *default:/ { v=$0; sub(/^ *default:[ \t]*/, "", v); gsub(/"/, "", v); print v; exit }' "$f")
  want=$chimera_v; [ "$key" = chimera_gui_recipe ] && want=$chimera_gui
  compare "$key:$(rel "$f")" "$key default" "$want" "$got"
 done
done
compare "go-mod:go.mod" "go directive" "$go_v" "$(awk '$1 == "go" { print $2; exit }' "$dir/go.mod")"

# the conformance script re-encodes the floor as arithmetic literals
gmin=${go_v#*.}; gmin=${gmin%%.*}
gpat=0
case "${go_v#*.*.}" in "$go_v") ;; *) gpat=${go_v#*.*.} ;; esac
script=scripts/check-lib-conformance.sh
if [ -f "$dir/$script" ]; then
	if grep -F -e "\"\$minor\" -gt $gmin ]" "$dir/$script" >/dev/null &&
		grep -F -e "\"\$minor\" -eq $gmin ]" "$dir/$script" >/dev/null &&
		grep -F -e "\"\$patch\" -ge $gpat ]" "$dir/$script" >/dev/null &&
		grep -F -e "the $go_v floor" "$dir/$script" >/dev/null; then
		res PASS "go-floor-script:$script" "floor literals encode $go_v"
	else
		res FAIL "go-floor-script:$script" "floor literals (minor $gmin, patch $gpat, message '$go_v floor') do not match go.version $go_v"
	fi
else
	res FAIL "go-floor-script:$script" "file missing"
fi

# workflow files: every preset template and folio's own CI
files=$( { find "$dir/presets" -type f -path '*/files/.github/workflows/*' 2>/dev/null; find "$dir/.github/workflows" -type f 2>/dev/null; } | sort)
[ -n "$files" ] || { echo "no workflow files found under $dir" >&2; exit 2; }

tags() { # file action-regex -> the @tag of every use
	grep -o -e "$2@[A-Za-z0-9._-]*" "$1" | sed 's/.*@//'
}
lint_versions() { # the `version:` of each golangci-lint-action step
	awk '
		/golangci-lint-action@/ { f = 1; next }
		f && /^ *- / { f = 0 }
		f && /^ *version:/ { v = $2; gsub(/"/, "", v); print v; f = 0 }
	' "$1"
}
seen_checkout=0; seen_setup=0; seen_lintact=0; seen_lintver=0; seen_vuln=0
for f in $files; do
	r=$(rel "$f")
	t=$(tags "$f" 'actions/checkout')
	[ -n "$t" ] && { seen_checkout=1; compare "checkout:$r" "actions/checkout" "$checkout_v" "$t"; }
	t=$(tags "$f" 'actions/setup-go')
	[ -n "$t" ] && { seen_setup=1; compare "setup-go:$r" "actions/setup-go" "$setupgo_v" "$t"; }
	t=$(tags "$f" 'golangci/golangci-lint-action')
	if [ -n "$t" ]; then
		seen_lintact=1; compare "golangci-action:$r" "golangci-lint-action" "$lintact_v" "$t"
		seen_lintver=1; compare "golangci-version:$r" "golangci-lint version" "$lintver_v" "$(lint_versions "$f")"
	fi
	t=$(grep -o -e 'govulncheck@[A-Za-z0-9._-]*' "$f" | sed 's/.*@//')
	[ -n "$t" ] && { seen_vuln=1; compare "govulncheck:$r" "govulncheck" "$vuln_v" "$t"; }
	# comment lines may talk about @latest ("Pinned, not @latest"); only code counts
	if grep -v -E -e '^[[:space:]]*#' "$f" | grep -E -e '@latest|version: *"?latest' >/dev/null; then
		res FAIL "no-latest:$r" "uses @latest or version: latest"
		grep -v -E -e '^[[:space:]]*#' "$f" | grep -E -e '@latest|version: *"?latest' | head -n 3 | sed 's/^/    /'
	else
		res PASS "no-latest:$r" "no @latest / version: latest"
	fi
done
# A pin nobody carries is not evidence: each tool must be seen at least once.
[ "$seen_checkout" = 1 ] || res FAIL "checkout:none" "no workflow uses actions/checkout"
[ "$seen_setup" = 1 ] || res FAIL "setup-go:none" "no workflow uses actions/setup-go"
[ "$seen_lintact" = 1 ] || res FAIL "golangci-action:none" "no workflow uses golangci-lint-action"
[ "$seen_vuln" = 1 ] || res FAIL "govulncheck:none" "no workflow runs govulncheck"

# the ratified checklist
if [ -n "$checklist" ]; then
	if [ -f "$checklist" ]; then
		c4=$(grep -E -e '^- \[.\] \*\*C4\.' "$checklist" | head -n 1)
		cpin=$(printf '%s' "$c4" | grep -o -e '@v[0-9][0-9.]*' | head -n 1)
		if [ "$cpin" = "@$vuln_v" ] && grep -F -e 'platform-pins.yaml' "$checklist" >/dev/null; then
			res PASS "checklist-govulncheck" "C4 cites @$vuln_v and names platform-pins.yaml"
		else
			res FAIL "checklist-govulncheck" "C4 cites '${cpin:-nothing}' (want @$vuln_v) and must name platform-pins.yaml"
		fi
	else
		res FAIL "checklist-govulncheck" "checklist not found: $checklist"
	fi
else
	res N/A "checklist-govulncheck" "no --checklist given"
fi

[ "$examined" -gt 0 ] || { echo "nothing examined" >&2; exit 2; }
exit "$failed"
