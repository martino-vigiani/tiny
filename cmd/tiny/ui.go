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
	"strings"
	"unicode/utf8"

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

// sanitizeText strips the control bytes that would let supplied text drive the
// terminal instead of merely appearing on it.
//
// This is the rule internal/widgets applies to spec-supplied text in
// appendText, and it is duplicated here rather than shared because that
// function is unexported in a package cmd does not import. Removing the
// introducer byte — every C0 control and DEL, not only ESC — is what stops the
// terminal from ever recognising a sequence: an OSC 52 in a label or a commit
// message would otherwise write the user's clipboard.
//
// The two exceptions are per-command, and the reason they are parameters
// rather than four private copies is that the four copies disagreed:
//
//   - keepTab: say prints build logs and commit messages, where a tab is the
//     column separator holding an aligned block together. Anywhere a line is
//     measured and clipped to a width — spin's frame, bar's line, box's rows —
//     a tab must go, because it advances the cursor to the next tab stop while
//     measuring as a single cell, and the line then wraps out of reach of the
//     next redraw.
//   - keepNewline: box needs the caller's line breaks to survive into its
//     body; a single-line command does not.
func sanitizeText(s string, keepTab, keepNewline bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 0x20 && c != 0x7f:
		case keepTab && c == '\t':
		case keepNewline && c == '\n':
		default:
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// cellWidth is how many terminal columns a string occupies, ignoring any ANSI
// sequence in it — a theme's colour escapes are zero columns wide, and a box
// measured without allowing for that comes out visibly crooked.
func cellWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				j++ // the final byte of the CSI sequence
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += runeCells(r)
		i += size
	}
	return n
}

// runeCells reports a rune's display width: two columns for East Asian Wide
// and Fullwidth ranges and emoji, none for combining and zero-width marks.
//
// It is not a complete UAX #11 implementation — that needs a generated table —
// but it covers what people actually put in a label, which is what keeps a
// redraw from breaking. The zero-width cases must stay ahead of the `r < 0x1100`
// fast path: with the order reversed they are unreachable, and a decomposed
// accent silently measures a column too wide.
func runeCells(r rune) int {
	switch {
	case r >= 0x0300 && r <= 0x036f: // combining diacriticals
		return 0
	case r >= 0x200b && r <= 0x200f, r == 0xfeff: // zero-width and marks
		return 0
	case r < 0x1100:
		return 1
	case r >= 0x1100 && r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0x303e, // CJK radicals, Kangxi, punctuation
		r >= 0x3041 && r <= 0x33ff, // Hiragana, Katakana, compat
		r >= 0x3400 && r <= 0x4dbf, // CJK extension A
		r >= 0x4e00 && r <= 0x9fff, // CJK unified
		r >= 0xa000 && r <= 0xa4cf, // Yi
		r >= 0xac00 && r <= 0xd7a3, // Hangul syllables
		r >= 0xf900 && r <= 0xfaff, // CJK compatibility ideographs
		r >= 0xfe30 && r <= 0xfe6f, // CJK compatibility forms
		r >= 0xff00 && r <= 0xff60, // fullwidth forms
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f, // emoji
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd: // CJK extensions B onward
		return 2
	default:
		return 1
	}
}

// truncateCells clips s to at most width cells, ending with ellipsis when
// something was cut. A width too small even for the ellipsis yields as much of
// it as fits, since returning nothing would hide that anything was there.
func truncateCells(s, ellipsis string, width int) string {
	if width <= 0 {
		return ""
	}
	if cellWidth(s) <= width {
		return s
	}
	keep := width - cellWidth(ellipsis)
	if keep < 0 {
		keep = 0
	}
	cells, cut := 0, len(s)
	for i, r := range s {
		w := runeCells(r)
		if cells+w > keep {
			cut = i
			break
		}
		cells += w
	}
	return s[:cut] + ellipsis
}

// padCells right-pads s with spaces to exactly width cells, so a column of
// them lines up whatever scripts the text is written in.
func padCells(s string, width int) string {
	if pad := width - cellWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// usageError reports a bad invocation the way the rest of tiny does, and
// returns the exit code the caller should use.
func usageError(ui *UI, format string, a ...any) int {
	fmt.Fprintf(ui.Err, "tiny: "+format+"\n", a...)
	return 2
}
