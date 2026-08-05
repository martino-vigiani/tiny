package main

// The demo: `tiny demo`, from anywhere, with no files.
//
// A tool whose whole pitch is what it looks like has to be able to show that
// on a machine that has never seen its repository. The showcase spec is
// compiled into the binary, so the one-word command works from any directory
// with nothing checked out — which is also the only version of "try it" that
// survives being pasted into a README.
//
// The embedded copy is kept identical to examples/showcase.yaml by
// TestDemoMatchesTheExample, so the file people read and the wizard people run
// cannot drift apart.

import (
	_ "embed"

	"github.com/martino-vigiani/tiny/internal/spec"
)

//go:embed demo/showcase.yaml
var demoSpec []byte

// demoArg is the word that runs the built-in showcase.
const demoArg = "demo"

// loadDemo parses the embedded showcase. A failure here is a build-time
// mistake shipped to a user, so it is reported like any other invalid spec
// rather than panicking.
func loadDemo() (*spec.Wizard, error) {
	return spec.Parse(demoSpec)
}
