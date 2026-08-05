package main

// box: draw a bordered box around piped text, with an optional title.
//
//	tiny box "Execution plan" < plan.md
//	echo "3 files will be rewritten" | tiny box
//	tiny box "Warning" --width 60 < text.txt
//
// A box is static — no animation, no cursor movement — so ui.TTY affects only
// which width is assumed by default, never how the box is drawn.

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// minInnerWidth is the smallest content width worth drawing a box around.
// Below it there is no room for a single character between the borders, so a
// request for less is a usage error rather than a degenerate empty box.
const minInnerWidth = 1

// boxOverhead is what the borders and padding cost on top of the content:
// one border column, one padding space, the content, one padding space, one
// border column — on each of the top/bottom and left/right pairs.
const boxOverhead = 4

// defaultMaxWidth caps the box when no --width is given. A wizard step wraps
// at the terminal width because that is genuinely how much space there is;
// a box is read like a paragraph, and paragraphs stretched across a 300-column
// terminal are harder to read, not easier.
const defaultMaxWidth = 96

// stdinIsTerminal reports whether stdin is a live keyboard rather than a pipe
// or file. It is a var, not a direct term.IsTerminal call, so a test can
// force either branch without a real tty.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func runBox(args []string, ui *UI) int {
	title, titleSet, widthFlag, widthSet, err := parseBoxArgs(args)
	if err != nil {
		return usageError(ui, "box: %v", err)
	}

	// os.Stdin, not a field on UI: box is the one display command that reads
	// its payload rather than being told it on argv, and UI carries only the
	// output stream every display command shares. Reading is skipped entirely
	// when stdin is a terminal: io.ReadAll blocks until EOF, and a keyboard
	// never sends one, so `tiny box "Title"` typed directly at a prompt would
	// hang forever waiting for input nobody is going to pipe in. A title with
	// no piped body still draws — just an empty box.
	var body []byte
	if !stdinIsTerminal() {
		body, err = io.ReadAll(os.Stdin)
		if err != nil {
			return usageError(ui, "box: reading stdin: %v", err)
		}
	}
	hasBody := len(body) > 0

	if !titleSet && !hasBody {
		return usageError(ui, "box: nothing to draw — give a title or pipe text on stdin")
	}

	total := defaultMaxWidth
	if w := ui.Width(); w < total {
		total = w
	}
	if widthSet {
		// An explicit --width is taken as the caller meaning it, not clamped
		// to the terminal: the output may be going to a file or a wider pane
		// than the current one (`tiny box --width 120 > report.txt`), and a
		// human typing --width 60 on an 80-column terminal clearly wants 60,
		// not "60 unless the terminal disagrees". A width wider than the
		// terminal wraps and looks broken there, but that is the caller's
		// call to make, the same way `--width` for `bar`/other commands would
		// be.
		total = widthFlag
	}
	inner := total - boxOverhead
	if inner < minInnerWidth {
		return usageError(ui, "box: --width %d is too small to hold anything", total)
	}

	ui.Write(renderBox(title, string(body), inner, ui.Theme))
	return 0
}

// parseBoxArgs reads the one optional positional title and the --width flag.
// It is hand-rolled rather than flag.FlagSet because the title is positional
// and may itself start with "-" (a title like "-verbose mode-" is legitimate
// text, not a flag), so a generic parser would misclassify it.
func parseBoxArgs(args []string) (title string, titleSet bool, width int, widthSet bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--width":
			if i+1 >= len(args) {
				return "", false, 0, false, fmt.Errorf("--width requires a value")
			}
			i++
			n, convErr := strconv.Atoi(args[i])
			if convErr != nil {
				return "", false, 0, false, fmt.Errorf("invalid --width %q", args[i])
			}
			width, widthSet = n, true
		case strings.HasPrefix(a, "--width="):
			n, convErr := strconv.Atoi(strings.TrimPrefix(a, "--width="))
			if convErr != nil {
				return "", false, 0, false, fmt.Errorf("invalid --width %q", a)
			}
			width, widthSet = n, true
		case !titleSet:
			title, titleSet = a, true
		default:
			return "", false, 0, false, fmt.Errorf("unexpected argument %q", a)
		}
	}
	return title, titleSet, width, widthSet, nil
}

// borderSet is the glyphs a box draws with.
type borderSet struct {
	topLeft, topRight, bottomLeft, bottomRight string
	horizontal, vertical                       string
}

var unicodeBorders = borderSet{"┌", "┐", "└", "┘", "─", "│"}
var asciiBorders = borderSet{"+", "+", "+", "+", "-", "|"}

// bordersFor picks the glyph set for the theme in force. "ascii" and "plain"
// are the presets that exist specifically for terminals or log pipelines that
// cannot be assumed to render Unicode (see their doc comments in
// internal/theme/theme.go); every other theme gets real box-drawing
// characters, since a terminal capable of rendering the theme's own colors
// and symbols can render U+2500 line-drawing just as well.
func bordersFor(name string) borderSet {
	if name == "ascii" || name == "plain" {
		return asciiBorders
	}
	return unicodeBorders
}

// renderBox draws the complete box — top border with title, wrapped body,
// bottom border — as one string ready to write to ui.Err.
func renderBox(title, body string, inner int, th *theme.Resolved) string {
	b := bordersFor(th.Name)
	var out strings.Builder

	out.WriteString(topBorder(title, inner, b, th))
	out.WriteByte('\n')

	for _, line := range strings.Split(strings.TrimRight(sanitizeBox(body), "\n"), "\n") {
		for _, wrapped := range wrapCells(line, inner) {
			out.WriteString(b.vertical)
			out.WriteByte(' ')
			out.WriteString(padCells(wrapped, inner))
			out.WriteByte(' ')
			out.WriteString(b.vertical)
			out.WriteByte('\n')
		}
	}

	out.WriteString(b.bottomLeft)
	out.WriteString(strings.Repeat(b.horizontal, inner+2))
	out.WriteString(b.bottomRight)
	out.WriteByte('\n')
	return out.String()
}

// topBorder draws the top edge, embedding the title in the theme's header
// style when one was given, truncated with the theme's ellipsis when it does
// not fit. ruleWidth is the number of horizontal-rule cells between the
// corners — inner+2, matching the content rows (border + space + content +
// space + border).
//
// The title's decoration is the theme's own header open/close — "— " / " —"
// for default, "== " / " ==" for ascii and plain, nothing but a color escape
// for sleek — with only the leading/trailing newlines trimmed, since those
// exist for PrintHeader's standalone "\n— Title —\n" and would otherwise
// split the border across lines. Reusing them rather than inventing a
// box-specific ornament is what makes a titled box look like the same theme
// as everything else tiny draws.
func topBorder(title string, inner int, b borderSet, th *theme.Resolved) string {
	ruleWidth := inner + 2
	if title == "" {
		return b.topLeft + strings.Repeat(b.horizontal, ruleWidth) + b.topRight
	}

	title = sanitizeBox(title)
	open := strings.TrimLeft(th.HeaderOpen, "\n")
	shut := strings.TrimRight(th.HeaderClose, "\n")
	decor := cellWidth(open) + cellWidth(shut)

	maxTitle := ruleWidth - decor
	if maxTitle < 1 {
		// No room even for a one-cell title once the theme's own decoration
		// is accounted for; fall back to a plain rule instead of letting the
		// border overflow or the decoration collide with the title.
		return b.topLeft + strings.Repeat(b.horizontal, ruleWidth) + b.topRight
	}
	title = truncateCells(title, th.Ellipsis, maxTitle)

	remaining := ruleWidth - decor - cellWidth(title)
	left := remaining / 2
	right := remaining - left
	return b.topLeft + strings.Repeat(b.horizontal, left) +
		open + title + shut +
		strings.Repeat(b.horizontal, right) + b.topRight
}

// sanitizeBox strips control characters and ANSI escapes (which are always
// introduced by a control byte) from text that came in over stdin or argv.
//
// Duplicated rather than shared with internal/widgets' appendText: that
// function is unexported in a package this command does not import, and the
// reasoning is the same one documented there — an OSC 52 sequence in
// supplied text could write to the user's clipboard, and removing the
// introducer byte (not just ESC but every C0 control and DEL) is what stops
// the terminal from ever recognising it as a sequence to interpret, leaving
// only inert bytes that render as nothing. Newlines are kept: the caller
// still needs them to preserve line breaks.
func sanitizeBox(s string) string {
	return sanitizeText(s, false, true)
}

// wrapCells wraps s to width terminal cells, breaking on spaces and hard
// breaking any single word wider than the box. An empty line becomes a
// single empty output line, which is how a blank line in the body survives.
func wrapCells(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	var lines []string
	var cur strings.Builder
	curCells := 0

	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curCells = 0
	}

	for _, word := range strings.Fields(s) {
		wc := cellWidth(word)
		for wc > width {
			// The word alone does not fit even on an empty line: hard-break
			// it at the box edge rather than let it overflow the border.
			if curCells > 0 {
				flush()
			}
			head, rest := splitCells(word, width)
			lines = append(lines, head)
			word, wc = rest, cellWidth(rest)
		}
		need := wc
		if curCells > 0 {
			need += 1 // the separating space
		}
		if curCells > 0 && curCells+need > width {
			flush()
		}
		if curCells > 0 {
			cur.WriteByte(' ')
			curCells++
		}
		cur.WriteString(word)
		curCells += wc
	}
	if curCells > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// splitCells cuts s after as many whole runes as fit in width cells,
// returning the head that fits and the untouched remainder.
func splitCells(s string, width int) (head, rest string) {
	cells := 0
	for i, r := range s {
		w := runeCells(r)
		if cells+w > width {
			return s[:i], s[i:]
		}
		cells += w
	}
	return s, ""
}
