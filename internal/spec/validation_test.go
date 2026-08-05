package spec_test

import (
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestRuleChecks(t *testing.T) {
	wiz, err := spec.Parse([]byte(`
steps:
  - type: input
    key: project_name
    title: "Project name"
    pattern: '^[a-z][a-z0-9-]{2,31}$'
    message: "Lowercase letters, digits and dashes; 3-32 characters."
  - type: input
    key: bio
    title: "Bio"
    min_length: 10
    max_length: 20
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	name := wiz.Steps[0].Rule()
	if problem := name.Check("my-app"); problem != "" {
		t.Errorf("Check(%q) = %q, want it accepted", "my-app", problem)
	}
	for _, bad := range []string{"My-App", "1app", "x", strings.Repeat("a", 40)} {
		problem := name.Check(bad)
		if problem == "" {
			t.Errorf("Check(%q) = accepted, want rejected", bad)
			continue
		}
		if problem != "Lowercase letters, digits and dashes; 3-32 characters." {
			t.Errorf("Check(%q) = %q, want the step's own message", bad, problem)
		}
	}

	bio := wiz.Steps[1].Rule()
	if problem := bio.Check("just right ok"); problem != "" {
		t.Errorf("Check(valid) = %q, want it accepted", problem)
	}
	if problem := bio.Check("short"); !strings.Contains(problem, "at least 10") {
		t.Errorf("Check(short) = %q, want the generated minimum message", problem)
	}
	if problem := bio.Check(strings.Repeat("x", 30)); !strings.Contains(problem, "at most 20") {
		t.Errorf("Check(long) = %q, want the generated maximum message", problem)
	}
}

// Length is counted in characters, not bytes: a rule written for people should
// not reject an accented name for being "too long".
func TestRuleCountsRunesNotBytes(t *testing.T) {
	wiz, err := spec.Parse([]byte("steps:\n  - {type: input, key: k, title: K, max_length: 5}\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if problem := wiz.Steps[0].Rule().Check("èéêëì"); problem != "" {
		t.Errorf("Check(5 accented runes) = %q, want it accepted", problem)
	}
}

// A step with no validation fields has no rule, and a nil rule accepts.
func TestNilRuleAccepts(t *testing.T) {
	wiz, err := spec.Parse([]byte("steps:\n  - {type: input, key: k, title: K}\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if wiz.Steps[0].Rule() != nil {
		t.Error("a step without validation fields should have no rule")
	}
	if problem := wiz.Steps[0].Rule().Check("anything at all"); problem != "" {
		t.Errorf("nil rule rejected %q", problem)
	}
}

// The whole reason validation is declarative: a generated spec whose default
// breaks its own rule must die in the dry run, not in front of a person.
func TestDefaultMustSatisfyItsOwnRule(t *testing.T) {
	_, err := spec.Parse([]byte(`
steps:
  - type: input
    key: name
    title: "Name"
    pattern: '^[a-z]+$'
    default: "Not Lowercase"
`))
	if err == nil {
		t.Fatal("Parse() = nil error, want the default rejected by its own pattern")
	}
	if !strings.Contains(err.Error(), "fails its own validation") {
		t.Errorf("error = %v, want it to say the default fails its own rule", err)
	}
}

// RE2 has no lookahead, and that is the usual reason a copied pattern breaks,
// so the error has to name it.
func TestInvalidPatternExplainsRE2(t *testing.T) {
	_, err := spec.Parse([]byte("steps:\n  - {type: input, key: k, title: K, pattern: '(?=.*[A-Z])'}\n"))
	if err == nil {
		t.Fatal("Parse() = nil error, want the lookahead rejected")
	}
	if !strings.Contains(err.Error(), "RE2") {
		t.Errorf("error = %v, want it to mention RE2", err)
	}
}

func TestContradictoryLengths(t *testing.T) {
	_, err := spec.Parse([]byte("steps:\n  - {type: input, key: k, title: K, min_length: 10, max_length: 5}\n"))
	if err == nil {
		t.Fatal("Parse() = nil error, want min_length > max_length rejected")
	}
}
