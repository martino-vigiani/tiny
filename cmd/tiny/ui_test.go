package main

import (
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// A tab must not survive into a line that is drawn at a fixed width.
//
// The four display commands each arrived with their own copy of the
// sanitising rule, and the copies disagreed: say keeps tabs (its input is a
// build log, where a tab is a column separator), box and bar strip them. spin
// borrowed say's copy — and a tab defeats exactly the clipping spin does to
// stay on one line, because it advances the cursor to the next tab stop while
// measuring as a single cell. The line then wraps, and a wrapped line cannot
// be erased by the next frame: the spinner leaves debris for the rest of the
// run.
func TestSanitisingIsPerCommandAndTabsNeverReachAFixedWidthLine(t *testing.T) {
	const withTab = "a\tb"

	// The one place a tab is meant to survive.
	if got := sanitizeText(withTab, true, false); got != withTab {
		t.Errorf("say-style sanitising dropped the tab: %q", got)
	}
	// Everywhere a line is measured and clipped, it must not.
	if got := sanitizeText(withTab, false, false); strings.ContainsRune(got, '\t') {
		t.Errorf("inline sanitising kept a tab: %q", got)
	}
	// A box keeps newlines, since its caller still needs the line breaks.
	if got := sanitizeText("a\nb", false, true); got != "a\nb" {
		t.Errorf("box-style sanitising dropped the newline: %q", got)
	}
	// Whatever the policy, the escape introducer always goes.
	for _, keepTab := range []bool{true, false} {
		if got := sanitizeText("x\x1b]52;c;aGk=\x07y", keepTab, true); strings.ContainsRune(got, 0x1b) {
			t.Errorf("an ESC survived sanitising: %q", got)
		}
	}
}

// spin draws a fixed-width line, so its message must be sanitised inline —
// this is the regression guard for the tab rule above reaching the animation.
func TestSpinMessageHasNoTabs(t *testing.T) {
	resolved, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	var out strings.Builder
	ui := &UI{Err: &out, Theme: resolved, TTY: false, Width: func() int { return 80 }}
	if code := runSpin([]string{"col\tonna", "--", "true"}, ui); code != 0 {
		t.Fatalf("runSpin = %d, want 0", code)
	}
	if strings.ContainsRune(out.String(), '\t') {
		t.Errorf("a tab reached the spinner line: %q", out.String())
	}
}

// A global flag written after the subcommand used to be swallowed as text:
// `tiny say -theme house.yaml "Deployed"` printed the flag instead of applying
// it, with nothing to say the theme had been ignored. Found by an agent that
// was given only the documentation and told to integrate tiny — it hit this on
// its first attempt at a themed script.
func TestMisplacedGlobalFlagIsRecognised(t *testing.T) {
	if got := misplacedGlobalFlag([]string{"-theme", "house.yaml", "Deployed"}); got != "-theme" {
		t.Errorf("misplacedGlobalFlag = %q, want -theme", got)
	}
	if got := misplacedGlobalFlag([]string{"--theme=house.yaml", "Deployed"}); got != "--theme" {
		t.Errorf("the =value form was not recognised: %q", got)
	}
	if got := misplacedGlobalFlag([]string{"Deployed", "--style", "note"}); got != "" {
		t.Errorf("a command's own flag was reported as misplaced: %q", got)
	}
	// Past `--` the words belong to whoever the command hands them to.
	if got := misplacedGlobalFlag([]string{"Building", "--", "go", "build", "-o", "out"}); got != "" {
		t.Errorf("the child command's own -o was reported as misplaced: %q", got)
	}
	if got := misplacedGlobalFlag([]string{"--", "-theme"}); got != "" {
		t.Errorf("a literal word after -- was reported as misplaced: %q", got)
	}
}
