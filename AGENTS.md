# AGENTS.md — Contributing to tiny

Guidance for AI agents (and humans) building, testing and extending the `tiny` codebase.
This file is for modifying the Go code. If you are *using* tiny instead:

- generating a `wizard.yaml` → `SPEC.md`
- calling tiny from an agent or a script → `AGENT-GUIDE.md`
- restyling the wizard → `THEMING.md`

## Build & verify

```sh
go build ./...
go vet ./...
go test ./...
go test -bench . -benchmem ./...   # rendering must stay allocation-free
gofmt -l .                         # must print nothing
go build -trimpath -ldflags "-s -w" -o /tmp/tiny ./cmd/tiny
```

The project is a single Go module (`github.com/martino-vigiani/tiny`) with no external
runtime dependencies. Do not run `go mod tidy` unless you actually changed imports.

## Architecture

```
cmd/tiny/main.go        Entry point: flag parsing, spec read (file or stdin via
                        "-"), spec.Parse, theme resolution, the answer source,
                        signal handling, exit codes, payload writing.
internal/spec/          Single source of truth for the schema and validation:
                        Wizard/Step structs, Validate(), and the default-resolution
                        helpers shared by widgets and the headless runner.
internal/theme/         The look: Theme (declarative YAML form), the built-in presets,
                        Merge/Expand for layering, and Resolve, which compiles a theme
                        into the exact strings widgets print.
internal/widgets/       Widget contract (registry + Register/RegisterTest) plus the 12
                        widgets (one file each), the themed render helpers and shared
                        I/O helpers.
internal/runner/        Execution: runner.go (Run + JSON/YAML/env serializers),
                        headless.go (RunDefaults + defaultForStep).
```

Layering: `cmd/tiny` → `runner` → `widgets` + `spec` → `theme`. `widgets` depend on `spec`
and `theme` but never on `runner`; `spec` depends only on `theme` (for the inline `theme:`
block); `theme` is a leaf with no imports from the rest of the project.

## The two stream rules

1. **`Env.Out` is the UI stream and carries nothing else.** `cmd/tiny` points it at
   stderr and serializes the answers to stdout separately, which is what makes
   `tiny w.yaml | jq` work while a human answers. A widget that writes an answer, a
   diagnostic or a stray newline to any other destination breaks the contract; the
   end-to-end test `TestStdoutCarriesOnlyThePayload` guards it.
2. **Everything a widget draws goes through the theme.** Never `fmt.Fprint(env.Out, ...)`
   directly: use `PrintHeader`, `PrintList`, `PrintPrompt`, `PrintError(f)`,
   `PrintNote(f)` and `Hint` from `widgets.go`. They render through `env.look()`, so a
   hardcoded symbol silently opts that widget out of theming.

## Mandatory conventions

- **User-facing messages are in English**, start with a capital letter and end with a
  period (e.g. `This field is required.`, `Invalid choice, pick a number between 1 and N.`).
- **Prompt marker:** every widget that asks for input calls `PrintPrompt`, which draws the
  theme's marker (`> ` by default). Anything specific to the widget is passed as the
  `extra` argument and follows the marker: confirm passes `(y/n) [y]: `, number its range
  hint, date its format. Passwords never print the value.
- **Headers:** every widget prints its section title exactly once via `PrintHeader`
  (`\n— <title> —\n` under the default theme). Do not print the title anywhere else.
- **Errors vs notes:** `PrintError` is for a rejected answer ("This field is required."),
  `PrintNote` for an instruction ("Enter key=value pairs, one per line"). They carry
  different prefixes and colors, so picking the wrong one is a visible bug.
- **Canonical return types** per step kind (widgets must return exactly these Go types;
  the runner serializes them and the parity test compares them):

  | Type            | Go type              |
  |-----------------|----------------------|
  | select          | `string`             |
  | multiselect     | `[]string`           |
  | confirm         | `bool`               |
  | input           | `string`             |
  | password        | `string`             |
  | search          | `string`             |
  | number          | `float64`            |
  | date            | `string`             |
  | keyvalue        | `map[string]string`  |
  | editor          | `string`             |
  | autocomplete    | `string`             |
  | table           | `int` (selectable) or `nil` |

- **EOF policy (non-TTY):** on EOF with no pending data a widget applies the default; if
  the step is `required` and has no default it returns
  `unexpectedEOF` (`unexpected end of input for required step "<key>"`); otherwise it
  returns the type's zero value. Use the shared `ReadInput`/`ReadLine`/`ReadPassword`
  helpers in `widgets.go`, never read the stream directly.
- **Headless == interactive invariant:** `RunDefaults` (headless) and `Run` with an empty
  stdin (EOF) must produce the identical answer map. This is enforced by
  `TestDefaultsInteractiveParity` in `internal/runner/parity_test.go`. Any behavioral
  change must keep this test green.
- **Default helpers live in `internal/spec`** (single source of truth, reused by widgets
  and headless): `SplitMultiDefault`, `ParseConfirmDefault`, `ToFloat`, `ToStringMap`,
  `ValidateDateString`, `OrderByOptions`, `CanonicalOption`. Widgets must call these
  helpers instead of re-implementing default semantics.

## Adding a new widget type (step-by-step)

1. **`internal/spec/spec.go`**: add a `TypeX` constant to the `StepType` const block and a
   validation branch in `Step.validate()`. If the type needs new step fields, add them to
   the `Step` struct.
2. **`internal/widgets/<type>.go`**: implement the widget (`Run(ctx, step, env) (any, error)`)
   and register it in `init()` with `Register(spec.TypeX, widget{})`. Return the canonical
   Go type and follow the EOF policy and prompt conventions above.
3. **`internal/runner/headless.go`**: add a case for the new type in `defaultForStep`
   (or the shared branches), returning the same canonical value the interactive widget
   produces at EOF-with-default.
4. **Tests:** add a step of the new type to `parityWizard()` in `parity_test.go` (this
   extends `TestDefaultsInteractiveParity`) and add a widget test file
   `internal/widgets/<type>_test.go` covering interactive input, EOF, and validation.
5. **Docs:** document the type in `SPEC.md` (when to use, fields, input, return type,
   default semantics, validation) and add it to the 12-type list in `README.md`.

## Adding a theme symbol or style

A new drawable element needs all five of these, or it will be invisible to theme authors:

1. **`internal/theme/theme.go`**: add the field to `Symbols` or `Styles` (a `*string`, so
   Merge can tell "unset" from "set to empty"), give it a value in the `default` preset,
   list it in `Styles.all()` if it is a style (that is what validates it), and wire it into
   `Resolve` so it lands on `Resolved` with its escapes already attached.
2. **`mergeSymbols` / `mergeStyles`**: add the matching `pick(...)` line. Forgetting this
   makes the field silently un-overridable.
3. **`internal/widgets/widgets.go`**: use it from a render helper — never from a widget
   directly.
4. **Tests:** cover it in `internal/theme/theme_test.go` (resolution and inheritance) and
   in `internal/widgets/render_test.go` (that a custom value actually changes the output).
5. **Docs:** `THEMING.md` (the symbol table and the template in `themes/brand.yaml`) and
   the theme table in `SPEC.md`.

Presets are ordinary `Theme` values in the `presets` map and inherit through `preset:`, so
a new preset is one map entry plus a row in the `THEMING.md` table. `TestPresetsResolve`
picks it up automatically.

## Practical rules

- Do **not** run `go mod tidy` unless imports actually changed.
- `RegisterTest` / `RestoreTestRegistry` are **test-only hooks** (used by runner tests to
  simulate widgets); never call them in production code.
- `delimiter` on `Step` is parsed but unused; do not build on it without also implementing
  it and updating `SPEC.md`.
- Keep user-facing messages ending with a period (the table widget's error strings and
  `rangeMessage` are formatted this way on purpose).
- **Rendering is on the hot path and is allocation-free by design.** The render helpers
  append into the reusable `Env.buf` and write once; `Resolved` holds strings that are
  already concatenated with their ANSI escapes, so drawing a step does no formatting.
  Reintroducing `fmt.Sprintf` into a render helper undoes that — benchmark before and after
  with `go test -bench . -benchmem ./...`.
- **Case-insensitive matching scans with `strings.EqualFold`.** Lowering both
  sides allocates one string per option per comparison, in the helpers every
  widget calls; and a lowercase lookup map costs more to build than an option
  list of a handful of entries ever saves in lookups. `spec.CanonicalOption` and
  `widgets.indexFold` are the two scans — use them instead of adding a third.
- **A multiselect's default is resolved once.** `spec.MarkOptions` returns the
  `[]bool` mask, which both the list rendering and the answer are derived from
  (`spec.OptionsFor`). Resolving it separately for display and for the result is
  what made this widget the allocation outlier.
- `-format env` output is shell-quoted and its keys are normalised
  (`internal/runner/runner.go`). Answers are arbitrary user text: never emit one into a
  `KEY=VALUE` line without `shellQuote`.
