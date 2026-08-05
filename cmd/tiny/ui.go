package main

// The display commands: the half of a harness that shows rather than asks.
//
//	tiny spin "Building…" -- go build ./...
//	tiny say "Deployed" --style ok
//	tiny box "Plan" < plan.md
//	tiny bar 3 12 "files"
//
// A wizard step produces an answer; these produce none. That is what makes
// them safe to add: there is no headless/interactive parity to keep, because
// there is nothing to be equal about. What they do inherit is the rule that
// matters most — everything is drawn on **stderr**, so a caller's payload
// stays pipeable while its progress is on screen.
//
// Without a terminal they degrade to one plain line and no animation. A CI log
// full of spinner frames is the reason people turn spinners off.

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// UI is what a display command draws with.
type UI struct {
	// Err is where everything is written. It is stderr in production and a
	// buffer in tests; a display command must never write to stdout.
	Err io.Writer
	// Theme is the compiled look, already resolved for colour support.
	Theme *theme.Resolved
	// TTY reports whether Err is a terminal. False means: no animation, no
	// cursor movement, no escape sequences beyond what the theme carries.
	TTY bool
	// Width is the terminal width in columns, or a sane default.
	Width func() int
}

// displayCommand runs one command and returns the process exit code.
type displayCommand func(args []string, ui *UI) int

// displayCommands is the registry the main dispatch consults. A name here
// shadows nothing: none of these is a step type or a spec path anyone writes.
var displayCommands = map[string]displayCommand{
	"spin": runSpin,
	"say":  runSay,
	"box":  runBox,
	"bar":  runBar,
}

// newUI builds the drawing context for a display command.
func newUI(themeRef string, noColor bool) (*UI, error) {
	resolved, err := resolveTheme(nil, themeRef, noColor)
	if err != nil {
		return nil, err
	}
	return &UI{
		Err:   os.Stderr,
		Theme: resolved,
		TTY:   term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb",
		Width: stderrWidth,
	}, nil
}

// stderrWidth is the terminal width, or 80 where there is no terminal to ask.
func stderrWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// Writef writes formatted text to the UI stream.
func (u *UI) Writef(format string, a ...any) {
	fmt.Fprintf(u.Err, format, a...)
}

// Write writes text to the UI stream.
func (u *UI) Write(s string) {
	_, _ = io.WriteString(u.Err, s)
}

// Styled wraps text in a theme pair, which is empty when the element has no
// style or colour is off — so this is also how a command stays honest under
// -no-color without checking for it.
func (u *UI) Styled(open, text, close string) string {
	return open + text + close
}

// usageError reports a bad invocation the way the rest of tiny does, and
// returns the exit code the caller should use.
func usageError(ui *UI, format string, a ...any) int {
	fmt.Fprintf(ui.Err, "tiny: "+format+"\n", a...)
	return 2
}
