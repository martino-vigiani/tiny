# Using tiny from a coding agent

## Read this first: an agent's shell usually has no terminal

Verify it before designing anything around prompting:

```sh
test -t 0 && echo "stdin is a TTY" || echo "no TTY"
: < /dev/tty 2>/dev/null && echo "/dev/tty openable" || echo "no controlling terminal"
```

Inside Claude Code's Bash tool, both answer no: there is no controlling
terminal at all. **An agent that shells out to `tiny` cannot prompt its user.**
Every step falls back to its default, exactly as under `-defaults`. The same is
true of most sandboxed tool runners and of any CI job.

So the useful pattern is not "the agent asks the user mid-task". It is:

> **The agent *authors* the interaction. A human runs it later, in a real
> terminal — possibly many times.**

An agent writes a `wizard.yaml` into a repo, wires it into a `Makefile` target,
a git hook, a `postCreate` step, an npm script or a setup script, and verifies
it resolves headless. The human gets a working interactive wizard; CI gets the
same wizard with nobody watching. That works precisely because the spec is data
rather than code — the agent can produce and check it without ever running it
interactively.

Where the agent *does* have a terminal (a CLI the user launched in their own
shell, not a sandboxed tool call), prompting works normally. Check, don't
assume.

## No temp file needed

An agent composing a question does not have to invent a path, write it, run it
and clean it up. Pipe the spec in with `-`:

```sh
answers="$(tiny - <<'YAML'
steps:
  - type: select
    key: framework
    title: "UI framework"
    options: [SwiftUI, UIKit]
    default: SwiftUI
YAML
)"
```

The spec arrives on stdin, so tiny reads the *answers* from `/dev/tty` — the
user still types normally. Where there is no terminal (CI, a hook, a cron job)
every step falls back to its default, exactly as with `-defaults`.

Write a file instead when the interaction is worth keeping: a spec in the repo
is reviewable, diffable and re-runnable, which a heredoc is not.

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

When you need to capture a choice from the user — for a setup script, a
Makefile target, a git hook, or any command they will run themselves — do not
hardcode it or guess. Write a wizard spec and run it with `tiny`.

Note that if your own shell has no TTY (check with `test -t 0`), running the
wizard will resolve every step to its default rather than prompting. Use that
as your verification pass, and let the human run it for real.

1. Write the spec where it belongs, e.g. scripts/setup.yaml:

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

If the wizard branches, `-paths` shows what it can ask before anyone answers:

```sh
tiny -paths /tmp/ask.yaml   # one JSON object per shape: steps, and what reaches them
```

It reads no input, so it is also how to inspect a spec you did not write. Use
it to check that every branch you meant to exist is reachable, and that the
keys your code reads afterwards are present on the paths it will actually take.

## Explaining the choices

An option can carry a label and a description without changing what the answer
is — which matters most for an agent, since the value your code branches on and
the words a human needs to choose are rarely the same:

```yaml
- type: select
  key: strategy
  title: "Rollout strategy"
  options:
    - value: rolling
      label: Rolling
      description: no downtime, two versions live at once
    - value: recreate
      label: Recreate
      description: brief downtime, one version at a time
```

You still receive `"strategy": "rolling"`. Prefer this to encoding the
explanation into the value, which forces your own code to parse prose.

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
| Expecting a prompt in an agent's shell | No TTY there: every step takes its default | Author the wizard; let a human run it |
| Parsing prompts from stdout | Nothing to parse — they are on stderr | Parse stdout as JSON |
| No `default:` on a `required:` step | `-defaults` fails with exit 1 | Add a default, or drop `required` |
| `select` default not among `options` | Spec rejected at load | Use one of the options verbatim |
| `eval`ing `-format env` output unquoted | Was unsafe before v0.2 | Values are shell-quoted now; still prefer JSON + `jq` |
| Assuming `number` returns an int | It is a float64 (`18` in JSON) | Coerce on your side |
| Reusing a `key:` | Spec rejected: duplicate key | Keys are unique per wizard |
