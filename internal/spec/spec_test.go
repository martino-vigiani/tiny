package spec

import (
	"strings"
	"testing"
)

const validWizard = `
steps:
  - type: select
    key: framework
    title: "Scegli il framework"
    options: [SwiftUI, UIKit]
    default: SwiftUI
  - type: multiselect
    key: langs
    options: [go, rust, swift]
    default: [go, rust]
  - type: confirm
    key: init_git
    default: "yes"
  - type: input
    key: project_name
    default: "MiaApp"
  - type: password
    key: db_password
  - type: search
    key: repo
    options: [tiny, big]
  - type: number
    key: replicas
    min: 1
    max: 10
    default: 3
  - type: date
    key: release
    format: datetime
  - type: keyvalue
    key: env
  - type: editor
    key: notes
  - type: autocomplete
    key: editor_bin
    options: [vim, nano]
  - type: table
    key: matrix
    headers: [a, b]
    rows:
      - [1, 2]
      - [3, 4]
`

func TestParseValidWizard(t *testing.T) {
	w, err := Parse([]byte(validWizard))
	if err != nil {
		t.Fatalf("Parse(valid) error: %v", err)
	}
	if len(w.Steps) != 12 {
		t.Fatalf("expected 12 steps, got %d", len(w.Steps))
	}
}

// TestParseOldFormat locks backward compatibility: the original 3-type wizard
// shape (select/input/confirm, string defaults) must keep validating.
func TestParseOldFormat(t *testing.T) {
	old := `
steps:
  - type: select
    key: framework
    options: [SwiftUI, UIKit, AppKit]
    default: SwiftUI
  - type: input
    key: project_name
    default: "MiaApp"
  - type: confirm
    key: init_git
    default: "yes"
`
	if _, err := Parse([]byte(old)); err != nil {
		t.Fatalf("Parse(old format) error: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"empty wizard", "steps: []", "at least one step"},
		{"missing key", "steps:\n  - type: input\n", "missing key"},
		{
			"duplicate key",
			"steps:\n  - {type: input, key: a}\n  - {type: input, key: a}\n",
			"duplicate key",
		},
		{"unknown type", "steps:\n  - {type: foo, key: a}\n", "unsupported type"},
		{
			"select without options",
			"steps:\n  - {type: select, key: a}\n",
			"at least one option",
		},
		{
			"search without options",
			"steps:\n  - {type: search, key: a}\n",
			"at least one option",
		},
		{
			"select default not in options",
			"steps:\n  - type: select\n    key: a\n    options: [x]\n    default: y\n",
			"not one of the options",
		},
		{
			"multiselect default not in options",
			"steps:\n  - type: multiselect\n    key: a\n    options: [x]\n    default: [y]\n",
			"not one of the options",
		},
		{
			"number min > max",
			"steps:\n  - {type: number, key: a, min: 10, max: 1}\n",
			"min",
		},
		{
			"date invalid format",
			"steps:\n  - {type: date, key: a, format: iso8601}\n",
			"format",
		},
		{
			"table without headers or rows",
			"steps:\n  - {type: table, key: a}\n",
			"headers or rows",
		},
		{
			"table selectable without rows",
			"steps:\n  - type: table\n    key: a\n    headers: [h]\n    selectable: true\n",
			"at least one row",
		},
		{
			"number default out of range",
			"steps:\n  - {type: number, key: a, min: 1, max: 10, default: 99}\n",
			"above max",
		},
		{
			"number default below min",
			"steps:\n  - {type: number, key: a, min: 1, max: 10, default: 0}\n",
			"below min",
		},
		{
			"confirm default not parseable",
			"steps:\n  - {type: confirm, key: a, default: \"sì\"}\n",
			"invalid confirm default",
		},
		{
			"date default invalid",
			"steps:\n  - {type: date, key: a, default: \"not-a-date\"}\n",
			"does not match format",
		},
		{
			"keyvalue default with list value",
			"steps:\n  - type: keyvalue\n    key: a\n    default:\n      A: [1, 2]\n",
			"must be a scalar",
		},
		{
			"table default index out of range",
			"steps:\n  - type: table\n    key: a\n    rows: [[x], [y]]\n    selectable: true\n    default: 99\n",
			"out of range",
		},
		{
			"table default when not selectable",
			"steps:\n  - type: table\n    key: a\n    rows: [[x]]\n    default: \"x\"\n",
			"default only valid when selectable",
		},
		{
			"search default not among options",
			"steps:\n  - {type: search, key: a, options: [x], default: y}\n",
			"not one of the options",
		},
		{
			"autocomplete default not among options",
			"steps:\n  - {type: autocomplete, key: a, options: [x], default: y}\n",
			"not one of the options",
		},
		{
			"key with spaces",
			"steps:\n  - {type: input, key: \"my key\"}\n",
			"invalid key",
		},
		{
			"duplicate options case-insensitive",
			"steps:\n  - {type: select, key: a, options: [x, X]}\n",
			"duplicate option",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestPrompt(t *testing.T) {
	s := Step{PromptText: "prompt text", Title: "title text"}
	if got := s.Prompt(); got != "prompt text" {
		t.Errorf("Prompt() = %q, want prompt text", got)
	}
	s2 := Step{Title: "title text"}
	if got := s2.Prompt(); got != "title text" {
		t.Errorf("Prompt() = %q, want title text", got)
	}
}

func TestMultiselectCommaDefault(t *testing.T) {
	yml := `
steps:
  - type: multiselect
    key: a
    options: [go, rust, swift]
    default: "go, rust"
`
	if _, err := Parse([]byte(yml)); err != nil {
		t.Fatalf("comma-separated multiselect default should validate: %v", err)
	}
}
