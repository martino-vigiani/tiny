package runner_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// fakeSelect / fakeInput / fakeConfirm mimic the behaviour the real widgets
// will have, so the runner wiring (dispatch + error wrapping + answer map) is
// testable while the production widgets are still stubs.
type fakeSelect struct{}

func (fakeSelect) Run(ctx context.Context, s spec.Step, env *widgets.Env) (any, error) {
	line, err := widgets.ReadLine(env)
	if err != nil {
		return nil, err
	}
	if line == "" {
		return s.Default, nil
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(s.Options) {
		return s.Options[n-1], nil
	}
	return line, nil
}

type fakeInput struct{}

func (fakeInput) Run(ctx context.Context, s spec.Step, env *widgets.Env) (any, error) {
	line, err := widgets.ReadLine(env)
	if err != nil {
		return nil, err
	}
	if line == "" {
		return s.Default, nil
	}
	return line, nil
}

type fakeConfirm struct{}

func (fakeConfirm) Run(ctx context.Context, s spec.Step, env *widgets.Env) (any, error) {
	line, err := widgets.ReadLine(env)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return false, nil
}

func TestRunInteractive(t *testing.T) {
	t.Cleanup(widgets.RestoreTestRegistry)
	widgets.RegisterTest(spec.TypeSelect, fakeSelect{})
	widgets.RegisterTest(spec.TypeInput, fakeInput{})
	widgets.RegisterTest(spec.TypeConfirm, fakeConfirm{})

	input := strings.NewReader("1\nMiaApp\ny\n")
	r := runner.New(input, io.Discard, 0, false)
	wiz := &spec.Wizard{Steps: []spec.Step{
		{Type: spec.TypeSelect, Key: "framework", Options: []string{"SwiftUI", "UIKit"}, Default: "SwiftUI"},
		{Type: spec.TypeInput, Key: "project_name", Default: "MiaApp"},
		{Type: spec.TypeConfirm, Key: "init_git"},
	}}
	got, err := r.Run(context.Background(), wiz)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := map[string]any{
		"framework":    "SwiftUI",
		"project_name": "MiaApp",
		"init_git":     true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Run() = %#v, want %#v", got, want)
	}
}

type fakeFailing struct{}

func (fakeFailing) Run(ctx context.Context, s spec.Step, env *widgets.Env) (any, error) {
	return nil, fmt.Errorf("widget boom")
}

func TestRunPropagatesWidgetError(t *testing.T) {
	t.Cleanup(widgets.RestoreTestRegistry)
	widgets.RegisterTest(spec.TypeEditor, fakeFailing{})

	r := runner.New(strings.NewReader(""), io.Discard, 0, false)
	wiz := &spec.Wizard{Steps: []spec.Step{
		{Type: spec.TypeEditor, Key: "ed"},
	}}
	_, err := r.Run(context.Background(), wiz)
	if err == nil {
		t.Fatal("expected error from widget")
	}
	if !strings.Contains(err.Error(), `step "ed"`) {
		t.Errorf("error %q is not wrapped with the step key", err)
	}
	if !strings.Contains(err.Error(), "widget boom") {
		t.Errorf("error %q does not surface the widget error", err)
	}
}

func TestRunDefaults(t *testing.T) {
	wiz := &spec.Wizard{Steps: []spec.Step{
		{Type: spec.TypeSelect, Key: "sel", Options: []string{"a", "b"}, Default: "a"},
		{Type: spec.TypeMultiselect, Key: "multi", Options: []string{"a", "b"}, Default: "a,b"},
		{Type: spec.TypeConfirm, Key: "conf", Default: "yes"},
		{Type: spec.TypeInput, Key: "inp", Default: "x"},
		{Type: spec.TypePassword, Key: "pwd", Default: "s3cret"},
		{Type: spec.TypeSearch, Key: "search", Options: []string{"a", "b"}, Default: "b"},
		{Type: spec.TypeNumber, Key: "num", Min: float64Ptr(1), Max: float64Ptr(10), Default: 3},
		{Type: spec.TypeDate, Key: "date", Format: "date", Default: "2024-01-01"},
		{Type: spec.TypeKeyValue, Key: "kv", Default: map[string]any{"FOO": "bar"}},
		{Type: spec.TypeEditor, Key: "ed", Default: "body"},
		{Type: spec.TypeAutocomplete, Key: "ac", Options: []string{"a", "b"}, Default: "a"},
		{Type: spec.TypeTable, Key: "tbl", Selectable: true, Rows: [][]string{{"x"}, {"y"}}, Default: "x"},
	}}
	r := runner.New(strings.NewReader(""), io.Discard, 0, false)
	got, err := r.RunDefaults(wiz)
	if err != nil {
		t.Fatalf("RunDefaults error: %v", err)
	}
	want := map[string]any{
		"sel":    "a",
		"multi":  []string{"a", "b"},
		"conf":   true,
		"inp":    "x",
		"pwd":    "s3cret",
		"search": "b",
		"num":    float64(3),
		"date":   "2024-01-01",
		"kv":     map[string]string{"FOO": "bar"},
		"ed":     "body",
		"ac":     "a",
		"tbl":    int(0),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunDefaults() = %#v, want %#v", got, want)
	}
}

func TestRunDefaultsMissingRequired(t *testing.T) {
	wiz := &spec.Wizard{Steps: []spec.Step{
		{Type: spec.TypeInput, Key: "name", Required: true},
	}}
	r := runner.New(strings.NewReader(""), io.Discard, 0, false)
	if _, err := r.RunDefaults(wiz); err == nil {
		t.Fatal("expected error for required step without default")
	}
}

func TestRunDefaultsConfirmVariants(t *testing.T) {
	for _, tc := range []struct {
		def  any
		want bool
	}{
		{"yes", true},
		{"y", true},
		{"true", true},
		{true, true},
		{"no", false},
		{"n", false},
		{"false", false},
		{false, false},
		{nil, false},
	} {
		wiz := &spec.Wizard{Steps: []spec.Step{
			{Type: spec.TypeConfirm, Key: "c", Default: tc.def},
		}}
		r := runner.New(strings.NewReader(""), io.Discard, 0, false)
		got, err := r.RunDefaults(wiz)
		if err != nil {
			t.Fatalf("default %#v: unexpected error: %v", tc.def, err)
		}
		if got["c"] != tc.want {
			t.Errorf("default %#v: got %v, want %v", tc.def, got["c"], tc.want)
		}
	}
}

func TestPrintEnv(t *testing.T) {
	var buf bytes.Buffer
	answers := map[string]any{
		"init_git": true,
		"langs":    []string{"go", "rust"},
		"name":     "MiaApp",
	}
	if err := runner.PrintEnv(&buf, answers); err != nil {
		t.Fatalf("PrintEnv error: %v", err)
	}
	want := "init_git=true\nlangs='go, rust'\nname=MiaApp\n"
	if buf.String() != want {
		t.Errorf("PrintEnv() = %q, want %q", buf.String(), want)
	}
}

func TestPrintEnvMapAndNil(t *testing.T) {
	var buf bytes.Buffer
	answers := map[string]any{
		"env":    map[string]string{"B": "2", "A": "1"},
		"nilval": nil,
		"count":  float64(3),
	}
	if err := runner.PrintEnv(&buf, answers); err != nil {
		t.Fatalf("PrintEnv error: %v", err)
	}
	want := "count=3\nenv_A=1\nenv_B=2\n"
	if buf.String() != want {
		t.Errorf("PrintEnv() = %q, want %q", buf.String(), want)
	}
	if strings.Contains(buf.String(), "nilval") || strings.Contains(buf.String(), "map[") {
		t.Errorf("nil/map leaked into env output: %q", buf.String())
	}
}

func TestPrintJSONAndYAML(t *testing.T) {
	answers := map[string]any{"name": "MiaApp", "init_git": true}

	var jb bytes.Buffer
	if err := runner.PrintJSON(&jb, answers); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}
	if !strings.Contains(jb.String(), `"name": "MiaApp"`) || !strings.Contains(jb.String(), `"init_git": true`) {
		t.Errorf("PrintJSON output missing keys: %s", jb.String())
	}

	var yb bytes.Buffer
	if err := runner.PrintYAML(&yb, answers); err != nil {
		t.Fatalf("PrintYAML error: %v", err)
	}
	if !strings.Contains(yb.String(), "name: MiaApp") || !strings.Contains(yb.String(), "init_git: true") {
		t.Errorf("PrintYAML output missing keys: %s", yb.String())
	}
}

// An answer is arbitrary user text: the env format must survive spaces,
// quotes and shell metacharacters without the consumer eval'ing them.
func TestPrintEnvQuotesHostileValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"plain", "MyApp", "k=MyApp\n"},
		{"empty", "", "k=\n"},
		{"safe punctuation", "v1.2-beta/x@y:z%,+=", "k=v1.2-beta/x@y:z%,+=\n"},
		{"spaces", "My App", "k='My App'\n"},
		{"command substitution", "$(rm -rf ~)", "k='$(rm -rf ~)'\n"},
		{"semicolon", "x; echo pwned", "k='x; echo pwned'\n"},
		{"single quote", "it's", `k='it'\''s'` + "\n"},
		{"newline", "a\nb", "k='a\nb'\n"},
		{"backtick", "`id`", "k='`id`'\n"},
		{"list", []string{"iOS", "macOS Tahoe"}, "k='iOS, macOS Tahoe'\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := runner.PrintEnv(&buf, map[string]any{"k": tc.value}); err != nil {
				t.Fatalf("PrintEnv error: %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("PrintEnv(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// Wizard keys allow "." and "-", which no shell accepts in a variable name.
func TestPrintEnvNormalisesKeys(t *testing.T) {
	var buf bytes.Buffer
	answers := map[string]any{"project.name": "MyApp", "min-deploy": float64(18)}
	if err := runner.PrintEnv(&buf, answers); err != nil {
		t.Fatalf("PrintEnv error: %v", err)
	}
	want := "min_deploy=18\nproject_name=MyApp\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintEnv() = %q, want %q", got, want)
	}
}

// Two keys normalising to the same identifier would silently drop an answer,
// so the env format refuses them instead.
func TestPrintEnvRejectsCollidingKeys(t *testing.T) {
	var buf bytes.Buffer
	answers := map[string]any{"a.b": "1", "a-b": "2"}
	err := runner.PrintEnv(&buf, answers)
	if err == nil {
		t.Fatal("PrintEnv() = nil error, want a collision failure")
	}
	if !strings.Contains(err.Error(), "a_b") {
		t.Errorf("error = %v, want it to name the colliding env variable", err)
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
