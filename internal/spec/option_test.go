package spec

import "testing"

func TestPlainOptionsCarryNoText(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - type: select
    key: db
    title: Database
    options: [postgres, mysql]
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	s := w.Steps[0]
	if len(s.Options) != 2 || s.Options[0] != "postgres" {
		t.Fatalf("options = %v", s.Options)
	}
	// A spec that uses no labels must allocate none: it is the common case and
	// the one that has to stay free.
	if s.Labels != nil || s.Descriptions != nil {
		t.Errorf("a plain option list allocated labels=%v descriptions=%v", s.Labels, s.Descriptions)
	}
}

func TestRichOptionsAnswerWithTheValue(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - type: select
    key: db
    title: Database
    default: cockroach
    options:
      - postgres
      - value: cockroach
        label: CockroachDB
        description: distributed, wire-compatible with postgres
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	s := w.Steps[0]
	if got := s.Options[1]; got != "cockroach" {
		t.Errorf("the answer value = %q, want the value not the label", got)
	}
	if got := s.OptionLabel(1); got != "CockroachDB" {
		t.Errorf("OptionLabel = %q", got)
	}
	if got := s.OptionLabel(0); got != "postgres" {
		t.Errorf("an unlabelled option must draw as its value, got %q", got)
	}
	if got := s.OptionDescription(1); got == "" {
		t.Error("the description was dropped")
	}
}

// A label is typeable, because it is the word the person actually read.
func TestOptionLookupAcceptsValueOrLabel(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - type: select
    key: db
    title: Database
    options:
      - value: cockroach
        label: CockroachDB
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	s := w.Steps[0]
	for _, typed := range []string{"cockroach", "COCKROACH", "CockroachDB", "cockroachdb"} {
		if got := s.IndexOfOption(typed); got != 0 {
			t.Errorf("IndexOfOption(%q) = %d, want 0", typed, got)
		}
	}
	if got := s.IndexOfOption("mysql"); got != -1 {
		t.Errorf("IndexOfOption(mysql) = %d, want -1", got)
	}
}

// A label that equals another option's value would make one typed word mean
// two things, decided by whichever lookup happened to scan first.
func TestLabelCollidingWithAValueIsRejected(t *testing.T) {
	_, err := Parse([]byte(`
steps:
  - type: select
    key: db
    title: Database
    options:
      - postgres
      - value: cockroach
        label: postgres
`))
	if err == nil {
		t.Fatal("a label colliding with another option was accepted")
	}
}

// A default still names the value, and validation must resolve it.
func TestRichOptionDefaultIsCheckedAgainstValues(t *testing.T) {
	if _, err := Parse([]byte(`
steps:
  - type: select
    key: db
    title: Database
    default: nope
    options:
      - value: cockroach
        label: CockroachDB
`)); err == nil {
		t.Fatal("a default that is not an option was accepted")
	}
}

// A mapping with only a label has an obvious value; making the author type the
// same word twice would buy nothing.
func TestLabelOnlyOptionUsesItsLabelAsTheValue(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - type: select
    key: k
    title: T
    options:
      - label: Only
        description: no value given
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := w.Steps[0].Options[0]; got != "Only" {
		t.Errorf("value = %q, want the label", got)
	}
}

func TestOptionMappingWithoutValueOrLabelIsRejected(t *testing.T) {
	if _, err := Parse([]byte(`
steps:
  - type: select
    key: k
    title: T
    options:
      - description: nothing to answer with
`)); err == nil {
		t.Fatal("an option with neither value nor label was accepted")
	}
}

// Numbers and booleans stay usable as options: `options: [80, 443]` is a list
// of ports, not a type error.
func TestScalarOptionsAreNotOnlyStrings(t *testing.T) {
	w, err := Parse([]byte(`
steps:
  - type: select
    key: port
    title: Port
    options: [80, 443]
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := w.Steps[0].Options; got[0] != "80" || got[1] != "443" {
		t.Errorf("options = %v", got)
	}
}

// Rewriting a step's options must drop text that described the old ones, or a
// listing would carry labels belonging to entries that are gone.
func TestReplaceOptionsForgetsTheOldText(t *testing.T) {
	s := Step{
		Options:      []string{"a"},
		Labels:       []string{"Alpha"},
		Descriptions: []string{"the first"},
	}
	s.ReplaceOptions([]string{"x", "y"})
	if s.Labels != nil || s.Descriptions != nil {
		t.Errorf("labels=%v descriptions=%v survived a rewrite", s.Labels, s.Descriptions)
	}
	if got := s.OptionLabel(1); got != "y" {
		t.Errorf("OptionLabel(1) = %q", got)
	}
}
