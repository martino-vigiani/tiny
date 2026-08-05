package runner

import (
	"fmt"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// RunDefaults resolves every step to its default value without reading stdin,
// so the wizard can run headless (e.g. in CI). It never touches the input
// stream; missing defaults on required steps are errors.
func (r *Runner) RunDefaults(wiz *spec.Wizard) (map[string]any, error) {
	answers := make(map[string]any, len(wiz.Steps))
	for _, s := range wiz.Steps {
		// Conditions read only earlier answers, so they resolve the same way
		// here as in an interactive run: the two paths skip the same steps.
		if !s.Condition().Eval(answers) {
			continue
		}
		value, err := defaultForStep(s)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", s.Key, err)
		}
		answers[s.Key] = value
	}
	return answers, nil
}

func defaultForStep(s spec.Step) (any, error) {
	switch s.Type {
	case spec.TypeSelect, spec.TypeSearch, spec.TypeAutocomplete:
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return "", nil
		}
		str, ok := s.Default.(string)
		if !ok {
			return nil, fmt.Errorf("default for %s must be a string", s.Type)
		}
		if canon, ok := spec.CanonicalOption(str, s.Options); ok {
			return canon, nil
		}
		return str, nil

	case spec.TypeMultiselect:
		items, err := spec.SplitMultiDefault(s.Default)
		if err != nil {
			return nil, err
		}
		return spec.OrderByOptions(items, s.Options), nil

	case spec.TypeConfirm:
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return false, nil
		}
		return spec.ParseConfirmDefault(s.Default)

	case spec.TypeInput, spec.TypePassword, spec.TypeEditor:
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return "", nil
		}
		str, ok := s.Default.(string)
		if !ok {
			return nil, fmt.Errorf("default for %s must be a string", s.Type)
		}
		return str, nil

	case spec.TypeNumber:
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return float64(0), nil
		}
		f, err := spec.ToFloat(s.Default)
		if err != nil {
			return nil, err
		}
		if s.Min != nil && f < *s.Min {
			return nil, fmt.Errorf("default %v is below min %v", f, *s.Min)
		}
		if s.Max != nil && f > *s.Max {
			return nil, fmt.Errorf("default %v is above max %v", f, *s.Max)
		}
		return f, nil

	case spec.TypeDate:
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return "", nil
		}
		str, ok := s.Default.(string)
		if !ok {
			return nil, fmt.Errorf("default for date must be a string")
		}
		format := s.Format
		if format == "" {
			format = "date"
		}
		if err := spec.ValidateDateString(str, format); err != nil {
			return nil, err
		}
		return str, nil

	case spec.TypeKeyValue:
		if s.Default == nil {
			return map[string]string{}, nil
		}
		return spec.ToStringMap(s.Default)

	case spec.TypeTable:
		if !s.Selectable {
			return nil, nil
		}
		if s.Default == nil {
			if s.Required {
				return nil, fmt.Errorf("required step has no default")
			}
			return nil, nil
		}
		idx, err := tableIndex(s)
		if err != nil {
			return nil, err
		}
		return idx, nil

	default:
		return nil, fmt.Errorf("unsupported step type %q", s.Type)
	}
}

// tableIndex resolves a selectable-table default to a 0-based row index: an
// integer is range-checked against the rows, a string is matched
// case-insensitively against the first column.
func tableIndex(s spec.Step) (int, error) {
	switch d := s.Default.(type) {
	case int:
		if d >= 0 && d < len(s.Rows) {
			return d, nil
		}
		return 0, fmt.Errorf("default row index %d out of range", d)
	case int64:
		if d >= 0 && int(d) < len(s.Rows) {
			return int(d), nil
		}
		return 0, fmt.Errorf("default row index %d out of range", d)
	case float64:
		if d >= 0 && int(d) < len(s.Rows) {
			return int(d), nil
		}
		return 0, fmt.Errorf("default row index %d out of range", int(d))
	case string:
		for i, row := range s.Rows {
			if len(row) > 0 && strings.EqualFold(row[0], d) {
				return i, nil
			}
		}
		return 0, fmt.Errorf("default row %q not found", d)
	default:
		return 0, fmt.Errorf("default for selectable table must be a string or a row index")
	}
}
