package widgets

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestKeyValueMultiLine(t *testing.T) {
	env, _ := dataTestEnv("FOO=bar\nBAZ=qux\n\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestKeyValueLineWithoutEqualsIgnored(t *testing.T) {
	env, buf := dataTestEnv("just text\nFOO=bar\n\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"FOO": "bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if !strings.Contains(buf.String(), `Expected key=value, got "just text".`) {
		t.Errorf("expected skip message, got: %q", buf.String())
	}
}

func TestKeyValueTrimsSpaces(t *testing.T) {
	env, _ := dataTestEnv("  FOO =  bar  \n\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"FOO": "bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestKeyValueSplitsOnFirstEquals(t *testing.T) {
	env, _ := dataTestEnv("URL=https://example.com/x=y\n\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"URL": "https://example.com/x=y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestKeyValueDuplicateLastWins(t *testing.T) {
	env, _ := dataTestEnv("A=1\nA=2\n\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"A": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestKeyValueDefaultMapWhenEmpty(t *testing.T) {
	for name, def := range map[string]any{
		"string map": map[string]string{"FOO": "bar"},
		"any map":    map[string]any{"FOO": "bar"},
	} {
		t.Run(name, func(t *testing.T) {
			env, _ := dataTestEnv("\n")
			got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars", Default: def}, env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := map[string]string{"FOO": "bar"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestKeyValueEmptyInputEmptyMap(t *testing.T) {
	env, _ := dataTestEnv("\n")
	got, err := keyvalueWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeKeyValue, Key: "kv", Title: "Env vars"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
