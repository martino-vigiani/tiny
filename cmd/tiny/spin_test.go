package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns what was written to it. runSpin wires the child's Stdout straight
// to os.Stdout (that is the whole point of the command), so a test has no
// other way to observe it than swapping the real file descriptor.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return buf.String()
}

func newSpinUI(tty bool) (*UI, *bytes.Buffer) {
	return newSpinUIWidth(tty, 80)
}

func newSpinUIWidth(tty bool, width int) (*UI, *bytes.Buffer) {
	var buf bytes.Buffer
	resolved, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		panic(err)
	}
	return &UI{
		Err:   &buf,
		Theme: resolved,
		TTY:   tty,
		Width: func() int { return width },
	}, &buf
}

func TestSpinExitCodeIsChildsSuccess(t *testing.T) {
	ui, _ := newSpinUI(false)
	code := runSpin([]string{"Running", "--", "/bin/sh", "-c", "exit 0"}, ui)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestSpinExitCodeIsChildsFailure(t *testing.T) {
	ui, _ := newSpinUI(false)
	code := runSpin([]string{"Running", "--", "/bin/sh", "-c", "exit 3"}, ui)
	if code != 3 {
		t.Errorf("exit code = %d, want 3 (the child's)", code)
	}
}

func TestSpinExitCodeIsChildsFailureTTY(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, _ := newSpinUI(true)
	code := runSpin([]string{"Running", "--", "/bin/sh", "-c", "exit 7"}, ui)
	if code != 7 {
		t.Errorf("exit code = %d, want 7 (the child's)", code)
	}
}

func TestSpinStdoutPassesThroughUntouched(t *testing.T) {
	ui, errBuf := newSpinUI(false)
	var code int
	out := captureStdout(t, func() {
		code = runSpin([]string{"Echoing", "--", "/bin/echo", "hello-payload"}, ui)
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out) != "hello-payload" {
		t.Errorf("stdout = %q, want the child's echoed line", out)
	}
	if strings.Contains(errBuf.String(), "hello-payload") {
		t.Errorf("payload leaked into ui.Err: %q", errBuf.String())
	}
}

func TestSpinUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no --", []string{"Building", "go", "build"}},
		{"no message", []string{"--", "go", "build"}},
		{"empty command", []string{"Building", "--"}},
		{"nothing at all", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui, buf := newSpinUI(false)
			code := runSpin(tc.args, ui)
			if code != 2 {
				t.Errorf("exit code = %d, want 2 (usage error)", code)
			}
			if !strings.HasPrefix(buf.String(), "tiny: ") {
				t.Errorf("message = %q, want it to start with %q", buf.String(), "tiny: ")
			}
		})
	}
}

func TestSpinNonTTYIsPlainAndAnsiFree(t *testing.T) {
	ui, buf := newSpinUI(false)
	var code int
	out := captureStdout(t, func() {
		code = runSpin([]string{"Building the thing", "--", "/bin/sh", "-c", "exit 0"}, ui)
	})
	_ = out
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()
	if !strings.Contains(got, "Building the thing") {
		t.Errorf("output = %q, want it to contain the message", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("output = %q, want no ANSI escapes without a TTY", got)
	}
	// One plain line: exactly one newline-terminated message, nothing else
	// printed by the spinner itself.
	if strings.Count(got, "\n") != 1 {
		t.Errorf("output = %q, want exactly one line", got)
	}
}

// shrinkFrameInterval lowers the spinner's redraw interval for the duration
// of a test so the TTY path exercises real animation without a real sleep of
// meaningful length, and restores it afterwards.
func shrinkFrameInterval(t *testing.T) func() {
	t.Helper()
	orig := frameInterval
	frameInterval = 2 * time.Millisecond
	return func() { frameInterval = orig }
}

func TestSpinTTYAnimatesAndLeavesNoResidue(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUI(true)

	code := runSpin([]string{"Working", "--", "/bin/sh", "-c", "sleep 0.03"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()

	if !strings.HasPrefix(got, theme.HideCursor) {
		t.Errorf("output does not start by hiding the cursor: %q", got)
	}
	if !strings.HasSuffix(got, theme.ShowCursor) {
		t.Errorf("output does not end by showing the cursor: %q", got)
	}
	if !strings.Contains(got, theme.EraseEOL) {
		t.Errorf("output = %q, want at least one erase-to-end-of-line (a redrawn frame)", got)
	}
	// No residue: the very last thing wiped is the spinner line itself, so
	// the erase immediately precedes the cursor being shown again.
	if !strings.HasSuffix(got, theme.EraseEOL+theme.ShowCursor) {
		t.Errorf("output = %q, want the final erase to immediately precede showing the cursor (no leftover frame)", got)
	}
}

func TestSpinChildStderrDoesNotFightTheAnimation(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUI(true)

	var out string
	var code int
	out = captureStdout(t, func() {
		code = runSpin([]string{"Working", "--", "/bin/sh", "-c", "sleep 0.02; echo out; echo err 1>&2"}, ui)
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out) != "out" {
		t.Errorf("stdout = %q, want only the child's stdout line", out)
	}
	got := buf.String()
	if !strings.Contains(got, "err\n") {
		t.Errorf("output = %q, want the child's stderr flushed after the spinner", got)
	}
	// The captured stderr must land after the spinner line was wiped, i.e.
	// after the last EraseEOL, and before the cursor is shown again — never
	// interleaved with a frame redraw.
	lastErase := strings.LastIndex(got, theme.EraseEOL)
	errAt := strings.Index(got, "err\n")
	showAt := strings.Index(got, theme.ShowCursor)
	if !(lastErase < errAt && errAt < showAt) {
		t.Errorf("stderr was not placed between the final erase and the cursor being shown: %q", got)
	}
}

func TestSpinCommandNotFound(t *testing.T) {
	ui, buf := newSpinUI(false)
	code := runSpin([]string{"Running", "--", "tiny-spin-does-not-exist-xyz"}, ui)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "tiny-spin-does-not-exist-xyz") {
		t.Errorf("message = %q, want it to name the missing command", buf.String())
	}
}

// Sanity check that exec.ExitError is what we expect from a signaled or
// abnormally-exited process, so exitCodeFrom's assumptions hold.
func TestExitCodeFromExitError(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 5")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T (%v)", err, err)
	}
	if got := exitCodeFrom(exitErr.ProcessState); got != 5 {
		t.Errorf("exitCodeFrom = %d, want 5", got)
	}
}

// A message is caller-controlled text, not the theme's own output. An escape
// sequence smuggled inside it (e.g. built from a file the user did not write
// themselves) must never reach the terminal, on either the plain or the
// animated path — that is the OSC-52-writes-the-clipboard hole sanitize
// exists to close.
func TestSpinMessageIsSanitizedNonTTY(t *testing.T) {
	ui, buf := newSpinUI(false)
	evil := "Building\x1b]52;c;ZXZpbA==\x07 now"
	code := runSpin([]string{evil, "--", "/bin/sh", "-c", "exit 0"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("output = %q, want the escape sequence stripped from the message", buf.String())
	}
	if !strings.Contains(buf.String(), "Building") || !strings.Contains(buf.String(), "now") {
		t.Errorf("output = %q, want the rest of the message intact", buf.String())
	}
}

func TestSpinMessageIsSanitizedTTY(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUI(true)
	evil := "Building\x1b]52;c;ZXZpbA==\x07 now"
	code := runSpin([]string{evil, "--", "/bin/sh", "-c", "exit 0"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()
	// Stripping the ESC (and the BEL that terminates the OSC sequence) is
	// what neutralises it: what is left is inert text, not a live escape
	// sequence, even though the digits and semicolons themselves are
	// ordinary printable characters and stay.
	if strings.Contains(got, "\x1b]52") || strings.Contains(got, "\x07") {
		t.Errorf("output = %q, want the message's OSC sequence neutralised (no ESC, no BEL)", got)
	}
}

// A message wider than the terminal must not wrap: ColumnZero+EraseEOL only
// touches the line the cursor is on, so a wrapped line leaves every earlier
// physical row behind on the next redraw and the screen fills with debris.
func TestSpinLongMessageIsClippedToWidth(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUIWidth(true, 20)

	long := "this message is much longer than the twenty column terminal it is drawn on"
	code := runSpin([]string{long, "--", "/bin/sh", "-c", "sleep 0.02"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()

	// Every frame is one write of ColumnZero, the clipped line, then
	// EraseEOL; split on ColumnZero and check each drawn segment fits.
	for _, segment := range strings.Split(got, theme.ColumnZero) {
		line, _, found := strings.Cut(segment, theme.EraseEOL)
		if !found {
			continue // not a drawn frame (e.g. the leading HideCursor chunk)
		}
		if w := cellWidth(line); w > 19 { // width - 1, per animate's margin
			t.Errorf("drawn line %q = %d cells, want at most 19 (width 20 minus margin)", line, w)
		}
	}
	if !strings.Contains(got, ui.Theme.Ellipsis) {
		t.Errorf("output = %q, want the clipped line to end in the theme ellipsis", got)
	}
}

// Same clipping, but with a CJK message: cellWidth must count a wide
// ideograph as two columns, not one, or the line silently overflows the
// terminal by up to 2x.
func TestSpinCJKMessageIsClippedByCellsNotRunes(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUIWidth(true, 20)

	cjk := "これは二十桁の端末には長すぎるメッセージです"
	code := runSpin([]string{cjk, "--", "/bin/sh", "-c", "sleep 0.02"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()

	for _, segment := range strings.Split(got, theme.ColumnZero) {
		line, _, found := strings.Cut(segment, theme.EraseEOL)
		if !found {
			continue
		}
		if w := cellWidth(line); w > 19 {
			t.Errorf("drawn line %q = %d cells, want at most 19", line, w)
		}
	}
}

// --stream disables the animation (no HideCursor/ShowCursor, no frames) and
// lets the child's stderr reach ui.Err as it happens rather than only after
// the child exits.
func TestSpinStreamFlagDisablesAnimationAndStreamsStderr(t *testing.T) {
	ui, buf := newSpinUI(true) // TTY, but --stream should still go plain
	code := runSpin([]string{"--stream", "Deploying", "--", "/bin/sh", "-c", "echo live 1>&2"}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()
	if strings.Contains(got, theme.HideCursor) || strings.Contains(got, theme.ShowCursor) {
		t.Errorf("output = %q, want no cursor escapes under --stream", got)
	}
	if !strings.Contains(got, "live\n") {
		t.Errorf("output = %q, want the child's stderr line", got)
	}
}

// Buffered stderr must not grow without bound: past capturedStderrLimit,
// spin stops keeping it and says so instead of buffering forever.
func TestSpinTruncatesOversizedCapturedStderr(t *testing.T) {
	restore := shrinkFrameInterval(t)
	defer restore()
	ui, buf := newSpinUI(true)

	// Print well over the cap in one call so the test stays fast: `yes` piped
	// through head is not portable enough, so use awk to emit ~100KB.
	script := "awk 'BEGIN{s=sprintf(\"%0100000d\",0); print s > \"/dev/stderr\"}'"
	code := runSpin([]string{"Working", "--", "/bin/sh", "-c", script}, ui)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := buf.String()
	if !strings.Contains(got, "truncated") {
		t.Errorf("output = %q, want a truncation notice", got)
	}
}
