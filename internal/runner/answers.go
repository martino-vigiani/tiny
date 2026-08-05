package runner

// Answer pre-seeding: run the wizard headless, but with some answers supplied.
//
// `-defaults` is all or nothing. "In CI, take the defaults but set env to
// production" therefore meant rewriting the spec, which is the one thing a
// caller sharing a wizard with a human must not have to do. A preset supplies
// the answers it knows and lets every other step fall back to its default, so
// the same wizard file serves the person and the pipeline.
//
// A preset value is *the text a user would have typed at that prompt*, not a
// pre-coerced value: one form, understandable without reading this file, and
// the same form whether it arrived from -set or from a file. It is then put
// through the very coercion and validation the default path uses, so a preset
// can never smuggle a value into the answers that the wizard itself would have
// rejected.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// Preset holds answers supplied by the caller, keyed by step key, as the raw
// text a user would have typed at that step's prompt.
type Preset map[string]string

// ParsePresets parses `key=value` pairs, as collected by a repeatable -set
// flag. The value may itself contain "=", so only the first one splits.
func ParsePresets(pairs []string) (Preset, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	p := make(Preset, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("invalid -set %q: want key=value, e.g. -set env=production", pair)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid -set %q: the key is empty, want key=value", pair)
		}
		p[key] = value
	}
	return p, nil
}

// LoadPresets reads a file of answers: a mapping of step key to value.
//
// The file is YAML, which also makes it JSON — every JSON document is valid
// YAML, so a caller generating answers with a JSON encoder needs nothing else.
// (encoding/json is deliberately not linked into this binary.)
//
// Values may be any scalar, a list (a multiselect) or a mapping (a keyvalue);
// each is rendered into the same text form ParsePresets produces. Keeping one
// representation is the point: a value from a file and the same value from -set
// then go through one code path, so they cannot be coerced differently.
func LoadPresets(path string) (Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	p := make(Preset, len(raw))
	for key, value := range raw {
		text, err := presetText(value)
		if err != nil {
			return nil, fmt.Errorf("%s: key %q: %w", path, key, err)
		}
		p[key] = text
	}
	return p, nil
}

// presetText renders a loaded value as the text a user would have typed.
func presetText(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case uint64:
		return strconv.FormatUint(t, 10), nil
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			text, err := presetScalarText(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ","), nil
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			text, err := presetScalarText(t[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, k+"="+text)
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unsupported value of type %T", v)
	}
}

// presetScalarText renders one element of a list or mapping. Nesting is
// rejected: no step type has an answer shaped like that, so a nested value is a
// mistake, and rendering it would only hide the mistake until it surfaced as a
// baffling coercion error.
func presetScalarText(v any) (string, error) {
	switch v.(type) {
	case []any, map[string]any, map[any]any:
		return "", fmt.Errorf("nested lists and mappings are not valid answers")
	}
	return presetText(v)
}

// RunPresets runs the wizard headless, answering each step from the preset when
// it names that step and from the step's default otherwise. It never reads the
// input stream, so it runs anywhere -defaults runs.
//
// Conditions and interpolation are applied exactly as in Run and RunDefaults:
// the answers accumulate step by step, so a preset for a step whose `when:`
// does not hold is skipped and contributes no key.
func (r *Runner) RunPresets(wiz *spec.Wizard, p Preset) (map[string]any, error) {
	if err := checkPresetKeys(wiz, p); err != nil {
		return nil, err
	}
	answers := make(map[string]any, len(wiz.Steps))
	used := make(map[string]bool, len(p))
	for _, s := range wiz.Steps {
		if !s.Condition().Eval(answers) {
			continue
		}
		step := s.Interpolate(answers)
		raw, preseeded := p[s.Key]
		var value any
		var err error
		if preseeded {
			used[s.Key] = true
			value, err = presetForStep(step, raw)
			if err != nil {
				return nil, fmt.Errorf("preset for step %q: %w", s.Key, err)
			}
		} else {
			value, err = defaultForStep(step)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", s.Key, err)
			}
		}
		answers[s.Key] = value
	}
	r.reportUnusedPresets(p, used)
	return answers, nil
}

// checkPresetKeys rejects a preset naming a step the wizard does not have.
// Such a key can never be consumed by any run of this wizard, so it is a typo,
// and a typo that silently did nothing would be indistinguishable from the
// feature not working.
func checkPresetKeys(wiz *spec.Wizard, p Preset) error {
	if len(p) == 0 {
		return nil
	}
	known := make(map[string]bool, len(wiz.Steps))
	valid := make([]string, 0, len(wiz.Steps))
	for _, s := range wiz.Steps {
		known[s.Key] = true
		valid = append(valid, s.Key)
	}
	unknown := make([]string, 0)
	for key := range p {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("no step named %s in this wizard (steps: %s)",
		quoteList(unknown), strings.Join(valid, ", "))
}

// reportUnusedPresets notes the presets that named a real step which did not
// run this time.
//
// This is a note, not an error: a preset is written once and reused, and a
// wizard that branches has answers for branches a given run does not take —
// "set env=production and db_name=app" must keep working when a `when:` skips
// the database entirely. Erroring would push the caller back to maintaining one
// preset per branch, which is the problem this feature exists to remove. The
// note goes to the UI stream, so stdout still carries only the payload.
func (r *Runner) reportUnusedPresets(p Preset, used map[string]bool) {
	if len(p) == len(used) || r.env == nil || r.env.Out == nil {
		return
	}
	skipped := make([]string, 0, len(p)-len(used))
	for key := range p {
		if !used[key] {
			skipped = append(skipped, key)
		}
	}
	sort.Strings(skipped)
	widgets.PrintNotef(r.env, "Preset %s ignored: the step did not run.", quoteList(skipped))
}

func quoteList(keys []string) string {
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = strconv.Quote(k)
	}
	return strings.Join(quoted, ", ")
}

// presetForStep coerces a preset's raw text into the step's canonical answer.
//
// Every branch goes through the same helper the default path uses, so a preset
// is accepted exactly when the equivalent default would be. A value the step
// cannot accept is an error, never a silently coerced answer: the caller asked
// for a specific answer and has to be told it was not usable.
func presetForStep(s spec.Step, raw string) (any, error) {
	switch s.Type {
	case spec.TypeSelect, spec.TypeSearch, spec.TypeAutocomplete:
		canon, ok := spec.CanonicalOption(raw, s.Options)
		if ok {
			return canon, nil
		}
		// allow_custom means the typed text is itself a valid answer, so a
		// preset may carry text that is not an option — but only then.
		if s.AllowCustom && raw != "" {
			return raw, nil
		}
		return nil, fmt.Errorf("%q is not one of the options (%s)", raw, strings.Join(s.Options, ", "))

	case spec.TypeMultiselect:
		items, err := spec.SplitMultiDefault(raw)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if _, ok := spec.CanonicalOption(item, s.Options); !ok {
				return nil, fmt.Errorf("%q is not one of the options (%s)", item, strings.Join(s.Options, ", "))
			}
		}
		// Once every item is known to be an option, the default path resolves
		// the selection: ordering, canonical casing and any bound the step puts
		// on how many options it accepts are then applied in exactly one place.
		s.Default = items
		return defaultForStep(s)

	case spec.TypeConfirm:
		return spec.ParseConfirmDefault(raw)

	case spec.TypeInput, spec.TypePassword, spec.TypeEditor, spec.TypeFile:
		if raw == "" && s.Required {
			return nil, fmt.Errorf("the step is required, so an empty answer is not one")
		}
		if problem := s.Rule().Check(raw); problem != "" {
			return nil, fmt.Errorf("%q is rejected by the step's own validation: %s", raw, problem)
		}
		return raw, nil

	case spec.TypeNumber:
		f, err := spec.ToFloat(raw)
		if err != nil {
			return nil, err
		}
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("%q is not a finite number", raw)
		}
		if s.Min != nil && f < *s.Min {
			return nil, fmt.Errorf("%v is below min %v", f, *s.Min)
		}
		if s.Max != nil && f > *s.Max {
			return nil, fmt.Errorf("%v is above max %v", f, *s.Max)
		}
		return f, nil

	case spec.TypeDate:
		format := s.Format
		if format == "" {
			format = "date"
		}
		if err := spec.ValidateDateString(raw, format); err != nil {
			return nil, err
		}
		return raw, nil

	case spec.TypeKeyValue:
		pairs, err := splitPresetPairs(raw)
		if err != nil {
			return nil, err
		}
		return spec.ToStringMap(pairs)

	case spec.TypeTable:
		if !s.Selectable {
			return nil, fmt.Errorf("a table that is not selectable has no answer to preset")
		}
		idx, err := presetTableIndex(s, raw)
		if err != nil {
			return nil, err
		}
		// The row is now known; the default path turns it into the answer, so
		// whatever a chosen row answers with stays defined in one place.
		s.Default = idx
		return defaultForStep(s)

	default:
		return nil, fmt.Errorf("unsupported step type %q", s.Type)
	}
}

// splitPresetPairs parses `k=v,k=v` into a map, the same way the keyvalue
// widget parses one typed line: the first "=" splits, and both sides are
// trimmed. A comma is the separator, so a key or value containing one has to be
// given in a preset file — as a mapping — rather than on the command line.
func splitPresetPairs(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			return nil, fmt.Errorf("expected key=value pairs separated by commas, got %q", part)
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out, nil
}

// presetTableIndex resolves a preset for a selectable table.
//
// A preset is what the user would have typed, and at a table prompt that is the
// row number shown beside the row, counting from 1. A non-numeric preset falls
// back to the same first-column match a `default:` uses. Note the asymmetry
// with a spec `default:`, where a bare integer is the 0-based index an author
// writes: the two are different audiences, and matching what the prompt shows
// is what keeps a preset checkable against a screenshot.
func presetTableIndex(s spec.Step, raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if n, err := strconv.Atoi(raw); err == nil {
		if n < 1 || n > len(s.Rows) {
			return 0, fmt.Errorf("row %d is out of range, the table has rows 1-%d", n, len(s.Rows))
		}
		return n - 1, nil
	}
	s.Default = raw
	return tableIndex(s)
}
