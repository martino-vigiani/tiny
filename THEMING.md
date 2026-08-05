# Theming tiny

`tiny` draws every wizard through a theme. A theme is a small YAML document
describing two things:

- **symbols** — the literal strings drawn around your content (the prompt
  marker, the header decoration, list bullets, the table rule);
- **styles** — the colors and attributes each of those elements is painted with.

Nothing else about the wizard changes: the same YAML spec asks the same
questions and returns the same JSON, whatever it looks like. Theming is
presentation only, which is what makes it safe to let every tool ship its own.

```sh
tiny -theme sleek wizard.yaml               # a built-in preset
tiny -theme themes/brand.yaml wizard.yaml   # your own file
TINY_THEME=themes/brand.yaml tiny w.yaml    # for a whole session
tiny -list-themes                           # what is built in
```

## The five presets

| Preset | Look | Use it for |
| --- | --- | --- |
| `default` | `— Title —`, `> ` prompt, subtle color | the reference look |
| `minimal` | bare title, no divider | embedding in a tool that already prints its own chrome |
| `sleek` | `❯` prompt, `●`/`○` markers, cyan | modern terminals |
| `ascii` | `== Title ==`, 7-bit only, still colored | terminals or log pipelines that mangle UTF-8 |
| `plain` | ASCII and never colored | snapshot tests, CI logs, byte-stable output |

## Writing a theme

Every field is optional. What you leave out is inherited from the preset named
in `preset:` (or from `default`), so a theme that changes one symbol is one
line long.

```yaml
name: brand
preset: minimal
color: auto        # auto (default) | always | never

symbols:
  header_prefix: "\n┌ "   # printed before a step's title
  header_suffix: "\n"     # printed after it
  prompt: "└ "            # the input marker, everywhere the wizard waits
  indent: "  "            # left padding of option lists
  default_marker: "▸"     # marks the default option in a list
  item_marker: " "        # marks every other option
  bullet: "."             # follows the option number: "1." instead of "1)"
  error_prefix: "! "      # prepended to validation errors
  note_prefix: ""         # prepended to instruction lines
  hint_open: "("          # wraps inline hints, e.g. the "[default]" marker
  hint_close: ")"
  table_rule: "─"         # repeated under a table's header row

styles:
  header: "bold"
  prompt: "cyan"
  option: ""
  index: "dim"
  marker: "bold cyan"
  hint: "dim"
  error: "bold red"
  note: "dim"
  table_header: "bold underline"
```

Setting a symbol to `""` removes it. That is how you strip decoration:

```yaml
symbols:
  header_prefix: "\n"
  header_suffix: "\n"
```

## Style expressions

A style is a space-separated list of tokens, applied in order.

| Token | Meaning |
| --- | --- |
| `bold` `dim` `italic` `underline` `reverse` | text attributes |
| `black` `red` `green` `yellow` `blue` `magenta` `cyan` `white` | the 8 base colors |
| `bright-red`, `bright-blue`, … | the bright variants |
| `bg-blue`, `bg-bright-white`, … | background colors |
| `color:0`–`color:255` | 256-color palette |
| `#bd93f9` | truecolor |
| `bg-color:236`, `bg-#282a36` | the same, as a background |
| `none` or `""` | no styling |

```yaml
styles:
  header: "bold #bd93f9"
  error: "bold bright-red"
  marker: "bg-color:236 green"
```

An unknown token is a hard error at load time, not a silently ignored line.

## Where a theme comes from

Sources are layered; **later wins**, and every field is merged individually, so
a later source only overrides what it actually sets:

1. the built-in `default` theme
2. the preset named by `preset:`
3. the `theme:` block inside the wizard file
4. `$TINY_THEME`
5. `-theme <preset|file>`
6. `-no-color` / `$NO_COLOR` — these only force colors off, never on

The order matters: a wizard **author** can suggest a look, and the person or
tool **running** it always has the last word. Shipping a wizard never imposes
your palette on someone else's terminal.

### Inline, in the wizard itself

```yaml
theme:
  preset: sleek
  symbols:
    prompt: "→ "

steps:
  - type: input
    key: name
    title: "Project name"
```

### As a standalone file

A theme file is the same document without `steps:`. `tiny` accepts either
form for `-theme`, so you can also point it at an existing wizard to reuse
its look.

## Colors and when they appear

With `color: auto` (the default) escapes are emitted only when the UI stream —
stderr — is a terminal and `$TERM` is not `dumb`. Piping or redirecting the
wizard therefore produces clean text with no escape sequences, and CI logs stay
readable.

- `color: always` forces them on even when redirected.
- `color: never`, `-no-color` and `$NO_COLOR` force them off, and win over
  `always`. `tiny` follows the [NO_COLOR](https://no-color.org) convention.

## Integrating tiny into your own CLI

The point of theming is that a wizard invoked from your tool should look like
your tool, not like `tiny`.

1. Copy [`themes/brand.yaml`](themes/brand.yaml) into your repo.
2. Change the symbols and colors to the ones your CLI already uses.
3. Export `TINY_THEME=/path/to/brand.yaml` wherever you invoke `tiny`, or pass
   `-theme`.

Because the theme lives outside the wizard specs, every wizard your tool runs —
including ones generated on the fly by an agent — picks it up automatically.

```sh
export TINY_THEME="$PWD/themes/brand.yaml"
answers="$(tiny /tmp/generated-wizard.yaml)"   # drawn in your brand
```

## Two rules worth knowing

**The UI never touches stdout.** Prompts, lists, errors and hints all go to
stderr; stdout carries only the answers. That is why `tiny w.yaml | jq` works
while a human is still answering.

**Themes cannot change answers.** Symbols and styles are the whole surface: a
theme cannot add, remove, reorder or rename a step, and it cannot alter what
the wizard returns. A wizard's JSON is identical under every theme, which
`TestThemeParity` in `internal/runner/parity_test.go` enforces by running the
whole spec under every preset and comparing the answers.
