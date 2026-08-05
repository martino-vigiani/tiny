package spec_test

import (
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestParseCondition(t *testing.T) {
	tests := []struct {
		expr string
		key  string
		op   spec.ConditionOp
		val  string
		vals []string
	}{
		{expr: "use_db", key: "use_db", op: spec.OpTruthy},
		{expr: "  use_db  ", key: "use_db", op: spec.OpTruthy},
		{expr: "!use_db", key: "use_db", op: spec.OpFalsy},
		{expr: "! use_db", key: "use_db", op: spec.OpFalsy},
		{expr: "framework == SwiftUI", key: "framework", op: spec.OpEqual, val: "SwiftUI"},
		{expr: "framework==SwiftUI", key: "framework", op: spec.OpEqual, val: "SwiftUI"},
		{expr: `label == "two words"`, key: "label", op: spec.OpEqual, val: "two words"},
		{expr: "env != production", key: "env", op: spec.OpNotEqual, val: "production"},
		{expr: "region in eu-west-1|us-east-1", key: "region", op: spec.OpIn, vals: []string{"eu-west-1", "us-east-1"}},
		{expr: "platforms contains iOS", key: "platforms", op: spec.OpContains, val: "iOS"},
	}
	for _, tc := range tests {
		got, err := spec.ParseCondition(tc.expr)
		if err != nil {
			t.Errorf("ParseCondition(%q) error: %v", tc.expr, err)
			continue
		}
		if got.Key != tc.key || got.Operator != tc.op {
			t.Errorf("ParseCondition(%q) = key %q op %q, want key %q op %q", tc.expr, got.Key, got.Operator, tc.key, tc.op)
		}
		if tc.val != "" && got.Value != tc.val {
			t.Errorf("ParseCondition(%q).Value = %q, want %q", tc.expr, got.Value, tc.val)
		}
		if len(tc.vals) > 0 && strings.Join(got.Values, ",") != strings.Join(tc.vals, ",") {
			t.Errorf("ParseCondition(%q).Values = %v, want %v", tc.expr, got.Values, tc.vals)
		}
	}
}

func TestParseConditionInvalid(t *testing.T) {
	for _, expr := range []string{
		"",
		"   ",
		"!",
		"== SwiftUI",
		"has spaces == x",
		"region in ",
		"platforms contains ",
	} {
		if _, err := spec.ParseCondition(expr); err == nil {
			t.Errorf("ParseCondition(%q) = nil error, want a failure", expr)
		}
	}
}

func TestConditionEval(t *testing.T) {
	answers := map[string]any{
		"use_db":    true,
		"skip":      false,
		"framework": "SwiftUI",
		"empty":     "",
		"replicas":  float64(3),
		"zero":      float64(0),
		"platforms": []string{"iOS", "macOS"},
		"none":      []string{},
		"env":       map[string]string{"LOG": "info"},
		"row":       int(2),
		"missing":   nil,
	}
	tests := []struct {
		expr string
		want bool
	}{
		{"use_db", true},
		{"skip", false},
		{"!skip", true},
		{"!use_db", false},
		{"framework", true},
		{"empty", false},
		{"replicas", true},
		{"zero", false},
		{"platforms", true},
		{"none", false},
		{"env", true},
		{"missing", false},

		{"framework == SwiftUI", true},
		{"framework == swiftui", true},
		{"framework == UIKit", false},
		{"framework != UIKit", true},
		{"use_db == true", true},
		{"use_db == yes", true},
		{"skip == no", true},
		{"replicas == 3", true},
		{"replicas == 4", false},
		{"row == 2", true},

		{"framework in SwiftUI|UIKit", true},
		{"framework in UIKit|AppKit", false},
		{"platforms in iOS|tvOS", true},

		{"platforms contains iOS", true},
		{"platforms contains tvOS", false},
		{"framework contains Swift", true},
		{"env contains LOG", true},

		// A key nobody answered is false, whatever the operator.
		{"never_asked", false},
		{"never_asked == x", false},
		{"!never_asked", false},
	}
	for _, tc := range tests {
		cond, err := spec.ParseCondition(tc.expr)
		if err != nil {
			t.Errorf("ParseCondition(%q) error: %v", tc.expr, err)
			continue
		}
		if got := cond.Eval(answers); got != tc.want {
			t.Errorf("Eval(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// An unconditional step must always run.
func TestNilConditionAlwaysHolds(t *testing.T) {
	var cond *spec.Condition
	if !cond.Eval(nil) {
		t.Error("a nil condition must evaluate true")
	}
}

func TestWizardRejectsForwardReference(t *testing.T) {
	yaml := []byte(`
steps:
  - type: input
    key: db_name
    title: "Database name"
    when: "use_db"
  - type: confirm
    key: use_db
    title: "Use a database?"
`)
	_, err := spec.Parse(yaml)
	if err == nil {
		t.Fatal("Parse() = nil error, want a failure: the condition reads a later answer")
	}
	if !strings.Contains(err.Error(), "use_db") {
		t.Errorf("error = %v, want it to name the offending key", err)
	}
}

func TestWizardRejectsSelfReference(t *testing.T) {
	yaml := []byte(`
steps:
  - type: input
    key: name
    title: "Name"
    when: "name"
`)
	if _, err := spec.Parse(yaml); err == nil {
		t.Fatal("Parse() = nil error, want a failure: a step cannot depend on its own answer")
	}
}

func TestWizardAcceptsBackwardReference(t *testing.T) {
	yaml := []byte(`
steps:
  - type: confirm
    key: use_db
    title: "Use a database?"
    default: "yes"
  - type: input
    key: db_name
    title: "Database name"
    default: "app"
    when: "use_db"
`)
	wiz, err := spec.Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	cond := wiz.Steps[1].Condition()
	if cond == nil || cond.Key != "use_db" {
		t.Errorf("Steps[1].Condition() = %+v, want the parsed condition on use_db", cond)
	}
	if wiz.Steps[0].Condition() != nil {
		t.Error("an unconditional step must have a nil condition")
	}
}
