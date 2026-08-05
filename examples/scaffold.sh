#!/usr/bin/env bash
# Demo: a real consumer of tiny.
#
# Asks the user how to scaffold a Swift project, then acts on the answers.
# The same script runs unattended in CI with --ci (tiny -defaults, no stdin).
#
#   ./examples/scaffold.sh          # interactive
#   ./examples/scaffold.sh --ci     # headless, uses the YAML defaults
#
# The interactive path works because tiny draws the wizard on stderr and writes
# only the JSON to stdout: the command substitution below captures the answers
# while the user still sees the questions.
#
# Requires: tiny and jq on $PATH, or set TINY=/path/to/tiny

set -euo pipefail

TINY="${TINY:-tiny}"
HEADLESS=""
[ "${1:-}" = "--ci" ] && HEADLESS="-defaults"

spec="$(mktemp -t scaffold-spec)"
trap 'rm -f "$spec"' EXIT

cat >"$spec" <<'YAML'
steps:
  - type: input
    key: name
    title: "Project name"
    default: "MyApp"
    required: true

  - type: select
    key: framework
    title: "UI framework"
    options: [SwiftUI, UIKit, AppKit]
    default: SwiftUI

  - type: multiselect
    key: platforms
    title: "Target platforms"
    options: [iOS, macOS, watchOS, tvOS]
    default: "iOS, macOS"

  - type: number
    key: deployment
    title: "Minimum iOS version"
    min: 15
    max: 26
    increment: 1
    default: 18

  - type: confirm
    key: tests
    title: "Add a test target?"
    default: "yes"

  - type: confirm
    key: git
    title: "Initialize a git repository?"
    default: "yes"
YAML

# JSON is the interchange format: every value keeps its type and jq handles
# quoting. (`-format env` is shell-quoted too and safe to eval, but it flattens
# every type to a string.)
answers="$("$TINY" $HEADLESS -format json "$spec")"

name="$(jq -r '.name' <<<"$answers")"
framework="$(jq -r '.framework' <<<"$answers")"
platforms="$(jq -r '.platforms | join(", ")' <<<"$answers")"
deployment="$(jq -r '.deployment' <<<"$answers")"
tests="$(jq -r '.tests' <<<"$answers")"
git_init="$(jq -r '.git' <<<"$answers")"

root="$(mktemp -d -t "scaffold")/$name"
mkdir -p "$root/Sources/$name"

cat >"$root/Sources/$name/${name}App.swift" <<SWIFT
import $framework

@main
struct ${name}App: App {
    var body: some Scene {
        WindowGroup { ContentView() }
    }
}
SWIFT

[ "$tests" = "true" ] && mkdir -p "$root/Tests/${name}Tests"
[ "$git_init" = "true" ] && git -C "$root" init -q

printf '\nScaffolded %s (%s, iOS %s, platforms: %s)\n' \
    "$name" "$framework" "$deployment" "$platforms"
find "$root" -not -path '*/.git/*' | sed "s|$root|.|"
