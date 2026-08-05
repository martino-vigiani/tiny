package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestPasswordNotEchoed(t *testing.T) {
	env, buf := newEnv("s3cr3t\n")
	got, err := passwordWidget{}.Run(context.Background(), spec.Step{Type: spec.TypePassword, Key: "pass", Title: "Password:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("Run = %v, want %q", got, "s3cr3t")
	}
	if strings.Contains(buf.String(), "s3cr3t") {
		t.Fatalf("password leaked in output: %q", buf.String())
	}
}

func TestPasswordEmptyUsesDefault(t *testing.T) {
	env, _ := newEnv("\n")
	s := spec.Step{Type: spec.TypePassword, Key: "pass", Title: "Password:", Default: "dflt"}
	got, err := passwordWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "dflt" {
		t.Fatalf("Run = %v, want %q", got, "dflt")
	}
}

func TestPasswordRequiredRetriesWithoutEcho(t *testing.T) {
	env, buf := newEnv("\nsek\n")
	s := spec.Step{Type: spec.TypePassword, Key: "pass", Title: "Password:", Required: true}
	got, err := passwordWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "sek" {
		t.Fatalf("Run = %v, want %q", got, "sek")
	}
	out := buf.String()
	if !strings.Contains(out, "This field is required.") {
		t.Errorf("output missing required message: %q", out)
	}
	if strings.Contains(out, "sek") {
		t.Errorf("password leaked in output: %q", out)
	}
}

func TestPasswordUsesPromptText(t *testing.T) {
	env, buf := newEnv("x\n")
	s := spec.Step{Type: spec.TypePassword, Key: "pass", PromptText: "API token:", Title: "Token:"}
	if _, err := (passwordWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(buf.String(), "API token:") {
		t.Fatalf("output missing prompt text: %q", buf.String())
	}
}
