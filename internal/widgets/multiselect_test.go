package widgets

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func multiStep() spec.Step {
	return spec.Step{
		Type:    spec.TypeMultiselect,
		Key:     "pick",
		Title:   "Pick some",
		Options: []string{"Apple", "Banana", "Cherry"},
	}
}

func TestMultiselectByNumbers(t *testing.T) {
	env, _ := menuTestEnv("1,3\n")
	got, err := RunStep(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectByNames(t *testing.T) {
	env, _ := menuTestEnv("cherry, APPLE\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectOrderAndDedup(t *testing.T) {
	env, _ := menuTestEnv("2,2,1\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Banana"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectAll(t *testing.T) {
	env, _ := menuTestEnv("all\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Banana", "Cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectNone(t *testing.T) {
	env, _ := menuTestEnv("none\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectEmptyNoDefault(t *testing.T) {
	env, _ := menuTestEnv("\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectDefaultString(t *testing.T) {
	step := multiStep()
	step.Default = "cherry, apple"
	env, _ := menuTestEnv("\n")
	got, err := multiselectWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectDefaultStringList(t *testing.T) {
	step := multiStep()
	step.Default = []string{"Banana", "Apple", "Banana"}
	env, _ := menuTestEnv("\n")
	got, err := multiselectWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Banana"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectDefaultAnyList(t *testing.T) {
	step := multiStep()
	step.Default = []any{"Cherry"}
	env, _ := menuTestEnv("\n")
	got, err := multiselectWidget{}.Run(context.Background(), step, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectInvalidTokenRetries(t *testing.T) {
	env, buf := menuTestEnv("1,bogus\n1,2\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Apple", "Banana"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !strings.Contains(buf.String(), "Invalid") {
		t.Fatalf("expected invalid-choice message, got output %q", buf.String())
	}
}

func TestMultiselectEOF(t *testing.T) {
	env, _ := menuTestEnv("1,bogus\n")
	got, err := multiselectWidget{}.Run(context.Background(), multiStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMultiselectContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := menuTestEnv("1\n")
	_, err := multiselectWidget{}.Run(ctx, multiStep(), env)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
