package spec_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// The short form must be sugar and nothing else: for each pair below, the argv
// words and the YAML must parse to structurally identical wizards. If they ever
// diverge, the short form has become a second dialect — which is the failure
// mode this whole design is meant to avoid.
func TestShortFormEquivalence(t *testing.T) {
	tests := []struct {
		name  string
		short []string
		yaml  string
	}{
		{
			name:  "select with derived title and implicit default",
			short: []string{"select", "framework", "SwiftUI", "UIKit", "AppKit"},
			yaml: `steps:
  - {type: select, key: framework, title: Framework, options: [SwiftUI, UIKit, AppKit], default: SwiftUI}`,
		},
		{
			name:  "key with separators becomes a sentence",
			short: []string{"input", "db_host", "localhost"},
			yaml: `steps:
  - {type: input, key: db_host, title: "Db host", default: localhost}`,
		},
		{
			name:  "explicit title wins over the derived one",
			short: []string{"input", "name", "MyApp", "--title", "Project name"},
			yaml: `steps:
  - {type: input, key: name, title: "Project name", default: MyApp}`,
		},
		{
			name:  "multiselect takes no implicit default",
			short: []string{"multiselect", "platforms", "iOS", "macOS", "--default", "iOS"},
			yaml: `steps:
  - {type: multiselect, key: platforms, title: Platforms, options: [iOS, macOS], default: iOS}`,
		},
		{
			name:  "number with bounds",
			short: []string{"number", "port", "8080", "--min", "1", "--max", "65535", "--increment", "1"},
			yaml: `steps:
  - {type: number, key: port, title: Port, default: "8080", min: 1, max: 65535, increment: 1}`,
		},
		{
			name:  "confirm",
			short: []string{"confirm", "git", "yes"},
			yaml: `steps:
  - {type: confirm, key: git, title: Git, default: "yes"}`,
		},
		{
			name:  "keyvalue pairs",
			short: []string{"keyvalue", "env", "LOG=info", "REGION=eu"},
			yaml: `steps:
  - {type: keyvalue, key: env, title: Env, default: {LOG: info, REGION: eu}}`,
		},
		{
			name:  "several steps and a condition",
			short: []string{"confirm", "use_db", "yes", "+", "input", "db_name", "app", "--when", "use_db"},
			yaml: `steps:
  - {type: confirm, key: use_db, title: "Use db", default: "yes"}
  - {type: input, key: db_name, title: "Db name", default: app, when: use_db}`,
		},
		{
			name:  "validation attributes",
			short: []string{"input", "name", "--pattern", "^[a-z]+$", "--message", "lowercase only", "--required"},
			yaml: `steps:
  - {type: input, key: name, title: Name, pattern: '^[a-z]+$', message: "lowercase only", required: true}`,
		},
		{
			name:  "height on a long list",
			short: []string{"search", "region", "eu", "us", "ap", "--height", "2"},
			yaml: `steps:
  - {type: search, key: region, title: Region, options: [eu, us, ap], default: eu, height: 2}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fromShort, err := spec.ParseShort(tc.short)
			if err != nil {
				t.Fatalf("ParseShort(%q) error: %v", tc.short, err)
			}
			fromYAML, err := spec.Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("Parse(yaml) error: %v", err)
			}
			if !reflect.DeepEqual(fromShort, fromYAML) {
				t.Errorf("short form and YAML disagree:\n short: %#v\n yaml:  %#v",
					fromShort.Steps, fromYAML.Steps)
			}
		})
	}
}

// Every step type must be either expressible in the short form or explicitly
// excluded, so a type added later cannot be silently forgotten.
func TestShortFormCoversEveryType(t *testing.T) {
	excluded := map[spec.StepType]bool{spec.TypeTable: true}
	// Derived from the registry rather than listed by hand: this test exists to
	// catch a type added without a decision about the short form, and a
	// hand-maintained list is exactly the thing that would silently miss it.
	all := spec.AllStepTypes()
	for _, typ := range all {
		expressible := spec.IsShortType(string(typ))
		if excluded[typ] && expressible {
			t.Errorf("%s is excluded from the short form but IsShortType accepts it", typ)
		}
		if !excluded[typ] && !expressible {
			t.Errorf("%s is neither expressible in the short form nor listed as excluded", typ)
		}
	}
}

func TestShortFormErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"choose", "x", "a"}, "unknown type"},
		{[]string{"select"}, "missing key"},
		{[]string{"select", "framework"}, "at least one option"},
		{[]string{"input", "name", "a", "b", "c"}, "at most one value"},
		{[]string{"number", "port", "--mim", "1"}, "unknown attribute"},
		{[]string{"number", "port", "--min"}, "needs a value"},
		{[]string{"number", "port", "--min", "low"}, "wants a number"},
		{[]string{"input", "a", "+"}, "empty step"},
		{[]string{"keyvalue", "env", "LOGinfo"}, "key=value"},
		{[]string{"input", "my name"}, "invalid key"},
	}
	for _, tc := range tests {
		_, err := spec.ParseShort(tc.args)
		if err == nil {
			t.Errorf("ParseShort(%q) = nil error, want one mentioning %q", tc.args, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseShort(%q) = %v, want it to mention %q", tc.args, err, tc.want)
		}
	}
}

// A value that has to start with the reserved characters stays reachable.
func TestShortFormEscapes(t *testing.T) {
	wiz, err := spec.ParseShort([]string{"input", "flag", `\--verbose`})
	if err != nil {
		t.Fatalf("ParseShort error: %v", err)
	}
	if got := wiz.Steps[0].Default; got != "--verbose" {
		t.Errorf("default = %v, want the literal --verbose", got)
	}
}
