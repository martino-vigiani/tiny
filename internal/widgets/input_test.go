package widgets

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func newEnv(input string) (*Env, *bytes.Buffer) {
	var buf bytes.Buffer
	return &Env{
		In:      bufio.NewReader(strings.NewReader(input)),
		Out:     &buf,
		StdinFd: 0,
		IsTTY:   false,
	}, &buf
}

func TestInputHappyPath(t *testing.T) {
	env, _ := newEnv("hello world\n")
	got, err := inputWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeInput, Key: "name", Title: "Name:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("Run = %v, want %q", got, "hello world")
	}
}

func TestInputEmptyUsesDefault(t *testing.T) {
	env, _ := newEnv("\n")
	s := spec.Step{Type: spec.TypeInput, Key: "name", Title: "Name:", Default: "demo"}
	got, err := inputWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "demo" {
		t.Fatalf("Run = %v, want %q", got, "demo")
	}
}

func TestInputRequiredRetries(t *testing.T) {
	env, buf := newEnv("\nworld\n")
	s := spec.Step{Type: spec.TypeInput, Key: "name", Title: "Name:", Required: true}
	got, err := inputWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "world" {
		t.Fatalf("Run = %v, want %q", got, "world")
	}
	if !strings.Contains(buf.String(), "This field is required.") {
		t.Fatalf("output missing required message: %q", buf.String())
	}
}

func TestInputEmptyOptionalReturnsEmpty(t *testing.T) {
	env, _ := newEnv("\n")
	got, err := inputWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeInput, Key: "x", Title: "X:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "" {
		t.Fatalf("Run = %v, want empty string", got)
	}
}

func TestInputWhitespaceOnlyRequiredRetries(t *testing.T) {
	env, buf := newEnv("   \nworld\n")
	s := spec.Step{Type: spec.TypeInput, Key: "name", Title: "Name:", Required: true}
	got, err := inputWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "world" {
		t.Fatalf("Run = %v, want %q", got, "world")
	}
	if !strings.Contains(buf.String(), "This field is required.") {
		t.Fatalf("output missing required message: %q", buf.String())
	}
}

func TestInputShowsPlaceholderAndDefaultHints(t *testing.T) {
	env, buf := newEnv("value\n")
	s := spec.Step{Type: spec.TypeInput, Key: "k", Title: "K:", Placeholder: "name...", Default: "demo"}
	if _, err := (inputWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[name...]") {
		t.Errorf("output missing placeholder hint: %q", out)
	}
	if !strings.Contains(out, "[demo]") {
		t.Errorf("output missing default hint: %q", out)
	}
}

func TestInputContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := newEnv("x\n")
	_, err := inputWidget{}.Run(ctx, spec.Step{Type: spec.TypeInput, Key: "k", Title: "K:"}, env)
	if err != context.Canceled {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}
