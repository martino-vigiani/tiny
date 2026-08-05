# tiny — Wizard YAML Specification

> Reference for the `tiny` declarative wizard format. Read this to **write a `wizard.yaml`
> that works on the first try**, without running anything. If you only need the quick
> start, see `examples/quickstart.yaml`.

## What tiny is

`tiny` is a single-binary CLI that reads a YAML file describing a list of questions
("steps") and renders them as an interactive terminal wizard, printing the collected
answers as **JSON** (default), **YAML** or **env** (`KEY=VALUE`).

It is an open, declarative standard designed so that **AI agents** (claude-code, opencode,
codex) can generate a wizard from a spec and run it in CI without a terminal.

### The key guarantee

> **Same wizard → same answer in interactive mode and `--defaults`.**

`tiny wizard.yaml` (interactive) and `tiny -defaults wizard.yaml` (headless) produce the
*identical* answer map — same keys, same Go types, same values — for any spec that
succeeds. This invariant is enforced by the test `TestDefaultsInteractiveParity`
(`internal/runner/parity_test.go`). Because of it, a wizard is always provable headless:
if `-defaults` passes, interactive mode returns exactly the same data.

### The stream contract

> **stdout is the payload. stderr is the wizard.**

Every prompt, option list, error and hint is written to **stderr**; **stdout** carries
nothing but the serialized answers. A caller can therefore capture the payload while a
human is still answering:

```sh
answers="$(tiny wizard.yaml)"   # the user sees the questions; $answers is pure JSON
tiny wizard.yaml | jq .framework
```

Never parse the prompts. Their wording and decoration are themable (see *Theme* below) and
are not part of this specification; only the payload is.

### The inertness guarantee

> **A spec asks questions. It cannot do anything else.**

No step runs a command, opens a network connection, or reads a file the person
running it did not point it at. A `file` step is confined to its `root`, with
symlinks resolved before the check. The `editor:` field is parsed for
compatibility and ignored: the editor comes from `$VISUAL`/`$EDITOR`. Control
characters are stripped from every piece of text a spec supplies, so an option
cannot emit escape sequences to the terminal.

This is what makes it safe to generate a spec from untrusted input, or to run
one you were sent.

### The presentation guarantee

> **How a wizard is drawn never changes what it answers.**

On a terminal the list steps draw a live prompt — arrow keys, type-to-filter,
space to toggle; anywhere else the same steps print a numbered list. Both return
the same Go type and the same value for the same choice, which
`TestRichAnswersMatchLineMode` enforces. The same is true of themes:

### The theme guarantee

> **A theme changes how a wizard looks, never what it answers.**

Themes control symbols and colors only. The same spec under any theme produces byte-identical
answers, enforced by `TestThemeParity`. Restyling a wizard can never break the code reading
its JSON.

---

## Schema

A wizard is a YAML document with one required top-level key, `steps`, and two optional
ones, `theme` and `include`:

```yaml
steps:
  - type: select
    key: framework
    title: "Framework"
    options: [SwiftUI, UIKit, AppKit]
    default: SwiftUI
```

- `steps` is a **list** of step objects, executed **in order**. It is required and must
  contain at least one step.
- Every step needs at least a `type`, a `key` and (in practice) a `title`.
- `theme` is optional and affects presentation only; see *Theme* below.
- `include` is an optional list of other spec files whose steps come first; see
  *Composition* below.

### Step fields

| Field         | Type                 | Default | Applies to                                      | Notes                                                                 |
|---------------|----------------------|---------|-------------------------------------------------|-----------------------------------------------------------------------|
| `type`        | string               | —       | all (required)                                  | One of the 12 types below.                                            |
| `key`         | string               | —       | all (required)                                  | Unique per wizard; must match `^[A-Za-z0-9][A-Za-z0-9_.-]*$`. Used as the answer key. |
| `title`       | string               | `""`    | all                                             | Short line printed as the section header; also the prompt fallback.   |
| `prompt`      | string               | `""`    | all                                             | Overrides `title` as the displayed header/prompt text.                |
| `default`     | any                  | nil     | all (type-dependent)                            | See the per-type sections.                                            |
| `required`    | bool                 | `false` | all                                             | True ⇒ the step must be answered (see EOF policy).                    |
| `options`     | `[]string` or `[]mapping` | nil | `select`, `multiselect`, `search`, `autocomplete` | At least 1 option; no two options may differ only by case. A mapping entry is `{value, label, description}` — see *Rich options*. |
| `placeholder` | string               | `""`    | `input`, `autocomplete`                         | `input`: hint shown as `[placeholder]`; `autocomplete`: "Hint: ..." line. |
| `min`         | number               | nil     | `number`                                        | Inclusive lower bound. Must be `<= max`.                              |
| `max`         | number               | nil     | `number`                                        | Inclusive upper bound.                                                |
| `increment`   | number               | `0`     | `number`                                        | Displayed as `(step: N)` in the prompt hint. **Not enforced** on input. |
| `headers`     | `[]string`           | nil     | `table`                                         | Column headers (drawn as a header row + separator).                   |
| `rows`        | `[][]string`         | nil     | `table`                                         | Data rows.                                                            |
| `selectable`  | bool                 | `false` | `table`                                         | True ⇒ the table becomes a row picker that answers a 0-based index.   |
| `editor`      | string               | `""`    | `editor`                                        | **Parsed and ignored.** The editor comes from `$VISUAL`/`$EDITOR` only — honouring it from a spec was arbitrary code execution. |
| `format`      | string               | `"date"`| `date`                                          | `"date"` (`YYYY-MM-DD`) or `"datetime"` (`YYYY-MM-DD HH:MM:SS`).      |
| `delimiter`   | string               | `""`    | —                                               | Parsed but **currently unused** (forward compatibility).              |
| `when`        | string               | `""`    | all                                             | Condition on an earlier answer; the step runs only if it holds. See *Conditional steps*. |
| `height`      | int                  | `0`     | `select`, `multiselect`, `search`, `autocomplete` | Show at most N options at once; the rest become a `… N more` marker. `0` shows all. |
| `match`       | string               | varies  | `select`, `search`, `autocomplete`, `multiselect` | How a typed filter is compared: `substring`, `prefix` or `fuzzy`. |
| `min_select`  | int                  | `0`     | `multiselect`                                   | Fewest options that may be chosen. `0` is unbounded.                  |
| `max_select`  | int                  | `0`     | `multiselect`                                   | Most options that may be chosen. `0` is unbounded.                    |
| `allow_custom`| bool                 | `false` | `search`, `autocomplete`                        | Accept typed text that is not one of the options.                     |
| `return_column`| int                 | `0`     | `table`                                         | Answer with this column's value (1-based) instead of the row index.   |
| `root`        | string               | `"."`   | `file`                                          | Directory the listing is confined to; it cannot walk above it.        |
| `dirs`        | bool                 | `false` | `file`                                          | Choose a directory instead of descending into it.                     |
| `hidden`      | bool                 | `false` | `file`                                          | Include dot-files in the listing.                                     |

**Convention:** give every step a `title` (short, sentence case, in English). If a step has
neither `title` nor `prompt`, the header is empty.

---

## The 12 step types

### `select`

Pick **one** option from a numbered list.

- **When to use:** a single choice out of a short, known set.
- **Fields:** `options` (required, ≥ 1); `default` optional (string, must be one of the
  `options`, matched case-insensitively).
- **Input:** a numbered list is printed (`1) Go  2) Rust  3) TypeScript`); answer by
  **number** (1-based) or by the **option name** (case-insensitive). Empty input returns
  the default. Invalid input shows `Invalid choice, pick a number between 1 and N.` and
  re-prompts.
- **Returns:** `string` — the **canonical** option (exact casing from `options`).
- **Default:** `default` must be a string among `options` (case-insensitive); the canonical
  option is returned. No default and not required ⇒ `""`.
- **Validation:** wizard rejected if `default` is not one of the `options`, or if options
  are duplicated case-insensitively.

```yaml
- type: select
  key: region
  title: "Primary region"
  options: [eu-west-1, us-east-1, ap-southeast-1]
  default: eu-west-1
```

---

### `multiselect`

Pick **zero or more** options.

- **When to use:** a set of independent choices.
- **Fields:** `options` (required, ≥ 1); `default` optional.
- **Input:** numbered list printed; answer with **comma-separated numbers or names**
  (`2,3` or `macOS, iOS`), case-insensitive. Keywords `all` (select everything) and `none`
  (clear) are also accepted. Empty input returns the default.
- **Returns:** `[]string` — **ordered as the `options`**, deduplicated, canonical casing.
  Never `null`; an empty selection is `[]`.
- **Default:** a **string with commas** (`"iOS, macOS"`) **or a list** (`[iOS, macOS]`).
  Each item must be one of the `options` (case-insensitive). The result is normalized to
  options order via `OrderByOptions`. No default ⇒ `[]`.
- **Validation:** every default item must match an option (case-insensitive).

```yaml
- type: multiselect
  key: regions
  title: "Replica regions"
  options: [eu-west-1, us-east-1, ap-southeast-1]
  default: [eu-west-1, us-east-1]
```

---

### `confirm`

A yes/no question returning a boolean.

- **When to use:** a single `true`/`false` decision.
- **Fields:** `default` optional (bool, or string `yes`/`y`/`true` / `no`/`n`/`false`).
- **Input:** prompt `(y/n) [y]: ` or `(y/n) [n]: ` (bracket shows the default). Accepts
  `y`/`yes`/`true` → `true`, `n`/`no`/`false` → `false` (case-insensitive). Empty input
  returns the default. Invalid input shows `Answer with y or n.` and re-prompts.
- **Returns:** `bool`.
- **Default:** `yes`/`y`/`true` or the bool `true` ⇒ `true`; `no`/`n`/`false` or the bool
  `false` ⇒ `false`. No default ⇒ `false`.
- **Validation:** a present `default` must parse to true/false; anything else rejects the
  wizard.

```yaml
- type: confirm
  key: init_git
  title: "Initialize a git repository?"
  default: "yes"
```

---

### `input`

Free-form single-line text.

- **When to use:** any short text answer (names, IDs, URLs).
- **Fields:** `placeholder` (optional, shown as `[placeholder]` in the prompt), `default`
  (optional string), `required` (optional).
- **Input:** `> ` prompt with optional hints `[placeholder] [default] `. Empty input
  returns the default; on a required step it shows `This field is required.` and re-prompts.
- **Returns:** `string` (as typed, trimmed of the trailing newline only).
- **Default:** any string, including `""`. No default and not required ⇒ `""`.
- **Validation:** none.

```yaml
- type: input
  key: project_name
  title: "Project name"
  placeholder: "my-project"
  default: "MyApp"
```

---

### `password`

Hidden text entry.

- **When to use:** secrets that must not echo on screen (API tokens, passphrases).
- **Fields:** `title` (required in practice — the prompt); `default` (optional string);
  `required` (optional).
- **Input:** on a TTY the input is read **without echo** (a newline is printed after).
  On a pipe it reads a plain line. The value is **never** shown in the prompt.
- **Returns:** `string`.
- **Default:** a string, including `""`. No default and not required ⇒ `""`.
- **Validation:** none. Note that `password` values are still emitted in the output, so
  treat the output channel accordingly.

```yaml
- type: password
  key: api_token
  title: "API token"
  default: ""
```

---

### `search`

Filter a list by typing a query, then pick one match.

- **When to use:** a choice from a large option set where scanning a list is slow.
- **Fields:** `options` (required, ≥ 1); `default` optional (string, must be an option,
  case-insensitive).
- **Input:** type a query at `> `; options containing the query (substring,
  case-insensitive) are listed. Then pick by **number** (1-based) or press Enter to take
  the default (or the first match when there is no default). Empty query with no matches
  shows `No matches, try again.`
- **Returns:** `string` — the canonical selected option.
- **Default:** must be among `options`; the canonical option is returned. No default and
  not required ⇒ `""`.
- **Validation:** same as `select`.

```yaml
- type: search
  key: owner
  title: "Repository owner (search)"
  options: [alice, bob, carol]
  default: alice
```

---

### `number`

A numeric answer with optional inclusive range.

- **When to use:** quantities, sizes, versions.
- **Fields:** `min` / `max` (optional numbers, `min <= max`), `increment` (optional, shown
  as `(step: N)`, **not enforced**), `default` (optional number or numeric string).
- **Input:** `> ` prompt with a hint such as `(min: 1, max: 8) (step: 1)`. The value must
  be a **finite** number: non-numeric input shows `Please enter a number.`, and
  **Inf/NaN are rejected** (`Please enter a finite number.`). Out-of-range input shows
  `Number must be between X and Y.` and re-prompts. Empty input returns the default.
- **Returns:** `float64`. YAML `default: 3` → `3` in JSON (Go `float64` serialized as a
  number).
- **Default:** a YAML integer or float, or a numeric string. It is validated against
  `min`/`max` (bounds **inclusive**); a default out of range rejects the wizard. No default
  and not required ⇒ `0` (as `float64`).
- **Validation:** `min > max` rejects the wizard; the default must be numeric and within
  `[min, max]`.

```yaml
- type: number
  key: workers
  title: "Number of workers"
  min: 1
  max: 8
  increment: 1
  default: 2
```

---

### `date`

A date (or date-time) answer.

- **When to use:** calendars, releases, schedules.
- **Fields:** `format` (`"date"` default → `YYYY-MM-DD`; `"datetime"` →
  `YYYY-MM-DD HH:MM:SS`), `default` (optional string matching the format).
- **Input:** `> (format: YYYY-MM-DD): ` (or the datetime variant). The value must parse
  exactly with the format; otherwise `Invalid date, expected YYYY-MM-DD.` and re-prompt.
  Empty input returns the default.
- **Returns:** `string`, exactly as entered, in the required layout.
- **Default:** a string validated against the format (`date` → `2006-01-02`;
  `datetime` → `2006-01-02 15:04:05`). A malformed default rejects the wizard. No default
  and not required ⇒ `""`.
- **Validation:** `format` must be `"date"` or `"datetime"`; the default must match.

```yaml
- type: date
  key: release_date
  title: "Release date"
  format: date
  default: "2026-01-15"
```

---

### `keyvalue`

Collect a set of `key=value` pairs.

- **When to use:** environment variables, labels, configuration maps.
- **Fields:** `default` (optional mapping with **scalar** values only).
- **Input:** after the header it prints
  `Enter key=value pairs, one per line; empty line to finish:` and reads lines of the form
  `key=value` (both sides trimmed). A line without `=` shows
  `Expected key=value, got "…".` and re-prompts. An empty line (or EOF) finishes.
- **Returns:** `map[string]string`. If the user enters nothing, the `default` map is
  returned; no default and nothing entered ⇒ an **empty map** (`{}`), never `null`.
- **Default:** a YAML mapping; every value must be a **scalar** (numbers are rendered via
  `%v`, nested maps/lists are rejected). Keys must be strings.
- **Validation:** nested values reject the wizard.

```yaml
- type: keyvalue
  key: env
  title: "Environment variables (KEY=VALUE)"
  default:
    LOG_LEVEL: info
    REGION: eu-west-1
```

---

### `editor`

Multi-line text captured via an external editor.

- **When to use:** long text — descriptions, changelogs, notes.
- **Fields:** `editor` (optional command override, e.g. `"nano"` or `"code -w"`),
  `default` (optional string, pre-filled into the editor buffer), `required` (optional).
- **Input:** opens the editor (see resolution order below) with the default pre-filled;
  the file content (trailing newlines trimmed) becomes the answer. If the editor cannot be
  used (no command configured, or stdin is not a TTY), it falls back to
  `Editor not available, type the content directly:` and reads a single line like `input`.
- **Editor resolution order:** step `editor` field → `$VISUAL` → `$EDITOR` → fallback to
  direct typing.
- **Returns:** `string` (multi-line content, trailing `\r\n` trimmed).
- **Default:** a string. No default and not required ⇒ `""`.
- **Validation:** none.

```yaml
- type: editor
  key: description
  title: "Project description (opens editor)"
  default: "Replace this text."
```

---

### `autocomplete`

Free text with suggestions from `options`.

- **When to use:** a known set of values where the user should still be able to see
  suggestions while typing.
- **Fields:** `options` (required, ≥ 1), `placeholder` (optional, printed as
  `Hint: …`), `default` (optional string, must be an option, case-insensitive).
- **Input:** type a prefix at `> `; options starting with the prefix (case-insensitive)
  are matched. One match → it is returned immediately. No match →
  `No matches, try again.` Multiple matches → numbered list; pick by number (1-based) or by
  exact name (case-insensitive); empty input takes the default (or the first match). Empty
  input at the main prompt takes the default.
- **Returns:** `string` — the canonical matched option.
- **Default:** must be among `options`; the canonical option is returned. No default and
  not required ⇒ `""`.
- **Validation:** same as `select`.

```yaml
- type: autocomplete
  key: editor_bin
  title: "Default editor (autocomplete)"
  options: [vim, nvim, nano, code]
  default: vim
```

---

### `table`

Display tabular data; optionally pick a row.

- **When to use:** showing structured data (permissions, plans, pricing) that the user
  reacts to; with `selectable: true` it doubles as a rich row picker.
- **Fields:** `headers` and `rows` (at least one of the two must be non-empty), `selectable`
  (optional bool).
- **Input:** the table is rendered with column widths derived from the widest cell (in
  runes, plus two spaces of padding). Non-selectable: `Press enter to continue.` and the
  step answers `nil`. Selectable: `Select row (number), empty for default: ` — answer by
  **number, 1-based** (the returned value is `n - 1`); empty input takes the default.
- **Returns:** `int` — the **0-based row index** when `selectable: true`; otherwise `nil`
  (serialized as `null`, omitted from env output).
- **Default** (selectable only): an **integer row index** (0-based, range-checked) **or a
  string matching the first column** (case-insensitive). A default on a non-selectable
  table rejects the wizard. No default and not required ⇒ `nil`; required without default
  ⇒ error in headless mode.
- **Validation:** `selectable: true` requires at least one row; a default requires
  `selectable: true`.

```yaml
- type: table
  key: permissions
  title: "Permissions by role"
  headers: [role, read, write]
  rows:
    - [admin, yes, yes]
    - [dev, yes, no]
  selectable: true
  default: "dev"   # or an index: 1
```

---

## Rich options

An option may be written as a mapping instead of a bare value:

```yaml
- type: select
  key: env
  title: "Environment"
  default: staging
  options:
    - dev
    - value: staging
      label: Staging
      description: production data shapes, no production traffic
    - value: prod
      label: Production
      description: real users, real pager
```

| Field         | Required | Meaning                                                        |
|---------------|----------|----------------------------------------------------------------|
| `value`       | yes*     | What the wizard answers with. *Defaults to `label` if omitted.  |
| `label`       | no       | What is drawn in the list and at the live prompt.               |
| `description` | no       | An explanation drawn beside the option, in the note style.      |

Rules:

- The answer is **always the value**. A label can never change what a caller receives.
- `default:` names a **value**, and is validated against the values.
- A label is accepted wherever a value is: typed at the prompt, typed into the live
  filter (which matches under both names), and passed to `-set`.
- A label that collides with another option's value is a **load error**: one typed word
  would otherwise mean two things. A label matching its own value is fine — `value:
  staging` with `label: Staging` is one option, capitalised.
- Both are interpolated like every other user-visible string.

## Composition

```yaml
include:
  - fragments/ownership.yaml
  - fragments/database.yaml
steps:
  - type: confirm
    key: apply
    title: "Apply?"
```

- Included steps land **before** the including file's own, in the order listed, so their
  answers are available to the `when:` conditions that follow.
- A fragment is validated only once assembled, because its steps may condition on keys
  the including file answers earlier.
- Themes layer: a fragment's theme is the base, the including file wins, and `-theme`
  still overrides both.
- Keys are unique across the whole assembly; a duplicate is a load error.

Confinement, because `include:` is the one feature that reads a second file:

| Rule | Reason |
|---|---|
| Relative paths only, resolved against the including file | An absolute path is refused outright. |
| Never outside the directory tree of the top-level spec | Symlinks are resolved *before* the check, so a link cannot smuggle a path out. |
| No cycles; depth ≤ 8; ≤ 64 files | A wizard past that is one nobody can read. |
| Refused for a spec with no directory (stdin, argv, embedded) | Resolving against the working directory would make the same bytes mean different things. |

## CLI

```
tiny [flags] <wizard.yaml>
tiny [flags] -                 # read the spec from stdin
tiny demo                      # run the showcase compiled into the binary
tiny -paths <wizard.yaml>      # list every path; ask nothing
```

Passing `-` instead of a path reads the spec from stdin, so a caller composing a
wizard does not need a temp file:

```sh
answers="$(tiny - <<'YAML'
steps:
  - type: confirm
    key: deploy
    title: "Deploy now?"
    default: "no"
YAML
)"
```

Because stdin then carries the spec, the answers are read from `/dev/tty`. Where
there is no terminal — CI, a hook, a cron job — there is nothing to read and every
step falls back to its default, exactly as under `-defaults`.

| Flag          | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `-defaults`   | Run headless: resolve every step to its `default`, read nothing from stdin. |
| `-format`     | Output format: `json` (default), `yaml`, `env`.                |
| `-o`          | Write the answers to this file instead of stdout.              |
| `-theme`      | Draw with a preset name or a theme YAML file.                  |
| `-no-color`   | Disable ANSI colors (same as `NO_COLOR`).                      |
| `-list-themes`| Print the built-in presets, one per line, and exit.            |
| `-require-tty`| Exit 3 instead of resolving every step to its default when there is no terminal. |
| `-compact`    | Print the JSON payload on one line.                            |
| `-plain`      | Ask one line at a time instead of drawing a live prompt.        |
| `-tty`        | Read answers from `/dev/tty`, leaving stdin to whatever owns it. |
| `-set`        | Pre-seed one answer as `key=value`; repeatable.                 |
| `-answers`    | Pre-seed answers from a JSON or YAML file.                      |
| `-paths`      | Print every distinct set of steps the wizard can ask, one JSON object per line, with the answers that reach it. Reads no input. |
| `-meta`       | Add a `_meta` block recording what produced the answers.        |
| `-version`    | Print `tiny v0.3.0` and exit.                                  |
| `-h`          | Print usage and exit.                                          |

Environment variables:

| Variable      | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `TINY_THEME`  | Theme preset name or path, overridden by `-theme`.             |
| `TINY_PLAIN`  | Any non-empty value disables the live interactive prompt.      |
| `NO_COLOR`    | Any non-empty value disables colors.                           |

### Exit codes

| Code | Meaning                                                        |
|------|----------------------------------------------------------------|
| `0`  | Success (also `-version` and `-h`).                            |
| `1`  | Execution errors: unreadable file, invalid wizard spec, step failure. |
| `2`  | Usage errors: unknown flag, wrong number of args, invalid `-format`. |
| `3`  | `-require-tty` was passed and there is no terminal to answer from.   |
| `4`  | The user cancelled the wizard (Ctrl-C at a prompt).                  |

### Pipe / headless behavior

- **TTY stdin:** full interactive widgets (hidden password entry, editor launch).
- **Non-TTY stdin (pipe/file):** widgets read **line by line** in plain mode
  (`password` reads a visible line, `editor` falls back to direct typing).
- **EOF policy:** on EOF with no pending data, every widget first tries the `default`;
  if there is none and the step is `required`, it fails with
  `unexpected end of input for required step "<key>"` (exit `1`); otherwise it returns the
  type's zero value (`""`, `false`, `0`, `[]`, `{}` or `nil`).
- This is exactly why `-defaults` exists: it produces the same answers **without reading
  stdin at all**, making wizards runnable in CI.

Examples:

```sh
tiny wizard.yaml                        # interactive
tiny -defaults wizard.yaml              # headless, uses defaults, JSON out
tiny -defaults -format yaml wizard.yaml # headless, YAML out
tiny -defaults -format env wizard.yaml  # headless, KEY=VALUE out
printf '2\nn\n' | tiny wizard.yaml      # piped: 2nd option, "n"
tiny wizard.yaml | jq .framework        # UI on stderr, payload piped to jq
tiny -o answers.json wizard.yaml        # payload to a file
tiny -theme sleek wizard.yaml           # a different look, identical answers
tiny -version                           # tiny v0.3.0
```

---

### -paths

`-paths` enumerates the shapes a branching wizard can take, without running it and
without reading input. One JSON object per line: `{"steps": [...], "answers": {...}}`,
sorted so two runs of the same spec print the same list.

Only keys some condition reads are branched, each over a declared domain:

| Step type                            | Values tried                                     |
|--------------------------------------|--------------------------------------------------|
| `confirm`                            | true and false                                   |
| `select`, `search`, `autocomplete`   | every option                                     |
| `multiselect`                        | nothing, each option alone, all of them          |
| `number`                             | its default, plus every numeric literal compared against it |
| everything else                      | its default, plus every literal compared against it |

It is a bounded exploration, not a proof: it cannot invent the one string that
satisfies a `==` the spec never mentions. It **does** guarantee that every path
printed is reachable by the answers printed beside it, and it reports on stderr when
the list was cut at `DefaultPathLimit` (500).

### -meta

`-meta` adds a `_meta` key. It cannot collide with a step, because a wizard key must
start with a letter or a digit. Every value is a string, so the block survives `json`,
`yaml` and `env` alike.

| Field         | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `tiny`        | The version that ran the wizard.                                |
| `source`      | The spec: a path, `-`, `demo`, or `argv`.                       |
| `spec_sha256` | sha256 of **every byte read**, includes and all.                |
| `mode`        | `interactive`, `defaults` or `preset`.                          |
| `tty`         | Whether a person could actually answer.                         |
| `answered`    | How many steps produced an answer.                              |
| `skipped`     | The keys never asked, comma-separated. Absent when none were.   |
| `started`     | RFC 3339 UTC.                                                   |
| `elapsed_ms`  | How long the wizard took.                                       |

It is off by default: a payload carrying a timestamp is not byte-reproducible.

## Output formats

Answers are serialized from the Go types listed above. The same values are printed in all
three formats, so `-defaults` output is a reliable preview of interactive output.

### JSON (default)

Indented two spaces, in the order keys were collected (insertion order of the steps).

```json
{
  "framework": "SwiftUI",
  "regions": ["iOS", "macOS"],
  "init_git": true,
  "workers": 2
}
```

### YAML

```yaml
framework: SwiftUI
regions:
    - iOS
    - macOS
init_git: true
workers: 2
```

### env

`KEY=VALUE`, one line per top-level key, **sorted**:

- `string` → shell-quoted; `bool` → `true`/`false`; `float64` → Go `'g'` formatting
  (`2` not `2.0`); `int` → decimal.
- `[]string` → joined with `", "`, then quoted.
- `map[string]string` → **one line per entry** as `KEY_mapkey=value`, map keys sorted
  (`env` above becomes `env_LOG_LEVEL=info` and `env_REGION=eu-west-1`).
- `nil` values are **omitted entirely** (no line).

**Quoting.** An answer is arbitrary user text, so values containing anything outside
`[A-Za-z0-9_-./:@%+,=]` are wrapped in single quotes with embedded quotes escaped. This
makes `eval "$(tiny -format env w.yaml)"` safe even when an answer contains a space, a
quote or a `;`.

**Key names.** Wizard keys may contain `.` and `-`, which no shell accepts in a variable
name, so they are normalised to `_` (`project.name` → `project_name`). Two keys that
normalise to the same identifier are a hard error (exit `1`) rather than a silently
dropped answer.

```sh
framework=SwiftUI
init_git=true
regions='iOS, macOS'
workers=2
env_LOG_LEVEL=info
env_REGION=eu-west-1
```

Prefer `-format json` piped through `jq` when values can be arbitrary; `env` is a
convenience, JSON is the interchange format.

---

## Filtering and matching

`select`, `search`, `autocomplete` and `multiselect` accept a typed filter. How
it is compared is the step's to decide:

| `match` | Meaning | Default for |
|---------|---------|-------------|
| `substring` | the option contains the text | `select`, `search`, `multiselect` |
| `prefix` | the option starts with the text | `autocomplete` |
| `fuzzy` | the text is a subsequence of the option | — |

A fuzzy step also **ranks** its matches: a match at the start of an option, on a
word boundary (after a space, `-`, `_`, `.`, `/`, or at a camelCase transition),
or in an unbroken run scores higher, and a shorter option wins a tie. Ranking
applies to the numbered list as well as to the live prompt, so "the second one"
means the same thing either way.

`allow_custom: true` makes a `search` or `autocomplete` accept text that matches
nothing, for the steps whose options are suggestions rather than an enumeration.

---

## Pre-seeding answers

`-defaults` answers every step from its `default`. To answer *some* steps and
leave the rest to their defaults:

```sh
tiny -set env=production -set version=v2.1.0 release.yaml
tiny -answers answers.json release.yaml
tiny -answers answers.json -set env=staging release.yaml   # the flag wins
```

- A preset is **the text a person would have typed at that prompt** — so a
  `multiselect` takes `iOS,macOS`, a `keyvalue` takes `A=1,B=2`, and a
  `confirm` takes `yes`/`no`. One consequence: a value containing a comma
  cannot be expressed this way; use a YAML spec default instead.
- **A selectable table takes the row number shown on screen (1-based)**, because
  a preset mirrors what you would type. A spec `default:` integer stays 0-based:
  that is the author's index, not the reader's.
- Every preset goes through the same coercion and validation a `default` does.
  A preset that is invalid for its step is an error naming the key, never a
  silently corrected value.
- A preset for a step some branch skipped is a note on stderr, not an error: a
  preset file is written once and reused, and a branching wizard legitimately
  carries answers for branches a given run does not take.
- A preset naming a step the wizard does not have is an error listing the valid
  keys — no run could ever consume it, so it is a typo.

`-answers` reads JSON or YAML; scalars, lists and mappings are all accepted.

---

## The short form

A wizard can be written as argv words instead of a file:

```sh
tiny select framework SwiftUI UIKit AppKit
tiny select framework SwiftUI UIKit + input name MyApp + confirm git yes
tiny number port 8080 --min 1 --max 65535
```

- The first argument naming a step type switches tiny into this mode; anything
  else is a path (a file called `select` is reachable as `./select`).
- Steps are separated by a bare `+`.
- Bare words after the key are the `options` for a list step, or the `default`
  for the others; `keyvalue` takes `k=v` words.
- Attributes use the YAML field names: `--title`, `--prompt`, `--default`,
  `--required`, `--placeholder`, `--min`, `--max`, `--increment`, `--format`,
  `--editor`, `--height`, `--when`, `--pattern`, `--message`, `--min-length`,
  `--max-length`.
- The title is derived from the key when none is given (`db_host` → "Db host").
- A `select`, `search` or `autocomplete` with no explicit default takes its
  first option, so a short wizard is always resolvable headless. This is the one
  place the short form is not a literal transcription of the minimal YAML.
- A value that must start with `--` or be `+` is escaped: `\--foo`, `\+`.
- `table` is not expressible: its rows are two-dimensional. Write YAML.

The short form is sugar over the same parser and the same validation, and
`TestShortFormEquivalence` requires each short form and its YAML to produce
structurally identical wizards.

---

## Conditional steps

A step carrying `when:` runs only if its condition holds against the answers
collected **before** it.

```yaml
steps:
  - type: confirm
    key: use_db
    title: "Does it need a database?"
    default: "yes"

  - type: select
    key: db_engine
    title: "Database engine"
    options: [postgres, mysql, sqlite]
    default: postgres
    when: "use_db"
```

**A skipped step contributes no key to the output.** The consumer sees which
questions were actually asked, rather than a value nobody chose. Code reading
the answers must therefore treat a missing key as "not asked" — which is also
how it distinguishes a skipped step from an empty answer.

### The condition language

Six forms, no precedence, no arithmetic. It exists so a wizard can branch, not
so a wizard can compute.

| Form | True when |
|------|-----------|
| `key` | the answer is truthy: `true`, a non-empty string or collection, a non-zero number |
| `!key` | the answer is falsy |
| `key == value` | the answer equals the value (text comparison, case-insensitive) |
| `key != value` | it does not |
| `key in a\|b\|c` | the answer equals any of the alternatives |
| `key contains value` | the collection holds the value (or the string contains it) |

Values with spaces are quoted: `when: 'label == "two words"'`. A `confirm`
answer accepts `true`/`false` and `yes`/`no`. For a `multiselect`, `==` and `in`
match when **any** chosen option matches, so `platforms in iOS|tvOS` means "at
least one of these was chosen".

### Rules

- A condition may only reference a key answered **earlier** in the wizard. A
  forward or self reference is a load error, not a step that silently never
  runs.
- **Skipping is transitive.** A condition on a key that was itself skipped is
  false, so a chain of dependent steps drops out together.
- Conditions read only answers, so they resolve identically interactively and
  under `-defaults`: both paths skip the same steps and return the same map.
  `TestConditionalParity` enforces it.
- A skipped step is never validated for `required:` — `required` means "must be
  answered *if asked*".

See `examples/branching.yaml` for a wizard exercising every form.

---

## Theme

The optional top-level `theme` key sets how the wizard is drawn. It is presentation only:
symbols and colors, never questions, keys, types or values.

```yaml
theme:
  preset: sleek          # default | minimal | sleek | ascii | plain
  color: auto            # auto (default) | always | never
  symbols:
    prompt: "→ "
    header_prefix: "\n"
    header_suffix: "\n"
  styles:
    header: "bold #bd93f9"
    error: "bold red"

steps:
  - type: input
    key: name
    title: "Project name"
```

Every symbol and style is optional; what is omitted is inherited from the preset. Setting a
symbol to `""` removes it.

| Symbol | Drawn |
|--------|-------|
| `header_prefix` / `header_suffix` | around a step's title |
| `prompt` | the input marker, wherever the wizard waits for typing |
| `indent`, `default_marker`, `item_marker`, `bullet` | option lists |
| `error_prefix`, `note_prefix` | before validation errors and instruction lines |
| `hint_open` / `hint_close` | around inline hints such as `[default]` |
| `table_rule` | repeated under a table's header row |

Style expressions are space-separated tokens: `bold` `dim` `italic` `underline` `reverse`,
the 8 base colors and their `bright-` variants, `bg-<color>`, `color:0..255`, `#rrggbb`, or
`none`. An unknown token is a load-time error.

**Precedence — later wins:** built-in default → `preset:` → the wizard's `theme:` block →
`$TINY_THEME` → `-theme` → `-no-color`/`$NO_COLOR` (which only force colors off). A wizard
author therefore *suggests* a look; whoever runs the wizard decides.

For generated wizards: **omit `theme:` unless the look is part of the request.** The host
tool usually sets `$TINY_THEME` already, and an inline theme competes with it for no
benefit. Full reference: `THEMING.md`.

---

## Best practices for generating wizards

Follow these rules so a wizard is correct, testable headless, and unambiguous:

1. **Use unique `key`s in `snake_case`.** Keys are the answer keys and the env variable
   names; they must match `^[A-Za-z0-9][A-Za-z0-9_.-]*$` and be unique across the wizard.
2. **Give every step a short `title` in English**, sentence case. It is the header users
   (and the `-defaults` parity logs) see.
3. **Always set a `default` when you can.** It makes the wizard runnable headless
   (`tiny -defaults wizard.yaml`) and is what agents/CI rely on. This is the single most
   important rule for the AI-agent use case.
4. **Use `required: true` sparingly** — only for steps where an answer is truly
   indispensable. Every `required` step without a default is a wizard that cannot run
   headless.
5. **Keep `options` unique case-insensitively** (`Swift` and `swift` are duplicates) and
   give `default` values exactly the option casing you want returned.
6. **For `multiselect`, order matters in the output**: the answer always follows the
   `options` order, so list options in the order you want them returned.
7. **For `table`, remember the answer is a 0-based index** (or `null` when not
   `selectable`).
8. **Set a `placeholder`** for free-text inputs to hint at the expected shape; it is also
   shown in non-interactive runs.
9. **Test every wizard headless** before shipping: `tiny -defaults wizard.yaml` must exit
   0 and print the expected JSON.
10. **Avoid `delimiter`**: it is parsed but not yet implemented.
11. **Never parse the prompts.** They go to stderr and their wording is themable; read the
    payload on stdout instead.

### Minimal wizard

```yaml
steps:
  - type: input
    key: project_name
    title: "Project name"
    default: "MyApp"
```

### Complete reference example

`examples/full.yaml` exercises all 12 types with valid defaults and is the best template:

```sh
tiny -defaults examples/full.yaml
```

See also `wizard.yaml` (a compact real-world wizard) and `examples/quickstart.yaml`
(the 30-second tour).
