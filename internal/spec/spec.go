// Package spec defines the declarative wizard schema (YAML) and its validation.
package spec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// StepType enumerates the supported wizard step kinds.
type StepType string

const (
	TypeSelect       StepType = "select"
	TypeMultiselect  StepType = "multiselect"
	TypeConfirm      StepType = "confirm"
	TypeInput        StepType = "input"
	TypePassword     StepType = "password"
	TypeSearch       StepType = "search"
	TypeNumber       StepType = "number"
	TypeDate         StepType = "date"
	TypeKeyValue     StepType = "keyvalue"
	TypeEditor       StepType = "editor"
	TypeAutocomplete StepType = "autocomplete"
	TypeTable        StepType = "table"
)

// Wizard is the root of the tiny spec: a sequence of steps to run in order,
// plus the optional theme the author wants them drawn with.
//
// Theme is only a suggestion: the -theme flag and $TINY_THEME are layered on
// top of it, so the person running the wizard always has the last word on how
// it looks.
type Wizard struct {
	Theme *theme.Theme `yaml:"theme,omitempty"`
	Steps []Step       `yaml:"steps"`
}

// Step is a single interactive question in the wizard.
//
// Default is an arbitrary value; the meaning depends on the step type
// (string for select/input/password/editor/date, scalar for confirm/number,
// list or comma-separated string for multiselect, map for keyvalue).
type Step struct {
	Type        StepType   `yaml:"type"`
	Key         string     `yaml:"key"`
	Title       string     `yaml:"title"`
	PromptText  string     `yaml:"prompt,omitempty"`
	Options     []string   `yaml:"options,omitempty"`
	Default     any        `yaml:"default,omitempty"`
	Required    bool       `yaml:"required,omitempty"`
	Placeholder string     `yaml:"placeholder,omitempty"`
	Min         *float64   `yaml:"min,omitempty"`
	Max         *float64   `yaml:"max,omitempty"`
	Increment   float64    `yaml:"increment,omitempty"`
	Headers     []string   `yaml:"headers,omitempty"`
	Rows        [][]string `yaml:"rows,omitempty"`
	Selectable  bool       `yaml:"selectable,omitempty"`
	EditorCmd   string     `yaml:"editor,omitempty"`
	Format      string     `yaml:"format,omitempty"`
	Delimiter   string     `yaml:"delimiter,omitempty"`

	// When is an optional condition on an earlier answer; the step runs only
	// if it holds. See condition.go for the expression language.
	When string `yaml:"when,omitempty"`

	// cond is When parsed, filled by Validate so every run reuses it.
	cond *Condition
}

// Condition returns the step's parsed `when:` expression, or nil when the step
// is unconditional. It is only populated after validation.
func (s Step) Condition() *Condition { return s.cond }

// Parse decodes a wizard spec from YAML bytes and validates it.
func Parse(data []byte) (*Wizard, error) {
	var w Wizard
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("parsing wizard yaml: %w", err)
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}

// Validate checks that the spec is well-formed before any prompt is shown.
func (w *Wizard) Validate() error {
	if len(w.Steps) == 0 {
		return fmt.Errorf("wizard must contain at least one step")
	}
	if err := w.Theme.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range w.Steps {
		s := &w.Steps[i]
		if s.Key == "" {
			return fmt.Errorf("step %d: missing key", i+1)
		}
		if seen[s.Key] {
			return fmt.Errorf("step %d: duplicate key %q", i+1, s.Key)
		}
		if err := s.parseCondition(seen); err != nil {
			return err
		}
		seen[s.Key] = true
		if err := s.validate(); err != nil {
			return err
		}
	}
	return nil
}

// parseCondition parses the step's `when:` expression and checks that it refers
// to a key answered before this step. A forward reference is always false, so
// rejecting it turns a step that would silently never run into a load error.
func (s *Step) parseCondition(earlier map[string]bool) error {
	if s.When == "" {
		s.cond = nil
		return nil
	}
	cond, err := ParseCondition(s.When)
	if err != nil {
		return fmt.Errorf("step %q: %v", s.Key, err)
	}
	if !earlier[cond.Key] {
		return fmt.Errorf("step %q: condition %q refers to %q, which is not answered before this step",
			s.Key, s.When, cond.Key)
	}
	s.cond = cond
	return nil
}

var keyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func (s Step) validate() error {
	if !keyRe.MatchString(s.Key) {
		return fmt.Errorf("step %q: invalid key %q (must match ^[A-Za-z0-9][A-Za-z0-9_.-]*$)", s.Key, s.Key)
	}
	switch s.Type {
	case TypeSelect, TypeMultiselect, TypeSearch, TypeAutocomplete:
		if len(s.Options) == 0 {
			return fmt.Errorf("step %q: %s requires at least one option", s.Key, s.Type)
		}
		if err := validateNoDuplicateOptions(s); err != nil {
			return err
		}
		switch s.Type {
		case TypeSelect, TypeSearch, TypeAutocomplete:
			if s.Default == nil {
				break
			}
			str, ok := s.Default.(string)
			if !ok {
				return fmt.Errorf("step %q: default for %s must be a string", s.Key, s.Type)
			}
			if _, ok := CanonicalOption(str, s.Options); !ok {
				return fmt.Errorf("step %q: default %q is not one of the options", s.Key, str)
			}
		case TypeMultiselect:
			items, err := SplitMultiDefault(s.Default)
			if err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
			lower := make(map[string]bool, len(s.Options))
			for _, o := range s.Options {
				lower[strings.ToLower(o)] = true
			}
			for _, item := range items {
				if !lower[strings.ToLower(item)] {
					return fmt.Errorf("step %q: default %q is not one of the options", s.Key, item)
				}
			}
		}
	case TypeNumber:
		if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
			return fmt.Errorf("step %q: min (%v) cannot be greater than max (%v)", s.Key, *s.Min, *s.Max)
		}
		if s.Default != nil {
			f, err := ToFloat(s.Default)
			if err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
			if s.Min != nil && f < *s.Min {
				return fmt.Errorf("step %q: default %v is below min %v", s.Key, f, *s.Min)
			}
			if s.Max != nil && f > *s.Max {
				return fmt.Errorf("step %q: default %v is above max %v", s.Key, f, *s.Max)
			}
		}
	case TypeConfirm:
		if s.Default != nil {
			if _, err := ParseConfirmDefault(s.Default); err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
		}
	case TypeDate:
		if s.Format != "" && s.Format != "date" && s.Format != "datetime" {
			return fmt.Errorf("step %q: format must be \"date\" or \"datetime\", got %q", s.Key, s.Format)
		}
		if s.Default != nil {
			str, ok := s.Default.(string)
			if !ok {
				return fmt.Errorf("step %q: default for date must be a string", s.Key)
			}
			format := s.Format
			if format == "" {
				format = "date"
			}
			if err := ValidateDateString(str, format); err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
		}
	case TypeKeyValue:
		if s.Default != nil {
			if _, err := ToStringMap(s.Default); err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
		}
	case TypeTable:
		if len(s.Headers) == 0 && len(s.Rows) == 0 {
			return fmt.Errorf("step %q: table requires non-empty headers or rows", s.Key)
		}
		if s.Selectable && len(s.Rows) == 0 {
			return fmt.Errorf("step %q: selectable table requires at least one row", s.Key)
		}
		if s.Default != nil {
			if !s.Selectable {
				return fmt.Errorf("step %q: default only valid when selectable", s.Key)
			}
			if err := validateTableDefault(s); err != nil {
				return fmt.Errorf("step %q: %v", s.Key, err)
			}
		}
	case TypeInput, TypePassword, TypeEditor:
		// no extra constraints
	default:
		return fmt.Errorf("step %q: unsupported type %q", s.Key, s.Type)
	}
	return nil
}

// Prompt returns the prompt text for the step, falling back to its title.
//
// The YAML field is "prompt" but is stored as PromptText because Go forbids a
// field and a method with the same name on one type.
func (s Step) Prompt() string {
	if s.PromptText != "" {
		return s.PromptText
	}
	return s.Title
}

// validateNoDuplicateOptions rejects options that differ only by case.
func validateNoDuplicateOptions(s Step) error {
	seen := make(map[string]bool, len(s.Options))
	for _, opt := range s.Options {
		lo := strings.ToLower(opt)
		if seen[lo] {
			return fmt.Errorf("step %q: duplicate option %q (case-insensitive)", s.Key, opt)
		}
		seen[lo] = true
	}
	return nil
}

// validateTableDefault checks that a selectable-table default is either a row
// index inside the table or a string matching the first column.
func validateTableDefault(s Step) error {
	switch d := s.Default.(type) {
	case int:
		if d >= 0 && d < len(s.Rows) {
			return nil
		}
		return fmt.Errorf("default row index %d out of range", d)
	case int64:
		if d >= 0 && int(d) < len(s.Rows) {
			return nil
		}
		return fmt.Errorf("default row index %d out of range", d)
	case float64:
		if d >= 0 && int(d) < len(s.Rows) {
			return nil
		}
		return fmt.Errorf("default row index %d out of range", int(d))
	case string:
		for _, row := range s.Rows {
			if len(row) > 0 && strings.EqualFold(row[0], d) {
				return nil
			}
		}
		return fmt.Errorf("default row %q not found", d)
	default:
		return fmt.Errorf("default for selectable table must be a string or a row index")
	}
}

// SplitMultiDefault normalises a multiselect default (string with commas or a
// list of strings) into a slice of option values.
func SplitMultiDefault(def any) ([]string, error) {
	switch v := def.(type) {
	case nil:
		return nil, nil
	case string:
		var out []string
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("multiselect default must contain only strings")
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("multiselect default must be a string or a list of strings")
	}
}

// ParseConfirmDefault maps yes/y/true (and their bool forms) to true and
// no/n/false to false. Anything else is an error.
func ParseConfirmDefault(def any) (bool, error) {
	switch v := def.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "yes", "y", "true":
			return true, nil
		case "no", "n", "false":
			return false, nil
		default:
			return false, fmt.Errorf("invalid confirm default %q (want yes/y/true or no/n/false)", v)
		}
	default:
		return false, fmt.Errorf("invalid confirm default %v", def)
	}
}

// ToFloat converts a YAML numeric scalar (int/float or numeric string) to
// float64.
func ToFloat(def any) (float64, error) {
	switch v := def.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("default %q is not a number", v)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("default for number must be a number, got %T", def)
	}
}

// ToStringMap converts a YAML mapping default into a map[string]string.
// Scalar values are rendered with %v; nested maps/slices are rejected.
func ToStringMap(def any) (map[string]string, error) {
	out := map[string]string{}
	switch v := def.(type) {
	case map[string]any:
		for k, val := range v {
			if isComposite(val) {
				return nil, fmt.Errorf("keyvalue default value for %q must be a scalar", k)
			}
			out[k] = fmt.Sprintf("%v", val)
		}
	case map[string]string:
		for k, val := range v {
			out[k] = val
		}
	case map[any]any:
		for k, val := range v {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("keyvalue default keys must be strings")
			}
			if isComposite(val) {
				return nil, fmt.Errorf("keyvalue default value for %q must be a scalar", ks)
			}
			out[ks] = fmt.Sprintf("%v", val)
		}
	case nil:
		// empty map
	default:
		return nil, fmt.Errorf("default for keyvalue must be a mapping")
	}
	return out, nil
}

func isComposite(v any) bool {
	switch v.(type) {
	case []any, []string, map[string]any, map[any]any:
		return true
	default:
		return false
	}
}

// ValidateDateString checks that def matches the layout implied by format
// ("date" -> 2006-01-02, "datetime" -> 2006-01-02 15:04:05). An empty format
// defaults to "date".
func ValidateDateString(def, format string) error {
	if format == "" {
		format = "date"
	}
	layout := "2006-01-02"
	if format == "datetime" {
		layout = "2006-01-02 15:04:05"
	}
	if _, err := time.Parse(layout, def); err != nil {
		return fmt.Errorf("default %q does not match format %q", def, format)
	}
	return nil
}

// CanonicalOption returns the canonical casing of value among options,
// matching case-insensitively, and whether it was found.
//
// This is the most-called helper in the package, so it scans with EqualFold
// rather than lowering both sides: the old form allocated one string per
// option on every call.
func CanonicalOption(value string, options []string) (string, bool) {
	for _, opt := range options {
		if strings.EqualFold(opt, value) {
			return opt, true
		}
	}
	return "", false
}

// MarkOptions reports, per option, whether selected contains it. It is the
// natural shape for a multiselect: the caller can render the mask directly and
// turn it into an answer with OptionsFor, without building intermediate maps.
//
// Matching is case-insensitive, and an item that is not an option is ignored.
func MarkOptions(selected, options []string) []bool {
	if len(selected) == 0 || len(options) == 0 {
		return nil
	}
	marks := make([]bool, len(options))
	for _, item := range selected {
		for i, opt := range options {
			if strings.EqualFold(opt, item) {
				marks[i] = true
				break
			}
		}
	}
	return marks
}

// OptionsFor returns the marked options, in the order they appear in options
// and with their canonical casing.
func OptionsFor(marks []bool, options []string) []string {
	out := make([]string, 0, len(options))
	for i, opt := range options {
		if i < len(marks) && marks[i] {
			out = append(out, opt)
		}
	}
	return out
}

// OrderByOptions returns the items from selected that appear in options,
// deduplicated and ordered by their position in options. Matching is
// case-insensitive; the canonical option casing is used in the result.
func OrderByOptions(selected, options []string) []string {
	return OptionsFor(MarkOptions(selected, options), options)
}
