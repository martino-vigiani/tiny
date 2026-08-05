package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// newSayTestUI builds a UI over a buffer instead of the real stderr, with a
// theme resolved for or against color the way ui.go says a caller must:
// theme.Resolve(theme.Default(), true/false).
func newSayTestUI(t *testing.T, color bool) (*UI, *bytes.Buffer) {
	t.Helper()
	resolved, err := theme.Resolve(theme.Default(), color)
	if err != nil {
		t.Fatalf("theme.Resolve: %v", err)
	}
	var buf bytes.Buffer
	return &UI{Err: &buf, Theme: resolved, TTY: false, Width: func() int { return 80 }}, &buf
}

// withSayInput swaps the reader say uses when there is no positional
// argument, and restores it — sayInput defaults to os.Stdin in production.
func withSayInput(t *testing.T, r io.Reader) {
	t.Helper()
	prev := sayInput
	sayInput = r
	t.Cleanup(func() { sayInput = prev })
}

// withSayStdinIsTerminal forces sayStdinIsTerminal's answer for a test,
// without needing a real tty on the machine running the suite.
func withSayStdinIsTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	prev := sayStdinIsTerminal
	sayStdinIsTerminal = func() bool { return isTerminal }
	t.Cleanup(func() { sayStdinIsTerminal = prev })
}

func TestSayStylesUseTheCorrespondingThemePair(t *testing.T) {
	ui, _ := newSayTestUI(t, true)
	cases := []struct {
		style       string
		open, close string
	}{
		{"header", ui.Theme.HeaderOpen, ui.Theme.HeaderClose},
		{"note", ui.Theme.NoteOpen, ui.Theme.NoteClose},
		{"error", ui.Theme.ErrorOpen, ui.Theme.ErrorClose},
		{"hint", ui.Theme.HintOpen, ui.Theme.HintClose},
		{"option", ui.Theme.OptionOpen, ui.Theme.OptionClose},
		{"table-header", ui.Theme.TableHeaderOpen, ui.Theme.TableHeaderClose},
		{"plain", "", ""},
	}
	for _, c := range cases {
		t.Run(c.style, func(t *testing.T) {
			_, buf := newSayTestUI(t, true)
			ui := &UI{Err: buf, Theme: ui.Theme, TTY: false, Width: func() int { return 80 }}
			if code := runSay([]string{"--style", c.style, "hi"}, ui); code != 0 {
				t.Fatalf("runSay returned %d", code)
			}
			want := c.open + "hi" + c.close + "\n"
			if got := buf.String(); got != want {
				t.Errorf("style %q: got %q, want %q", c.style, got, want)
			}
		})
	}
}

func TestSayDefaultStyleIsNote(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	if code := runSay([]string{"hi"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	want := ui.Theme.NoteOpen + "hi" + ui.Theme.NoteClose + "\n"
	if got := buf.String(); got != want {
		t.Errorf("default style: got %q, want %q", got, want)
	}
}

func TestSayReadsTextFromStdinWhenNoPositionalArg(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	withSayInput(t, strings.NewReader("piped text\n"))
	withSayStdinIsTerminal(t, false)
	if code := runSay(nil, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	want := ui.Theme.NoteOpen + "piped text" + ui.Theme.NoteClose + "\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A person who runs `tiny say` (or forgets the message after --style) at an
// interactive prompt must not hang: stdin is the keyboard, so io.ReadAll
// would block forever waiting for an EOF that never comes. This is a usage
// error, not a read.
func TestSayWithNoTextOnATerminalIsUsageError(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	withSayInput(t, strings.NewReader("would hang if read"))
	withSayStdinIsTerminal(t, true)
	code := runSay(nil, ui)
	if code != 2 {
		t.Fatalf("runSay returned %d, want 2", code)
	}
	if buf.Len() == 0 {
		t.Error("no usage message was printed")
	}
}

// Piping input still works when stdin is not a terminal, even though the
// same code path refuses to read a keyboard.
func TestSayReadsPipedStdinEvenThoughATerminalWouldBeRefused(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	withSayInput(t, strings.NewReader("from a pipe"))
	withSayStdinIsTerminal(t, false)
	if code := runSay([]string{"--style", "plain"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if got, want := buf.String(), "from a pipe\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSayStdinIsIgnoredWhenAPositionalArgIsGiven(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	withSayInput(t, strings.NewReader("should not be read"))
	if code := runSay([]string{"argv text"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if strings.Contains(buf.String(), "should not be read") {
		t.Errorf("stdin leaked into output despite a positional argument: %q", buf.String())
	}
}

func TestSayMultilineTextIsStyledOnEveryLine(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	if code := runSay([]string{"--style", "error", "line one\nline two\nline three"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	want := ui.Theme.ErrorOpen + "line one" + ui.Theme.ErrorClose + "\n" +
		ui.Theme.ErrorOpen + "line two" + ui.Theme.ErrorClose + "\n" +
		ui.Theme.ErrorOpen + "line three" + ui.Theme.ErrorClose + "\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An empty message is noise in a log — a styled line with a prefix and
// nothing after it — so say prints nothing rather than dressing up silence.
func TestSayEmptyTextPrintsNothingAndExitsZero(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	if code := runSay([]string{"--style", "error", ""}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if buf.Len() != 0 {
		t.Errorf("empty text produced output: %q", buf.String())
	}
}

// Text that is only a trailing newline is empty after trimming, and must be
// treated the same as an empty string, not as one blank line.
func TestSayTextThatIsOnlyANewlinePrintsNothing(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	withSayInput(t, strings.NewReader("\n"))
	withSayStdinIsTerminal(t, false)
	if code := runSay(nil, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if buf.Len() != 0 {
		t.Errorf("newline-only text produced output: %q", buf.String())
	}
}

// "--" stops flag parsing, so a message that happens to contain a token
// that looks like a flag is not swallowed by the parser.
func TestSayDoubleDashTerminatesFlagParsing(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	if code := runSay([]string{"--", "--style", "error"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	// No --style was recognised, so the default style (note) applies to the
	// literal words "--style error".
	want := ui.Theme.NoteOpen + "--style error" + ui.Theme.NoteClose + "\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A tab in piped text is not decoration to strip: say's input is build logs
// and commit messages, where a tab is often the column separator that keeps
// an aligned block readable. appendText (widgets.go) strips it because a
// wizard's title or option label never legitimately contains one; say's
// input does, so it is kept. See the comment on sanitize for the same
// rationale.
func TestSayPreservesTabsInText(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	if code := runSay([]string{"--style", "plain", "a\tb\tc"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if got, want := buf.String(), "a\tb\tc\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSayUnknownStyleIsUsageErrorListingValidStyles(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	code := runSay([]string{"--style", "success", "hi"}, ui)
	if code != 2 {
		t.Fatalf("runSay returned %d, want 2", code)
	}
	out := buf.String()
	for _, name := range sayStyleNames {
		if !strings.Contains(out, name) {
			t.Errorf("usage error does not list valid style %q: %q", name, out)
		}
	}
}

func TestSayEscapeSequenceInInputIsNeutralised(t *testing.T) {
	ui, buf := newSayTestUI(t, true)
	// An OSC 52 clipboard-write sequence disguised as ordinary text.
	malicious := "safe\x1b]52;c;ZXZpbA==\x07text"
	if code := runSay([]string{"--style", "plain", malicious}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("ESC byte reached the output: %q", got)
	}
	if strings.Contains(got, "\x07") {
		t.Errorf("BEL byte reached the output: %q", got)
	}
}

func TestSayNeverWritesStdout(t *testing.T) {
	ui, _ := newSayTestUI(t, true)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	code := runSay([]string{"hello"}, ui)
	w.Close()

	var captured bytes.Buffer
	if _, err := io.Copy(&captured, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	if code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	if captured.Len() != 0 {
		t.Errorf("say wrote to stdout: %q", captured.String())
	}
}

func TestSayWithNoThemeColorPrintsBareTextPlusPrefix(t *testing.T) {
	// sleek defines visible prefixes (unlike default, whose are empty), so
	// this is where "prefix survives -no-color, escapes do not" is visible.
	sleek, ok := theme.Preset("sleek")
	if !ok {
		t.Fatal("sleek preset not found")
	}
	resolved, err := theme.Resolve(sleek, false)
	if err != nil {
		t.Fatalf("theme.Resolve: %v", err)
	}
	var buf bytes.Buffer
	ui := &UI{Err: &buf, Theme: resolved, TTY: false, Width: func() int { return 80 }}

	if code := runSay([]string{"--style", "error", "Build failed"}, ui); code != 0 {
		t.Fatalf("runSay returned %d", code)
	}
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("colour escape leaked with color off: %q", got)
	}
	want := resolved.ErrorPrefix + "Build failed\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
