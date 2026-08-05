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

Every CLI reinvents the same question loop, and no agent can ask one at all.
`tiny` makes an interaction a **file** instead of code — so it can be
generated, reviewed, diffed, versioned, replayed, and run without a human.

Three properties do the work:

**One spec, two modes.** The same file runs interactively for a human and
headless in CI with `-defaults`, returning the same shape of answers.
`TestDefaultsInteractiveParity` enforces it.

**stdout is only the payload.** The wizard is drawn on stderr, so
`tiny w.yaml | jq` works *while somebody is answering it*.

**Anyone can restyle it, nobody can restyle the output.** Themes control
symbols and colors — never the questions or the JSON.

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
tiny wizard.yaml                      # interactive; JSON on stdout
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
| `-version`, `-h` | Version, usage. |

Environment: `TINY_THEME` (preset name or path), `NO_COLOR`.

Exit codes: `0` success · `1` execution or spec error · `2` usage error.

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

Full schema and per-type reference: **[SPEC.md](SPEC.md)**.

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

An agent can't ask its user a structured question. With `tiny` it writes a YAML
file, runs one command, and reads JSON — and the same interaction still works
unattended in CI.

```sh
answers="$(tiny /tmp/ask.yaml)"    # prompts on stderr, pure JSON captured
```

**[AGENT-GUIDE.md](AGENT-GUIDE.md)** has a ready-made block to paste into your
`CLAUDE.md` / `AGENTS.md`, the verification workflow (`-defaults` as a dry run),
and the mistakes to avoid.

## Example

[`examples/scaffold.sh`](examples/scaffold.sh) is a working consumer: it
generates a spec, runs it, parses the JSON with `jq`, scaffolds a Swift project
from the answers — and runs headless with `--ci`.

```sh
./examples/scaffold.sh          # interactive
./examples/scaffold.sh --ci     # unattended
```

## Contributing

Architecture, conventions, and how to add a widget or a theme symbol:
**[AGENTS.md](AGENTS.md)**.

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .
```

## License

MIT.
