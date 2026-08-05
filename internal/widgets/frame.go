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
	"unicode/utf8"

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
	e.buf = append(e.buf, '\n')
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
// theme's ellipsis.
//
// Clipping is not cosmetic. A line wider than the terminal wraps onto a second
// physical line, and the frame's line count — which the whole redraw depends on
// — would then be wrong, so the next frame would overwrite the wrong lines.
//
// Width is counted in runes. A CJK or emoji character occupies two cells, so a
// line of them can still wrap; the one-column margin absorbs the common case.
func (e *Env) appendClipped(s string, used int) int {
	limit := e.frame.width - 1 - used
	if limit <= 0 {
		return 0
	}
	if utf8.RuneCountInString(s) <= limit {
		e.buf = append(e.buf, s...)
		return utf8.RuneCountInString(s)
	}
	ellipsis := e.look().Ellipsis
	keep := limit - utf8.RuneCountInString(ellipsis)
	if keep < 1 {
		keep = 1
	}
	n := 0
	for i, r := range s {
		if n == keep {
			e.buf = append(e.buf, s[:i]...)
			break
		}
		n++
		_ = r
	}
	e.buf = append(e.buf, ellipsis...)
	return limit
}

// hideCursor and showCursor bracket a live prompt: the caret is drawn as a
// themed symbol, so the hardware cursor would only flicker at the end of the
// frame.
func (e *Env) hideCursor() { WriteUI(e, theme.HideCursor) }
func (e *Env) showCursor() { WriteUI(e, theme.ShowCursor) }

// visibleRows is how many options fit: the step's own height cap, further
// limited by the space the terminal actually has after the prompt, the header
// and the hint line.
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
