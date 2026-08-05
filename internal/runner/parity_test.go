package runner_test

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// parityWizard exercises all 12 step types with valid defaults.
func parityWizard() *spec.Wizard {
	return &spec.Wizard{Steps: []spec.Step{
		{Type: spec.TypeSelect, Key: "select", Options: []string{"a", "b"}, Default: "a"},
		{Type: spec.TypeSearch, Key: "search", Options: []string{"a", "b"}, Default: "b"},
		{Type: spec.TypeAutocomplete, Key: "autocomplete", Options: []string{"a", "b"}, Default: "a"},
		{Type: spec.TypeMultiselect, Key: "multiselect", Options: []string{"a", "b"}, Default: "a,b"},
		{Type: spec.TypeConfirm, Key: "confirm", Default: "yes"},
		{Type: spec.TypeInput, Key: "input", Default: "x"},
		{Type: spec.TypePassword, Key: "password", Default: "s"},
		{Type: spec.TypeNumber, Key: "number", Min: float64Ptr(1), Max: float64Ptr(10), Default: 3},
		{Type: spec.TypeDate, Key: "date", Default: "2024-01-01"},
		{Type: spec.TypeKeyValue, Key: "keyvalue", Default: map[string]any{"A": "1"}},
		{Type: spec.TypeEditor, Key: "editor", Default: "body"},
		{Type: spec.TypeTable, Key: "table", Selectable: true, Rows: [][]string{{"x"}, {"y"}}, Default: "x"},
	}}
}

// TestDefaultsInteractiveParity is the central product invariant: the headless
// RunDefaults path and the interactive Run path fed an empty stdin (EOF at the
// very first prompt) must produce the SAME Go types and SAME values.
func TestDefaultsInteractiveParity(t *testing.T) {
	widgets.RestoreTestRegistry()

	wiz := parityWizard()

	defaults, err := runner.New(strings.NewReader(""), io.Discard, 0, false).RunDefaults(wiz)
	if err != nil {
		t.Fatalf("RunDefaults error: %v", err)
	}
	interactive, err := runner.New(strings.NewReader(""), io.Discard, 0, false).Run(context.Background(), wiz)
	if err != nil {
		t.Fatalf("Run(empty stdin) error: %v", err)
	}

	if !reflect.DeepEqual(defaults, interactive) {
		t.Errorf("defaults and interactive diverged:\n defaults:    %#v\n interactive: %#v", defaults, interactive)
	}

	if got := defaults["table"]; got != int(0) {
		t.Errorf("table default = %#v (%T), want int(0)", got, got)
	}
	if got := interactive["table"]; got != int(0) {
		t.Errorf("interactive table = %#v (%T), want int(0)", got, got)
	}
}

// Conditional steps must not open a hole in the central invariant: a `when:`
// reads only earlier answers, so the headless and interactive paths have to
// skip exactly the same steps and return exactly the same map.
func TestConditionalParity(t *testing.T) {
	widgets.RestoreTestRegistry()

	wiz, err := spec.Parse([]byte(`
steps:
  - type: confirm
    key: use_db
    title: "Use a database?"
    default: "yes"
  - type: input
    key: db_name
    title: "Database name"
    default: "app"
    when: "use_db"
  - type: confirm
    key: cache
    title: "Enable cache?"
    default: "no"
  - type: number
    key: cache_size
    title: "Cache size"
    default: 64
    when: "cache"
  - type: input
    key: cache_dir
    title: "Cache directory"
    default: "/tmp"
    when: "cache_size"
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	defaults, err := runner.New(strings.NewReader(""), io.Discard, 0, false).RunDefaults(wiz)
	if err != nil {
		t.Fatalf("RunDefaults error: %v", err)
	}
	interactive, err := runner.New(strings.NewReader(""), io.Discard, 0, false).Run(context.Background(), wiz)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !reflect.DeepEqual(defaults, interactive) {
		t.Errorf("conditional wizard diverged:\n defaults:    %#v\n interactive: %#v", defaults, interactive)
	}

	// use_db is true, so db_name is asked; cache is false, so cache_size is
	// skipped — and cache_dir, which depends on the skipped step, is skipped
	// too rather than reappearing.
	if _, ok := defaults["db_name"]; !ok {
		t.Error("db_name is missing: its condition holds, so it should have been asked")
	}
	for _, skipped := range []string{"cache_size", "cache_dir"} {
		if _, ok := defaults[skipped]; ok {
			t.Errorf("%s is present: a skipped step must contribute no key", skipped)
		}
	}
}

// TestThemeParity is the invariant that makes theming safe to expose: a theme
// changes what the wizard looks like and nothing else. The same spec run under
// every preset must return byte-identical answers, so a tool can restyle a
// wizard without any risk of restyling its output.
func TestThemeParity(t *testing.T) {
	widgets.RestoreTestRegistry()

	wiz := parityWizard()
	var reference map[string]any
	for _, name := range theme.PresetNames() {
		preset, ok := theme.Preset(name)
		if !ok {
			t.Fatalf("Preset(%q) not found although listed", name)
		}
		resolved, err := theme.Resolve(preset, true)
		if err != nil {
			t.Fatalf("Resolve(%q) error: %v", name, err)
		}
		r := runner.New(strings.NewReader(""), io.Discard, 0, false)
		r.SetTheme(resolved)
		answers, err := r.Run(context.Background(), wiz)
		if err != nil {
			t.Fatalf("Run(theme %q) error: %v", name, err)
		}
		if reference == nil {
			reference = answers
			continue
		}
		if !reflect.DeepEqual(reference, answers) {
			t.Errorf("theme %q changed the answers:\n reference: %#v\n got:       %#v", name, reference, answers)
		}
	}
}
