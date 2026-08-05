package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func metaWizard(t *testing.T) *spec.Wizard {
	t.Helper()
	w, err := spec.Parse([]byte(`
steps:
  - type: confirm
    key: db
    title: Database?
    default: "no"
  - type: select
    key: engine
    title: Engine
    options: [postgres, mysql]
    default: postgres
    when: "db"
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return w
}

func TestMetaRecordsTheRun(t *testing.T) {
	wiz := metaWizard(t)
	answers := map[string]any{"db": false}
	start := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	Meta{
		Version: "v1.2.3",
		Source:  "w.yaml",
		Spec:    []byte("steps: []"),
		Mode:    ModeDefaults,
		TTY:     false,
		Started: start,
		Ended:   start.Add(1500 * time.Millisecond),
	}.Attach(answers, wiz)

	m, ok := MetaOf(answers)
	if !ok {
		t.Fatal("the meta block is missing or not a string map")
	}
	for key, want := range map[string]string{
		"tiny":       "v1.2.3",
		"source":     "w.yaml",
		"mode":       ModeDefaults,
		"tty":        "false",
		"answered":   "1",
		"started":    "2026-08-05T12:00:00Z",
		"elapsed_ms": "1500",
		// The step the condition turned away is named, so the payload can say
		// on its own why there is no engine.
		"skipped": "engine",
	} {
		if got := m[key]; got != want {
			t.Errorf("meta[%q] = %q, want %q", key, got, want)
		}
	}
	if len(m["spec_sha256"]) != 64 {
		t.Errorf("spec_sha256 = %q, want a hex sha256", m["spec_sha256"])
	}
}

// The same spec must hash the same and a changed spec must not, or the field
// is decoration rather than provenance.
func TestSpecHashTracksTheSpec(t *testing.T) {
	wiz := metaWizard(t)
	hash := func(spec string) string {
		answers := map[string]any{}
		Meta{Spec: []byte(spec)}.Attach(answers, wiz)
		m, _ := MetaOf(answers)
		return m["spec_sha256"]
	}
	if hash("a: 1") != hash("a: 1") {
		t.Error("the same spec hashed differently")
	}
	if hash("a: 1") == hash("a: 2") {
		t.Error("two different specs hashed the same")
	}
}

// Nothing skipped means no skipped field, rather than an empty one a consumer
// would have to special-case.
func TestNoSkippedStepsLeavesTheFieldOut(t *testing.T) {
	wiz := metaWizard(t)
	answers := map[string]any{"db": true, "engine": "postgres"}
	Meta{Version: "v0"}.Attach(answers, wiz)
	m, _ := MetaOf(answers)
	if _, present := m["skipped"]; present {
		t.Errorf("skipped = %q on a run that skipped nothing", m["skipped"])
	}
}

// _meta cannot collide with a step: a wizard key must start with a letter or a
// digit, so no spec can declare one.
func TestMetaKeyCannotBeAStepKey(t *testing.T) {
	_, err := spec.Parse([]byte(`
steps:
  - type: input
    key: ` + MetaKey + `
    title: T
`))
	if err == nil {
		t.Fatalf("a step called %q was accepted, so the meta block can be overwritten", MetaKey)
	}
}

// The block survives every output format, because provenance that only exists
// in JSON is provenance a shell caller cannot check.
func TestMetaSurvivesEveryFormat(t *testing.T) {
	wiz := metaWizard(t)
	answers := map[string]any{"db": false}
	Meta{Version: "v9", Source: "s", Mode: ModePreset}.Attach(answers, wiz)

	var json, yaml, env strings.Builder
	if err := PrintJSON(&json, answers); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}
	if err := PrintYAML(&yaml, answers); err != nil {
		t.Fatalf("PrintYAML error: %v", err)
	}
	if err := PrintEnv(&env, answers); err != nil {
		t.Fatalf("PrintEnv error: %v", err)
	}
	for name, out := range map[string]string{"json": json.String(), "yaml": yaml.String(), "env": env.String()} {
		if !strings.Contains(out, "v9") || !strings.Contains(out, ModePreset) {
			t.Errorf("the %s output lost the meta block:\n%s", name, out)
		}
	}
}
