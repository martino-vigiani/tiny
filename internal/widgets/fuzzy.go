package widgets

// Fuzzy matching: the one filter behaviour a substring scan cannot express.
//
// A fuzzy filter treats the typed text as a *subsequence* — "gof" finds
// "go-fmt" — which is what makes a long option list navigable by initials
// instead of by exact spelling. Order still matters ("abc" never finds "acb"),
// so the filter stays predictable: what you type is read left to right.
//
// Everything here works in runes decoded one at a time rather than on
// lowercased copies. strings.ToLower allocates a whole string per call, and the
// picker calls these once per option per keystroke; the same reasoning that
// keeps rendering allocation-free (see AGENTS.md) applies with more force here,
// because filtering runs over every option while drawing only runs over the
// visible window.

import (
	"unicode"
	"unicode/utf8"
)

// Scoring weights. The exact numbers matter less than their ordering, which
// encodes what "a better match" means, from strongest to weakest signal:
//
//	fuzzyStart        the match begins at the option's first rune
//	fuzzyBoundary     a rune lands at a word boundary ("go-FMT", "goFMT")
//	fuzzyConsecutive  a rune continues an unbroken run of matched runes
//	fuzzyMatched      flat credit, so a longer filter outranks a shorter one
//	fuzzyGap/fuzzyLead penalties for scattering the match or starting it late
//
// A rune earns the larger of the boundary and consecutive bonuses, never both:
// in a camel-cased run like "GoFmt" every rune is simultaneously consecutive
// and a boundary, and stacking would let one naming style dominate the ranking
// for reasons that have nothing to do with match quality.
const (
	fuzzyMatched     = 16
	fuzzyStart       = 32
	fuzzyBoundary    = 18
	fuzzyConsecutive = 20
	fuzzyGap         = 2  // per skipped rune between two matched runes
	fuzzyGapCap      = 12 // ...but one wide gap should not sink the whole match
	fuzzyLead        = 1  // per rune before the match starts
	fuzzyLeadCap     = 15

	// The raw score is scaled so that option length can only ever break a tie
	// between equally good matches: the smallest raw difference the weights can
	// produce (2) outweighs the largest length penalty (fuzzyLenCap).
	fuzzyRawWeight = 64
	fuzzyLenCap    = 50
)

// fuzzyMatch reports whether filter appears in option as a case-insensitive
// subsequence. It has the signature of matchFunc, so it drops into the picker
// beside matchSubstring and matchPrefix.
func fuzzyMatch(option, filter string) bool {
	if filter == "" {
		return true
	}
	// Deciding *whether* the filter fits needs none of the scoring machinery:
	// a single greedy pass from the front succeeds exactly when a subsequence
	// exists, and this is the call the picker makes for every option on every
	// keystroke.
	fi := 0
	fr, fsz := utf8.DecodeRuneInString(filter)
	for i := 0; i < len(option); {
		r, sz := utf8.DecodeRuneInString(option[i:])
		if foldEqual(r, fr) {
			fi += fsz
			if fi == len(filter) {
				return true
			}
			fr, fsz = utf8.DecodeRuneInString(filter[fi:])
		}
		i += sz
	}
	return false
}

// fuzzyScore ranks a match: higher is better, and the value is meaningful only
// against other options scored with the same filter. An empty filter matches
// everything and carries no ranking information, so it scores every option
// zero and leaves the caller's original order intact.
func fuzzyScore(option, filter string) (score int, matched bool) {
	if filter == "" {
		return 0, true
	}
	_, _, raw, ok := fuzzyBest(option, filter)
	if !ok {
		return 0, false
	}
	return raw*fuzzyRawWeight - min(utf8.RuneCountInString(option), fuzzyLenCap), true
}

// fuzzyPositions appends the rune indices of the matched runes to dst and
// returns it. The caller owns the slice so highlighting a redrawn list reuses
// one buffer instead of allocating per option, the same discipline Env.buf
// follows. A filter that does not match appends nothing.
//
// The indices are the ones the score was computed from, so the highlight and
// the ranking always tell the user the same story about why an option is here.
func fuzzyPositions(option, filter string, dst []int) []int {
	if filter == "" {
		return dst
	}
	start, at, _, ok := fuzzyBest(option, filter)
	if !ok {
		return dst
	}
	fi := 0
	fr, fsz := utf8.DecodeRuneInString(filter)
	for i := start; i < len(option); {
		r, sz := utf8.DecodeRuneInString(option[i:])
		if foldEqual(r, fr) {
			dst = append(dst, at)
			fi += fsz
			if fi == len(filter) {
				return dst
			}
			fr, fsz = utf8.DecodeRuneInString(filter[fi:])
		}
		at++
		i += sz
	}
	return dst
}

// fuzzyBest finds the window the option is judged by: the highest-scoring of
// the matches starting at each rune that could begin one. It returns that
// window's byte offset, its rune index (what a highlight counts in) and its raw
// score.
//
// Scoring only the first match a left-to-right scan finds is cheaper but reads
// wrong: in "source/src.go" the filter "src" would be pinned to the scattered
// "s..r.c" of "source" and never see the exact token further along, so the
// option would rank below worse ones. Trying every start costs O(len(option) ×
// len(filter)) on the worst input and far less on real ones, because a start
// that cannot complete the filter ends the scan — every later start is strictly
// more constrained, so none of them can complete it either.
func fuzzyBest(option, filter string) (start, at, raw int, ok bool) {
	fr, _ := utf8.DecodeRuneInString(filter)
	best := fuzzyNoMatch
	for i, lead := 0, 0; i < len(option); lead++ {
		r, sz := utf8.DecodeRuneInString(option[i:])
		if foldEqual(r, fr) {
			s := fuzzyWindow(option, filter, i, lead)
			if s == fuzzyNoMatch {
				break
			}
			if best == fuzzyNoMatch || s > best {
				best, start, at = s, i, lead
			}
		}
		i += sz
	}
	if best == fuzzyNoMatch {
		return 0, 0, 0, false
	}
	return start, at, best, true
}

// fuzzyNoMatch is the raw score meaning "the filter does not fit here". A real
// score can be negative (a late, scattered match), so it cannot be zero.
const fuzzyNoMatch = -1 << 30

// fuzzyWindow scores the match that starts at byte offset start, taking each
// filter rune at its earliest chance — which is also its most compact
// placement, since every bonus here rewards runes that stay together. lead is
// the rune index of start, the distance the match sits from the option's front.
func fuzzyWindow(option, filter string, start, lead int) int {
	raw := -min(lead*fuzzyLead, fuzzyLeadCap)

	// prev is the rune before the one being scored, which is what decides
	// whether a match sits on a word boundary. It starts as the rune before the
	// window; DecodeLastRune yields RuneError on an empty prefix, which is not
	// a boundary — exactly what we want at the front of the option.
	prev, _ := utf8.DecodeLastRuneInString(option[:start])
	fi := 0
	fr, fsz := utf8.DecodeRuneInString(filter)
	prevEnd := -1 // byte offset just past the previously matched rune
	gap := 0

	for i := start; i < len(option); {
		r, sz := utf8.DecodeRuneInString(option[i:])
		if foldEqual(r, fr) {
			raw += fuzzyMatched
			switch {
			case i == 0:
				raw += fuzzyStart
			case i == prevEnd:
				raw += max(fuzzyConsecutive, boundaryBonus(prev, r))
			default:
				raw += boundaryBonus(prev, r)
			}
			raw -= min(gap*fuzzyGap, fuzzyGapCap)
			gap = 0
			prevEnd = i + sz
			fi += fsz
			if fi == len(filter) {
				return raw
			}
			fr, fsz = utf8.DecodeRuneInString(filter[fi:])
		} else {
			gap++
		}
		prev = r
		i += sz
	}
	return fuzzyNoMatch
}

// boundaryBonus credits a rune that starts a word: after one of the separators
// people actually type in identifiers and paths, or at a lower-to-upper
// transition, which is where a camel-cased name hides its word breaks.
func boundaryBonus(prev, cur rune) int {
	switch prev {
	case ' ', '-', '_', '.', '/':
		return fuzzyBoundary
	}
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return fuzzyBoundary
	}
	return 0
}

// foldEqual compares two runes case-insensitively without lowering a string.
func foldEqual(a, b rune) bool {
	return a == b || unicode.ToLower(a) == unicode.ToLower(b)
}
