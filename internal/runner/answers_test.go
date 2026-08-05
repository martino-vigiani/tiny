package runner_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// presetWizard exercises every step type with a valid default, so a test can
// preset one step and watch the rest fall back.
const presetWizardYAML = `
steps:
  - type: select
    key: sel
    title: Select
    options: [alpha, beta]
    default: alpha
  - type: search
    key: srch
    title: Search
    options: [alpha, beta]
    default: alpha
  - type: autocomplete
    key: ac
    title: Autocomplete
    options: [alpha, beta]
    default: alpha
  - type: multiselect
    key: multi
    title: Multiselect
    options: [a, b, c]
    default: a
  - type: confirm
    key: conf
    title: Confirm
    default: "no"
  - type: input
    key: name
    title: Name
    default: app
    pattern: "^[a-z0-9]+$"
  - type: password
    key: pwd
    title: Password
    default: s3cret
  - type: number
    key: num
    title: Number
    min: 1
    max: 10
    default: 3
  - type: date
    key: day
    title: Date
    default: "2024-01-01"
  - type: keyvalue
    key: kv
    title: KeyValue
    default:
      A: "1"
  - type: editor
    key: notes
    title: Notes
    default: body
  - type: table
    key: tbl
    title: Table
    selectable: true
    rows: [[x], [y]]
    default: x
`

func mustParse(t *testing.T, y string) *spec.Wizard {
	t.Helper()
	wiz, err := spec.Parse([]byte(y))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return wiz
}

func TestParsePresets(t *testing.T) {
	got, err := runner.ParsePresets([]string{"env=production", "url=https://x/y?a=b", "empty="})
	if err != nil {
		t.Fatalf("ParsePresets error: %v", err)
	}
	want := runner.Preset{"env": "production", "url": "https://x/y?a=b", "empty": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParsePresets() = %#v, want %#v", got, want)
	}
}

func TestParsePresetsRejectsPairWithoutEquals(t *testing.T) {
	_, err := runner.ParsePresets([]string{"env=production", "justakey"})
	if err == nil {
		t.Fatal("expected an error for an argument with no =")
	}
	if !strings.Contains(err.Error(), "justakey") {
		t.Errorf("error %q does not name the offending argument", err)
	}
	if !strings.Contains(err.Error(), "key=value") {
		t.Errorf("error %q does not show the correct form", err)
	}
	if _, err := runner.ParsePresets([]string{"=value"}); err == nil {
		t.Error("expected an error for an empty key")
	}
}

// A preset file is YAML, and every JSON document is YAML: the same file has to
// load either way, and both have to reach the same text form as -set.
func TestLoadPresets(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "answers.json")
	if err := os.WriteFile(jsonPath, []byte(`{
	  "name": "myapp",
	  "conf": true,
	  "num": 7,
	  "multi": ["a", "c"],
	  "kv": {"B": "2", "A": "1"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, "answers.yaml")
	if err := os.WriteFile(yamlPath, []byte(
		"name: myapp\nconf: true\nnum: 7\nmulti: [a, c]\nkv:\n  B: \"2\"\n  A: \"1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := runner.Preset{
		"name":  "myapp",
		"conf":  "true",
		"num":   "7",
		"multi": "a,c",
		"kv":    "A=1,B=2",
	}
	for _, path := range []string{jsonPath, yamlPath} {
		got, err := runner.LoadPresets(path)
		if err != nil {
			t.Fatalf("LoadPresets(%s) error: %v", filepath.Base(path), err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("LoadPresets(%s) = %#v, want %#v", filepath.Base(path), got, want)
		}
	}
}

func TestLoadPresetsRejectsNesting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.yaml")
	if err := os.WriteFile(path, []byte("multi:\n  - [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runner.LoadPresets(path)
	if err == nil {
		t.Fatal("expected an error for a nested value")
	}
	if !strings.Contains(err.Error(), "multi") {
		t.Errorf("error %q does not name the key", err)
	}
	if _, err := runner.LoadPresets(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

// Every step type is presettable, and a preset lands in the answers as the
// canonical Go type for that type — the JSON encoder and the parity test both
// depend on it.
func TestRunPresetsEveryType(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)

	p := runner.Preset{
		"sel":   "beta",
		"srch":  "beta",
		"ac":    "beta",
		"multi": "a,c",
		"conf":  "yes",
		"name":  "myapp",
		"pwd":   "hunter2",
		"num":   "7",
		"day":   "2024-06-01",
		"kv":    "A=1,B=2",
		"notes": "hello",
		"tbl":   "2",
	}
	got, err := runner.New(strings.NewReader(""), io.Discard, 0, false).RunPresets(wiz, p)
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	want := map[string]any{
		"sel":   "beta",
		"srch":  "beta",
		"ac":    "beta",
		"multi": []string{"a", "c"},
		"conf":  true,
		"name":  "myapp",
		"pwd":   "hunter2",
		"num":   float64(7),
		"day":   "2024-06-01",
		"kv":    map[string]string{"A": "1", "B": "2"},
		"notes": "hello",
		"tbl":   int(1),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunPresets() = %#v\nwant %#v", got, want)
	}
}

// A preset matches an option case-insensitively and lands in the answers with
// the option's own casing, exactly as a typed answer would.
func TestRunPresetsCanonicalisesOptions(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)

	got, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"sel": "BETA", "multi": "C,A"})
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	if got["sel"] != "beta" {
		t.Errorf("sel = %#v, want %q", got["sel"], "beta")
	}
	// Ordered by the options, not by the order they were written.
	if !reflect.DeepEqual(got["multi"], []string{"a", "c"}) {
		t.Errorf("multi = %#v, want [a c]", got["multi"])
	}
}

// Steps with no preset keep taking their defaults: that is the whole point —
// "the defaults, but with this one answer changed".
func TestRunPresetsLeavesTheRestOnDefaults(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)
	r := runner.New(strings.NewReader(""), io.Discard, 0, false)

	defaults, err := r.RunDefaults(wiz)
	if err != nil {
		t.Fatalf("RunDefaults error: %v", err)
	}
	got, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"name": "other"})
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	if got["name"] != "other" {
		t.Errorf("name = %#v, want %q", got["name"], "other")
	}
	delete(got, "name")
	delete(defaults, "name")
	if !reflect.DeepEqual(got, defaults) {
		t.Errorf("presetting one step changed the others:\n got:      %#v\n defaults: %#v", got, defaults)
	}
}

// A preset that the step could never have accepted is an error naming the key
// and saying why — never a value silently coerced into the payload.
func TestRunPresetsRejectsInvalidValues(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)

	for _, tc := range []struct {
		name   string
		key    string
		value  string
		reason string
	}{
		{"select not an option", "sel", "gamma", "not one of the options"},
		{"search not an option", "srch", "gamma", "not one of the options"},
		{"autocomplete not an option", "ac", "gamma", "not one of the options"},
		{"multiselect item not an option", "multi", "a,zzz", "not one of the options"},
		{"confirm not a yes/no", "conf", "maybe", "confirm"},
		{"input fails its pattern", "name", "My App", "validation"},
		{"number not a number", "num", "soon", "number"},
		{"number below min", "num", "0", "below min"},
		{"number above max", "num", "99", "above max"},
		{"number not finite", "num", "Inf", "finite"},
		{"date wrong format", "day", "01/02/2024", "format"},
		{"keyvalue without =", "kv", "novalue", "key=value"},
		{"table row out of range", "tbl", "9", "out of range"},
		{"table row not found", "tbl", "zzz", "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
				RunPresets(wiz, runner.Preset{tc.key: tc.value})
			if err == nil {
				t.Fatalf("preset %s=%q was accepted", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), `"`+tc.key+`"`) {
				t.Errorf("error %q does not name the step key", err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("error %q does not say why (want %q)", err, tc.reason)
			}
		})
	}
}

// An empty preset for a required step is an answer nobody could have given at
// the prompt either.
func TestRunPresetsRejectsEmptyRequired(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, `
steps:
  - type: input
    key: name
    title: Name
    required: true
    default: app
`)
	_, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"name": ""})
	if err == nil {
		t.Fatal("an empty preset for a required step was accepted")
	}
	if !strings.Contains(err.Error(), `"name"`) {
		t.Errorf("error %q does not name the step key", err)
	}
}

// A table that is not selectable has no answer, so presetting it is a mistake
// rather than a no-op.
func TestRunPresetsRejectsNonSelectableTable(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, `
steps:
  - type: table
    key: tbl
    title: Table
    headers: [name]
    rows: [[x], [y]]
`)
	if _, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"tbl": "1"}); err == nil {
		t.Fatal("a preset for a non-selectable table was accepted")
	}
}

// A key no step in the wizard has is a typo: no run could ever consume it, so
// it fails before a single step is resolved, and the message lists what the
// wizard does ask.
func TestRunPresetsRejectsUnknownKey(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)

	_, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"nmae": "myapp"})
	if err == nil {
		t.Fatal("an unknown preset key was accepted")
	}
	if !strings.Contains(err.Error(), "nmae") {
		t.Errorf("error %q does not name the unknown key", err)
	}
	for _, key := range []string{"sel", "name", "tbl"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not list the valid key %q", err, key)
		}
	}
}

const branchWizardYAML = `
steps:
  - type: confirm
    key: use_db
    title: "Use a database?"
    default: "no"
  - type: input
    key: db_name
    title: "Database name"
    default: app
    when: use_db
`

// A preset for a step whose `when:` does not hold is skipped like any other
// step: it contributes no key, and the caller is told the preset went unused.
func TestRunPresetsSkipsStepWhoseConditionIsFalse(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, branchWizardYAML)

	var ui bytes.Buffer
	got, err := runner.New(strings.NewReader(""), &ui, 0, false).
		RunPresets(wiz, runner.Preset{"db_name": "orders"})
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	if _, ok := got["db_name"]; ok {
		t.Errorf("db_name is present although its condition is false: %#v", got)
	}
	if !strings.Contains(ui.String(), "db_name") {
		t.Errorf("the unused preset was not reported on the UI stream: %q", ui.String())
	}
}

// The capability the feature exists for: a preset can open a branch that
// -defaults alone never reaches, and the steps inside it then take their own
// defaults.
func TestRunPresetsUnlocksAConditionalBranch(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, branchWizardYAML)

	defaults, err := runner.New(strings.NewReader(""), io.Discard, 0, false).RunDefaults(wiz)
	if err != nil {
		t.Fatalf("RunDefaults error: %v", err)
	}
	if _, ok := defaults["db_name"]; ok {
		t.Fatalf("the branch is already reachable with -defaults: %#v", defaults)
	}

	var ui bytes.Buffer
	got, err := runner.New(strings.NewReader(""), &ui, 0, false).
		RunPresets(wiz, runner.Preset{"use_db": "yes"})
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	if got["use_db"] != true || got["db_name"] != "app" {
		t.Errorf("RunPresets() = %#v, want use_db true and db_name %q", got, "app")
	}
	if strings.Contains(ui.String(), "ignored") {
		t.Errorf("a preset that was used was reported as ignored: %q", ui.String())
	}
}

// Interpolation is applied to the steps a preset unlocks, exactly as in the
// other two paths: a preset changes the answers, never how a step reads them.
func TestRunPresetsInterpolates(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, `
steps:
  - type: input
    key: project
    title: Project
    default: app
  - type: input
    key: module
    title: "Module for ${project}"
    default: "github.com/me/${project}"
`)
	got, err := runner.New(strings.NewReader(""), io.Discard, 0, false).
		RunPresets(wiz, runner.Preset{"project": "orders"})
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	if got["module"] != "github.com/me/orders" {
		t.Errorf("module = %#v, want %q", got["module"], "github.com/me/orders")
	}
}

// The key invariant, and the reason a preset is raw text: a wizard answered
// entirely from a preset must produce exactly the map the same wizard produces
// when a person types those answers at the prompts.
func TestPresetInteractiveParity(t *testing.T) {
	widgets.RestoreTestRegistry()
	wiz := mustParse(t, presetWizardYAML)

	p := runner.Preset{
		"sel":   "beta",
		"srch":  "beta",
		"ac":    "beta",
		"multi": "a,c",
		"conf":  "yes",
		"name":  "myapp",
		"pwd":   "hunter2",
		"num":   "7",
		"day":   "2024-06-01",
		"kv":    "A=1,B=2",
		"notes": "hello",
		"tbl":   "2",
	}
	// The same answers, in the order the prompts ask for them: the search step
	// filters then picks, keyvalue ends on a blank line, the editor on a dot.
	typed := strings.Join([]string{
		"beta",      // sel
		"beta", "1", // srch: filter, then pick the only match
		"beta",           // ac
		"a,c",            // multi
		"yes",            // conf
		"myapp",          // name
		"hunter2",        // pwd
		"7",              // num
		"2024-06-01",     // day
		"A=1", "B=2", "", // kv
		"hello", ".", // notes
		"2", // tbl
	}, "\n") + "\n"

	preset, err := runner.New(strings.NewReader(""), io.Discard, 0, false).RunPresets(wiz, p)
	if err != nil {
		t.Fatalf("RunPresets error: %v", err)
	}
	interactive, err := runner.New(strings.NewReader(typed), io.Discard, 0, false).
		Run(context.Background(), wiz)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !reflect.DeepEqual(preset, interactive) {
		t.Errorf("preset and interactive diverged:\n preset:      %#v\n interactive: %#v", preset, interactive)
	}
}
