package widgets_test

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// themedEnv builds an Env drawing with the given theme and reading from input.
func themedEnv(t *testing.T, th theme.Theme, colorCapable bool, input string) (*widgets.Env, *bytes.Buffer) {
	t.Helper()
	resolved, err := theme.Resolve(th, colorCapable)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	var out bytes.Buffer
	return &widgets.Env{
		In:    bufio.NewReader(strings.NewReader(input)),
		Out:   &out,
		Theme: resolved,
	}, &out
}

func TestPrintHeaderUsesTheme(t *testing.T) {
	env, out := themedEnv(t, theme.Default(), false, "")
	widgets.PrintHeader(env, "Framework")
	if got, want := out.String(), "\n— Framework —\n"; got != want {
		t.Errorf("PrintHeader = %q, want %q", got, want)
	}

	custom := theme.Theme{Symbols: theme.Symbols{
		HeaderPrefix: strPtr("\n>> "),
		HeaderSuffix: strPtr(" <<\n"),
	}}
	env, out = themedEnv(t, custom, false, "")
	widgets.PrintHeader(env, "Framework")
	if got, want := out.String(), "\n>> Framework <<\n"; got != want {
		t.Errorf("themed PrintHeader = %q, want %q", got, want)
	}
}

func TestPrintListUsesTheme(t *testing.T) {
	env, out := themedEnv(t, theme.Default(), false, "")
	widgets.PrintList(env, []string{"Go", "Rust"}, 1)
	want := "    1) Go\n  > 2) Rust\n"
	if got := out.String(); got != want {
		t.Errorf("PrintList = %q, want %q", got, want)
	}

	custom := theme.Theme{Symbols: theme.Symbols{
		Indent:        strPtr(""),
		DefaultMarker: strPtr("*"),
		ItemMarker:    strPtr("-"),
		Bullet:        strPtr("."),
	}}
	env, out = themedEnv(t, custom, false, "")
	widgets.PrintList(env, []string{"Go", "Rust"}, 0)
	want = "* 1. Go\n- 2. Rust\n"
	if got := out.String(); got != want {
		t.Errorf("themed PrintList = %q, want %q", got, want)
	}
}

func TestPrintListColored(t *testing.T) {
	env, out := themedEnv(t, theme.Default(), true, "")
	widgets.PrintList(env, []string{"Go"}, 0)
	got := out.String()
	if !strings.Contains(got, "\x1b[36m") {
		t.Errorf("PrintList = %q, want the marker painted cyan", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("PrintList = %q, want the style reset", got)
	}
}

func TestPrintErrorAndNote(t *testing.T) {
	sleek, _ := theme.Preset("sleek")
	env, out := themedEnv(t, sleek, false, "")
	widgets.PrintError(env, "Nope.")
	widgets.PrintNote(env, "Type something.")
	want := "✗ Nope.\n· Type something.\n"
	if got := out.String(); got != want {
		t.Errorf("error+note = %q, want %q", got, want)
	}
}

func TestPromptMarkerIsThemed(t *testing.T) {
	custom := theme.Theme{Symbols: theme.Symbols{Prompt: strPtr("❯ ")}}
	env, out := themedEnv(t, custom, false, "")
	widgets.PrintPrompt(env, "(y/n) ")
	if got, want := out.String(), "❯ (y/n) "; got != want {
		t.Errorf("PrintPrompt = %q, want %q", got, want)
	}
}

// A theme must reach the widgets themselves, not just the helpers: the select
// widget renders its header, list and prompt through the env's theme.
func TestSelectWidgetHonoursTheme(t *testing.T) {
	sleek, _ := theme.Preset("sleek")
	env, out := themedEnv(t, sleek, false, "2\n")
	step := spec.Step{
		Type:    spec.TypeSelect,
		Key:     "lang",
		Title:   "Language",
		Options: []string{"Go", "Rust"},
	}
	got, err := widgets.RunStep(context.Background(), step, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Rust" {
		t.Errorf("answer = %v, want Rust", got)
	}
	ui := out.String()
	for _, want := range []string{"\nLanguage\n", "❯ ", "○ 1. Go"} {
		if !strings.Contains(ui, want) {
			t.Errorf("select UI = %q, want it to contain %q", ui, want)
		}
	}
}

// With a non-TTY stdin nothing echoes the user's Enter, so the widget has to
// close the prompt line itself — otherwise a match list or an error message
// runs into the prompt on the same line.
func TestPipedInputClosesThePromptLine(t *testing.T) {
	env, out := themedEnv(t, theme.Default(), false, "zzz\n1\n")
	step := spec.Step{
		Type:    spec.TypeSearch,
		Key:     "owner",
		Title:   "Owner",
		Options: []string{"alice", "bob"},
	}
	if _, err := widgets.RunStep(context.Background(), step, env); err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if strings.Contains(out.String(), "> No matches") {
		t.Errorf("UI = %q, want the message on its own line, not glued to the prompt", out.String())
	}
	if !strings.Contains(out.String(), "> \nNo matches") {
		t.Errorf("UI = %q, want the prompt line closed before the message", out.String())
	}
}

// A zero Env (no theme) must still render, using the built-in default.
func TestZeroEnvUsesDefaultTheme(t *testing.T) {
	var out bytes.Buffer
	env := &widgets.Env{In: bufio.NewReader(strings.NewReader("")), Out: &out}
	widgets.PrintHeader(env, "Title")
	if got, want := out.String(), "\n— Title —\n"; got != want {
		t.Errorf("PrintHeader = %q, want the default theme %q", got, want)
	}
}

func strPtr(s string) *string { return &s }
