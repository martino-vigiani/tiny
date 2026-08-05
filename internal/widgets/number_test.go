package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestNumberHappyPath(t *testing.T) {
	env, _ := newEnv("5\n")
	got, err := numberWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 5.0 {
		t.Fatalf("Run = %v, want 5", got)
	}
}

func TestNumberOutOfRangeRetries(t *testing.T) {
	min, max := 1.0, 10.0
	env, buf := newEnv("0\n5\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Min: &min, Max: &max}
	got, err := numberWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 5.0 {
		t.Fatalf("Run = %v, want 5", got)
	}
	if !strings.Contains(buf.String(), "Number must be between") {
		t.Fatalf("output missing range message: %q", buf.String())
	}
}

func TestNumberAboveMaxRetries(t *testing.T) {
	min, max := 1.0, 10.0
	env, _ := newEnv("99\n10\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Min: &min, Max: &max}
	got, err := numberWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 10.0 {
		t.Fatalf("Run = %v, want 10", got)
	}
}

func TestNumberNonNumericRetries(t *testing.T) {
	env, buf := newEnv("abc\n7\n")
	got, err := numberWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 7.0 {
		t.Fatalf("Run = %v, want 7", got)
	}
	if !strings.Contains(buf.String(), "Please enter a number.") {
		t.Fatalf("output missing parse message: %q", buf.String())
	}
}

func TestNumberDefaultInt(t *testing.T) {
	env, _ := newEnv("\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Default: 2}
	got, err := numberWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 2.0 {
		t.Fatalf("Run = %v, want 2", got)
	}
}

func TestNumberDefaultString(t *testing.T) {
	env, _ := newEnv("\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Default: "3.5"}
	got, err := numberWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 3.5 {
		t.Fatalf("Run = %v, want 3.5", got)
	}
}

func TestNumberRejectsInf(t *testing.T) {
	env, buf := newEnv("Inf\n5\n")
	got, err := numberWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 5.0 {
		t.Fatalf("Run = %v, want 5", got)
	}
	if !strings.Contains(buf.String(), "Please enter a finite number.") {
		t.Fatalf("output missing finite-number message: %q", buf.String())
	}
}

func TestNumberRejectsNaN(t *testing.T) {
	env, buf := newEnv("NaN\n7\n")
	got, err := numberWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 7.0 {
		t.Fatalf("Run = %v, want 7", got)
	}
	if !strings.Contains(buf.String(), "Please enter a finite number.") {
		t.Fatalf("output missing finite-number message: %q", buf.String())
	}
}

func TestNumberRejectsNonNumeric(t *testing.T) {
	env, _ := newEnv("abc\n5\n")
	got, err := numberWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:"}, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != 5.0 {
		t.Fatalf("Run = %v, want 5", got)
	}
}

func TestNumberInvalidDefaultErrors(t *testing.T) {
	env, _ := newEnv("\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Default: "abc"}
	if _, err := (numberWidget{}).Run(context.Background(), s, env); err == nil {
		t.Fatal("expected error for non-numeric default")
	}
}

func TestNumberShowsHints(t *testing.T) {
	min, max := 1.0, 10.0
	env, buf := newEnv("2\n")
	s := spec.Step{Type: spec.TypeNumber, Key: "n", Title: "N:", Min: &min, Max: &max, Increment: 1}
	if _, err := (numberWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "(min: 1, max: 10)") {
		t.Errorf("output missing min/max hint: %q", out)
	}
	if !strings.Contains(out, "(step: 1)") {
		t.Errorf("output missing step hint: %q", out)
	}
}
