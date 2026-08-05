package runner

// This is the only place encoding/json appears: it is the oracle the
// hand-written encoder is checked against. Being a _test.go import, it is
// compiled into the test binary and never into the shipped one — the parity
// guarantee costs the release nothing.

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
	"unicode/utf8"
)

// reference encodes answers the way tiny did before the hand-written encoder.
func reference(t *testing.T, answers map[string]any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(answers); err != nil {
		t.Fatalf("encoding/json failed on %#v: %v", answers, err)
	}
	return buf.String()
}

func assertParity(t *testing.T, answers map[string]any) {
	t.Helper()
	var got bytes.Buffer
	if err := PrintJSON(&got, answers); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}
	if want := reference(t, answers); got.String() != want {
		t.Errorf("mismatch for %#v\n got: %q\nwant: %q", answers, got.String(), want)
	}
}

func TestPrintJSONParity(t *testing.T) {
	cases := []map[string]any{
		{},
		{"a": "hello"},
		{"b": true, "c": false},
		{"nil": nil},
		{"i": int(3)},
		{"list": []string{"x", "y"}},
		{"empty": []string{}},
		{"nillist": []string(nil)},
		{"m": map[string]string{"k": "v", "a": "b"}},
		{"em": map[string]string{}},
		{"nm": map[string]string(nil)},
		// Sorting and nesting together.
		{"z": "last", "a": "first", "m": map[string]string{"z": "1", "a": "2"}},
		// Keys are escaped like values.
		{"key<with>&chars": "v", "key\"quote": "v", "tab\there": "v"},
	}
	for _, c := range cases {
		assertParity(t, c)
	}
}

// The float format switches notation at 1e-6 and 1e21 and trims the exponent's
// leading zero; every boundary has to land on the same bytes.
func TestPrintJSONParityFloats(t *testing.T) {
	for _, f := range []float64{
		0, math.Copysign(0, -1), 1, -1, 2, 3.5, -0.25,
		1e-6, 9.999999e-7, 1e-7, 1e-9, 1e-21, 1e-323,
		1e20, 1e21, 1.0000001e21, 1e22, math.MaxFloat64, math.SmallestNonzeroFloat64,
		0.1, 1.0 / 3.0, 18, 1234567890123,
	} {
		assertParity(t, map[string]any{"n": f})
	}
}

// An answer is arbitrary user text — the editor widget returns whatever bytes
// the file held. These are the cases a naive encoder gets wrong: HTML escaping,
// the JavaScript line terminators, and invalid UTF-8, which must become the
// escaped replacement character rather than raw bytes in the payload.
func TestPrintJSONParityHostileStrings(t *testing.T) {
	for _, s := range []string{
		"",
		"plain",
		`quote" backslash\ slash/`,
		"nl\n tab\t cr\r",
		"ctrl\x00\x01\x1f",
		"html < > & escaped",
		"accented é ü ñ, emoji 😀, CJK 漢字",
		"line separator",
		"paragraph separator",
		"invalid\xffbytes",
		"lone surrogate \xed\xa0\x80",
		"truncated \xf0\x9f",
		"\xff",
		"trailing invalid \xc3",
	} {
		assertParity(t, map[string]any{"s": s})
		assertParity(t, map[string]any{s: "as a key"})
		assertParity(t, map[string]any{"list": []string{s}})
		assertParity(t, map[string]any{"map": map[string]string{s: s}})
	}
}

// A wizard's whole answer map, of the shape the runner actually produces.
func TestPrintJSONParityRealisticAnswers(t *testing.T) {
	assertParity(t, map[string]any{
		"framework":    "SwiftUI",
		"platforms":    []string{"iOS", "macOS"},
		"init_git":     true,
		"min_deploy":   float64(18),
		"row":          int(2),
		"env":          map[string]string{"LOG_LEVEL": "info", "REGION": "eu-west-1"},
		"notes":        "multi\nline & <tagged>",
		"skipped":      nil,
		"passphrase":   "",
		"no_platforms": []string{},
	})
}

// Randomised strings over the byte ranges that matter, so the parity claim is
// not limited to the cases somebody thought to write down.
func TestPrintJSONParityFuzzedStrings(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	buf := make([]byte, 0, 64)
	for i := 0; i < 2000; i++ {
		buf = buf[:0]
		for n := rng.Intn(24); n > 0; n-- {
			switch rng.Intn(4) {
			case 0: // control and ASCII punctuation
				buf = append(buf, byte(rng.Intn(0x40)))
			case 1: // printable ASCII
				buf = append(buf, byte(0x20+rng.Intn(0x5f)))
			case 2: // arbitrary high bytes, often invalid UTF-8
				buf = append(buf, byte(0x80+rng.Intn(0x80)))
			case 3: // a valid multi-byte rune
				buf = utf8.AppendRune(buf, rune(rng.Intn(0x2200)))
			}
		}
		assertParity(t, map[string]any{"s": string(buf)})
	}
}
