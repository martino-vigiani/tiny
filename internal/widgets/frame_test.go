package widgets

import "testing"

// A decomposed character measures the same as its precomposed form: the
// combining mark draws no column of its own.
//
// This branch was dead code for a while \u2014 it sat below an `r < 0x1100`
// fast path that already claimed the range \u2014 and the cost was invisible:
// every clip on accented text landed one column early, with nothing on screen
// to say why. The two forms are written with explicit escapes rather than as
// literals, because a decomposed sequence pasted into a source file is one
// editor normalisation away from silently becoming the precomposed one, and
// the test would then pass without testing anything.
func TestCombiningMarksAreZeroWidth(t *testing.T) {
	const precomposed = "caff\u00e8" // one rune
	const decomposed = "caffe\u0300" // e + combining grave, two runes
	if len([]rune(decomposed)) != len([]rune(precomposed))+1 {
		t.Fatal("the decomposed form is not decomposed; the test would prove nothing")
	}
	if got, want := cellWidth(decomposed), cellWidth(precomposed); got != want {
		t.Errorf("cellWidth(decomposed) = %d, want %d (the same word, written two ways)", got, want)
	}
	if got := runeCells('\u0300'); got != 0 {
		t.Errorf("runeCells(combining grave) = %d, want 0", got)
	}
	// The fast path the zero-width cases now sit in front of must still work.
	if got := runeCells('a'); got != 1 {
		t.Errorf("runeCells('a') = %d, want 1", got)
	}
	if got := runeCells('\u4e16'); got != 2 {
		t.Errorf("runeCells(CJK) = %d, want 2", got)
	}
	if got := runeCells('\u200b'); got != 0 {
		t.Errorf("runeCells(zero-width space) = %d, want 0", got)
	}
}
