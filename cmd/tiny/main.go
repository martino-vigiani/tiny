// Command tiny reads a wizard YAML spec and runs it as an interactive
// terminal wizard, printing the collected answers in json/yaml/env format.
//
// The wizard is drawn on stderr and the answers go to stdout, so the payload
// stays pipeable while a human still sees the questions.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/keys"
	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
)

const version = "v0.3.0"

// repeatable collects a flag given more than once, in the order it was given.
type repeatable []string

func (r *repeatable) String() string     { return strings.Join(*r, ", ") }
func (r *repeatable) Set(v string) error { *r = append(*r, v); return nil }

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("tiny", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	useDefaults := fs.Bool("defaults", false, "run headless using step defaults (no interaction)")
	format := fs.String("format", "json", "output format: json|yaml|env")
	themeRef := fs.String("theme", "", "theme to draw with: a preset name or a path to a theme YAML file")
	noColor := fs.Bool("no-color", false, "disable ANSI colors (same as NO_COLOR=1)")
	output := fs.String("o", "", "write the answers to this file instead of stdout")
	listThemes := fs.Bool("list-themes", false, "print the built-in theme presets and exit")
	requireTTY := fs.Bool("require-tty", false, "fail instead of falling back to defaults when there is no terminal")
	useTTY := fs.Bool("tty", false, "read answers from /dev/tty, leaving stdin to whatever is already using it")
	compact := fs.Bool("compact", false, "print the JSON payload on one line")
	plain := fs.Bool("plain", false, "disable the interactive prompt; ask one line at a time")
	var sets repeatable
	fs.Var(&sets, "set", "pre-seed one answer as key=value (repeatable); other steps take their defaults")
	answersFile := fs.String("answers", "", "pre-seed answers from a JSON or YAML file of key: value")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n  tiny [flags] <wizard.yaml>\n"+
			"  tiny [flags] -                        read the spec from stdin\n"+
			"  tiny [flags] <type> <key> [value...]  write the wizard as arguments\n\n"+
			"    e.g. tiny select framework SwiftUI UIKit + confirm git yes\n"+
			"    types: %s\n\nFlags:\n", spec.ShortTypeNames())
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nThe wizard is drawn on stderr and the answers are written to stdout,\n"+
			"so `tiny w.yaml | jq` works while the user still sees the prompts.\n"+
			"\nEnvironment:\n"+
			"  TINY_THEME    theme preset name or path, overridden by -theme\n"+
			"  TINY_PLAIN    any value disables the live interactive prompt\n"+
			"  NO_COLOR      any value disables colors\n"+
			"\nSee README.md for the wizard YAML format and THEMING.md for themes;\n"+
			"ready-made specs live in examples/ and themes in themes/.\n")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Printf("tiny %s\n", version)
		return 0
	}
	if *listThemes {
		for _, name := range theme.PresetNames() {
			fmt.Println(name)
		}
		return 0
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	switch *format {
	case "json", "yaml", "env":
	default:
		fmt.Fprintf(os.Stderr, "tiny: invalid -format %q: must be json, yaml or env\n", *format)
		return 2
	}

	// A first argument naming a step type means the wizard was written as argv
	// words; anything else is a path, or "-" for stdin. flag.Parse stops at the
	// first non-flag argument, so the whole short form — its own --attributes
	// included — arrives untouched in fs.Args().
	var wiz *spec.Wizard
	var err error
	if spec.IsShortType(fs.Arg(0)) {
		wiz, err = spec.ParseShort(fs.Args())
		if err != nil {
			fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
			return 2
		}
	} else {
		if fs.NArg() != 1 {
			fs.Usage()
			return 2
		}
		data, readErr := readSpec(fs.Arg(0))
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "tiny: %v\n", readErr)
			return 1
		}
		wiz, err = spec.Parse(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tiny: invalid wizard: %v\n", err)
			return 1
		}
	}

	resolved, err := resolveTheme(wiz.Theme, *themeRef, *noColor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}

	// A preset file first, then -set on top, so the command line always wins
	// over a checked-in answer file.
	presets := runner.Preset{}
	if *answersFile != "" {
		loaded, loadErr := runner.LoadPresets(*answersFile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "tiny: %v\n", loadErr)
			return 1
		}
		for k, v := range loaded {
			presets[k] = v
		}
	}
	fromFlags, setErr := runner.ParsePresets(sets)
	if setErr != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", setErr)
		return 2
	}
	for k, v := range fromFlags {
		presets[k] = v
	}

	// Signals are handled by exiting, not merely by cancelling a context: the
	// line-based widgets block in a read that never observes cancellation, so a
	// context alone left tiny alive and still eating the user's keystrokes
	// after Ctrl-C. SIGHUP is included because a closed pane or a dropped ssh
	// session sends it, and the terminal it leaves behind outlives the process.
	ctx := context.Background()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-signals
		keys.RestoreAll()
		fmt.Fprintf(os.Stderr, "\ntiny: cancelled\n")
		os.Exit(4)
	}()

	in, closeIn, haveInput := answerSource(*useTTY || fs.Arg(0) == "-")
	defer closeIn()

	isTTY := term.IsTerminal(int(in.Fd()))
	if *requireTTY && !isTTY {
		fmt.Fprintf(os.Stderr, "tiny: no terminal available — every step would fall back to its default.\n"+
			"      Run this from a terminal, or pass -defaults to say so on purpose.\n")
		return 3
	}
	// Falling back to defaults because nobody is there to answer is a normal
	// mode, but a silent one: without this note a caller gets a well-formed
	// payload of answers no human ever gave.
	if !*useDefaults && len(presets) == 0 && !haveInput {
		fmt.Fprintf(os.Stderr, "tiny: no terminal and no input — every step falls back to its default.\n")
	}

	// The UI goes to stderr; stdout carries only the answers.
	ui := bufio.NewWriter(os.Stderr)
	r := runner.New(in, ui, int(in.Fd()), isTTY)
	r.SetTheme(resolved)
	r.SetRich(richCapable(isTTY, *plain))

	// Raw mode disables the terminal's own signal handling, so a terminal left
	// raw is a shell the user has to repair by hand. The signal handler above
	// restores it; this covers every ordinary exit path, panic included.
	defer keys.RestoreAll()

	var answers map[string]any
	switch {
	case len(presets) > 0:
		answers, err = r.RunPresets(wiz, presets)
	case *useDefaults:
		answers, err = r.RunDefaults(wiz)
	default:
		answers, err = r.Run(ctx, wiz)
	}
	if flushErr := ui.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}
	if err != nil {
		// A cancelled wizard is not a broken one. A gate that has to tell
		// "the user said no" from "the spec is wrong" can only do it by the
		// exit code, since both write to stderr.
		if errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "tiny: cancelled\n")
			return 4
		}
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}

	if err := writeAnswers(*output, *format, *compact, answers); err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}
	return 0
}

// readSpec reads the wizard spec from path, or from stdin when path is "-".
//
// Reading from stdin is what makes tiny usable without touching the disk: a
// caller — a script, or an agent that just composed a question — can pipe a
// heredoc straight in instead of inventing a temp file for every interaction.
func readSpec(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("cannot read the spec from stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return data, nil
}

// answerSource returns the file the wizard reads answers from.
//
// Normally that is stdin. When stdin is spoken for — the spec arrived on it, or
// the caller passed -tty because something else owns it, as a git pre-push hook
// owns its ref list — the answers come from the terminal directly.
// Where there is no terminal — CI, a cron job — there is nothing to read and
// every step falls back to its default, which is exactly what -defaults does.
// It also reports whether anything can actually be read: false means every step
// will silently take its default, which the caller warns about.
func answerSource(specOnStdin bool) (in *os.File, closeIn func(), haveInput bool) {
	if !specOnStdin {
		return os.Stdin, func() {}, true
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		f, closeF := emptyInput()
		return f, closeF, false
	}
	return tty, func() { tty.Close() }, true
}

// emptyInput returns a file that is already at EOF, so widgets apply their
// defaults instead of blocking on input that can never arrive.
func emptyInput() (*os.File, func()) {
	r, w, err := os.Pipe()
	if err != nil {
		// Nothing left to fall back to; stdin is spent but still a valid fd.
		return os.Stdin, func() {}
	}
	w.Close()
	return r, func() { r.Close() }
}

// writeAnswers serialises answers in the requested format to path, or to
// stdout when path is empty.
func writeAnswers(path, format string, compact bool, answers map[string]any) error {
	var w io.Writer = os.Stdout
	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("cannot write %s: %w", path, err)
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	var err error
	switch format {
	case "yaml":
		err = runner.PrintYAML(bw, answers)
	case "env":
		err = runner.PrintEnv(bw, answers)
	default:
		if compact {
			err = runner.PrintJSONCompact(bw, answers)
		} else {
			err = runner.PrintJSON(bw, answers)
		}
	}
	if err != nil {
		return err
	}
	return bw.Flush()
}

// resolveTheme layers the theme sources in precedence order — the wizard's own
// `theme:` block first, then $TINY_THEME, then -theme — so the person running
// the wizard can always override the author, and compiles the result.
//
// Colors are enabled when stderr is a capable terminal, unless a theme sets
// color: always/never or the user passes -no-color / NO_COLOR.
func resolveTheme(inline *theme.Theme, ref string, noColor bool) (*theme.Resolved, error) {
	t := theme.Default()
	if inline != nil {
		expanded, err := theme.Expand(*inline)
		if err != nil {
			return nil, err
		}
		t = theme.Merge(t, expanded)
	}
	for _, src := range []string{os.Getenv("TINY_THEME"), ref} {
		if src == "" {
			continue
		}
		loaded, err := loadTheme(src)
		if err != nil {
			return nil, err
		}
		expanded, err := theme.Expand(*loaded)
		if err != nil {
			return nil, err
		}
		t = theme.Merge(t, expanded)
	}
	if noColor || os.Getenv("NO_COLOR") != "" {
		t.Color = theme.ColorNever
	}
	return theme.Resolve(t, colorCapable())
}

// loadTheme resolves a theme reference: a built-in preset name, or a path to a
// YAML file (a standalone theme, or any wizard file with a `theme:` block).
func loadTheme(ref string) (*theme.Theme, error) {
	if preset, ok := theme.Preset(ref); ok {
		return &preset, nil
	}
	data, err := os.ReadFile(ref)
	if err != nil {
		if !strings.ContainsAny(ref, "/.\\") {
			return nil, fmt.Errorf("unknown theme %q: not a preset (%s) and not a readable file",
				ref, strings.Join(theme.PresetNames(), ", "))
		}
		return nil, fmt.Errorf("cannot read theme %s: %w", ref, err)
	}
	return theme.Parse(data)
}

// richCapable decides whether to draw the live, redrawing prompt.
//
// It needs a terminal on both ends — stdin to read keys from, stderr to draw on
// — and a TERM that means something. Anywhere else (a pipe, CI, an agent's
// sandboxed shell) the line-based path runs unchanged, which is what keeps the
// headless and interactive answers identical.
func richCapable(stdinIsTTY, plain bool) bool {
	if plain || os.Getenv("TINY_PLAIN") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	return stdinIsTTY && isTerminal(os.Stderr)
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// colorCapable reports whether the UI stream can render ANSI escapes: stderr
// must be a terminal and TERM must not be "dumb".
func colorCapable() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}
