// Package keys decodes terminal input into keystrokes.
//
// It is a leaf: stdlib plus golang.org/x/term, nothing from the rest of tiny.
// A Key is a value type, so decoding a keystroke allocates nothing.
package keys

import (
	"bufio"
)

// Type enumerates the keystrokes a wizard reacts to. Anything else decodes as
// Unknown, which every caller ignores — an unrecognised escape sequence must
// never leak through as text the widget would treat as typing.
type Type uint8

const (
	Unknown Type = iota
	Rune
	Enter
	Space
	Tab
	ShiftTab
	Backspace
	Delete
	Up
	Down
	Right
	Left
	Home
	End
	PageUp
	PageDown
	Esc
	CtrlC
	CtrlD
	CtrlU
	CtrlW
	CtrlA
)

// Key is one keystroke. Rune is only meaningful when Type is Rune or Space.
type Key struct {
	Type Type
	Rune rune
}

// Source yields keystrokes. It is the seam that lets a widget be tested with a
// scripted sequence instead of a terminal.
type Source interface {
	Next() (Key, error)
}

// Script is a Source replaying a fixed sequence, then reporting EOF. Tests use
// it to drive a widget without a terminal.
type Script struct {
	Keys []Key
	at   int
}

// Next returns the next scripted key, or io.EOF once the script is spent.
func (s *Script) Next() (Key, error) {
	if s.at >= len(s.Keys) {
		return Key{}, errEndOfScript
	}
	k := s.Keys[s.at]
	s.at++
	return k, nil
}

// Decoder reads keystrokes from a buffered reader.
//
// It reads through the same *bufio.Reader the line-based path uses rather than
// from the file descriptor: bytes already buffered would otherwise be lost, and
// Buffered() is what makes the Esc heuristic below possible without a timer.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r *bufio.Reader) *Decoder { return &Decoder{r: r} }

// Next reads and decodes one keystroke.
//
// Esc is ambiguous: alone it means "cancel", followed by more bytes it is the
// prefix of an arrow or function key. A terminal in raw mode delivers a whole
// escape sequence in a single write, so bufio holds all of it after the first
// byte — if nothing is buffered behind the Esc, it was pressed on its own. That
// resolves the ambiguity with no timer, no deadline and no goroutine.
//
// The heuristic is wrong only if the terminal splits a sequence across two
// writes (a loaded tmux, a slow ssh link), in which case an arrow key reads as
// Esc followed by two runes. TestEscSplitAcrossReads pins that known limit
// rather than hiding it.
func (d *Decoder) Next() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{}, err
	}
	switch b {
	case 0x03:
		return Key{Type: CtrlC}, nil
	case 0x04:
		return Key{Type: CtrlD}, nil
	case 0x15:
		return Key{Type: CtrlU}, nil
	case 0x17:
		return Key{Type: CtrlW}, nil
	case 0x01:
		return Key{Type: CtrlA}, nil
	case 0x09:
		return Key{Type: Tab}, nil
	case 0x0d, 0x0a:
		return Key{Type: Enter}, nil
	case 0x7f, 0x08:
		return Key{Type: Backspace}, nil
	case 0x20:
		return Key{Type: Space, Rune: ' '}, nil
	case 0x0e: // Ctrl-N, readline's "next line"
		return Key{Type: Down}, nil
	case 0x10: // Ctrl-P, readline's "previous line"
		return Key{Type: Up}, nil
	case 0x1b:
		if d.r.Buffered() == 0 {
			return Key{Type: Esc}, nil
		}
		return d.escape()
	}
	if b < 0x20 {
		return Key{Type: Unknown}, nil
	}
	if b < 0x80 {
		return Key{Type: Rune, Rune: rune(b)}, nil
	}
	// A multi-byte rune: hand the byte back and let bufio reassemble it.
	if err := d.r.UnreadByte(); err != nil {
		return Key{Type: Unknown}, nil
	}
	ru, _, err := d.r.ReadRune()
	if err != nil {
		return Key{}, err
	}
	return Key{Type: Rune, Rune: ru}, nil
}

func (d *Decoder) escape() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{Type: Esc}, nil
	}
	switch b {
	case '[':
		return d.csi()
	case 'O':
		// Application cursor keys — what Apple Terminal sends for the arrows.
		c, err := d.r.ReadByte()
		if err != nil {
			return Key{Type: Unknown}, nil
		}
		switch c {
		case 'A':
			return Key{Type: Up}, nil
		case 'B':
			return Key{Type: Down}, nil
		case 'C':
			return Key{Type: Right}, nil
		case 'D':
			return Key{Type: Left}, nil
		case 'H':
			return Key{Type: Home}, nil
		case 'F':
			return Key{Type: End}, nil
		}
	}
	return Key{Type: Unknown}, nil
}

// csi consumes a control sequence to its final byte, so an unrecognised one is
// swallowed whole instead of leaving its tail to be read as typing.
func (d *Decoder) csi() (Key, error) {
	var params [8]byte // on the stack: decoding stays allocation-free
	n := 0
	for {
		b, err := d.r.ReadByte()
		if err != nil {
			return Key{Type: Unknown}, nil
		}
		switch {
		case b >= 0x30 && b <= 0x3f: // parameter bytes
			if n < len(params) {
				params[n] = b
				n++
			}
		case b >= 0x20 && b <= 0x2f: // intermediate bytes, ignored
		default: // final byte
			return csiKey(params[:n], b), nil
		}
	}
}

func csiKey(params []byte, final byte) Key {
	switch final {
	case 'A':
		return Key{Type: Up}
	case 'B':
		return Key{Type: Down}
	case 'C':
		return Key{Type: Right}
	case 'D':
		return Key{Type: Left}
	case 'H':
		return Key{Type: Home}
	case 'F':
		return Key{Type: End}
	case 'Z':
		return Key{Type: ShiftTab}
	case '~':
		// Modifiers arrive as extra parameters (ESC [ 1 ; 5 A); taking only the
		// first degrades Ctrl-Home to Home rather than to nothing.
		switch firstParam(params) {
		case 1, 7:
			return Key{Type: Home}
		case 3:
			return Key{Type: Delete}
		case 4, 8:
			return Key{Type: End}
		case 5:
			return Key{Type: PageUp}
		case 6:
			return Key{Type: PageDown}
		}
	}
	return Key{Type: Unknown}
}

func firstParam(params []byte) int {
	n := 0
	seen := false
	for _, b := range params {
		if b < '0' || b > '9' {
			break
		}
		n = n*10 + int(b-'0')
		seen = true
	}
	if !seen {
		return 0
	}
	return n
}
