package theme_test

import (
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/theme"
)

func TestParseStyle(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"", ""},
		{"none", ""},
		{"bold", "\x1b[1m"},
		{"bold cyan", "\x1b[1;36m"},
		{"bright-red", "\x1b[91m"},
		{"bg-blue", "\x1b[44m"},
		{"color:33", "\x1b[38;5;33m"},
		{"bg-color:200", "\x1b[48;5;200m"},
		{"#ff8800", "\x1b[38;2;255;136;0m"},
		{"bg-#000000", "\x1b[48;2;0;0;0m"},
		{"underline #bd93f9", "\x1b[4;38;2;189;147;249m"},
	}
	for _, tc := range tests {
		got, err := theme.ParseStyle(tc.expr)
		if err != nil {
			t.Errorf("ParseStyle(%q) error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseStyle(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestParseStyleInvalid(t *testing.T) {
	for _, expr := range []string{"chartreuse", "color:256", "color:-1", "#ff88", "bg-nope"} {
		if _, err := theme.ParseStyle(expr); err == nil {
			t.Errorf("ParseStyle(%q) = nil error, want failure", expr)
		}
	}
}

func TestResolveDefaultLook(t *testing.T) {
	r, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if r.HeaderOpen != "\n— " || r.HeaderClose != " —\n" {
		t.Errorf("default header = %q..%q, want the em-dash divider", r.HeaderOpen, r.HeaderClose)
	}
	if r.Prompt != "> " {
		t.Errorf("default prompt = %q, want %q", r.Prompt, "> ")
	}
	if r.MarkerOpen != "" {
		t.Errorf("colors must stay off when the stream is not color capable, got %q", r.MarkerOpen)
	}
}

func TestResolveColorCapability(t *testing.T) {
	colored, err := theme.Resolve(theme.Default(), true)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !strings.Contains(colored.HeaderOpen, "\x1b[1m") {
		t.Errorf("header = %q, want the bold escape when color is available", colored.HeaderOpen)
	}
	if !strings.HasPrefix(colored.HeaderOpen, "\n") {
		t.Errorf("header = %q, want the newline outside the escape sequence", colored.HeaderOpen)
	}

	never := theme.Default()
	never.Color = theme.ColorNever
	plain, err := theme.Resolve(never, true)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if strings.Contains(plain.HeaderOpen, "\x1b") {
		t.Errorf("color: never still emitted an escape: %q", plain.HeaderOpen)
	}

	always := theme.Default()
	always.Color = theme.ColorAlways
	forced, err := theme.Resolve(always, false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if !strings.Contains(forced.HeaderOpen, "\x1b") {
		t.Errorf("color: always emitted no escape on a non-capable stream: %q", forced.HeaderOpen)
	}
}

func TestPresetsResolve(t *testing.T) {
	for _, name := range theme.PresetNames() {
		p, ok := theme.Preset(name)
		if !ok {
			t.Fatalf("Preset(%q) not found although listed", name)
		}
		r, err := theme.Resolve(p, true)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", name, err)
			continue
		}
		if r.Name != name {
			t.Errorf("Resolve(%q).Name = %q, want %q", name, r.Name, name)
		}
		if r.TableRule == "" {
			t.Errorf("preset %q resolved with an empty table rule", name)
		}
	}
}

func TestPlainPresetNeverColors(t *testing.T) {
	p, _ := theme.Preset("plain")
	r, err := theme.Resolve(p, true)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	for _, s := range []string{r.HeaderOpen, r.HeaderClose, r.Prompt, r.ErrorOpen, r.MarkerOpen} {
		if strings.Contains(s, "\x1b") {
			t.Errorf("plain preset emitted an escape: %q", s)
		}
	}
}

func TestPartialThemeInheritsDefault(t *testing.T) {
	partial := theme.Theme{Symbols: theme.Symbols{Prompt: strPtr("❯ ")}}
	r, err := theme.Resolve(partial, false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if r.Prompt != "❯ " {
		t.Errorf("prompt = %q, want the override", r.Prompt)
	}
	if r.HeaderOpen != "\n— " {
		t.Errorf("header = %q, want the default kept for unset fields", r.HeaderOpen)
	}
}

func TestEmptySymbolRemovesDecoration(t *testing.T) {
	stripped := theme.Theme{Symbols: theme.Symbols{
		HeaderPrefix: strPtr(""),
		HeaderSuffix: strPtr(""),
	}}
	r, err := theme.Resolve(stripped, false)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if r.HeaderOpen != "" || r.HeaderClose != "" {
		t.Errorf("header = %q..%q, want both emptied", r.HeaderOpen, r.HeaderClose)
	}
}

func TestParse(t *testing.T) {
	bare := []byte("name: mine\npreset: sleek\nsymbols:\n  prompt: \"$ \"\n")
	got, err := theme.Parse(bare)
	if err != nil {
		t.Fatalf("Parse(bare) error: %v", err)
	}
	if got.Name != "mine" || got.Preset != "sleek" {
		t.Errorf("Parse(bare) = %+v, want name mine / preset sleek", got)
	}

	wrapped := []byte("theme:\n  name: inline\n  color: never\nsteps:\n  - type: input\n    key: a\n")
	got, err = theme.Parse(wrapped)
	if err != nil {
		t.Fatalf("Parse(wrapped) error: %v", err)
	}
	if got.Name != "inline" || got.Color != theme.ColorNever {
		t.Errorf("Parse(wrapped) = %+v, want the inline theme block", got)
	}
}

func TestValidate(t *testing.T) {
	bad := []theme.Theme{
		{Color: "sometimes"},
		{Preset: "nope"},
		{Styles: theme.Styles{Header: strPtr("chartreuse")}},
	}
	for _, tc := range bad {
		if err := tc.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", tc)
		}
	}
	if err := (&theme.Theme{Color: theme.ColorAuto, Preset: "minimal"}).Validate(); err != nil {
		t.Errorf("Validate(valid) error: %v", err)
	}
	var nilTheme *theme.Theme
	if err := nilTheme.Validate(); err != nil {
		t.Errorf("Validate(nil) error: %v", err)
	}
}

func TestMergeLastWins(t *testing.T) {
	base := theme.Theme{Name: "base", Symbols: theme.Symbols{Prompt: strPtr("> "), Indent: strPtr("  ")}}
	over := theme.Theme{Name: "over", Symbols: theme.Symbols{Prompt: strPtr("$ ")}}
	got := theme.Merge(base, over)
	if got.Name != "over" {
		t.Errorf("Name = %q, want the override", got.Name)
	}
	if *got.Symbols.Prompt != "$ " {
		t.Errorf("Prompt = %q, want the override", *got.Symbols.Prompt)
	}
	if got.Symbols.Indent == nil || *got.Symbols.Indent != "  " {
		t.Errorf("Indent = %v, want the base value kept", got.Symbols.Indent)
	}
}

func TestExpandFlattensPresetChain(t *testing.T) {
	// sleek builds on default: expanding it must carry the default's symbols.
	sleek, _ := theme.Preset("sleek")
	got, err := theme.Expand(sleek)
	if err != nil {
		t.Fatalf("Expand error: %v", err)
	}
	if got.Preset != "" {
		t.Errorf("Preset = %q, want it consumed by Expand", got.Preset)
	}
	if got.Symbols.Indent == nil || *got.Symbols.Indent != "  " {
		t.Errorf("Indent = %v, want the default inherited through the chain", got.Symbols.Indent)
	}
	if got.Symbols.Prompt == nil || *got.Symbols.Prompt != "❯ " {
		t.Errorf("Prompt = %v, want sleek's own value to win", got.Symbols.Prompt)
	}
}

func strPtr(s string) *string { return &s }
