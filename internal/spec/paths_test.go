package spec

import (
	"fmt"
	"strings"
	"testing"
)

func pathsOf(t *testing.T, yaml string) []Path {
	t.Helper()
	w, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	paths, truncated := Paths(w, 0)
	if truncated {
		t.Fatalf("the enumeration was truncated for a small wizard")
	}
	SortPaths(paths)
	return paths
}

func shapes(paths []Path) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = strings.Join(p.Steps, ",")
	}
	return out
}

func TestPathsOfAWizardWithoutConditions(t *testing.T) {
	paths := pathsOf(t, `
steps:
  - {type: input, key: a, title: A, default: x}
  - {type: input, key: b, title: B, default: y}
`)
	if len(paths) != 1 {
		t.Fatalf("a wizard with no conditions has one shape, got %d: %v", len(paths), shapes(paths))
	}
	if got := shapes(paths)[0]; got != "a,b" {
		t.Errorf("steps = %s", got)
	}
	if len(paths[0].Answers) != 0 {
		t.Errorf("nothing branches, so nothing should be reported as deciding: %v", paths[0].Answers)
	}
}

func TestConfirmBranchesBothWays(t *testing.T) {
	paths := pathsOf(t, `
steps:
  - {type: confirm, key: db, title: "DB?", default: "no"}
  - {type: input, key: dsn, title: DSN, default: x, when: "db"}
`)
	got := shapes(paths)
	if len(got) != 2 || got[0] != "db" || got[1] != "db,dsn" {
		t.Fatalf("shapes = %v, want [db db,dsn]", got)
	}
	// Every path carries the answers that reach it, so the list can be acted
	// on rather than merely read.
	for _, p := range paths {
		if p.Answers["db"] == "" {
			t.Errorf("path %v does not say what db was", p.Steps)
		}
	}
}

// A chain of conditions drops out together, and the enumeration has to see
// that rather than offering a step whose own condition can never hold.
func TestChainedConditionsDropOutTogether(t *testing.T) {
	got := shapes(pathsOf(t, `
steps:
  - {type: confirm, key: db, title: "DB?", default: "yes"}
  - type: select
    key: engine
    title: Engine
    options: [postgres, sqlite]
    default: postgres
    when: "db"
  - {type: number, key: size, title: Size, default: 10, when: "engine != sqlite"}
`))
	want := map[string]bool{
		"db":                  true, // no database at all
		"db,engine":           true, // sqlite needs no volume
		"db,engine,size":      true, // postgres does
		"db,engine,size,size": false,
	}
	for _, shape := range got {
		if !want[shape] {
			t.Errorf("unexpected shape %q", shape)
		}
		delete(want, shape)
	}
	for shape, expected := range want {
		if expected {
			t.Errorf("missing shape %q", shape)
		}
	}
}

// A select branches over its options; a key nothing reads does not branch at
// all, or every wizard would enumerate its whole answer space.
func TestOnlyConditionedKeysBranch(t *testing.T) {
	paths := pathsOf(t, `
steps:
  - type: select
    key: colour
    title: Colour
    options: [red, green, blue]
    default: red
  - type: select
    key: mode
    title: Mode
    options: [fast, slow]
    default: fast
  - {type: input, key: why, title: Why, default: x, when: "mode == slow"}
`)
	if len(paths) != 2 {
		t.Fatalf("colour is read by nothing and must not branch; shapes = %v", shapes(paths))
	}
	for _, p := range paths {
		if _, present := p.Answers["colour"]; present {
			t.Errorf("colour was reported as deciding a path: %v", p.Answers)
		}
	}
}

// `contains` needs a multiselect to be explored as a set, not as a single
// value.
func TestMultiselectContains(t *testing.T) {
	got := shapes(pathsOf(t, `
steps:
  - type: multiselect
    key: parts
    title: Parts
    options: [api, worker]
    default: api
  - {type: number, key: workers, title: Workers, default: 1, when: "parts contains worker"}
`))
	if len(got) != 2 {
		t.Fatalf("shapes = %v, want one with workers and one without", got)
	}
}

// A free-text step can hold anything, so the literals a condition compares it
// against are the values worth trying.
func TestTextStepUsesTheLiteralsInItsConditions(t *testing.T) {
	got := shapes(pathsOf(t, `
steps:
  - {type: input, key: env, title: Env, default: dev}
  - {type: confirm, key: approved, title: "Approved?", default: "no", when: "env == prod"}
`))
	if len(got) != 2 {
		t.Fatalf("shapes = %v; the literal \"prod\" was never tried", got)
	}
}

// Every printed path must be reachable by the answers printed with it — that
// is the one guarantee the feature makes, so it is checked by replaying them.
func TestEveryPathIsReachableByItsOwnAnswers(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - {type: confirm, key: db, title: "DB?", default: "yes"}
  - type: select
    key: engine
    title: Engine
    options: [postgres, sqlite]
    default: postgres
    when: "db"
  - {type: number, key: size, title: Size, default: 10, when: "engine != sqlite"}
  - {type: confirm, key: apply, title: "Apply?", default: "no"}
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	paths, _ := Paths(w, 0)
	for _, p := range paths {
		// Replay the wizard with exactly the answers the path reports.
		answers := map[string]any{}
		var reached []string
		for _, s := range w.Steps {
			if c := s.Condition(); c != nil && !c.Eval(answers) {
				continue
			}
			reached = append(reached, s.Key)
			if text, ok := p.Answers[s.Key]; ok {
				answers[s.Key] = replayValue(s, text)
			} else {
				answers[s.Key] = defaultAnswer(s)
			}
		}
		if strings.Join(reached, ",") != strings.Join(p.Steps, ",") {
			t.Errorf("path %v is not reproduced by its own answers %v: got %v",
				p.Steps, p.Answers, reached)
		}
	}
}

// replayValue turns a reported answer back into a value of the right type.
func replayValue(s Step, text string) any {
	switch s.Type {
	case TypeConfirm:
		b, err := ParseConfirmDefault(text)
		if err != nil {
			return false
		}
		return b
	case TypeNumber:
		f, err := ToFloat(text)
		if err != nil {
			return float64(0)
		}
		return f
	case TypeMultiselect:
		items, _ := SplitMultiDefault(text)
		return OrderByOptions(items, s.Options)
	default:
		return text
	}
}

// A wizard with more shapes than the limit says so, rather than presenting a
// cut list as the complete one.
func TestTruncationIsReported(t *testing.T) {
	// Six independent switches: 64 shapes, well past a limit of 4.
	var b strings.Builder
	b.WriteString("steps:\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "  - {type: confirm, key: c%d, title: C, default: \"no\"}\n", i)
		fmt.Fprintf(&b, "  - {type: input, key: d%d, title: D, default: x, when: \"c%d\"}\n", i, i)
	}
	w, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	paths, truncated := Paths(w, 4)
	if !truncated {
		t.Fatalf("a wizard with more shapes than the limit reported none cut; got %d", len(paths))
	}
	if len(paths) > 4 {
		t.Errorf("the limit was exceeded: %d paths", len(paths))
	}
}
