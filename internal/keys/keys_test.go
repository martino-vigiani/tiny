package keys_test

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/martino-vigiani/tiny/internal/keys"
)

func decode(t *testing.T, input string) []keys.Key {
	t.Helper()
	d := keys.NewDecoder(bufio.NewReader(bytes.NewReader([]byte(input))))
	var got []keys.Key
	for {
		k, err := d.Next()
		if err != nil {
			if keys.IsEOF(err) {
				return got
			}
			t.Fatalf("Next error: %v", err)
		}
		got = append(got, k)
	}
}

func TestDecodeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []keys.Type
	}{
		{"csi arrows", "\x1b[A\x1b[B\x1b[C\x1b[D", []keys.Type{keys.Up, keys.Down, keys.Right, keys.Left}},
		{"application arrows", "\x1bOA\x1bOB", []keys.Type{keys.Up, keys.Down}},
		{"home and end", "\x1b[H\x1b[F\x1b[1~\x1b[4~", []keys.Type{keys.Home, keys.End, keys.Home, keys.End}},
		{"paging", "\x1b[5~\x1b[6~", []keys.Type{keys.PageUp, keys.PageDown}},
		{"delete", "\x1b[3~", []keys.Type{keys.Delete}},
		{"shift tab", "\x1b[Z", []keys.Type{keys.ShiftTab}},
		{"enter both ways", "\r\n", []keys.Type{keys.Enter, keys.Enter}},
		{"backspace both ways", "\x7f\x08", []keys.Type{keys.Backspace, keys.Backspace}},
		{"control keys", "\x03\x04\x15\x17\x01", []keys.Type{keys.CtrlC, keys.CtrlD, keys.CtrlU, keys.CtrlW, keys.CtrlA}},
		{"readline movement", "\x10\x0e", []keys.Type{keys.Up, keys.Down}},
		{"tab and space", "\t ", []keys.Type{keys.Tab, keys.Space}},
		{"modified arrow degrades to the base key", "\x1b[1;5A", []keys.Type{keys.Up}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decode(t, tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("decoded %d keys, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].Type != tc.want[i] {
					t.Errorf("key %d = %v, want %v", i, got[i].Type, tc.want[i])
				}
			}
		})
	}
}

func TestDecodeRunes(t *testing.T) {
	got := decode(t, "aZ9è漢")
	want := []rune{'a', 'Z', '9', 'è', '漢'}
	if len(got) != len(want) {
		t.Fatalf("decoded %d keys, want %d: %+v", len(got), len(want), got)
	}
	for i, r := range want {
		if got[i].Type != keys.Rune || got[i].Rune != r {
			t.Errorf("key %d = %+v, want the rune %q", i, got[i], r)
		}
	}
}

// An unrecognised escape must be swallowed whole. If its tail leaked through it
// would be read as typing, which in a filter means the user's search box fills
// with garbage when they press an unsupported key.
func TestUnknownSequenceIsConsumedWhole(t *testing.T) {
	got := decode(t, "\x1b[200~x")
	if len(got) != 2 {
		t.Fatalf("decoded %+v, want the unknown sequence plus one rune", got)
	}
	if got[0].Type != keys.Unknown {
		t.Errorf("first key = %v, want Unknown", got[0].Type)
	}
	if got[1].Type != keys.Rune || got[1].Rune != 'x' {
		t.Errorf("second key = %+v, want the rune x", got[1])
	}
}

// Esc with nothing behind it is a keypress; Esc with a sequence behind it is a
// prefix. Both come from one Read, which is what makes the timer unnecessary.
func TestEscAloneVersusPrefix(t *testing.T) {
	if got := decode(t, "\x1b"); len(got) != 1 || got[0].Type != keys.Esc {
		t.Errorf("lone Esc decoded as %+v, want Esc", got)
	}
	if got := decode(t, "\x1b[A"); len(got) != 1 || got[0].Type != keys.Up {
		t.Errorf("Esc-prefixed arrow decoded as %+v, want Up", got)
	}
}

// chunkReader hands out one chunk per Read, so a test can force an escape
// sequence to arrive split the way a loaded tmux or a slow link would split it.
type chunkReader struct{ chunks [][]byte }

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[0])
	c.chunks = c.chunks[1:]
	return n, nil
}

// The known limit of the timerless heuristic, pinned rather than hidden: if the
// terminal splits a sequence across two writes, the Esc reads as a lone Esc and
// the rest as text. Documented in the Decoder.Next comment.
func TestEscSplitAcrossReads(t *testing.T) {
	d := keys.NewDecoder(bufio.NewReader(&chunkReader{chunks: [][]byte{
		[]byte("\x1b"), []byte("[A"),
	}}))
	first, err := d.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if first.Type != keys.Esc {
		t.Errorf("first key = %v; a split sequence is expected to read as a lone Esc", first.Type)
	}
	second, err := d.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if second.Type != keys.Rune || second.Rune != '[' {
		t.Errorf("second key = %+v; the split tail is expected to read as text", second)
	}
}

func TestScriptSource(t *testing.T) {
	s := &keys.Script{Keys: []keys.Key{{Type: keys.Down}, {Type: keys.Enter}}}
	for _, want := range []keys.Type{keys.Down, keys.Enter} {
		k, err := s.Next()
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if k.Type != want {
			t.Errorf("key = %v, want %v", k.Type, want)
		}
	}
	if _, err := s.Next(); !keys.IsEOF(err) {
		t.Errorf("a spent script returned %v, want EOF", err)
	}
}

// Restoration must survive being asked twice: Close runs on the widget's defer
// and RestoreAll runs from the signal path, and both can happen.
func TestCloseIsIdempotent(t *testing.T) {
	var s *keys.Session
	if err := s.Close(); err != nil {
		t.Errorf("Close on a nil session = %v, want nil", err)
	}
	keys.RestoreAll() // must not panic with nothing registered
}

func BenchmarkDecodeArrow(b *testing.B) {
	input := bytes.Repeat([]byte("\x1b[A"), b.N)
	d := keys.NewDecoder(bufio.NewReader(bytes.NewReader(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Next(); err != nil {
			b.Fatal(err)
		}
	}
}
