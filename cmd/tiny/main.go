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

	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
)

const version = "v0.2.0"

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
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n  tiny [flags] <wizard.yaml>\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nThe wizard is drawn on stderr and the answers are written to stdout,\n"+
			"so `tiny w.yaml | jq` works while the user still sees the prompts.\n"+
			"\nEnvironment:\n"+
			"  TINY_THEME    theme preset name or path, overridden by -theme\n"+
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
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	switch *format {
	case "json", "yaml", "env":
	default:
		fmt.Fprintf(os.Stderr, "tiny: invalid -format %q: must be json, yaml or env\n", *format)
		return 2
	}

	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiny: cannot read %s: %v\n", fs.Arg(0), err)
		return 1
	}
	wiz, err := spec.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiny: invalid wizard: %v\n", err)
		return 1
	}

	resolved, err := resolveTheme(wiz.Theme, *themeRef, *noColor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The UI goes to stderr; stdout carries only the answers.
	ui := bufio.NewWriter(os.Stderr)
	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	r := runner.New(os.Stdin, ui, int(os.Stdin.Fd()), isTTY)
	r.SetTheme(resolved)

	var answers map[string]any
	if *useDefaults {
		answers, err = r.RunDefaults(wiz)
	} else {
		answers, err = r.Run(ctx, wiz)
	}
	if flushErr := ui.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}

	if err := writeAnswers(*output, *format, answers); err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}
	return 0
}

// writeAnswers serialises answers in the requested format to path, or to
// stdout when path is empty.
func writeAnswers(path, format string, answers map[string]any) error {
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
		err = runner.PrintJSON(bw, answers)
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

// colorCapable reports whether the UI stream can render ANSI escapes: stderr
// must be a terminal and TERM must not be "dumb".
func colorCapable() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}
