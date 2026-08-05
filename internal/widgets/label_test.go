package widgets_test

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/keys"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// labelled builds a select whose second option is drawn under another name and
// carries an explanation.
func labelled() spec.Step {
	return spec.Step{
		Type:         spec.TypeSelect,
		Key:          "db",
		Title:        "Database",
		Options:      []string{"postgres", "cockroach"},
		Labels:       []string{"", "CockroachDB"},
		Descriptions: []string{"", "distributed, wire-compatible"},
	}
}

// The list shows the label and the description; the answer is still the value.
func TestLineListDrawsLabelsAndDescriptions(t *testing.T) {
	var out bytes.Buffer
	env := &widgets.Env{In: bufio.NewReader(strings.NewReader("2\n")), Out: &out}
	got, err := widgets.RunStep(context.Background(), labelled(), env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "cockroach" {
		t.Errorf("answer = %v, want the value", got)
	}
	text := out.String()
	if !strings.Contains(text, "CockroachDB") {
		t.Errorf("the label was not drawn:\n%s", text)
	}
	if !strings.Contains(text, "distributed, wire-compatible") {
		t.Errorf("the description was not drawn:\n%s", text)
	}
}

// Typing the label answers with the value behind it.
func TestLineAnswerByLabel(t *testing.T) {
	var out bytes.Buffer
	env := &widgets.Env{In: bufio.NewReader(strings.NewReader("CockroachDB\n")), Out: &out}
	got, err := widgets.RunStep(context.Background(), labelled(), env)
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	if got != "cockroach" {
		t.Errorf("answer = %v, want cockroach", got)
	}
}

// The live prompt draws the label too, and filtering finds an option under
// either name — the one on screen or the one the caller receives.
func TestRichPickerLabelsFilterUnderBothNames(t *testing.T) {
	for _, typed := range []string{"Cockroach", "cockroa"} {
		script := []keys.Key{}
		for _, r := range typed {
			script = append(script, runeKey(r))
		}
		script = append(script, key(keys.Enter))
		env, out := richEnv(t, script, 80, 24)
		got, err := widgets.RunStep(context.Background(), labelled(), env)
		if err != nil {
			t.Fatalf("RunStep error for %q: %v", typed, err)
		}
		if got != "cockroach" {
			t.Errorf("filtering by %q answered %v, want cockroach", typed, got)
		}
		if !strings.Contains(out.String(), "CockroachDB") {
			t.Errorf("the live prompt did not draw the label for %q:\n%s", typed, out.String())
		}
	}
}

// Rich and line must answer the same thing from the same keystrokes; a label
// that changed the answer would break the invariant the whole design rests on.
func TestLabelledAnswerIsTheSameInBothPaths(t *testing.T) {
	env, _ := richEnv(t, []keys.Key{key(keys.Down), key(keys.Enter)}, 80, 24)
	rich, err := widgets.RunStep(context.Background(), labelled(), env)
	if err != nil {
		t.Fatalf("rich RunStep error: %v", err)
	}
	var out bytes.Buffer
	line, err := widgets.RunStep(context.Background(),
		labelled(), &widgets.Env{In: bufio.NewReader(strings.NewReader("2\n")), Out: &out})
	if err != nil {
		t.Fatalf("line RunStep error: %v", err)
	}
	if rich != line {
		t.Errorf("rich answered %v, line answered %v", rich, line)
	}
}

// A multiselect shows labels as well, and answers with values.
func TestMultiselectLabels(t *testing.T) {
	s := labelled()
	s.Type = spec.TypeMultiselect
	var out bytes.Buffer
	got, err := widgets.RunStep(context.Background(),
		s, &widgets.Env{In: bufio.NewReader(strings.NewReader("CockroachDB\n")), Out: &out})
	if err != nil {
		t.Fatalf("RunStep error: %v", err)
	}
	values, ok := got.([]string)
	if !ok || len(values) != 1 || values[0] != "cockroach" {
		t.Errorf("answer = %v, want [cockroach]", got)
	}
	if !strings.Contains(out.String(), "CockroachDB") {
		t.Errorf("the label was not drawn:\n%s", out.String())
	}
}
