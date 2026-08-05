package widgets_test

import (
	"bufio"
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/keys"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// richEnv builds an Env driving the interactive path from a scripted key
// sequence. Because Keys is set, no terminal is touched and raw mode is never
// entered — which is what makes the live prompt testable at all.
func richEnv(t *testing.T, script []keys.Key, width, height int) (*widgets.Env, *bytes.Buffer) {
	t.Helper()
	resolved, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	var out bytes.Buffer
	return &widgets.Env{
		In:    bufio.NewReader(strings.NewReader("")),
		Out:   &out,
		IsTTY: true,
		Rich:  true,
		Theme: resolved,
		Keys:  &keys.Script{Keys: script},
		Size:  func() (int, int) { return width, height },
	}, &out
}

func key(t keys.Type) keys.Key         { return keys.Key{Type: t} }
func runeKey(r rune) keys.Key          { return keys.Key{Type: keys.Rune, Rune: r} }
func step(typ spec.StepType) spec.Step { return spec.Step{Type: typ, Key: "k", Title: "Pick"} }
func withOptions(s spec.Step, o ...string) spec.Step {
	s.Options = o
	return s
}

func TestRichSelectArrowsAndEnter(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.Down), key(keys.Down), key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Zig" {
		t.Errorf("answer = %v, want Zig", got)
	}
}

func TestRichSelectTypeToFilter(t *testing.T) {
	env, out := richEnv(t, []keys.Key{runeKey('u'), runeKey('s'), key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Rust" {
		t.Errorf("answer = %v, want the filtered match Rust", got)
	}
	if !strings.Contains(out.String(), "us") {
		t.Errorf("UI = %q, want the typed filter echoed", out.String())
	}
}

// Backspace widens the match set again, and the cursor stays on the option it
// was already on — losing your place every time you edit the filter is the
// thing that makes a picker feel unpredictable.
func TestRichSelectBackspaceKeepsThePlace(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{
		runeKey('z'), key(keys.Backspace), key(keys.Enter),
	}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Zig" {
		t.Errorf("answer = %v, want the cursor still on Zig after the filter widened", got)
	}
}

func TestRichSelectStartsOnTheDefault(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	s.Default = "Zig"
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Zig" {
		t.Errorf("answer = %v, want the default under the cursor", got)
	}
}

func TestRichMultiselectSpaceToggles(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{
		key(keys.Space), key(keys.Down), key(keys.Down), key(keys.Space), key(keys.Enter),
	}, 80, 24)
	s := withOptions(step(spec.TypeMultiselect), "iOS", "macOS", "tvOS")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	want := []string{"iOS", "tvOS"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("answer = %v, want %v in spec order", got, want)
	}
}

func TestRichMultiselectStartsFromTheDefault(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeMultiselect), "iOS", "macOS", "tvOS")
	s.Default = "iOS, tvOS"
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	want := []string{"iOS", "tvOS"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("answer = %v, want the default preselected", got)
	}
}

func TestRichMultiselectSelectAll(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.CtrlA), key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeMultiselect), "iOS", "macOS")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"iOS", "macOS"}) {
		t.Errorf("answer = %v, want everything selected", got)
	}
}

// Ctrl-C at a live prompt cancels the wizard rather than answering it.
func TestRichCancelReturnsCanceled(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.CtrlC)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust")
	_, err := widgets.RunStep(context.Background(), s, env)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// Esc clears the filter first and only cancels when there is nothing to clear,
// so a mistyped filter is not a lost wizard.
func TestRichEscClearsFilterBeforeCancelling(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{runeKey('z'), key(keys.Esc), key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	got, err := widgets.RunStep(context.Background(), s, env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "Zig" {
		t.Errorf("answer = %v, want Esc to have cleared the filter without cancelling", got)
	}
}

// The invariant that makes the live prompt safe to ship: it is a different way
// of choosing, not a different set of answers. For each case the rich path and
// the line-based path must return the same Go type and the same value.
func TestRichAnswersMatchLineMode(t *testing.T) {
	tests := []struct {
		name   string
		step   spec.Step
		script []keys.Key
		typed  string
	}{
		{
			name:   "select by cursor vs by number",
			step:   withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig"),
			script: []keys.Key{key(keys.Down), key(keys.Enter)},
			typed:  "2\n",
		},
		{
			name:   "select by filter vs by name",
			step:   withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig"),
			script: []keys.Key{runeKey('z'), key(keys.Enter)},
			typed:  "Zig\n",
		},
		{
			name:   "multiselect by toggling vs by list",
			step:   withOptions(step(spec.TypeMultiselect), "iOS", "macOS", "tvOS"),
			script: []keys.Key{key(keys.Space), key(keys.Down), key(keys.Space), key(keys.Enter)},
			typed:  "1,2\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			richEnvironment, _ := richEnv(t, tc.script, 80, 24)
			rich, err := widgets.RunStep(context.Background(), tc.step, richEnvironment)
			if err != nil {
				t.Fatalf("rich RunStep error: %v", err)
			}

			resolved, _ := theme.Resolve(theme.Default(), false)
			var out bytes.Buffer
			lineEnv := &widgets.Env{
				In:    bufio.NewReader(strings.NewReader(tc.typed)),
				Out:   &out,
				Theme: resolved,
			}
			line, err := widgets.RunStep(context.Background(), tc.step, lineEnv)
			if err != nil {
				t.Fatalf("line RunStep error: %v", err)
			}

			if !reflect.DeepEqual(rich, line) {
				t.Errorf("rich = %#v (%T), line = %#v (%T); the two modes must agree",
					rich, rich, line, line)
			}
		})
	}
}

// A list longer than the terminal must be windowed, and no line may exceed the
// width — a wrapped line would desynchronise the redraw and corrupt the frame.
func TestRichFrameFitsTheTerminal(t *testing.T) {
	options := make([]string, 60)
	for i := range options {
		options[i] = strings.Repeat("x", 200)
	}
	env, out := richEnv(t, []keys.Key{key(keys.Enter)}, 40, 10)
	s := withOptions(step(spec.TypeSelect), options...)
	if _, err := widgets.RunStep(context.Background(), s, env); err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	for _, line := range strings.Split(stripEscapes(out.String()), "\n") {
		if n := len([]rune(line)); n > 40 {
			t.Errorf("line of %d runes exceeds the 40-column terminal: %q", n, line)
		}
	}
}

// Redrawing must wind back exactly as many lines as the previous frame drew.
func TestRichRedrawsInPlace(t *testing.T) {
	env, out := richEnv(t, []keys.Key{key(keys.Down), key(keys.Enter)}, 80, 24)
	s := withOptions(step(spec.TypeSelect), "Go", "Rust", "Zig")
	if _, err := widgets.RunStep(context.Background(), s, env); err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	ui := out.String()
	if !strings.Contains(ui, "\x1b[?25l") || !strings.Contains(ui, "\x1b[?25h") {
		t.Errorf("UI = %q, want the hardware cursor hidden and restored", ui)
	}
	// The default header opens with a blank line, so a frame is: blank, title,
	// filter, three options — six lines, and the next frame winds back five.
	if !strings.Contains(ui, "\r\x1b[6A") {
		t.Errorf("UI = %q, want the second frame to wind back over the first", ui)
	}
}

// stripEscapes removes ANSI sequences so a test can measure visible width.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e && i > 0 && s[i-1] != 0x1b) {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
