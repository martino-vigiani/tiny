# Using tiny from a coding agent

A coding agent — Claude Code, Codex, opencode, Cursor, an Agent SDK script —
cannot ask its user a structured question. It can print text and hope, or it can
guess. `tiny` closes that gap: the agent **writes a YAML file**, **runs one
command**, and **gets JSON back**.

```
agent writes wizard.yaml  →  tiny renders it  →  agent reads JSON from stdout
```

The same file runs unattended in CI (`-defaults`), so an interaction an agent
designs for a human also works in a pipeline with nobody watching.

## The contract, in five lines

```sh
tiny wizard.yaml              # prompts on stderr, answers as JSON on stdout
tiny -defaults wizard.yaml    # no interaction, every step resolves to its default
tiny -format env wizard.yaml  # KEY=VALUE, shell-quoted and safe to eval
tiny -o answers.json w.yaml   # write the payload to a file instead of stdout
echo $?                       # 0 ok · 1 execution/spec error · 2 usage error
```

**stdout carries only the payload.** The wizard is drawn on stderr, so an agent
can capture the answers with a plain command substitution while the human sees
the questions:

```sh
answers="$(tiny wizard.yaml)"   # UI still visible; $answers is pure JSON
```

## Copy this into your project's agent instructions

Paste the block below into your `CLAUDE.md`, `AGENTS.md` or system prompt. It is
all an agent needs to use `tiny` correctly without reading the full spec.

```markdown
## Asking the user a structured question

When you need input from the user — a choice, a name, a confirmation, a set of
options — do not print a question and guess the answer. Write a wizard spec and
run `tiny`:

1. Write the spec to a temp file, e.g. /tmp/ask.yaml:

   steps:
     - type: select          # select|multiselect|confirm|input|password|search|
       key: framework        # number|date|keyvalue|editor|autocomplete|table
       title: "UI framework" # shown to the user
       options: [SwiftUI, UIKit]
       default: SwiftUI      # required if the step must work in CI

2. Run it and parse stdout as JSON:

   tiny /tmp/ask.yaml

   Output: {"framework": "SwiftUI"}

3. Keys in the JSON are exactly the `key:` fields. Types are fixed:
   select/input/password/search/editor/autocomplete → string,
   multiselect → array of strings, confirm → bool, number → number,
   date → string, keyvalue → object, table → the selected row index.

Rules:
- Give every step a `default:` so `tiny -defaults` works unattended.
- Never parse the prompts: they go to stderr and their wording is themable.
- Exit code 1 means the spec was invalid — read stderr, fix the YAML, retry.
- The full schema is in SPEC.md; verify a generated spec with
  `tiny -defaults /tmp/ask.yaml` before showing it to the user.
```

## Verify before you ask

A generated spec should never fail in front of the user. `-defaults` validates
the whole file and resolves every step without reading stdin, which makes it a
free dry run:

```sh
tiny -defaults /tmp/ask.yaml >/dev/null || echo "spec is broken"
```

Exit code 1 with a message on stderr means the spec is wrong — the error names
the offending step key, so it is enough to fix it and retry.

## Matching the host tool's look

An agent embedded in someone else's CLI should not make that CLI suddenly look
like `tiny`. Point `$TINY_THEME` at a theme once and every wizard the agent
generates inherits it:

```sh
export TINY_THEME="$PWD/themes/brand.yaml"
```

See [THEMING.md](THEMING.md). Themes cannot change answers, so restyling a
wizard can never break the code that consumes its JSON.

## A complete example

`examples/scaffold.sh` is a real consumer: it generates a spec, runs it, parses
the JSON with `jq`, and acts on the answers — and the same script runs headless
with `--ci`. It is the shortest honest illustration of the whole contract.

## Common mistakes

| Mistake | What happens | Fix |
| --- | --- | --- |
| Parsing prompts from stdout | Nothing to parse — they are on stderr | Parse stdout as JSON |
| No `default:` on a `required:` step | `-defaults` fails with exit 1 | Add a default, or drop `required` |
| `select` default not among `options` | Spec rejected at load | Use one of the options verbatim |
| `eval`ing `-format env` output unquoted | Was unsafe before v0.2 | Values are shell-quoted now; still prefer JSON + `jq` |
| Assuming `number` returns an int | It is a float64 (`18` in JSON) | Coerce on your side |
| Reusing a `key:` | Spec rejected: duplicate key | Keys are unique per wizard |
