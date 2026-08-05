#!/usr/bin/env bash
# Demo: run the showcase wizard and consume its answers.
#
#   ./examples/showcase.sh                  # interactive, neon theme
#   ./examples/showcase.sh --ci             # headless, every step defaulted
#   ./examples/showcase.sh --theme slate    # a preset or themes/*.yaml
#
# It prints the deploy plan the answers describe. Nothing is deployed and
# nothing outside this repository is read or written.
#
# Requires: tiny and jq on $PATH, or set TINY=/path/to/tiny
#
# Headless honesty: with --ci (or anywhere without a terminal — CI, a git hook,
# an agent's shell) tiny resolves every step to its `default` and reads nothing.
# The branches still branch, so the payload has a different *shape* depending on
# those defaults: no `vault_token` key when `push_secrets` is false, no
# `db_engine` when `needs_db` is false. That is the point of `when:` — a skipped
# step contributes no key, so the consumer can tell "not asked" from "answered
# with nothing".

set -euo pipefail

TINY="${TINY:-tiny}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$HERE")"
SPEC="$HERE/showcase.yaml"

THEME="$ROOT/themes/neon.yaml"
HEADLESS=""
while [ $# -gt 0 ]; do
    case "$1" in
        --ci) HEADLESS="-defaults" ;;
        --theme)
            shift
            # A bare name means a built-in preset or themes/<name>.yaml.
            if [ -f "$ROOT/themes/$1.yaml" ]; then THEME="$ROOT/themes/$1.yaml"
            else THEME="$1"; fi
            ;;
        -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
        *) printf 'unknown option: %s\n' "$1" >&2; exit 2 ;;
    esac
    shift
done

command -v jq >/dev/null || { echo "showcase.sh needs jq" >&2; exit 1; }

# The wizard is drawn on stderr and only the JSON goes to stdout, so this
# command substitution captures the payload while the questions are on screen.
# The file step lists paths relative to the repository, so run from there.
cd "$ROOT"
answers="$("$TINY" $HEADLESS -theme "$THEME" -format json "$SPEC")"

# jq keeps every value's type, and `// empty` is how a skipped step reads: the
# key is absent, not null.
service=$(jq -r '.service'                  <<<"$answers")
runtime=$(jq -r '.runtime'                  <<<"$answers")
region=$(jq -r '.region'                    <<<"$answers")
plan=$(jq -r '.plan'                        <<<"$answers")
replicas=$(jq -r '.replicas'                <<<"$answers")
components=$(jq -r '.components | join(" ")' <<<"$answers")
module=$(jq -r '.module'                    <<<"$answers")
overlay=$(jq -r '.overlay'                  <<<"$answers")
window=$(jq -r '.window'                    <<<"$answers")
apply=$(jq -r '.apply'                      <<<"$answers")
engine=$(jq -r '.db_engine // empty'        <<<"$answers")
volume=$(jq -r '.db_size_gb // empty'       <<<"$answers")
workers=$(jq -r '.queue_workers // empty'   <<<"$answers")
static=$(jq -r '.static_binary // empty'    <<<"$answers")

printf '\n── deploy plan ─────────────────────────────────────────────\n'
printf '  service     %s (%s)\n'   "$service" "$runtime"
printf '  module      %s\n'        "$module"
printf '  region      %s\n'        "$region"
printf '  plan        %s × %s\n'   "$plan" "$replicas"
printf '  components  %s\n'        "$components"
printf '  overlay     %s\n'        "$overlay"
printf '  window      %s\n'        "$window"
# Written as `if`, not `[ … ] && printf`: under `set -e` a false test as the
# last command of a line would end the script.
if [ -n "$static" ]; then
    printf '  static link %s\n' "$static"
fi
if [ -n "$workers" ]; then
    printf '  queue       %s workers\n' "$workers"
fi
if [ -n "$engine" ]; then
    printf '  database    %s' "$engine"
    if [ -n "$volume" ]; then
        printf ' (%s GB)' "$volume"
    fi
    printf '\n'
else
    printf '  database    not asked for — the branch was skipped\n'
fi

# The environment map arrives as an object; render it the way a deploy would.
printf '  env\n'
jq -r '.env | to_entries[] | "                \(.key)=\(.value)"' <<<"$answers"

printf '\n'
if [ "$apply" = "true" ]; then
    printf 'This is where a real tool would deploy %s to %s.\n' \
        "$service" "$region"
else
    printf 'Not deploying: the last question answered no.\n'
fi

# The same payload, ready for anything else to consume.
printf '\nRaw payload:\n'
jq -c . <<<"$answers"
