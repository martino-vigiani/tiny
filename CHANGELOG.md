# Changelog

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

### Changed

- Every input point shows the theme's prompt marker, `confirm` included
  (`> (y/n) [y]: `). Previously `confirm` was the one widget without one.
- The README's binary size claim was wrong: it is ~3 MB, not ~5 MB.

## v0.1.0

First release: 12 step types, the YAML schema, JSON/YAML/env output, and the
headless/interactive parity guarantee.
