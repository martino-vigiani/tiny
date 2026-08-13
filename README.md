# tiny

**tiny asks the questions your scripts need to ask.**

You have a script — a deploy, a scaffold, a migration. Before it does the work,
it has to ask a human a few things. tiny is the command your script calls to ask
them, and it hands the answers back as JSON.

<p align="center">
  <img src="docs/media/tiny-demo.gif" alt="tiny: a YAML file becomes a terminal wizard, then the same file answers itself headless in CI" width="100%">
</p>

## Where it sits

tiny is not a framework and does not run your program. It is a command you call
from inside your own script, like `jq` or `curl`:

```
your script  ──calls──▶  tiny  ──reads──▶  deploy.yaml   (the questions)
                          │
                          ├──▶ draws the questions on screen   (for the human)
                          └──▶ prints JSON  ──▶  back to your script
```

## The problem it solves

Here is the same script twice. First, the way everyone writes it:

```sh
# deploy.sh — before
read -p "Environment (dev/staging/prod): " env
case "$env" in dev|staging|prod) ;; *) echo "invalid"; exit 1 ;; esac

read -p "Version (vX.Y.Z): " version
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "invalid"; exit 1; }

read -p "Deploy now? [y/N] " ok
[ "$ok" = "y" ] || exit 1

# ...and now CI, where nobody is there to answer:
if [ "$1" = "--yes" ]; then
  env=staging          # every default, written a second time
  version="v0.0.0"     # and kept in sync by hand, forever
  ok=y
fi
```

The questions are code. You cannot read them without running them, and the
unattended path is a copy of the defaults that drifts the first time somebody
edits one and forgets the other.

Now the same thing with tiny. The questions move into a file:

```yaml
# deploy.yaml
steps:
  - type: select
    key: env
    title: "Environment"
    options: [dev, staging, prod]
    default: staging

  - type: input
    key: version
    title: "Version"
    default: "v0.0.0"
    pattern: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    message: "Use vX.Y.Z."

  - type: confirm
    key: deploy
    title: "Deploy now?"
    default: "no"
```

and the script becomes three lines:

```sh
# deploy.sh — after
answers="$(tiny deploy.yaml)" || exit 1

env="$(jq -r .env         <<<"$answers")"
version="$(jq -r .version <<<"$answers")"
[ "$(jq -r .deploy <<<"$answers")" = true ] || exit 1
```

The validation, the re-prompting, the arrow keys and the filtering come with the
file. And the unattended path is no longer a second copy of anything:

```sh
tiny -defaults deploy.yaml      # in CI: no terminal, no questions, same answers
```

**One file, two modes.** That is the whole idea. A human answers it; CI runs the
same file and gets the same shape of JSON, with no second code path to keep in
sync.

## When to use it, and when not

**Use it** when a script has to ask a human something *and* has to run
unattended somewhere else — CI, a cron job, a git hook, a teammate's machine.

**Don't** when there is nothing to ask, or when you are building a full-screen
application with panes and streaming output. tiny asks a question and exits; it
is a piece of your script, not the thing that owns the screen.

## Install

```sh
brew install martino-vigiani/tap/tiny
```

That is a ~3 MB static binary. No Go, no runtime.

Or pick a release asset for your OS from
[Releases](https://github.com/martino-vigiani/tiny/releases), or build it:

```sh
go install github.com/martino-vigiani/tiny/cmd/tiny@v0.4.0
# or from a checkout:
git clone https://github.com/martino-vigiani/tiny
cd tiny && go build -trimpath -ldflags "-s -w -X main.version=v0.4.0" -o tiny ./cmd/tiny
```

Then, to see everything it can do without writing a file:

```sh
tiny demo
```

## Try it in thirty seconds

No file needed — pipe the questions straight in:

```sh
tiny - <<'YAML'
steps:
  - {type: select, key: env, title: "Environment", options: [dev, staging, prod], default: staging}
  - {type: confirm, key: go, title: "Deploy now?", default: "no"}
YAML
```

The questions are drawn on stderr and the JSON goes to stdout, so you can pipe
the payload into `jq` while somebody is still answering the questions on screen.

A release-gate spec that a human answers and CI resolves with `-defaults` is
[`examples/ci-gate.yaml`](examples/ci-gate.yaml); the matching workflow is
[`examples/github-action.yml`](examples/github-action.yml).

A complete working script — spec, run, `jq`, act on the answers, plus a `--ci`
mode — is [`examples/scaffold.sh`](examples/scaffold.sh):

```sh
./examples/scaffold.sh          # interactive
./examples/scaffold.sh --ci     # unattended, same file
```

<p align="center">
  <img src="docs/media/scaffold.svg" alt="scaffold.sh running unattended with --ci and scaffolding a project tree" width="100%">
</p>

## What you can ask

Thirteen kinds of question: `select`, `multiselect`, `confirm`, `input`,
`password`, `search`, `autocomplete`, `number`, `date`, `keyvalue`, `editor`,
`table`, `file`.

A question can be asked only when an earlier answer calls for it:

```yaml
  - type: input
    key: ticket
    title: "Change ticket"
    when: "env == prod"        # not asked for dev or staging
```

Answers can be validated (`pattern`, `min_length`), interpolated into later
questions (`${version}`), pre-seeded from the command line (`-set env=prod`),
and restyled to match your own tool (`-theme`). All of it is in
**[SPEC.md](SPEC.md)**.

## It also draws

Four commands show something instead of asking, because a script that asks its
questions well should not then go silent for two minutes while it works.

**`tiny spin`** — put it in front of any slow command you already run. It draws a
spinner until that command finishes, then gets out of the way:

```sh
tiny spin "Building…" -- go build ./...
```

The command keeps working exactly as before: `tiny spin` exits with **its exit
code**, and lets **its stdout through untouched**, so you can wrap something in
the middle of a pipeline and nothing downstream notices.

```sh
tiny spin "Fetching" -- curl -s "$url" | jq .id    # jq still gets the payload
```

**`tiny bar`** — for a loop where *you* know the count. Each call redraws the
same line instead of printing one per iteration, so a hundred files do not
become a hundred lines of log:

```sh
n=0; total=$(ls *.png | wc -l)
for f in *.png; do
  optimise "$f"
  n=$((n + 1))
  tiny bar "$n" "$total" "images"
done
```

**`tiny say`** — one line in the same style as the questions, so your script's
own output does not look like it came from a different tool:

```sh
tiny say "Deployed $version to $env" --style header
tiny say "Nothing to do" --style note
tiny say "Rollback required" --style error
```

**`tiny box`** — for the thing the user must actually read before answering: a
plan, a list of what is about to be overwritten, a warning. A frame is what
keeps it from scrolling past unnoticed:

```sh
git diff --stat | tiny box "About to commit"
```

<p align="center">
  <img src="docs/media/display.svg" alt="say, spin, bar and box drawing in sequence in the terminal" width="100%">
</p>

Without a terminal — CI, a cron job, a log file — all four drop the animation
and print one plain line with no escape sequences at all. A build log full of
spinner frames is why people turn spinners off; here there is nothing to turn
off.

## Letting an agent write the questions

This is about **who writes the prompts**, not about an agent answering them.
Nothing here lets Claude Code or Codex ask *you* something mid-task — an agent's
shell has no terminal, so it cannot draw a prompt at all.

Ask an agent for interactive prompts in bash and it writes them blind:

```sh
read -p "Environment: " env      # the agent cannot run this to check it —
                                 # there is no terminal in its shell
```

It has no way to try them, so you are the one who finds out at the prompt that a
default is missing or a validation rejects a valid answer.

Ask for the same thing with tiny and it can check its own work:

```sh
tiny -defaults deploy.yaml       # resolves every question, reads no input,
                                 # fails loudly and names the broken step
```

The questions are a file, so writing them and proving they work are both things
an agent can do with no terminal. That is the whole scope of it.

It was tested rather than assumed: an agent was handed these docs, forbidden
from reading any source, and asked to build a release gate for an imaginary
project. It worked on the first run — and the three things it could not find in
the docs have been fixed.

Hand it **[AGENT-GUIDE.md](AGENT-GUIDE.md)**: the contract in one paragraph, a
block to paste into `CLAUDE.md` or `AGENTS.md`, and the mistakes to avoid.

## What it costs

Measured on this repository, not claimed:

- **3.0 MB** binary (`go build -trimpath -ldflags "-s -w"`).
- **2 direct dependencies** — `golang.org/x/term` and `gopkg.in/yaml.v3`.
- **5.8 MiB** peak resident memory, CPU time too small to register
  (`/usr/bin/time -l tiny -defaults examples/quickstart.yaml`).
- **342 tests**, all green (`go test -race ./...`).

## Compared to gum

[gum](https://github.com/charmbracelet/gum) does the styled-prompt part of this
well, and more of the general-purpose styling. The difference that matters here:
gum has no spec file and no headless mode. The questions stay inside your shell
script, so the unattended path is still yours to write and keep in sync.

## Reference

- **[SPEC.md](SPEC.md)** — every step type, every flag, conditions, `include:`,
  `-paths`, `-meta`.
- **[THEMING.md](THEMING.md)** — making it look like your own tool.
- **[AGENT-GUIDE.md](AGENT-GUIDE.md)** — using it from a coding agent.
- **[AGENTS.md](AGENTS.md)** — architecture and how to contribute.
- `tiny -h` — every flag, in the terminal.

Contributing: the check that has to pass before anything is merged is

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .
```

## License

MIT.
