package runner

// The audit trail: what produced this payload.
//
// A tiny payload is a decision, and decisions get read back later — in a
// review, in an incident, in a diff of two deploys. Answers alone cannot say
// whether a human typed them or a CI job defaulted them, which spec they came
// from, or which questions were never asked. `-meta` adds that under a key no
// step can ever have.
//
// It is off by default, because a payload with a timestamp in it is not
// byte-reproducible, and reproducibility is worth more than provenance to
// everyone who did not ask for provenance.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// MetaKey is where the run's provenance goes. It cannot collide with a step:
// a wizard key must start with a letter or a digit, so no step can be called
// "_meta".
const MetaKey = "_meta"

// Mode names how the answers were produced.
const (
	ModeInteractive = "interactive"
	ModeDefaults    = "defaults"
	ModePreset      = "preset"
)

// Meta describes one run.
type Meta struct {
	Version string    // the tiny that ran it
	Source  string    // the spec: a path, "-", "demo", or "argv"
	Spec    []byte    // the spec bytes, hashed so two runs can be compared
	Mode    string    // ModeInteractive, ModeDefaults or ModePreset
	TTY     bool      // whether a person could actually answer
	Started time.Time // when the wizard opened
	Ended   time.Time // when the last answer landed
}

// Attach adds the meta block to answers, in place.
//
// Every value is a string, so the block survives all three output formats
// unchanged — `-format env` has no shape for a nested list, and provenance
// that only exists in JSON is provenance a shell caller cannot check.
func (m Meta) Attach(answers map[string]any, wiz *spec.Wizard) {
	answers[MetaKey] = m.Map(answers, wiz)
}

// Map builds the meta block.
func (m Meta) Map(answers map[string]any, wiz *spec.Wizard) map[string]string {
	out := map[string]string{
		"tiny":     m.Version,
		"source":   m.Source,
		"mode":     m.Mode,
		"tty":      strconv.FormatBool(m.TTY),
		"answered": strconv.Itoa(len(answers)),
	}
	if len(m.Spec) > 0 {
		sum := sha256.Sum256(m.Spec)
		out["spec_sha256"] = hex.EncodeToString(sum[:])
	}
	if !m.Started.IsZero() {
		out["started"] = m.Started.UTC().Format(time.RFC3339)
		out["elapsed_ms"] = strconv.FormatInt(m.Ended.Sub(m.Started).Milliseconds(), 10)
	}
	// Which questions were never asked. A missing key already says a branch
	// was not taken, but only to someone holding the spec; naming them here
	// makes the payload answer "why is there no db_engine?" on its own.
	if skipped := skippedSteps(answers, wiz); skipped != "" {
		out["skipped"] = skipped
	}
	return out
}

// skippedSteps lists, in spec order, the steps that produced no answer. A step
// that runs always sets its key, so absence is exactly the set a condition
// turned away — no instrumentation of the run loops required, which is what
// keeps the three of them identical.
func skippedSteps(answers map[string]any, wiz *spec.Wizard) string {
	if wiz == nil {
		return ""
	}
	var out []string
	for _, s := range wiz.Steps {
		if _, ok := answers[s.Key]; !ok {
			out = append(out, s.Key)
		}
	}
	return strings.Join(out, ",")
}

// MetaOf returns the meta block of a payload, and whether it had one. It is
// how a consumer reads back what produced the answers it is about to act on.
func MetaOf(answers map[string]any) (map[string]string, bool) {
	m, ok := answers[MetaKey].(map[string]string)
	return m, ok
}

// SortedKeys is the deterministic order a meta block is printed in.
func SortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
