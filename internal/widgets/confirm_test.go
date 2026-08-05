package widgets

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// dataTestEnv builds a non-TTY Env feeding input into In and capturing output.
// It is shared by the data-family widget tests in this package.
func dataTestEnv(input string) (*Env, *bytes.Buffer) {
	var buf bytes.Buffer
	return &Env{
		In:      bufio.NewReader(strings.NewReader(input)),
		Out:     &buf,
		StdinFd: 0,
		IsTTY:   false,
	}, &buf
}

func TestConfirmYes(t *testing.T) {
	env, _ := dataTestEnv("y\n")
	got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("got %v, want true", got)
	}
}

func TestConfirmNo(t *testing.T) {
	env, _ := dataTestEnv("n\n")
	got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Errorf("got %v, want false", got)
	}
}

func TestConfirmDefaultTrue(t *testing.T) {
	env, buf := dataTestEnv("\n")
	got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?", Default: "yes"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("got %v, want true", got)
	}
	if !strings.Contains(buf.String(), "[y]") {
		t.Errorf("prompt does not show [y] default: %q", buf.String())
	}
}

func TestConfirmDefaultFalseAndAbsent(t *testing.T) {
	for name, tc := range map[string]struct {
		def  any
		want bool
		mark string
	}{
		"default false":  {def: false, want: false, mark: "[n]"},
		"default no":     {def: "no", want: false, mark: "[n]"},
		"default absent": {def: nil, want: false, mark: "[n]"},
	} {
		t.Run(name, func(t *testing.T) {
			env, buf := dataTestEnv("\n")
			got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?", Default: tc.def}, env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			if !strings.Contains(buf.String(), tc.mark) {
				t.Errorf("prompt does not show %s default: %q", tc.mark, buf.String())
			}
		})
	}
}

func TestConfirmUnparsableDefaultTreatedAsAbsent(t *testing.T) {
	env, _ := dataTestEnv("\n")
	got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?", Default: "gibberish"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Errorf("got %v, want false", got)
	}
}

func TestConfirmInvalidThenValid(t *testing.T) {
	env, buf := dataTestEnv("maybe\nyes\n")
	got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("got %v, want true", got)
	}
	if !strings.Contains(buf.String(), "Answer with y or n.") {
		t.Errorf("expected retry message, got: %q", buf.String())
	}
}

func TestConfirmAcceptsTrueVariants(t *testing.T) {
	for _, in := range []string{"y", "yes", "true"} {
		env, _ := dataTestEnv(in + "\n")
		got, err := confirmWidget{}.Run(context.Background(), spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?"}, env)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", in, err)
		}
		if got != true {
			t.Errorf("input %q: got %v, want true", in, got)
		}
	}
}

func TestConfirmCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env, _ := dataTestEnv("y\n")
	_, err := confirmWidget{}.Run(ctx, spec.Step{Type: spec.TypeConfirm, Key: "c", Title: "Git?"}, env)
	if err == nil {
		t.Fatal("expected ctx error")
	}
}
