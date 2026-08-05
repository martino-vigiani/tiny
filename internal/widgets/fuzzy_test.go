package widgets

import (
	"fmt"
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		option, filter string
		want           bool
	}{
		// The subsequence people expect from initials.
		{"go-fmt", "gof", true},
		{"GoFmt", "gof", true},
		{"go-fmt", "GOF", true},
		{"go-fmt", "gfm", true},
		{"go-fmt", "go-fmt", true},
		// Order is part of the contract: a subsequence is read left to right.
		{"acb", "abc", false},
		{"go-fmt", "fgo", false},
		// Every rune has to be there, and only once per position.
		{"go-fmt", "goff", false},
		{"go", "gooo", false},
		// An empty filter is the unfiltered list.
		{"anything", "", true},
		{"", "", true},
		{"", "a", false},
	}
	for _, c := range cases {
		if got := fuzzyMatch(c.option, c.filter); got != c.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", c.option, c.filter, got, c.want)
		}
	}
}

func TestFuzzyMatchNonASCII(t *testing.T) {
	cases := []struct {
		option, filter string
		want           bool
	}{
		{"café", "cf", true},
		{"café", "cé", true},
		{"café", "CÉ", true}, // folding has to survive the accent
		{"Über", "üb", true}, // ...in both directions
		{"Über", "ÜB", true},
		{"café", "fé", true},
		{"café", "éc", false}, // still a subsequence, not a bag of runes
		{"日本語", "本語", true},
		{"日本語", "日語", true},
		{"日本語", "語日", false},
		{"ελληνικά", "λλ", true},
	}
	for _, c := range cases {
		if got := fuzzyMatch(c.option, c.filter); got != c.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", c.option, c.filter, got, c.want)
		}
	}
}

// scoreOrder returns the options sorted best-first, so the ranking tests can
// assert relative order instead of raw numbers and survive a weight tweak.
func scoreOrder(t *testing.T, options []string, filter string) []string {
	t.Helper()
	type scored struct {
		opt   string
		score int
	}
	var all []scored
	for _, opt := range options {
		s, ok := fuzzyScore(opt, filter)
		if !ok {
			t.Fatalf("fuzzyScore(%q, %q) did not match", opt, filter)
		}
		all = append(all, scored{opt, s})
	}
	// Insertion sort: the sets here are tiny and a stable order keeps a tie
	// from looking like a ranking decision.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].score > all[j-1].score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.opt
	}
	return out
}

// rankBefore asserts that every option ranks strictly above the ones after it.
func rankBefore(t *testing.T, filter string, order ...string) {
	t.Helper()
	for i := 1; i < len(order); i++ {
		hi, ok := fuzzyScore(order[i-1], filter)
		if !ok {
			t.Fatalf("fuzzyScore(%q, %q) did not match", order[i-1], filter)
		}
		lo, ok := fuzzyScore(order[i], filter)
		if !ok {
			t.Fatalf("fuzzyScore(%q, %q) did not match", order[i], filter)
		}
		if hi <= lo {
			t.Errorf("filter %q: %q (%d) should rank above %q (%d)",
				filter, order[i-1], hi, order[i], lo)
		}
	}
}

func TestFuzzyScoreRanking(t *testing.T) {
	// The exact token wins; a whole "src" run at a path boundary beats a run
	// broken up by other letters; a scattered match comes last.
	rankBefore(t, "src", "src", "third/src/main.go", "source", "scratch")

	// The exact token wins even when the same option opens with a worse,
	// scattered match of the same filter — the whole reason every start is
	// scored rather than just the first.
	rankBefore(t, "src", "source/src.go", "source")

	// A prefix beats the same match found later in the string.
	rankBefore(t, "log", "logger", "backlog")

	// Consecutive beats scattered at the same starting point.
	rankBefore(t, "abc", "abcdef", "axbxcx")

	// Word boundaries count, in every naming style people actually use.
	rankBefore(t, "gf", "go-fmt", "goofy")
	rankBefore(t, "gf", "GoFmt", "goofy")
	rankBefore(t, "gf", "go_fmt", "goofy")
	rankBefore(t, "gf", "go.fmt", "goofy")
	rankBefore(t, "gf", "go fmt", "goofy")

	// Same match quality, shorter option first.
	rankBefore(t, "cfg", "cfg", "cfg-with-a-very-long-tail")
}

func TestFuzzyScoreOrder(t *testing.T) {
	got := scoreOrder(t, []string{"scratch", "source", "third/src/main.go", "src"}, "src")
	want := []string{"src", "third/src/main.go", "source", "scratch"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranking = %v, want %v", got, want)
		}
	}
}

func TestFuzzyScoreNoMatch(t *testing.T) {
	if score, ok := fuzzyScore("go-fmt", "zzz"); ok || score != 0 {
		t.Errorf("fuzzyScore(%q, %q) = (%d, %v), want (0, false)", "go-fmt", "zzz", score, ok)
	}
}

// An empty filter matches everything and says nothing about ranking, so every
// option has to score the same — otherwise opening a picker would reorder the
// list before the user typed anything.
func TestFuzzyScoreEmptyFilter(t *testing.T) {
	for _, opt := range []string{"", "a", "a much longer option"} {
		score, ok := fuzzyScore(opt, "")
		if !ok || score != 0 {
			t.Errorf("fuzzyScore(%q, \"\") = (%d, %v), want (0, true)", opt, score, ok)
		}
	}
}

func TestFuzzyScoreNonASCII(t *testing.T) {
	// Folding and boundaries work the same on non-ASCII text.
	rankBefore(t, "él", "élan", "modéliser")
	rankBefore(t, "日語", "日本語", "明日は語る")
}

func TestFuzzyPositions(t *testing.T) {
	cases := []struct {
		option, filter string
		want           []int
	}{
		{"go-fmt", "gof", []int{0, 1, 3}},
		{"GoFmt", "gof", []int{0, 1, 2}},
		{"go-fmt", "", nil},
		{"go-fmt", "zzz", nil},
		// Rune indices, not byte offsets: "é" is two bytes.
		{"café", "cé", []int{0, 3}},
		{"日本語", "語", []int{2}},
		{"日本語ですが日本語", "語です", []int{2, 3, 4}},
		// The highlight follows the best-scoring window, not the first one a
		// left-to-right scan stumbles into: the exact "src" run wins over the
		// scattered letters ahead of it.
		{"source/src.go", "src", []int{7, 8, 9}},
	}
	for _, c := range cases {
		got := fuzzyPositions(c.option, c.filter, nil)
		if len(got) != len(c.want) {
			t.Errorf("fuzzyPositions(%q, %q) = %v, want %v", c.option, c.filter, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("fuzzyPositions(%q, %q) = %v, want %v", c.option, c.filter, got, c.want)
				break
			}
		}
	}
}

// fuzzyPositions exists so the picker can highlight matched runes on every
// redraw; that is only affordable if reusing one slice stops allocating.
func TestFuzzyPositionsReusesDestination(t *testing.T) {
	dst := make([]int, 0, 16)
	options := fuzzyBenchOptions(50)
	allocs := testing.AllocsPerRun(200, func() {
		for _, opt := range options {
			dst = fuzzyPositions(opt, "opt", dst[:0])
		}
	})
	if allocs != 0 {
		t.Errorf("fuzzyPositions with a reused destination allocated %.1f times per run", allocs)
	}
}

func TestFuzzyMatchAllocFree(t *testing.T) {
	options := fuzzyBenchOptions(50)
	allocs := testing.AllocsPerRun(200, func() {
		for _, opt := range options {
			fuzzyMatch(opt, "opt")
		}
	})
	if allocs != 0 {
		t.Errorf("fuzzyMatch allocated %.1f times per run", allocs)
	}
}

func TestFuzzyScoreAllocFree(t *testing.T) {
	options := fuzzyBenchOptions(50)
	allocs := testing.AllocsPerRun(200, func() {
		for _, opt := range options {
			fuzzyScore(opt, "opt")
		}
	})
	if allocs != 0 {
		t.Errorf("fuzzyScore allocated %.1f times per run", allocs)
	}
}

// fuzzyMatch has to be usable as a picker matchFunc without a wrapper.
func TestFuzzyMatchIsAMatchFunc(t *testing.T) {
	var m matchFunc = fuzzyMatch
	if !m("go-fmt", "gof") {
		t.Error("fuzzyMatch as matchFunc did not match")
	}
}

// fuzzyBenchOptions builds n plausible option strings: a mix of separators and
// casings, so the scan exercises the boundary rules rather than one shape.
func fuzzyBenchOptions(n int) []string {
	shapes := []string{
		"option-%d-value",
		"Option%dValue",
		"third/option_%d/main.go",
		"an ordinary option %d",
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(shapes[i%len(shapes)], i)
	}
	return out
}

func BenchmarkFuzzyMatch(b *testing.B) {
	options := fuzzyBenchOptions(200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, opt := range options {
			fuzzyMatch(opt, "opt")
		}
	}
}

func BenchmarkFuzzyScore(b *testing.B) {
	options := fuzzyBenchOptions(200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, opt := range options {
			fuzzyScore(opt, "opt")
		}
	}
}

func BenchmarkFuzzyPositions(b *testing.B) {
	options := fuzzyBenchOptions(200)
	dst := make([]int, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, opt := range options {
			dst = fuzzyPositions(opt, "opt", dst[:0])
		}
	}
}
