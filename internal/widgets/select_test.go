package widgets

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func menuTestEnv(input string) (*Env, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	env := &Env{
		In:      bufio.NewReader(strings.NewReader(input)),
		Out:     buf,
		IsTTY:   false,
		StdinFd: 0,
	}
	return env, buf
}

func selectStep() spec.Step {
	return spec.Step{
		Type:    spec.TypeSelect,
		Key:     "pick",
		Title:   "Choose one",
		Options: []string{"Apple", "Banana", "Cherry"},
	}
}

func TestSelectByNumber(t *testing.T) {
	env, _ := menuTestEnv("2\n")
	got, err := RunStep(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Banana" {
		t.Fatalf("got %q, want %q", got, "Banana")
	}
}

func TestSelectByNameCaseInsensitive(t *testing.T) {
	env, _ := menuTestEnv("banana\n")
	got, err := selectWidget{}.Run(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Banana" {
		t.Fatalf("got %q, want %q", got, "Banana")
	}
}

func TestSelectDefaultOnEmpty(t *testing.T) {
	step := selectStep()
	step.Default = "Cherry"
	env, _ := menuTestEnv("\n")
	got, err := selectWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Cherry" {
		t.Fatalf("got %q, want %q", got, "Cherry")
	}
}

func TestSelectEmptyWithoutDefaultRetries(t *testing.T) {
	env, _ := menuTestEnv("\n3\n")
	got, err := selectWidget{}.Run(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Cherry" {
		t.Fatalf("got %q, want %q", got, "Cherry")
	}
}

func TestSelectInvalidThenValid(t *testing.T) {
	env, buf := menuTestEnv("9\n1\n")
	got, err := selectWidget{}.Run(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Apple" {
		t.Fatalf("got %q, want %q", got, "Apple")
	}
	if !strings.Contains(buf.String(), "Invalid choice") {
		t.Fatalf("expected invalid-choice message, got output %q", buf.String())
	}
}

func TestSelectEOFAfterInvalidInput(t *testing.T) {
	env, _ := menuTestEnv("9\n")
	got, err := selectWidget{}.Run(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestSelectImmediateEOF(t *testing.T) {
	env, _ := menuTestEnv("")
	got, err := selectWidget{}.Run(context.Background(), selectStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestSelectEOFRequiredErrors(t *testing.T) {
	step := selectStep()
	step.Required = true
	env, _ := menuTestEnv("")
	_, err := selectWidget{}.Run(context.Background(), step, env)
	if err == nil || !strings.Contains(err.Error(), "unexpected end of input") {
		t.Fatalf("expected unexpected-end-of-input error, got %v", err)
	}
}

func TestSelectContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := menuTestEnv("1\n")
	_, err := selectWidget{}.Run(ctx, selectStep(), env)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
