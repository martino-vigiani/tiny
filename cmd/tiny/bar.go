package main

// bar: a progress line meant to be called once per update from a shell loop.
//
//	tiny bar 3 12 "files processed"
//	for i in $(seq 0 20); do tiny bar $i 20 "step"; sleep 0.1; done
//
// Each call draws the whole line from scratch; there is no state between
// invocations. On a terminal it returns to column zero and erases the old
// line instead of scrolling, which is what turns repeated calls into an
// animation rather than a wall of output — except for the call that reaches
// the total, which ends with a newline so whatever the caller prints next
// does not land on top of a finished bar. Without a terminal it is one plain,
// newline-terminated line with no escapes at all: a CI log where every update
// overwrote the last one is a log with no progress in it.
//
// Unlike the other display commands, bar deliberately ignores the theme's
// color pairs. Its one hard requirement — zero escape sequences reach a
// non-terminal — has to hold no matter how the theme resolved color, so the
// simplest way to guarantee it is to never emit color for the bar at all.

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// barGlyphs are the two characters a bar is drawn from.
type barGlyphs struct {
	filled, empty string
}

// glyphsFor picks ASCII glyphs for the presets that exist specifically for
// terminals or pipelines that cannot render Unicode; everything else gets the
// block glyphs.
func glyphsFor(themeName string) barGlyphs {
	switch themeName {
	case "ascii", "plain":
		return barGlyphs{filled: "#", empty: "-"}
	default:
		return barGlyphs{filled: "█", empty: "░"}
	}
}

// runBar implements `tiny bar <current> <total> [label...]`.
func runBar(args []string, ui *UI) int {
	if len(args) < 2 {
		return usageError(ui, "bar requires <current> <total> [label]")
	}
	current, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return usageError(ui, "bar: invalid current value %q: not a number", args[0])
	}
	total, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return usageError(ui, "bar: invalid total value %q: not a number", args[1])
	}
	// A total of zero (or less) has no meaningful percentage — rejecting it
	// outright is what keeps the division below from ever running on it.
	if total <= 0 {
		return usageError(ui, "bar: total must be greater than zero, got %v", total)
	}
	// A negative current is not a value that can be clamped into something
	// meaningful, so it is a usage error like a non-numeric argument.
	if current < 0 {
		return usageError(ui, "bar: current must not be negative, got %v", current)
	}
	// Overshoot, by contrast, is what a caller's own rounding produces (e.g.
	// a float division catching up a fraction late): it is clamped to a
	// completed bar rather than rejected, so the loop that produced it does
	// not have to special-case its last iteration.
	if current > total {
		current = total
	}
	label := strings.Join(args[2:], " ")

	width := ui.Width()
	if width <= 0 {
		width = 80
	}
	line := renderBar(current, total, label, width, glyphsFor(ui.Theme.Name), ui.Theme.Ellipsis)
	done := current >= total

	if ui.TTY {
		ui.Write(theme.ColumnZero)
		ui.Write(theme.EraseEOL)
		ui.Write(line)
		if done {
			ui.Write("\n")
		}
		return 0
	}
	// No terminal: one plain line, always newline-terminated, no cursor
	// movement — the caller's log gets one final line per call, not a
	// redraw it can never see happen.
	ui.Write(line)
	ui.Write("\n")
	return 0
}

// renderBar builds the full "[bar] pct% (cur/total) label" line, never wider
// than width cells.
//
// The percentage and the counts are what makes the bar precisely readable, so
// they are the last things dropped as the terminal narrows; the bar itself is
// what shrinks first, down to zero cells if it has to. Whatever the layout
// math above decides, a final cell-based clip guarantees the invariant the
// caller relies on: this line never wraps, on any width, including the ones
// too narrow for any of this to look good.
func renderBar(current, total float64, label string, width int, glyphs barGlyphs, ellipsis string) string {
	if width < 1 {
		width = 1
	}
	pct := percentOf(current, total)
	pctStr := fmt.Sprintf("%3d%%", pct)
	countsStr := "(" + formatCount(current) + "/" + formatCount(total) + ")"
	label = sanitizeLabel(label)

	const brackets = 2 // "[" + "]"
	const minBarCells = 1

	fixed := brackets + 1 + cellWidth(pctStr) + 1 + cellWidth(countsStr)
	if width < fixed+minBarCells {
		// Not enough room for the counts: drop them first.
		countsStr = ""
		fixed = brackets + 1 + cellWidth(pctStr)
	}
	if width < fixed+minBarCells {
		// Still not enough: the percentage goes too, leaving just the bar.
		pctStr = ""
		fixed = brackets
	}
	barWidth := width - fixed
	if barWidth < 0 {
		barWidth = 0
	}
	// A label competes with the bar for the same leftover space. Without a
	// cap here the bar would spend every cell of a wide terminal on itself,
	// leaving nothing for the text explaining what it is counting — the
	// label would silently vanish on an ordinary 80-column window. Capping
	// the bar (only when there is a label to make room for) is what keeps
	// "tiny bar 3 12 files processed" showing the label at normal widths.
	const maxBarWidthWithLabel = 40
	if label != "" && barWidth > maxBarWidthWithLabel {
		barWidth = maxBarWidthWithLabel
	}

	filled := int(float64(barWidth) * float64(pct) / 100)
	if filled > barWidth {
		filled = barWidth
	}

	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < filled; i++ {
		b.WriteString(glyphs.filled)
	}
	for i := filled; i < barWidth; i++ {
		b.WriteString(glyphs.empty)
	}
	b.WriteByte(']')
	if pctStr != "" {
		b.WriteByte(' ')
		b.WriteString(pctStr)
	}
	if countsStr != "" {
		b.WriteByte(' ')
		b.WriteString(countsStr)
	}

	if used := cellWidth(b.String()); label != "" && width-used > 1 {
		room := width - used - 1 // the separating space
		b.WriteByte(' ')
		b.WriteString(clipCells(label, room, ellipsis))
	}

	return clipCells(b.String(), width, "")
}

// percentOf returns current/total as a rounded, clamped 0-100 percentage.
// total is guaranteed positive by the caller, but the clamp stays here too so
// this function has no precondition of its own to remember.
func percentOf(current, total float64) int {
	if total <= 0 {
		return 0
	}
	p := current / total * 100
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return int(p + 0.5)
}

// formatCount renders a count without a trailing ".0" for the common case of
// whole numbers, since "3/12" is what a progress bar is expected to show, not
// "3.0/12.0".
func formatCount(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// sanitizeLabel strips control characters from spec text before it reaches the
// terminal.
//
// This is a duplicate of widgets.Env.appendText's loop, not a reuse of it: it
// is unexported in another package, and the reason for the check is the same
// one documented there — a label can come from a script or a CI variable
// built from elsewhere, and a raw escape sequence in it (e.g. an OSC 52
// clipboard write) would execute on the terminal instead of just printing as
// text. Removing the ESC/control byte leaves any following printable
// characters visible but inert, which is enough: they can no longer be
// interpreted as part of a sequence.
func sanitizeLabel(text string) string {
	return sanitizeText(text, false, false)
}

// clipCells truncates s to at most limit terminal cells, appending ellipsis
// when it had to cut. It returns "" for a non-positive limit rather than
// panicking on a slice bound, which is what a very narrow terminal width
// would otherwise hit.
func clipCells(s string, limit int, ellipsis string) string {
	if limit <= 0 {
		return ""
	}
	if cellWidth(s) <= limit {
		return s
	}
	keep := limit - cellWidth(ellipsis)
	if keep < 0 {
		keep = 0
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
	return s[:cut] + ellipsis
}

// cellWidth and runeCells are shared with box.go: both files independently
// needed internal/widgets/frame.go's unexported cell-width logic (unreachable
// from this package) for the same reason — truncating by rune count instead
// of cells lets a CJK label overflow a line meant to stay a fixed size — so
// rather than declare the same two functions twice in one package, bar.go
// reuses box.go's copy. See box.go for the fuller rationale.
