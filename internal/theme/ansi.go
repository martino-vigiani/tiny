package theme

import "strconv"

// The escape sequences the interactive mode needs to redraw in place. They live
// here, with the rest of the ANSI knowledge, so no widget ever writes an escape
// by hand.
const (
	// HideCursor and ShowCursor bracket an interactive prompt: the caret is
	// drawn as a themed symbol, so the hardware cursor only flickers.
	HideCursor = "\x1b[?25l"
	ShowCursor = "\x1b[?25h"

	// EraseEOL clears from the cursor to the end of the line, which is what
	// stops a shorter line from leaving the tail of the longer one behind it.
	EraseEOL = "\x1b[K"

	// EraseBelow clears from the cursor to the end of the screen, wiping in one
	// sequence both the rest of the line and every line a taller previous frame
	// occupied.
	EraseBelow = "\x1b[J"

	// ColumnZero returns to the first column.
	ColumnZero = "\r"
)

// AppendCursorUp appends the sequence moving the cursor up n lines.
//
// CUU (ESC [ n A) rather than CPL (ESC [ n F): CUU is VT100 and works
// everywhere, including legacy consoles where CPL has gaps. It costs one byte
// more, paired with ColumnZero.
func AppendCursorUp(dst []byte, n int) []byte {
	if n <= 0 {
		return dst
	}
	if n == 1 {
		return append(dst, "\x1b[A"...)
	}
	dst = append(dst, "\x1b["...)
	dst = strconv.AppendInt(dst, int64(n), 10)
	return append(dst, 'A')
}
