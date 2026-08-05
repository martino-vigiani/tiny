package runner

// A hand-written JSON encoder for the closed value space widgets produce:
// string, bool, float64, int, []string, map[string]string and nil. Nothing
// here is generic — that is the point. Dropping encoding/json removes it and
// its share of the reflect machinery from the binary (about 150 KB, 5% of the
// shipped size) for code whose entire job is six cases.
//
// The output is byte-identical to `json.Encoder` with SetIndent("", "  "),
// including HTML escaping, the float format switch, and the UTF-8 rules.
// TestPrintJSONParity pins that against encoding/json itself, which is
// imported by the test only, so the guarantee costs the binary nothing.

import (
	"io"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"
)

const hexDigits = "0123456789abcdef"

// The two runes that are valid JSON but terminate a line in JavaScript;
// encoding/json escapes them, so this encoder must too.
const (
	lineSeparator      = 0x2028
	paragraphSeparator = 0x2029
)

// appendJSONString appends s as a JSON string literal.
//
// It reproduces encoding/json's default encoder: `<`, `>` and `&` are escaped
// (HTML escaping is on by default), invalid UTF-8 bytes become U+FFFD, and
// U+2028/U+2029 are escaped because they terminate a line in JavaScript. The
// last two matter here: an answer is arbitrary user text, and the editor widget
// hands back whatever bytes the file contained.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '\\', '"':
				b = append(b, '\\', c)
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', 'f', 'f', 'f', 'd')
			i += size
			start = i
			continue
		}
		if r == lineSeparator || r == paragraphSeparator {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}

// appendJSONFloat appends f in encoding/json's number format: %f notation
// except for magnitudes below 1e-6 or at/above 1e21, which switch to %e with
// the exponent's leading zero trimmed (e-09 becomes e-9).
//
// Widgets reject NaN and infinities before they reach here, so — unlike
// encoding/json, which errors on them — this cannot be handed one.
func appendJSONFloat(b []byte, f float64) []byte {
	format := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	b = strconv.AppendFloat(b, f, format, -1, 64)
	if format == 'e' {
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b
}

// PrintJSONCompact writes the same payload on a single line, with no indent
// and no spaces. It exists because a caller that reads the payload back — an
// agent, a script — pays for every byte of whitespace and gains nothing from
// it: the compact form is about 40% smaller for the same information.
func PrintJSONCompact(w io.Writer, answers map[string]any) error {
	keys := sortedKeys(answers)
	b := make([]byte, 0, 256)
	b = append(b, '{')
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendJSONString(b, k)
		b = append(b, ':')
		b = appendJSONValueCompact(b, answers[k])
	}
	b = append(b, '}', '\n')
	_, err := w.Write(b)
	return err
}

func appendJSONValueCompact(b []byte, v any) []byte {
	switch t := v.(type) {
	case []string:
		if t == nil {
			return append(b, "null"...)
		}
		b = append(b, '[')
		for i, s := range t {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendJSONString(b, s)
		}
		return append(b, ']')
	case map[string]string:
		if t == nil {
			return append(b, "null"...)
		}
		b = append(b, '{')
		for i, k := range sortedMapKeys(t) {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendJSONString(b, k)
			b = append(b, ':')
			b = appendJSONString(b, t[k])
		}
		return append(b, '}')
	default:
		return appendJSONValue(b, v)
	}
}

func sortedKeys(answers map[string]any) []string {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// PrintJSON serialises answers as indented JSON, keys sorted.
func PrintJSON(w io.Writer, answers map[string]any) error {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b := make([]byte, 0, 256)
	b = append(b, '{')
	if len(keys) == 0 {
		_, err := w.Write(append(b, '}', '\n'))
		return err
	}
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '\n', ' ', ' ')
		b = appendJSONString(b, k)
		b = append(b, ':', ' ')
		b = appendJSONValue(b, answers[k])
	}
	b = append(b, '\n', '}', '\n')
	_, err := w.Write(b)
	return err
}

// appendJSONValue encodes one answer. An unknown type encodes as null rather
// than failing: the canonical types are enforced by the widget contract and
// the parity test, so reaching the default case means a bug upstream, not
// user input worth erroring on mid-write.
func appendJSONValue(b []byte, v any) []byte {
	switch t := v.(type) {
	case nil:
		return append(b, "null"...)
	case bool:
		return strconv.AppendBool(b, t)
	case string:
		return appendJSONString(b, t)
	case float64:
		return appendJSONFloat(b, t)
	case int:
		return strconv.AppendInt(b, int64(t), 10)
	case []string:
		if t == nil {
			return append(b, "null"...)
		}
		if len(t) == 0 {
			return append(b, '[', ']')
		}
		b = append(b, '[')
		for i, s := range t {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, '\n', ' ', ' ', ' ', ' ')
			b = appendJSONString(b, s)
		}
		return append(b, '\n', ' ', ' ', ']')
	case map[string]string:
		if t == nil {
			return append(b, "null"...)
		}
		if len(t) == 0 {
			return append(b, '{', '}')
		}
		mk := make([]string, 0, len(t))
		for k := range t {
			mk = append(mk, k)
		}
		sort.Strings(mk)
		b = append(b, '{')
		for i, k := range mk {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, '\n', ' ', ' ', ' ', ' ')
			b = appendJSONString(b, k)
			b = append(b, ':', ' ')
			b = appendJSONString(b, t[k])
		}
		return append(b, '\n', ' ', ' ', '}')
	default:
		return append(b, "null"...)
	}
}
