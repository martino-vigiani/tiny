package widgets

// In-place redrawing for the interactive mode.
//
// A frame is the block of lines a live prompt occupies. Redrawing means winding
// the cursor back to where the last frame started and writing the new one over
// it, which is why the number of lines the previous frame occupied has to be
// tracked exactly. Everything is appended into the Env's reusable buffer and
// flushed with a single write, so a keystroke costs one write(2) and no
// allocation.
//
// There is no render loop, no timer and no goroutine: between keystrokes the
// process is blocked in read(2), using no CPU at all.

import (
	"strings"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// frame tracks the block of lines the live prompt occupies.
type frame struct {
	rows   int // lines in the frame being built
	drawn  int // lines the previous frame occupied
	width  int
	height int
}

// termSize returns the usable terminal size, falling back to a conventional
// 80x24 when it cannot be determined (a pipe, a terminal that will not say).
func (e *Env) termSize() (int, int) {
	if e.Size != nil {
		return e.Size()
	}
	w, h, err := term.GetSize(e.StdinFd)
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// begin winds the cursor back to the start of the previous frame.
// appendFramed appends a themed fragment, turning each newline into a carriage
// return plus newline.
//
// Theme decoration carries its own newlines — the default header opens with a
// blank line — and raw mode has cleared OPOST, so those newlines move the
// cursor down without returning it to column 0. Writing them verbatim renders
// the frame as a staircase.
func (e *Env) appendFramed(fragment string) {
	for {
		i := strings.IndexByte(fragment, '\n')
		if i < 0 {
			break
		}
		e.buf = append(e.buf, fragment[:i]...)
		e.buf = append(e.buf, '\r', '\n')
		fragment = fragment[i+1:]
	}
	e.buf = append(e.buf, fragment...)
}

// reset forgets the previous frame, so the next one starts where the cursor is
// instead of winding back into whatever the last widget left on screen.
func (e *Env) reset() {
	e.frame.drawn = 0
}

func (e *Env) begin() {
	e.frame.width, e.frame.height = e.termSize()
	e.buf = append(e.buf, theme.ColumnZero...)
	e.buf = theme.AppendCursorUp(e.buf, e.frame.drawn-1)
	e.frame.rows = 0
}

// endLine closes a line of the frame. EraseEOL is what stops a line shorter
// than the one it replaces from leaving the old tail visible to its right.
func (e *Env) endLine() {
	e.buf = append(e.buf, theme.EraseEOL...)
	// Raw mode clears OPOST, so the terminal no longer turns a newline into
	// carriage-return-plus-newline. A bare \n moves down without returning to
	// column 0, and the frame renders as a staircase.
	e.buf = append(e.buf, '\r', '\n')
	e.frame.rows++
}

// end finishes the frame and writes it.
//
// The last line deliberately does not end in a newline: the cursor stays on it,
// and that is the position begin() winds back from. EraseBelow clears in one
// sequence both the rest of that line and every line a taller previous frame
// occupied.
func (e *Env) end() {
	e.buf = append(e.buf, theme.EraseBelow...)
	e.frame.rows++
	e.frame.drawn = e.frame.rows
	e.emit()
}

// appendClipped appends s to the frame, clipped to the frame width with the
// theme's ellipsis, and reports the cells it occupied.
//
// Clipping is not cosmetic. A line wider than the terminal wraps onto a second
// physical row, the frame's line count — which the whole redraw depends on —
// goes wrong, and the next frame overwrites the wrong part of the screen.
//
// Width is counted in terminal CELLS, not runes: a CJK ideograph occupies two,
// so counting runes let an ordinary Japanese label wrap at about half the
// terminal width and corrupt every redraw after it.
func (e *Env) appendClipped(s string, used int) int {
	limit := e.frame.width - 1 - used
	if limit <= 0 {
		return 0
	}
	if cellWidth(s) <= limit {
		e.appendText(s)
		return cellWidth(s)
	}
	ellipsis := e.look().Ellipsis
	keep := limit - cellWidth(ellipsis)
	if keep < 1 {
		keep = 1
	}
	cells := 0
	cut := len(s)
	for i, r := range s {
		w := runeCells(r)
		if cells+w > keep {
			cut = i
			break
		}
		cells += w
	}
	e.appendText(s[:cut])
	e.buf = append(e.buf, ellipsis...)
	return cells + cellWidth(ellipsis)
}

// cellWidth is how many terminal columns a string occupies.
func cellWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeCells(r)
	}
	return n
}

// runeCells reports a rune's display width.
//
// This is the East Asian Wide and Fullwidth range, plus emoji, which terminals
// draw two columns wide; combining marks draw none. It is not a complete
// implementation of UAX #11 — that needs a generated table — but it covers the
// scripts a person actually puts in an option label, which is what stops the
// redraw from breaking.
func runeCells(r rune) int {
	switch {
	case r < 0x1100:
		return 1
	case r >= 0x0300 && r <= 0x036f: // combining diacriticals
		return 0
	case r >= 0x200b && r <= 0x200f, r == 0xfeff: // zero-width and marks
		return 0
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

// hideCursor and showCursor bracket a live prompt: the caret is drawn as a
// themed symbol, so the hardware cursor would only flicker at the end of the
// frame.
func (e *Env) hideCursor() { WriteUI(e, theme.HideCursor) }
func (e *Env) showCursor() { WriteUI(e, theme.ShowCursor) }

// visibleRows is how many options fit: the step's own height cap, further
// limited by the space the terminal actually has left after everything else the
// frame draws.
//
// Getting this wrong is not a cosmetic overflow. A frame taller than the
// terminal makes the screen scroll, and the next redraw then winds the cursor
// back to somewhere that is no longer where the frame started — so the display
// degrades further with every keystroke. The caller must therefore count ALL of
// its chrome, the header's own blank line included.
func (e *Env) visibleRows(stepHeight, chrome int) int {
	fits := e.frame.height - chrome
	if fits < 1 {
		fits = 1
	}
	if stepHeight > 0 && stepHeight < fits {
		return stepHeight
	}
	return fits
}
