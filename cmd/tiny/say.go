package main

// say draws one or more styled lines on stderr, in the caller's theme.
// Unlike box (which wraps to the terminal width) or spin (which animates),
// say does the simplest thing a harness needs: label output it already has,
// or announce a result, without touching what the text says.
//
//	tiny say "Deploy complete"
//	tiny say "Nothing to do" --style note
//	tiny say "Build failed" --style error
//	git log --oneline -5 | tiny say --style note

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// sayInput is where say reads text from when there is no positional
// argument. A package var rather than os.Stdin used directly, so tests can
// substitute a buffer without touching the process's real stdin.
var sayInput io.Reader = os.Stdin

// sayStdinIsTerminal reports whether sayInput is an interactive keyboard
// rather than a pipe or file. A package var, like sayInput, so a test can
// force either answer without a real tty attached to the test binary.
var sayStdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// sayStyleNames are the style names say accepts, in the order the usage
// error lists them. Kept as one slice so the error message and sayStyle's
// switch below cannot silently drift apart.
var sayStyleNames = []string{"header", "note", "error", "hint", "option", "table-header", "plain"}

// sayStyle returns the theme pair for a style name, or ok=false if name is
// not one of sayStyleNames.
//
// These are exactly the pairs theme.Resolved defines (see ui.go's comment
// on UI): there is no "success" pair, and adding one would mean editing the
// theme package, which say does not own. "plain" is the one name with no
// backing element — it opts out of styling entirely.
func sayStyle(t *theme.Resolved, name string) (open, close string, ok bool) {
	switch name {
	case "plain":
		return "", "", true
	case "header":
		return t.HeaderOpen, t.HeaderClose, true
	case "note":
		return t.NoteOpen, t.NoteClose, true
	case "error":
		return t.ErrorOpen, t.ErrorClose, true
	case "hint":
		return t.HintOpen, t.HintClose, true
	case "option":
		return t.OptionOpen, t.OptionClose, true
	case "table-header":
		return t.TableHeaderOpen, t.TableHeaderClose, true
	default:
		return "", "", false
	}
}

// runSay draws the positional text, or stdin's when there is none, styled
// with --style (default: note).
//
// --style is accepted anywhere in args, not only before the text, because
// `cmd | tiny say --style note` and `tiny say "msg" --style note` are both
// natural word orders for a command meant to sit at the end of a pipeline;
// the stdlib flag package would force one specific order.
func runSay(args []string, ui *UI) int {
	styleName := "note"
	var textWords []string
	literal := false // set by "--": everything after it is text, never a flag
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case literal:
			textWords = append(textWords, a)
		case a == "--":
			literal = true
		case a == "--style" || a == "-style":
			if i+1 >= len(args) {
				return usageError(ui, "--style needs a value: one of %s", strings.Join(sayStyleNames, ", "))
			}
			i++
			styleName = args[i]
		case strings.HasPrefix(a, "--style="):
			styleName = strings.TrimPrefix(a, "--style=")
		case strings.HasPrefix(a, "-style="):
			styleName = strings.TrimPrefix(a, "-style=")
		default:
			textWords = append(textWords, a)
		}
	}

	open, close, ok := sayStyle(ui.Theme, styleName)
	if !ok {
		return usageError(ui, "unknown style %q: must be one of %s", styleName, strings.Join(sayStyleNames, ", "))
	}

	var text string
	if len(textWords) > 0 {
		// Several bare shell words with no quoting is still one message;
		// joining them is what makes `tiny say Deploy complete` behave like
		// the quoted form instead of silently dropping words after the
		// first.
		text = strings.Join(textWords, " ")
	} else {
		// A keyboard never sends EOF, so reading it here would hang the
		// terminal until Ctrl-C: refuse instead of blocking. A pipe or
		// redirected file does send EOF, so those are still read normally.
		if sayStdinIsTerminal() {
			return usageError(ui, "no text: pass it as an argument or pipe it in")
		}
		data, err := io.ReadAll(sayInput)
		if err != nil {
			return usageError(ui, "cannot read stdin: %v", err)
		}
		text = string(data)
	}

	// The one trailing newline a shell or pipe ends its last line with is
	// not part of the message; keeping it would print one extra, empty
	// styled line.
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		// A styled prefix around nothing is noise in a log, not a message.
		return 0
	}

	for _, line := range strings.Split(text, "\n") {
		ui.Write(open)
		ui.Write(sanitize(line))
		ui.Write(close)
		ui.Write("\n")
	}
	return 0
}

// sanitize strips control characters, including the ESC byte that starts an
// ANSI escape sequence, from a single line of text before it reaches the
// terminal. Tab (0x09) is the one exception, kept rather than stripped —
// see below.
//
// The text say prints is not the theme's own output — it is whatever the
// caller passed on argv or piped from another command, e.g. a build log or
// a commit message. Left verbatim, a stray escape sequence in that text
// could retitle the window, move the cursor over lines say already drew, or
// write the clipboard via OSC 52. This is the same rule internal/widgets
// applies to spec-supplied text in appendText (widgets.go); it is
// duplicated here rather than exported because say does not own the
// widgets package and appendText is unexported there.
//
// Where this deliberately diverges from appendText: appendText strips tabs
// along with every other control character, which is right for a wizard's
// title or option label — those never legitimately contain one. say's input
// is a build log or a commit message, where a tab is often the column
// separator holding an aligned block together; stripping it would corrupt
// the very text the caller wanted read verbatim. A tab cannot move the
// cursor or write the clipboard the way ESC-led sequences can, so keeping
// it does not reopen the risk the rest of this function closes.
func sanitize(s string) string {
	return sanitizeText(s, true, false)
}
