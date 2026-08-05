package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// The built-in demo is the branchiest spec in the repository, so it is what
// the enumeration is exercised against.
func TestPrintPathsIsOneJSONObjectPerLine(t *testing.T) {
	wiz, err := loadDemo()
	if err != nil {
		t.Fatalf("loadDemo error: %v", err)
	}
	var out bytes.Buffer
	if code := printPaths(&out, wiz); code != 0 {
		t.Fatalf("printPaths returned %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("a branching wizard has more than one shape; got:\n%s", out.String())
	}
	for i, line := range lines {
		var row struct {
			Steps   []string          `json:"steps"`
			Answers map[string]string `json:"answers"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i+1, err, line)
		}
		if len(row.Steps) == 0 {
			t.Errorf("line %d has no steps", i+1)
		}
		if len(row.Answers) == 0 {
			t.Errorf("line %d does not say which answers reach it", i+1)
		}
	}
}

// Two runs of the same spec must print the same list, or a diff of the two is
// noise rather than a change.
func TestPathsOutputIsStable(t *testing.T) {
	wiz, err := loadDemo()
	if err != nil {
		t.Fatalf("loadDemo error: %v", err)
	}
	var first, second bytes.Buffer
	printPaths(&first, wiz)
	printPaths(&second, wiz)
	if first.String() != second.String() {
		t.Error("two enumerations of the same wizard printed different lists")
	}
}

// Every step named in a path exists in the spec, in the spec's own order.
func TestPathsOnlyNameRealStepsInOrder(t *testing.T) {
	wiz, err := loadDemo()
	if err != nil {
		t.Fatalf("loadDemo error: %v", err)
	}
	position := map[string]int{}
	for i, s := range wiz.Steps {
		position[s.Key] = i
	}
	paths, _ := spec.Paths(wiz, 0)
	for _, p := range paths {
		last := -1
		for _, key := range p.Steps {
			at, known := position[key]
			if !known {
				t.Fatalf("path names a step the wizard does not have: %q", key)
			}
			if at <= last {
				t.Fatalf("path %v is out of spec order at %q", p.Steps, key)
			}
			last = at
		}
	}
}
