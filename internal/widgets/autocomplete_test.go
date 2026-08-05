package widgets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func autoStep() spec.Step {
	return spec.Step{
		Type:        spec.TypeAutocomplete,
		Key:         "city",
		Title:       "Type a city",
		Placeholder: "e.g. New",
		Options:     []string{"New York", "New Delhi", "Los Angeles", "Chicago"},
	}
}

func TestAutocompleteSinglePrefixMatch(t *testing.T) {
	env, _ := menuTestEnv("chi\n")
	got, err := RunStep(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Chicago" {
		t.Fatalf("got %q, want %q", got, "Chicago")
	}
}

func TestAutocompleteMultipleMatchByNumber(t *testing.T) {
	env, _ := menuTestEnv("new\n2\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "New Delhi" {
		t.Fatalf("got %q, want %q", got, "New Delhi")
	}
}

func TestAutocompleteMultipleMatchEmptyFirst(t *testing.T) {
	env, _ := menuTestEnv("new\n\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "New York" {
		t.Fatalf("got %q, want %q", got, "New York")
	}
}

func TestAutocompleteMultipleMatchExactName(t *testing.T) {
	env, _ := menuTestEnv("new\nnew delhi\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "New Delhi" {
		t.Fatalf("got %q, want %q", got, "New Delhi")
	}
}

func TestAutocompleteNoMatchRetries(t *testing.T) {
	env, buf := menuTestEnv("zzz\nchi\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Chicago" {
		t.Fatalf("got %q, want %q", got, "Chicago")
	}
	if !strings.Contains(buf.String(), "No matches") {
		t.Fatalf("expected no-match message, got output %q", buf.String())
	}
}

func TestAutocompleteDefaultOnEmpty(t *testing.T) {
	step := autoStep()
	step.Default = "Chicago"
	env, _ := menuTestEnv("\n")
	got, err := autocompleteWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Chicago" {
		t.Fatalf("got %q, want %q", got, "Chicago")
	}
}

func TestAutocompleteInvalidSelectionRetries(t *testing.T) {
	env, buf := menuTestEnv("new\n9\n1\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "New York" {
		t.Fatalf("got %q, want %q", got, "New York")
	}
	if !strings.Contains(buf.String(), "Invalid choice") {
		t.Fatalf("expected invalid-choice message, got output %q", buf.String())
	}
}

func TestAutocompletePlaceholderShown(t *testing.T) {
	step := autoStep()
	step.Default = "Chicago"
	env, buf := menuTestEnv("\n")
	got, err := autocompleteWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Chicago" {
		t.Fatalf("got %q, want %q", got, "Chicago")
	}
	if !strings.Contains(buf.String(), "e.g. New") {
		t.Fatalf("expected placeholder hint, got output %q", buf.String())
	}
}

func TestAutocompleteEOF(t *testing.T) {
	env, _ := menuTestEnv("zzz\n")
	got, err := autocompleteWidget{}.Run(context.Background(), autoStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestAutocompleteEOFUsesDefault(t *testing.T) {
	step := autoStep()
	step.Default = "Chicago"
	env, _ := menuTestEnv("")
	got, err := autocompleteWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Chicago" {
		t.Fatalf("got %q, want %q", got, "Chicago")
	}
}

func TestAutocompleteContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := menuTestEnv("new\n")
	_, err := autocompleteWidget{}.Run(ctx, autoStep(), env)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
