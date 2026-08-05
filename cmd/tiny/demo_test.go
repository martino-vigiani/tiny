package main

import (
	"bytes"
	"os"
	"testing"
)

// The embedded copy and the file people read must be the same bytes, or the
// README would document a wizard nobody runs.
func TestDemoMatchesTheExample(t *testing.T) {
	onDisk, err := os.ReadFile("../../examples/showcase.yaml")
	if err != nil {
		t.Fatalf("reading examples/showcase.yaml: %v", err)
	}
	if !bytes.Equal(onDisk, demoSpec) {
		t.Errorf("cmd/tiny/demo/showcase.yaml has drifted from examples/showcase.yaml;\n" +
			"copy the example over it: cp examples/showcase.yaml cmd/tiny/demo/showcase.yaml")
	}
}

// The demo must resolve headless, since that is how it is tested and how a
// caller checks it before showing it to anyone.
func TestDemoRunsHeadless(t *testing.T) {
	wiz, err := loadDemo()
	if err != nil {
		t.Fatalf("the embedded demo does not parse: %v", err)
	}
	if len(wiz.Steps) == 0 {
		t.Fatal("the embedded demo has no steps")
	}
}
