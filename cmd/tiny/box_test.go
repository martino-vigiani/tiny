package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// testUI builds a UI with a buffer sink and the given theme preset, color off
// so width assertions are readable (escape sequences would otherwise occupy
// zero columns but still complicate substring checks).
func testUI(t *testing.T, preset string) (*UI, *bytes.Buffer) {
	t.Helper()
	th, ok := theme.Preset(preset)
	if !ok {
		t.Fatalf("no such preset %q", preset)
	}
	resolved, err := theme.Resolve(th, false)
	if err != nil {
		t.Fatalf("theme.Resolve: %v", err)
	}
	buf := &bytes.Buffer{}
	return &UI{
		Err:   buf,
		Theme: resolved,
		TTY:   false,
		Width: func() int { return 80 },
	}, buf
}

// forceStdinTerminal overrides stdinIsTerminal for the duration of the test,
// so the "typed at a keyboard" and "piped" branches of runBox can both be
// exercised without an actual tty.
func forceStdinTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return isTerminal }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

// withStdin replaces os.Stdin with a pipe fed with s for the duration of the
// test. runBox reads its body from the real os.Stdin because UI carries no
// input stream — box is the only display command that needs one.
func withStdin(t *testing.T, s string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		_, _ = w.WriteString(s)
		w.Close()
	}()
}

// lines returns the non-empty trailing-newline-stripped lines of out, in
// order — the rendered box, one row per line.
func boxLines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestBoxEveryLineSameWidth(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "line one\nline two is a bit longer\n")
	if code := runBox([]string{"Title"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	if len(ls) < 3 {
		t.Fatalf("got %d lines, want a bordered box: %q", len(ls), buf.String())
	}
	want := cellWidth(ls[0])
	for i, l := range ls {
		if got := cellWidth(l); got != want {
			t.Errorf("line %d (%q) = %d cells, want %d (line 0's width)", i, l, got, want)
		}
	}
}

func TestBoxCJKSquareEdges(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "日本語のテキストです\nさらに長い日本語のテキスト行がここにあります\n")
	if code := runBox([]string{"タイトル"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	if len(ls) < 3 {
		t.Fatalf("got %d lines: %q", len(ls), buf.String())
	}
	want := cellWidth(ls[0])
	for i, l := range ls {
		if got := cellWidth(l); got != want {
			t.Errorf("line %d (%q) = %d cells, want %d", i, l, got, want)
		}
	}
}

func TestBoxWrapsOnSpaces(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "one two three four five six seven eight nine ten\n")
	if code := runBox([]string{"", "--width", "20"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	// Every content line must fit inside the box width, and no line may have
	// been split mid-word (each wrapped line's trimmed content is made of
	// whole words from the input).
	for _, l := range ls {
		if strings.Contains(l, "onetwo") || strings.Contains(l, "twothree") {
			t.Errorf("word appears joined/split incorrectly: %q", l)
		}
	}
	if len(ls) < 4 {
		t.Errorf("expected wrapping to produce multiple content lines, got %d: %q", len(ls), ls)
	}
}

func TestBoxHardBreaksLongWord(t *testing.T) {
	ui, buf := testUI(t, "default")
	longWord := strings.Repeat("x", 100)
	withStdin(t, longWord+"\n")
	if code := runBox([]string{"", "--width", "20"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	want := cellWidth(ls[0])
	for i, l := range ls {
		if got := cellWidth(l); got != want {
			t.Errorf("line %d = %d cells, want %d (hard break must not overflow the box)", i, got, want)
		}
	}
	if len(ls) < 5 {
		t.Errorf("100 x's in a 20-wide box should hard-break into several lines, got %d: %q", len(ls), ls)
	}
}

func TestBoxTitleTruncatedWithEllipsis(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "body\n")
	longTitle := strings.Repeat("Very Long Title ", 10)
	if code := runBox([]string{longTitle, "--width", "24"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	if len(ls) == 0 {
		t.Fatal("empty output")
	}
	top := ls[0]
	if !strings.Contains(top, ui.Theme.Ellipsis) {
		t.Errorf("top border %q does not contain the theme ellipsis %q", top, ui.Theme.Ellipsis)
	}
	if got := cellWidth(top); got != 24 {
		t.Errorf("top border width = %d, want 24 (the requested --width)", got)
	}
}

func TestBoxASCIIBordersUnderAsciiAndPlainThemes(t *testing.T) {
	for _, preset := range []string{"ascii", "plain"} {
		ui, buf := testUI(t, preset)
		withStdin(t, "hi\n")
		if code := runBox([]string{"T"}, ui); code != 0 {
			t.Fatalf("[%s] runBox = %d, want 0", preset, code)
		}
		out := buf.String()
		for _, r := range out {
			if r > 0x7f {
				t.Errorf("[%s] output has non-ASCII rune %q, theme %q must draw with +-| only:\n%s", preset, r, preset, out)
				break
			}
		}
		if !strings.ContainsAny(out, "+") {
			t.Errorf("[%s] expected ASCII corner '+' somewhere in the box:\n%s", preset, out)
		}
	}
}

func TestBoxUnicodeBordersUnderDefaultTheme(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "hi\n")
	if code := runBox([]string{"T"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0", code)
	}
	out := buf.String()
	if !strings.ContainsAny(out, "┌┐└┘│─") {
		t.Errorf("expected box-drawing characters under the default theme:\n%s", out)
	}
}

func TestBoxBlankLinesSurvive(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "first\n\nthird\n")
	if code := runBox([]string{"T"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0", code)
	}
	ls := boxLines(buf.String())
	// Body lines are between the top and bottom border; one of them must be
	// entirely blank padding (border + spaces + border), reflecting the
	// blank line preserved from stdin.
	found := false
	for _, l := range ls[1 : len(ls)-1] {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			t.Errorf("body line %q trims to nothing at all — border characters lost", l)
		}
		inner := strings.Trim(l, "│| ")
		if inner == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a blank body row for the blank input line, got: %q", ls)
	}
}

func TestBoxNeutralisesANSIEscape(t *testing.T) {
	ui, buf := testUI(t, "default")
	// An OSC 52 clipboard-write sequence hidden in the body.
	evil := "before\x1b]52;c;ZXZpbA==\x07after"
	withStdin(t, evil+"\n")
	if code := runBox([]string{"T"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0", code)
	}
	out := buf.String()
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("output contains a raw ESC byte, escape sequence was not neutralised:\n%q", out)
	}
	ls := boxLines(out)
	want := cellWidth(ls[0])
	for i, l := range ls {
		if got := cellWidth(l); got != want {
			t.Errorf("line %d = %d cells, want %d; the stripped escape shifted the border", i, got, want)
		}
	}
}

func TestBoxNoStdinNoTitleIsUsageError(t *testing.T) {
	ui, buf := testUI(t, "default")
	withStdin(t, "")
	code := runBox(nil, ui)
	if code != 2 {
		t.Errorf("runBox with no title and empty stdin = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "tiny:") {
		t.Errorf("usage error output = %q, want a tiny: prefixed message", buf.String())
	}
}

func TestBoxAbsurdWidthIsUsageError(t *testing.T) {
	for _, w := range []string{"0", "2", "-5"} {
		ui, _ := testUI(t, "default")
		withStdin(t, "hi\n")
		code := runBox([]string{"T", "--width", w}, ui)
		if code != 2 {
			t.Errorf("--width %s = exit %d, want 2 (usage error, not a panic)", w, code)
		}
	}
}

func TestBoxNothingReachesStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	ui, _ := testUI(t, "default")
	withStdin(t, "hello\n")
	runBox([]string{"Title"}, ui)

	w.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	if out.Len() != 0 {
		t.Errorf("stdout got %q, want nothing — box must draw only to ui.Err", out.String())
	}
}

// TestBoxTerminalStdinIsNeverRead is the regression test for the hang: typed
// at a real keyboard, `tiny box "Title"` must not block waiting for an EOF a
// human is never going to send. os.Stdin is deliberately left as whatever the
// test binary's own stdin is (not replaced with a pipe) — if runBox tried to
// read it while stdinIsTerminal reports true, this test would hang exactly
// like the real bug did, rather than fail cleanly.
func TestBoxTerminalStdinIsNeverRead(t *testing.T) {
	forceStdinTerminal(t, true)
	ui, buf := testUI(t, "default")
	if code := runBox([]string{"Title"}, ui); code != 0 {
		t.Fatalf("runBox = %d, want 0; output: %s", code, buf.String())
	}
	ls := boxLines(buf.String())
	if len(ls) < 2 {
		t.Fatalf("expected an empty box (top + bottom border at least), got %q", buf.String())
	}
}

// TestBoxTerminalStdinNoTitleIsUsageError mirrors the piped case: with no
// title and stdin treated as a terminal (so nothing is read from it), there
// is still nothing to draw and box must fail the same way it does for an
// empty pipe.
func TestBoxTerminalStdinNoTitleIsUsageError(t *testing.T) {
	forceStdinTerminal(t, true)
	ui, buf := testUI(t, "default")
	if code := runBox(nil, ui); code != 2 {
		t.Errorf("runBox = %d, want 2; output: %s", code, buf.String())
	}
}

// TestBoxCombiningMarkMeasuresLikePrecomposed guards the runeCells ordering:
// the zero-width branch for combining diacriticals (U+0300-U+036F) must be
// checked before the `r < 0x1100` fast path, or a decomposed "e" + U+0301
// counts as two cells instead of one and a box containing it comes out one
// column narrow.
func TestBoxCombiningMarkMeasuresLikePrecomposed(t *testing.T) {
	precomposed := "caf\u00e9"      // "e with acute accent" as one precomposed rune
	decomposed := "caf" + "e\u0301" // "e" followed by a combining acute accent (U+0301)
	if precomposed == decomposed {
		t.Fatal("test fixture is broken: the two strings must differ")
	}
	if got, want := cellWidth(decomposed), cellWidth(precomposed); got != want {
		t.Errorf("cellWidth(decomposed) = %d, want %d (same as the precomposed form)", got, want)
	}
}
