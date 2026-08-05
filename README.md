# tiny

YAML in, JSON out, one interactive wizard in the terminal.

```yaml
# wizard.yaml
steps:
  - type: select
    key: framework
    title: "UI framework"
    options: [SwiftUI, UIKit, AppKit]
    default: SwiftUI

  - type: input
    key: project_name
    title: "Project name"

  - type: confirm
    key: init_git
    title: "Initialize a git repository?"
    default: "yes"
```

```console
$ tiny wizard.yaml | jq
{
  "framework": "SwiftUI",
  "project_name": "MyApp",
  "init_git": true
}
```

<!-- MEDIA: terminal recording of `tiny wizard.yaml` being answered interactively, then piped to jq -->

## Why

Every internal script grows the same two things over time: a block of
`read -p` prompts, and a `--yes` flag that duplicates every one of those
prompts' defaults a second time, by hand, to keep CI working.

`tiny` replaces both with one file. A human runs `tiny wizard.yaml` and
answers it. CI runs the exact same file with `-defaults` and gets the exact
same shape of answers, no interaction, no drift between the two paths. The
interaction becomes a file — reviewable, diffable, generated if it needs to
be — instead of a growing pile of shell.

[gum](https://github.com/charmbracelet/gum) does the styled-prompt part of
this well. It has no headless mode and no spec file: the questions live in
your shell script, so there's still a `--yes` path to maintain by hand.

## Cheap

Measured on this repo, not claimed:

- **3.0 MB** binary (`go build -trimpath -ldflags "-s -w"`, `du -h`).
- **2 direct dependencies** — `golang.org/x/term` and `gopkg.in/yaml.v3` (see `go.mod`).
- **Under 6 MB** peak resident memory and CPU time too small to register,
  running a wizard headless (`/usr/bin/time -l tiny -defaults examples/quickstart.yaml`).
- **341 tests**, all green (`go test ./...`).

No framework, no runtime dependency beyond the two above, nothing to
install alongside it.

## Install

```sh
go install github.com/martino-vigiani/tiny/cmd/tiny@latest
```

Or build from source:

```sh
git clone https://github.com/martino-vigiani/tiny
cd tiny && go build -trimpath -ldflags "-s -w" -o tiny ./cmd/tiny
```

There is no tagged release yet, so there are no prebuilt binaries to
download — both commands above compile it locally.

## Use it in a script

[`examples/scaffold.sh`](examples/scaffold.sh) is a complete, working
example: it writes a wizard spec, runs it, parses the answers with `jq`, and
scaffolds a Swift project from them. It runs interactively by default and
headless in CI with `--ci`:

```sh
./examples/scaffold.sh          # interactive
./examples/scaffold.sh --ci     # unattended, uses the YAML defaults
```

The shape of it:

```sh
answers="$(tiny $HEADLESS -format json "$spec")"   # $HEADLESS is "-defaults" in CI, empty otherwise

name="$(jq -r '.name' <<<"$answers")"
framework="$(jq -r '.framework' <<<"$answers")"
tests="$(jq -r '.tests' <<<"$answers")"

# ...act on the answers: write files, run git init, etc.
```

stdout carries only the JSON — the wizard itself draws on stderr — so the
command substitution above captures clean answers while a human still sees
the questions on screen. In CI, `-defaults` skips stdin entirely and
resolves every step to its default.

<!-- MEDIA: terminal recording of scaffold.sh run interactively, then run again with --ci -->

## Coding agents

Most agent shells have no TTY — `/dev/tty` can't even be opened inside
Claude Code's Bash tool — so an agent can't prompt you mid-task, with `tiny`
or anything else. What it can do is **author** the interaction: write the
`wizard.yaml`, wire it into a Makefile target or a git hook, and verify it
resolves cleanly with `-defaults` — all without a terminal. You run it
later, for real, because the interaction is a file the agent can generate
and check without ever performing it.

**[AGENT-GUIDE.md](AGENT-GUIDE.md)** has the TTY check, a block to paste
into `CLAUDE.md`/`AGENTS.md`, and the mistakes to avoid.

## Showing, not just asking

Four commands draw something instead of collecting an answer, so nothing
from them reaches stdout:

```sh
tiny spin "Building…" -- go build ./...   # spinner while a command runs
tiny say "Deploy complete" --style header # one styled line
tiny box "Plan" < plan.md                 # a bordered box
tiny bar 3 12 "files"                     # a progress line, redrawn in place
```

Without a terminal, each one degrades to a single plain line with no escape
sequences — useful in a CI log, where animated frames are why people turn
spinners off. See them all in fifteen seconds: [`examples/show.sh`](examples/show.sh).

<!-- MEDIA: terminal recording of spin, say, box, bar running in sequence -->

## Reference

The full spec — every step type, `-format`/`-set`/`-answers`/`-paths`,
conditional steps, `include:`, theming — is in **[SPEC.md](SPEC.md)**.
`tiny -h` covers every flag. `THEMING.md` covers themes.

## Contributing

Architecture, conventions, and how to add a widget or a theme symbol:
**[AGENTS.md](AGENTS.md)**.

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .
```

## License

MIT.
