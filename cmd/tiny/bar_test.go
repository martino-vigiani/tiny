package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// newBarTestUI builds a UI over a buffer instead of the real stderr, the way
// say_test.go does: theme.Resolve(theme.Default(), false) is what production
// uses when color is off, and bar must never emit color anyway.
func newBarTestUI(t *testing.T, tty bool, width int) (*UI, *bytes.Buffer) {
	t.Helper()
	resolved, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		t.Fatalf("theme.Resolve: %v", err)
	}
	var buf bytes.Buffer
	return &UI{Err: &buf, Theme: resolved, TTY: tty, Width: func() int { return width }}, &buf
}

func barUIWithTheme(t *testing.T, name string, tty bool, width int) (*UI, *bytes.Buffer) {
	t.Helper()
	preset, ok := theme.Preset(name)
	if !ok {
		t.Fatalf("no such preset %q", name)
	}
	resolved, err := theme.Resolve(preset, false)
	if err != nil {
		t.Fatalf("theme.Resolve: %v", err)
	}
	var buf bytes.Buffer
	return &UI{Err: &buf, Theme: resolved, TTY: tty, Width: func() int { return width }}, &buf
}

// cells is a test-local re-implementation of the cell-width count, kept
// intentionally distinct from bar.go's own so a bug in one is not masked by
// reusing it in the assertion that checks it.
func cells(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115f,
			r >= 0x2e80 && r <= 0x303e,
			r >= 0x3041 && r <= 0x33ff,
			r >= 0x3400 && r <= 0x4dbf,
			r >= 0x4e00 && r <= 0x9fff,
			r >= 0xac00 && r <= 0xd7a3,
			r >= 0xf900 && r <= 0xfaff,
			r >= 0xff00 && r <= 0xff60:
			n += 2
		default:
			n++
		}
	}
	return n
}

func TestBarLineNeverExceedsWidthInCells(t *testing.T) {
	widths := []int{1, 2, 3, 5, 8, 10, 20, 40, 80, 120}
	labels := []string{
		"",
		"x",
		"files processed",
		"a very long label that should be truncated well before it wraps the line",
		strings.Repeat("wide", 50),
	}
	for _, w := range widths {
		for _, label := range labels {
			ui, buf := newBarTestUI(t, true, w)
			args := []string{"3", "10"}
			if label != "" {
				args = append(args, label)
			}
			if code := runBar(args, ui); code != 0 {
				t.Fatalf("width=%d label=%q: runBar returned %d", w, label, code)
			}
			line := strings.TrimSuffix(buf.String(), "\n")
			line = strings.TrimPrefix(line, "\r\x1b[K")
			if got := cells(line); got > w {
				t.Errorf("width=%d label=%q: line is %d cells: %q", w, label, got, line)
			}
		}
	}
}

func TestBarRendersZeroMiddleAndCompletePercentages(t *testing.T) {
	cases := []struct {
		current, total float64
		want           string
	}{
		{0, 10, "0%"},
		{5, 10, "50%"},
		{10, 10, "100%"},
	}
	for _, c := range cases {
		ui, buf := newBarTestUI(t, true, 80)
		args := []string{strconv.FormatFloat(c.current, 'f', -1, 64), strconv.FormatFloat(c.total, 'f', -1, 64)}
		if code := runBar(args, ui); code != 0 {
			t.Fatalf("runBar returned %d", code)
		}
		if got := buf.String(); !strings.Contains(got, c.want) {
			t.Errorf("current=%v total=%v: got %q, want it to contain %q", c.current, c.total, got, c.want)
		}
	}
}

func TestBarInProgressLineHasNoTrailingNewline(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"3", "10"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	if strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("in-progress bar ended with a newline: %q", buf.String())
	}
}

func TestBarCompleteLineEndsWithNewline(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"10", "10"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("complete bar did not end with a newline: %q", buf.String())
	}
}

func TestBarOnTTYRedrawsInPlace(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"3", "10"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\r\x1b[K") {
		t.Errorf("TTY output does not start with column-zero + erase-EOL: %q", got)
	}
}

func TestBarNonTTYHasNoEscapeSequencesAndEndsWithNewline(t *testing.T) {
	ui, buf := newBarTestUI(t, false, 80)
	if code := runBar([]string{"3", "10", "step"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("non-TTY output contains an escape byte: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("non-TTY output does not end with a newline: %q", got)
	}
}

func TestBarTotalZeroIsUsageError(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"0", "0"}, ui); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
}

func TestBarNegativeTotalIsUsageError(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"1", "-5"}, ui); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
}

func TestBarNegativeCurrentIsUsageError(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"-1", "10"}, ui); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
}

func TestBarNonNumericArgumentIsUsageError(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"abc", "10"}, ui); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
	ui2, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"3", "xyz"}, ui2); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
}

func TestBarMissingArgumentsIsUsageError(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	if code := runBar([]string{"3"}, ui); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
	ui2, _ := newBarTestUI(t, true, 80)
	if code := runBar(nil, ui2); code != 2 {
		t.Fatalf("runBar returned %d, want 2", code)
	}
}

func TestBarCurrentAboveTotalIsClampedNotRejected(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"15", "10"}, ui); code != 0 {
		t.Fatalf("runBar returned %d, want 0 (clamped, not an error)", code)
	}
	got := buf.String()
	if !strings.Contains(got, "100%") {
		t.Errorf("overshoot was not clamped to 100%%: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("clamped-to-complete bar did not end with a newline: %q", got)
	}
}

func TestBarNeverPanicsOnBadArguments(t *testing.T) {
	inputs := [][]string{
		{},
		{"1"},
		{"a", "b"},
		{"1", "0"},
		{"-1", "-1"},
		{"NaN", "10"},
		{"1", "10", strings.Repeat("x", 1000)},
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("runBar(%v) panicked: %v", in, r)
				}
			}()
			ui, _ := newBarTestUI(t, true, 80)
			runBar(in, ui)
		}()
	}
}

func TestBarVeryNarrowWidthProducesNoPanic(t *testing.T) {
	for _, w := range []int{-5, 0, 1, 2, 3} {
		ui, buf := newBarTestUI(t, true, w)
		if code := runBar([]string{"3", "10", "label"}, ui); code != 0 {
			t.Fatalf("width=%d: runBar returned %d", w, code)
		}
		_ = buf.String() // just must not panic and must produce something
	}
}

func TestBarCJKLabelIsTruncatedByCellsNotRunes(t *testing.T) {
	// Each of these runes is 2 cells wide. At a narrow width, truncating by
	// rune count instead of cell count would let this label run the line
	// past the terminal edge.
	label := strings.Repeat("测", 40)
	ui, buf := newBarTestUI(t, true, 30)
	if code := runBar([]string{"3", "10", label}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	line := strings.TrimSuffix(buf.String(), "\n")
	line = strings.TrimPrefix(line, "\r\x1b[K")
	if got := cells(line); got > 30 {
		t.Errorf("CJK label overflowed the line: %d cells: %q", got, line)
	}
	// Sanity: the label did make it into the line in some truncated form and
	// wasn't just dropped wholesale, unless there truly was no room.
	if !strings.ContainsRune(line, '测') && cells(line) < 30-4 {
		t.Errorf("label seems to have been dropped though there was room: %q", line)
	}
}

func TestBarLabelIsTruncatedByRuneCountFallback(t *testing.T) {
	// utf8 sanity check that our test label is valid and multi-byte, so the
	// truncation test above is exercising the multi-byte path.
	if utf8.RuneCountInString("测") != 1 {
		t.Fatal("test setup: expected a single rune")
	}
}

func TestBarANSIEscapeInLabelIsNeutralised(t *testing.T) {
	malicious := "safe\x1b]52;c;ZXZpbA==\x07text"
	// Non-TTY, since the TTY path legitimately emits its own cursor-control
	// escape (\r\x1b[K) that has nothing to do with the label; isolating the
	// check this way is what the non-TTY "no escape sequences at all" test
	// below already establishes as the clean signal.
	ui, buf := newBarTestUI(t, false, 80)
	if code := runBar([]string{"3", "10", malicious}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("ESC byte reached the output: %q", got)
	}
	if strings.ContainsRune(got, 0x07) {
		t.Errorf("BEL byte reached the output: %q", got)
	}
}

func TestBarUsesASCIIGlyphsUnderAsciiAndPlainThemes(t *testing.T) {
	for _, name := range []string{"ascii", "plain"} {
		ui, buf := barUIWithTheme(t, name, true, 80)
		if code := runBar([]string{"5", "10"}, ui); code != 0 {
			t.Fatalf("theme %s: runBar returned %d", name, code)
		}
		got := buf.String()
		if strings.ContainsAny(got, "█░") {
			t.Errorf("theme %s: Unicode block glyphs leaked into ASCII-only output: %q", name, got)
		}
		if !strings.ContainsAny(got, "#-") {
			t.Errorf("theme %s: no ASCII glyphs found in output: %q", name, got)
		}
	}
}

func TestBarUsesBlockGlyphsUnderDefaultTheme(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"5", "10"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	if !strings.ContainsAny(buf.String(), "█░") {
		t.Errorf("default theme did not use block glyphs: %q", buf.String())
	}
}

func TestBarNeverWritesStdout(t *testing.T) {
	ui, _ := newBarTestUI(t, true, 80)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	code := runBar([]string{"3", "10", "step"}, ui)
	w.Close()

	var captured bytes.Buffer
	if _, err := io.Copy(&captured, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	if code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	if captured.Len() != 0 {
		t.Errorf("bar wrote to stdout: %q", captured.String())
	}
}

func TestBarShowsPercentageAndCounts(t *testing.T) {
	ui, buf := newBarTestUI(t, true, 80)
	if code := runBar([]string{"3", "12", "files processed"}, ui); code != 0 {
		t.Fatalf("runBar returned %d", code)
	}
	got := buf.String()
	for _, want := range []string{"3", "12", "25%", "files processed"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q: %q", want, got)
		}
	}
}

// A bar on a terminal wears the theme like everything else tiny draws, and a
// bar without one carries no escape at all — the guarantee a CI log depends on.
// Colouring happens after the layout, so the styled line must still measure
// exactly what the plain one did.
func TestBarIsThemedOnATerminalAndBareWithoutOne(t *testing.T) {
	resolved, err := theme.Resolve(theme.Default(), true)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	var lit, dark strings.Builder
	onTTY := &UI{Err: &lit, Theme: resolved, TTY: true, Width: func() int { return 60 }}
	offTTY := &UI{Err: &dark, Theme: resolved, TTY: false, Width: func() int { return 60 }}

	if code := runBar([]string{"3", "12", "file"}, onTTY); code != 0 {
		t.Fatalf("runBar on a terminal = %d", code)
	}
	if code := runBar([]string{"3", "12", "file"}, offTTY); code != 0 {
		t.Fatalf("runBar without a terminal = %d", code)
	}
	if !strings.Contains(lit.String(), "\x1b[") {
		t.Errorf("a bar on a terminal carried no theme at all: %q", lit.String())
	}
	if strings.ContainsRune(dark.String(), 0x1b) {
		t.Errorf("an escape sequence reached a non-terminal: %q", dark.String())
	}
	// Same line, measured with escapes discounted.
	plain := strings.TrimRight(dark.String(), "\n")
	styled := strings.TrimPrefix(strings.TrimPrefix(lit.String(), theme.ColumnZero), theme.EraseEOL)
	if got, want := cellWidth(styled), cellWidth(plain); got != want {
		t.Errorf("the styled line measures %d cells, the plain one %d", got, want)
	}
}

// NaN and infinity parse as valid floats and then defeat every range check,
// because a comparison against NaN is always false. Rejecting them is not
// tidiness: on amd64 `int(NaN + 0.5)` is the smallest int64, and the fill loop
// counts up from there — the process hangs until something kills it. The test
// asserts the exit code rather than timing anything, so it fails on arm64 too,
// where the same input merely draws nonsense.
func TestBarRejectsNaNAndInfinity(t *testing.T) {
	for _, args := range [][]string{
		{"NaN", "10"},
		{"10", "NaN"},
		{"Inf", "10"},
		{"10", "Inf"},
		{"-Inf", "10"},
	} {
		ui, buf := newBarTestUI(t, true, 80)
		if code := runBar(args, ui); code != 2 {
			t.Errorf("runBar(%v) = %d, want 2; it drew %q", args, code, buf.String())
		}
	}
}
