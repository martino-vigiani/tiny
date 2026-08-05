package widgets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func searchStep() spec.Step {
	return spec.Step{
		Type:    spec.TypeSearch,
		Key:     "find",
		Title:   "Search a fruit",
		Options: []string{"Apple", "Apricot", "Banana", "Cherry"},
	}
}

func TestSearchHappyPath(t *testing.T) {
	env, _ := menuTestEnv("ap\n2\n")
	got, err := RunStep(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Apricot" {
		t.Fatalf("got %q, want %q", got, "Apricot")
	}
}

func TestSearchEmptySelectsFirst(t *testing.T) {
	env, _ := menuTestEnv("ap\n\n")
	got, err := searchWidget{}.Run(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Apple" {
		t.Fatalf("got %q, want %q", got, "Apple")
	}
}

func TestSearchDefaultAmongMatches(t *testing.T) {
	step := searchStep()
	step.Default = "Apricot"
	env, _ := menuTestEnv("ap\n\n")
	got, err := searchWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Apricot" {
		t.Fatalf("got %q, want %q", got, "Apricot")
	}
}

func TestSearchNoMatchRetries(t *testing.T) {
	env, buf := menuTestEnv("zzz\nap\n1\n")
	got, err := searchWidget{}.Run(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Apple" {
		t.Fatalf("got %q, want %q", got, "Apple")
	}
	if !strings.Contains(buf.String(), "No matches") {
		t.Fatalf("expected no-match message, got output %q", buf.String())
	}
}

func TestSearchByNumber(t *testing.T) {
	env, _ := menuTestEnv("a\n3\n")
	got, err := searchWidget{}.Run(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Banana" {
		t.Fatalf("got %q, want %q", got, "Banana")
	}
}

func TestSearchInvalidSelectionRetries(t *testing.T) {
	env, buf := menuTestEnv("a\n9\n3\n")
	got, err := searchWidget{}.Run(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Banana" {
		t.Fatalf("got %q, want %q", got, "Banana")
	}
	if !strings.Contains(buf.String(), "Invalid choice") {
		t.Fatalf("expected invalid-choice message, got output %q", buf.String())
	}
}

func TestSearchEOF(t *testing.T) {
	env, _ := menuTestEnv("zzz\n")
	got, err := searchWidget{}.Run(context.Background(), searchStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestSearchEOFUsesDefault(t *testing.T) {
	step := searchStep()
	step.Default = "Banana"
	env, _ := menuTestEnv("")
	got, err := searchWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Banana" {
		t.Fatalf("got %q, want %q", got, "Banana")
	}
}

func TestSearchContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := menuTestEnv("a\n")
	_, err := searchWidget{}.Run(ctx, searchStep(), env)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
