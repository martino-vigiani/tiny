# tiny

**The declarative terminal wizard. Write YAML, get JSON.**

`tiny` is a single ~3 MB binary that turns a YAML file into an interactive
terminal wizard and prints the answers as structured JSON.

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

## Why it exists

Every internal script has the same block of `read -p` at the top — and a
`--yes` flag that duplicates every one of its defaults a second time.

`tiny` replaces both with one file: a human answers the questions, the pipeline
runs the same file with `-defaults`, and both get the same JSON. An interaction
becomes a **file** instead of code, so it can be generated, reviewed, diffed,
versioned and verified before anyone runs it.

Three properties do the work:

**One spec, two modes.** The same file runs interactively for a human and
headless in CI with `-defaults`, returning the same shape of answers.
`TestDefaultsInteractiveParity` enforces it.

**stdout is only the payload.** The wizard is drawn on stderr, so
`tiny w.yaml | jq` works *while somebody is answering it*.

**Anyone can restyle it, nobody can restyle the output.** Themes control
symbols and colors — never the questions or the JSON.

## Interactive

On a real terminal, `select`, `multiselect`, `search` and `autocomplete` draw a
live prompt: arrow keys move the cursor, typing filters the list as you go,
space toggles, and the frame redraws in place.

```
Framework
❯ ▏ · type to filter
❯ SwiftUI
  UIKit
  AppKit
```

Anywhere else — a pipe, CI, an agent's shell — the same wizard falls back to a
numbered list, and `TestRichAnswersMatchLineMode` requires both ways of choosing
to return the same value. Looks cannot change answers.

It costs 5.9 ns and no allocation to decode a keystroke, one `write(2)` per
frame, and nothing at all between keystrokes: there is no render loop, no
polling and no goroutine — the process is simply blocked reading. Pass `-plain`
(or set `TINY_PLAIN`) to ask one line at a time instead.

## Install

```sh
go install github.com/martino-vigiani/tiny/cmd/tiny@latest
```

Or build it:

```sh
go build -trimpath -ldflags "-s -w" -o tiny ./cmd/tiny
```

No runtime dependencies, no config file, nothing to install alongside it.

## Use it

```sh
tiny select framework SwiftUI UIKit   # the whole wizard as arguments
tiny wizard.yaml                      # interactive; JSON on stdout
tiny - <<'YAML'                       # spec from stdin, no temp file
steps: [{type: input, key: name, title: "Name"}]
YAML
tiny -defaults wizard.yaml            # headless: uses every default, reads nothing
tiny -format env wizard.yaml          # KEY=VALUE, shell-quoted
tiny -format yaml wizard.yaml         # YAML
tiny -o answers.json wizard.yaml      # write to a file instead of stdout
tiny -theme sleek wizard.yaml         # a different look
printf '2\nn\n' | tiny wizard.yaml    # piped, no TTY needed
```

| Flag | Meaning |
| --- | --- |
| `-defaults` | Run headless, resolving every step to its `default`. Reads nothing from stdin. |
| `-format json\|yaml\|env` | Output format (default `json`). |
| `-o <file>` | Write the answers to a file instead of stdout. |
| `-theme <preset\|file>` | Draw with a preset or a theme file. |
| `-no-color` | Disable ANSI colors (same as `NO_COLOR`). |
| `-list-themes` | Print the built-in presets. |
| `-require-tty` | Fail (exit 3) instead of silently taking every default when there is no terminal. |
| `-compact` | Print the JSON payload on one line. |
| `-plain` | Ask one line at a time instead of drawing a live prompt. |
| `-version`, `-h` | Version, usage. |

Environment: `TINY_THEME` (preset name or path), `NO_COLOR`.

Exit codes: `0` success · `1` execution or spec error · `2` usage error ·
`3` `-require-tty` with no terminal.

**When nobody is there to answer** — CI, a hook, an agent's sandboxed shell —
every step falls back to its default and `tiny` says so on stderr. Pass
`-require-tty` to make that a hard failure instead: a payload of answers no
human gave is worse than an error.

## Step types (12)

| Type | Answer | Description |
| --- | --- | --- |
| `select` | `string` | Pick one option from a numbered list. |
| `multiselect` | `[]string` | Pick zero or more; `all` and `none` accepted. |
| `confirm` | `bool` | Yes/no question. |
| `input` | `string` | Free-form text. |
| `password` | `string` | Text entry without echo on a TTY. |
| `search` | `string` | Type to filter a long option list. |
| `number` | `number` | Numeric value with `min`/`max`/`increment`. |
| `date` | `string` | Date or date+time (`format: date`/`datetime`). |
| `keyvalue` | `object` | `key=value` pairs, one per line. |
| `editor` | `string` | Opens `$EDITOR` for multi-line text. |
| `autocomplete` | `string` | Free text with prefix suggestions. |
| `table` | row index | Shows a table; `selectable: true` returns the chosen row's index. |

Any step with `options` accepts `height: N` to cap how many are shown at once;
the rest are summarised as `… N more above/below`, and the option numbers stay
the same either way. `editor` collects multi-line text directly when no
`$EDITOR` is available — finish with a line containing only `.`

Full schema and per-type reference: **[SPEC.md](SPEC.md)**.

## The whole wizard as arguments

For a question that is not worth a file, write it as plain words:

```sh
tiny select framework SwiftUI UIKit AppKit
tiny select framework SwiftUI UIKit + input name MyApp + confirm git yes
tiny number port 8080 --min 1 --max 65535
```

The title is derived from the key (`db_host` becomes "Db host") and a
single-choice step defaults to its first option, so a short wizard is
headless-safe without saying so. Measured against gum, input plus output: 12
tokens against 16 for one question, 32 against 46 for three, 74 against 111 for
six — the shape is bare words rather than punctuation precisely because
tokenizers charge for `,` `:` `=` and not for spaces.

It is sugar: the same `Validate`, the same execution path, and a test requires
each short form and the YAML it stands for to parse to identical wizards.
`table` is the one type left out — its rows are two-dimensional, and YAML says
that better.

## Conditional steps

A step runs only if its `when:` condition holds, so one spec can ask different
questions depending on the answers:

```yaml
- type: select
  key: db_engine
  title: "Database engine"
  options: [postgres, mysql, sqlite]
  when: "use_db"
```

Six forms — `key`, `!key`, `==`, `!=`, `in a|b`, `contains` — evaluated against
earlier answers only, so branching behaves identically headless. A skipped step
contributes no key to the JSON: the consumer can tell "not asked" from "answered
with nothing". See [`examples/branching.yaml`](examples/branching.yaml).

## Headless

`-defaults` resolves every step to its `default` and never touches stdin:

- `select`/`search`/`autocomplete`: the default must be one of the `options`.
- `multiselect`: a list or comma-separated string; absent → empty list.
- `confirm`: `yes`/`y`/`true` → `true`, `no`/`n`/`false` → `false`, absent → `false`.
- `input`/`password`/`editor`: string default (empty allowed).
- `number`: validated against `min`/`max`; absent → `0`.
- `date`: validated against `format`.
- `keyvalue`: mapping, otherwise an empty map.
- `table`: with `selectable` and a default, the row index; otherwise `null`.

A `required: true` step with no default is an error in headless mode — which
makes `tiny -defaults w.yaml` a free validity check for a generated spec.

## Theming

Every wizard is drawn through a theme: symbols (prompt marker, header
decoration, bullets, table rule) and styles (colors, attributes). Five presets
ship in the binary — `default`, `minimal`, `sleek`, `ascii`, `plain` — and any
of them can be overridden by a few lines of YAML.

```yaml
theme:
  preset: sleek
  symbols:
    prompt: "→ "
  styles:
    header: "bold #bd93f9"
```

Sources layer in order — preset, the wizard's own `theme:` block, `$TINY_THEME`,
`-theme` — so a wizard author can *suggest* a look and whoever runs it always
has the last word. Colors follow [NO_COLOR](https://no-color.org) and switch
off automatically when the output is not a terminal.

Full reference and a copyable template: **[THEMING.md](THEMING.md)**.

## For coding agents

An agent's own shell usually has no terminal — inside Claude Code's Bash tool
`/dev/tty` cannot even be opened — so an agent cannot prompt you mid-task, with
`tiny` or with anything else. What it *can* do is **author** the interaction:
write the spec, wire it into a Makefile target or a git hook, and verify it
resolves with `-defaults`, all without a terminal. You run it later, for real.

That works because the interaction is data: it can be generated and checked
without being performed.

**[AGENT-GUIDE.md](AGENT-GUIDE.md)** has the TTY check, a ready-made block to
paste into your `CLAUDE.md` / `AGENTS.md`, and the mistakes to avoid.

## Example

[`examples/scaffold.sh`](examples/scaffold.sh) is a working consumer: it
generates a spec, runs it, parses the JSON with `jq`, scaffolds a Swift project
from the answers — and runs headless with `--ci`.

```sh
./examples/scaffold.sh          # interactive
./examples/scaffold.sh --ci     # unattended
```

## What a spec cannot do

A tiny spec is **inert**. It cannot run a command, read a file, or reach the
network — there is no `exec:` and there will not be one. You can receive one by
email, generate one from an untrusted issue, or `curl | tiny -` it, and the
worst it can do is ask you a question.

That is a deliberate refusal, not a missing feature. The moment a spec can run
something, every one of those becomes remote code execution, and a coding agent
reading a poisoned issue becomes the delivery mechanism. To fill a list from
live data, let the caller produce it — they already have the permissions:

```sh
tiny select branch $(git branch --format='%(refname:short)')
```

## Contributing

Architecture, conventions, and how to add a widget or a theme symbol:
**[AGENTS.md](AGENTS.md)**.

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .
```

## License

MIT.
