package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func tableStep() spec.Step {
	return spec.Step{
		Type:    spec.TypeTable,
		Key:     "tbl",
		Title:   "Releases",
		Headers: []string{"Name", "Status"},
		Rows: [][]string{
			{"v1.0", "shipped"},
			{"v1.1", "beta"},
			{"v2.0", "planned"},
		},
		Selectable: true,
	}
}

func nonSelectableStep() spec.Step {
	s := tableStep()
	s.Selectable = false
	return s
}

func stepWithDefault(def any) spec.Step {
	s := tableStep()
	s.Default = def
	return s
}

func TestTableNotSelectableReturnsNil(t *testing.T) {
	env, buf := dataTestEnv("\n")
	got, err := tableWidget{}.Run(context.Background(), nonSelectableStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if !strings.Contains(buf.String(), "Press enter to continue.") {
		t.Errorf("expected continue prompt, got: %q", buf.String())
	}
}

func TestTableRendersHeaders(t *testing.T) {
	env, buf := dataTestEnv("1\n")
	_, err := tableWidget{}.Run(context.Background(), tableStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Name", "Status", "v1.0", "shipped", "v2.0", "planned"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "----") {
		t.Errorf("expected separator row, got: %q", out)
	}
}

func TestTableHeadersOnly(t *testing.T) {
	s := spec.Step{
		Type:    spec.TypeTable,
		Key:     "tbl",
		Title:   "Releases",
		Headers: []string{"Name", "Status"},
	}
	env, buf := dataTestEnv("\n")
	got, err := tableWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if !strings.Contains(buf.String(), "Name") || !strings.Contains(buf.String(), "Status") {
		t.Errorf("expected headers rendered, got: %q", buf.String())
	}
}

func TestTableSelectByNumber(t *testing.T) {
	env, _ := dataTestEnv("3\n")
	got, err := tableWidget{}.Run(context.Background(), tableStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Errorf("got %v, want 2", got)
	}
}

func TestTableSelectOutOfRangeRetries(t *testing.T) {
	env, buf := dataTestEnv("99\n2\n")
	got, err := tableWidget{}.Run(context.Background(), tableStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("got %v, want 1", got)
	}
	if !strings.Contains(buf.String(), "Invalid row number") {
		t.Errorf("expected invalid row message, got: %q", buf.String())
	}
}

func TestTableSelectInvalidInputRetries(t *testing.T) {
	env, _ := dataTestEnv("abc\n1\n")
	got, err := tableWidget{}.Run(context.Background(), tableStep(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %v, want 0", got)
	}
}

func TestTableDefaultByIndex(t *testing.T) {
	for name, def := range map[string]any{
		"int":   int(2),
		"int64": int64(2),
		"float": float64(2),
	} {
		t.Run(name, func(t *testing.T) {
			env, _ := dataTestEnv("\n")
			got, err := tableWidget{}.Run(context.Background(), stepWithDefault(def), env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != 2 {
				t.Errorf("got %v, want 2", got)
			}
		})
	}
}

func TestTableDefaultIndexOutOfRangeRetries(t *testing.T) {
	env, _ := dataTestEnv("\n1\n")
	got, err := tableWidget{}.Run(context.Background(), stepWithDefault(int(99)), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %v, want 0", got)
	}
}

func TestTableDefaultByStringMatch(t *testing.T) {
	env, _ := dataTestEnv("\n")
	got, err := tableWidget{}.Run(context.Background(), stepWithDefault("V1.1"), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

func TestTableDefaultStringNoMatchRetries(t *testing.T) {
	env, _ := dataTestEnv("\n1\n")
	got, err := tableWidget{}.Run(context.Background(), stepWithDefault("nope"), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %v, want 0", got)
	}
}
