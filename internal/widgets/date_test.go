package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestDateValid(t *testing.T) {
	env, _ := dataTestEnv("2024-01-15\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-01-15" {
		t.Errorf("got %v, want 2024-01-15", got)
	}
}

func TestDateInvalidThenRetry(t *testing.T) {
	env, buf := dataTestEnv("2024/01/15\n2024-01-15\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-01-15" {
		t.Errorf("got %v, want 2024-01-15", got)
	}
	if !strings.Contains(buf.String(), "Invalid date, expected YYYY-MM-DD.") {
		t.Errorf("expected invalid date message, got: %q", buf.String())
	}
}

func TestDateDatetime(t *testing.T) {
	env, buf := dataTestEnv("2024-01-15 12:30:45\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date", Format: "datetime"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-01-15 12:30:45" {
		t.Errorf("got %v, want 2024-01-15 12:30:45", got)
	}
	if !strings.Contains(buf.String(), "format: YYYY-MM-DD HH:MM:SS") {
		t.Errorf("expected datetime hint, got: %q", buf.String())
	}
}

func TestDateDatetimeRejectsDateOnly(t *testing.T) {
	env, _ := dataTestEnv("2024-01-15\n2024-01-15 12:30:45\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date", Format: "datetime"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-01-15 12:30:45" {
		t.Errorf("got %v, want 2024-01-15 12:30:45", got)
	}
}

func TestDateDefault(t *testing.T) {
	env, _ := dataTestEnv("\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date", Default: "2024-05-01"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-05-01" {
		t.Errorf("got %v, want 2024-05-01", got)
	}
}

func TestDateDefaultValidated(t *testing.T) {
	env, _ := dataTestEnv("\n2024-05-01\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date", Default: "not-a-date"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-05-01" {
		t.Errorf("got %v, want 2024-05-01", got)
	}
}

func TestDateRequiredEmptyRepeats(t *testing.T) {
	env, buf := dataTestEnv("\n2024-06-06\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date", Required: true}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2024-06-06" {
		t.Errorf("got %v, want 2024-06-06", got)
	}
	if !strings.Contains(buf.String(), "Invalid date, expected YYYY-MM-DD.") {
		t.Errorf("expected required retry message, got: %q", buf.String())
	}
}

func TestDateNotRequiredEmptyReturnsEmpty(t *testing.T) {
	env, _ := dataTestEnv("\n")
	got, err := dateWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeDate, Key: "d", Title: "Date"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %v, want empty string", got)
	}
}
