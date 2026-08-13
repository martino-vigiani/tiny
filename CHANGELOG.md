# Changelog

## Unreleased

## v0.4.0

First public release: tagged binaries, Homebrew, and the README walkthrough.

### Added

- **`tiny demo`**: the 20-step showcase compiled into the binary, so it runs
  from any directory with nothing checked out. A test keeps the embedded copy
  identical to `examples/showcase.yaml`.
- **Rich options**: an option may be `{value, label, description}`. The answer
  is always the value; the label is what is drawn and is typeable, filterable
  under either name, and accepted by `-set`. A label colliding with another
  option's value is a load error.
- **`include:`**: compose a wizard out of files. Included steps come first so
  their answers reach the conditions that follow; themes layer; a fragment is
  validated only once assembled. Confined to the tree the spec came from —
  relative paths, symlinks resolved before the containment check, no cycles,
  bounded depth, and refused for a spec with no directory of its own.
- **`-paths`**: print every distinct set of steps a branching wizard can ask,
  one JSON object per line, with the answers that reach it. Reads no input.
  Bounded exploration, reported as cut when it is; every path printed is
  reachable by the answers printed beside it.
- **The display commands** — `spin`, `say`, `box`, `bar`: the half of a harness
  that shows rather than asks. They produce no answer, so nothing about the
  payload changes; they draw on stderr, and degrade to one plain escape-free
  line where there is no terminal. `spin` exits with its child's code and lets
  the child's stdout through untouched.
- **`-meta`**: a `_meta` block recording the version, the spec and the sha256 of
  every byte read, the mode, whether there was a terminal, and which steps were
  never asked. Off by default, since a timestamp breaks byte-reproducibility.

### Fixed

- A `number` step with `min: 5` and no default answered `0`, below its own
  declared floor.
- A `file` step's `default:` was returned headless but never shown at the
  prompt it was a suggestion for.
- `match:` was honoured at the live prompt and inert in a pipe on `select`.

### Packaging

- `tiny -version` prints the link-time tag (`dev` on an untagged build).
- Release assets include a `.tar.gz` per platform for Homebrew (binary only,
  no Go toolchain).

## v0.3.0

### Added

- **Fuzzy matching** (`match: fuzzy|prefix|substring`), the reason people
  install gum. Matches rank by where they land — start of the option, word
  boundary, unbroken run — with the shorter option winning a tie, and the
  ranking applies to the numbered list as well as the live prompt so the two
  never disagree about order. ~24 ns per option, zero allocations.
- **A `file` step**: pick a path, with `root:`, `dirs:` and `hidden:`.
- **Answer pre-seeding**: `-set key=value` (repeatable) and `-answers <file>`.
  `-defaults` was all-or-nothing; this answers some steps and leaves the rest to
  their defaults. Presets go through the same coercion and validation as
  defaults, so an invalid one is an error naming the key.
- `min_select` / `max_select` on a multiselect, `allow_custom` on the steps
  whose options are suggestions, `return_column` so a selectable table answers
  with a cell instead of a row index.

- **Live interactive prompts.** On a real terminal `select`, `multiselect`,
  `search` and `autocomplete` draw a redrawing frame: arrows move a cursor,
  typing filters, space toggles. Built directly on the terminal — no TUI
  framework, no new dependency. 5.9 ns and zero allocations per keystroke, one
  write per frame, and no CPU at all between keystrokes. `-plain` / `TINY_PLAIN`
  opts out; a pipe or CI never enters it. `TestRichAnswersMatchLineMode` pins
  that both ways of choosing return the same value.
- **A wizard as arguments**: `tiny select framework SwiftUI UIKit + confirm git
  yes`. Fewer tokens than gum from the first question. Sugar over the same
  parser, pinned by an equivalence test against the YAML it stands for.
- **Conditional steps** (`when:`) with six forms. A skipped step contributes no
  key, skipping is transitive, and a forward reference is a load error.
- **Declarative validation**: `pattern`, `min_length`, `max_length`, `message` —
  and the step's default is checked against its own rule when the spec loads.
- **`${key}` interpolation** of earlier answers into titles, defaults, options.
- `tiny -` reads the spec from stdin; `-compact`, `-require-tty`, `-plain`,
  `height:` on option lists, multi-line `editor` entry without `$EDITOR`.

### Fixed

- Where there is no terminal, every step took its default **silently**, handing
  the caller a well-formed payload of answers nobody gave. tiny now says so, and
  `-require-tty` makes it a hard failure (exit 3).
- The `editor` step read a single line without `$EDITOR`, making multi-line text
  impossible in the step that exists for it.

### Performance

- The JSON encoder is hand-written; `encoding/json` is gone from the binary
  (-152 KB), pinned byte-for-byte by a fuzzed parity test against it.
- Case-insensitive matching scans with `EqualFold` instead of lowercasing:
  `select` 25 → 2 allocs/op, `multiselect` 55 → 5 with a default.

## v0.2.0

The release that makes tiny embeddable: the payload is finally separable from
the UI, and the UI is finally yours.

### Added

- **Themes.** Every wizard is drawn through a theme — symbols (prompt marker,
  header decoration, list markers, bullets, table rule) and styles (attributes,
  the 8 base colors and their bright variants, backgrounds, 256-color,
  truecolor). Five presets ship in the binary: `default`, `minimal`, `sleek`,
  `ascii`, `plain`. See `THEMING.md`.
- Themes come from four layered sources, later wins: `preset:`, the wizard's own
  `theme:` block, `$TINY_THEME`, `-theme`. A wizard author suggests a look; the
  person running it decides.
- `-theme <preset|file>`, `-no-color`, `-list-themes`, `-o <file>` flags.
- **`tiny -` reads the spec from stdin**, so a caller composing a question no
  longer has to invent a temp file. The answers then come from `/dev/tty`, and
  fall back to defaults where there is no terminal.
- `$TINY_THEME` and `$NO_COLOR` support ([no-color.org](https://no-color.org)).
- `themes/brand.yaml`, a documented template for fitting tiny into another CLI,
  and `themes/dracula.yaml` as a truecolor example.
- `AGENT-GUIDE.md`: how a coding agent should call tiny, with a block to paste
  into a project's `CLAUDE.md` / `AGENTS.md`.
- `examples/scaffold.sh`, a working consumer that generates a spec, runs it,
  parses the JSON and acts on the answers — interactively or with `--ci`.
- `examples/themed.yaml`, a wizard that ships its own look.
- `TestThemeParity`: the same spec under every preset must return identical
  answers. Theming can never change output.
- CI across Linux/macOS/Windows, plus release binaries for five platforms.

### Fixed

- **The wizard no longer writes to stdout.** Prompts, lists and errors go to
  stderr; stdout carries only the serialized answers. Before this, the UI and
  the payload were interleaved on stdout, so `tiny w.yaml | jq` and
  `answers=$(tiny w.yaml)` were impossible in interactive mode — the whole
  agent use case only worked with `-defaults`.
- **`-format env` values are shell-quoted.** They were emitted raw, so an answer
  containing a space broke `eval`, and an answer containing `;` or `$(...)`
  executed in the caller's shell. Values outside the safe character set are now
  single-quoted with embedded quotes escaped.
- **`-format env` keys are normalised.** Wizard keys may contain `.` and `-`,
  which no shell accepts in a variable name; they become `_`. Two keys that
  would collide are now an error instead of a silently dropped answer.
- **Piped runs are readable again.** With a non-TTY stdin nothing closed the
  prompt line, so anything printed after an answer — the `search` and
  `autocomplete` match lists, every validation error — ran into the prompt on
  the same line. The line is now closed the way a terminal's Enter closes it, so
  a piped transcript looks like an interactive one.
- `multiselect` marks the options its default preselects, instead of showing an
  unmarked list.
- The `number` prompt no longer runs its range hint into the caret.

### Performance

- The JSON encoder is hand-written and `encoding/json` is gone from the binary
  (-152 KB, 5% of the shipped size). Output is byte-identical, pinned by a
  fuzzed parity test that checks against `encoding/json` itself — imported by
  the test only. The fuzzing caught four escapes a hand-rolled encoder gets
  wrong, including invalid UTF-8, which would have emitted invalid JSON.
- Case-insensitive option matching uses `strings.EqualFold` instead of
  lowercasing both sides, and the lookup maps in `select`/`multiselect` are
  gone: `select` 25 → 2 allocs/op, `multiselect` 27 → 4, and 55 → 5 when the
  step has a default.
- Rendering a step allocates nothing: the render helpers append into a reusable
  buffer and write once. Header, option list and table are all 0 allocs/op.

### Changed

- Every input point shows the theme's prompt marker, `confirm` included
  (`> (y/n) [y]: `). Previously `confirm` was the one widget without one.
- The README's binary size claim was wrong: it is ~3 MB, not ~5 MB.

## v0.1.0

First release: 12 step types, the YAML schema, JSON/YAML/env output, and the
headless/interactive parity guarantee.
