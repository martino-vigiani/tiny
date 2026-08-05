package main

// -paths: print every shape the wizard can take, without asking anything.
//
// One JSON object per line — the steps in order, and the answers that reach
// them. JSONL rather than one array because the list is meant to be filtered
// and diffed:
//
//	tiny -paths deploy.yaml | jq -r '.steps | join(" -> ")'
//	tiny -paths deploy.yaml | wc -l           # how many shapes to review
//	tiny -paths deploy.yaml > paths.jsonl     # and diff it in review
//
// It reads no input and asks no questions, so it is safe to run against a spec
// you were sent — which is the case where knowing what it would ask matters
// most.

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/martino-vigiani/tiny/internal/runner"
	"github.com/martino-vigiani/tiny/internal/spec"
)

// printPaths writes the enumeration and returns the process exit code.
func printPaths(w io.Writer, wiz *spec.Wizard) int {
	paths, truncated := spec.Paths(wiz, spec.DefaultPathLimit)
	spec.SortPaths(paths)

	bw := bufio.NewWriter(w)
	for _, p := range paths {
		// The same encoder the payload uses, so the escaping of an answer is
		// the escaping a consumer already knows.
		if err := runner.PrintJSONCompact(bw, map[string]any{
			"steps":   p.Steps,
			"answers": p.Answers,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
			return 1
		}
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "tiny: %v\n", err)
		return 1
	}
	// A truncated list looks exactly like a complete one, so saying so is the
	// difference between "these are the paths" and "these are some of them".
	if truncated {
		fmt.Fprintf(os.Stderr, "tiny: more than %d distinct paths; the list above is cut short\n",
			spec.DefaultPathLimit)
	}
	return 0
}
