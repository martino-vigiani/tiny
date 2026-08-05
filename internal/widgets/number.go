package widgets

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeNumber, numberWidget{})
}

type numberWidget struct{}

func (numberWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	PrintHeader(env, s.Prompt())
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, numberHint(s))
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			def, ok, err := numberDefault(s)
			if err != nil {
				return nil, err
			}
			if ok {
				return def, nil
			}
			if eof {
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return float64(0), nil
			}
			if s.Required {
				PrintError(env, "This field is required.")
				continue
			}
			return float64(0), nil
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err != nil {
			PrintError(env, "Please enter a number.")
			continue
		}
		if math.IsInf(value, 0) || math.IsNaN(value) {
			PrintError(env, "Please enter a finite number.")
			continue
		}
		if !inRange(s, value) {
			PrintError(env, rangeMessage(s))
			continue
		}
		return value, nil
	}
}

func numberHint(s spec.Step) string {
	var parts []string
	switch {
	case s.Min != nil && s.Max != nil:
		parts = append(parts, fmt.Sprintf("(min: %v, max: %v)", *s.Min, *s.Max))
	case s.Min != nil:
		parts = append(parts, fmt.Sprintf("(min: %v)", *s.Min))
	case s.Max != nil:
		parts = append(parts, fmt.Sprintf("(max: %v)", *s.Max))
	}
	if s.Increment > 0 {
		parts = append(parts, fmt.Sprintf("(step: %v)", s.Increment))
	}
	if len(parts) == 0 {
		return ""
	}
	// The caret follows the hint, so it needs a space of its own.
	return strings.Join(parts, " ") + " "
}

// numberDefault resolves the step default via spec.ToFloat and validates the
// range. Errors are propagated, never silently degrading to 0.
func numberDefault(s spec.Step) (float64, bool, error) {
	if s.Default == nil {
		return 0, false, nil
	}
	f, err := spec.ToFloat(s.Default)
	if err != nil {
		return 0, false, err
	}
	if !inRange(s, f) {
		return 0, false, fmt.Errorf("default %v is out of range", f)
	}
	return f, true, nil
}

func inRange(s spec.Step, v float64) bool {
	if s.Min != nil && v < *s.Min {
		return false
	}
	if s.Max != nil && v > *s.Max {
		return false
	}
	return true
}

func rangeMessage(s spec.Step) string {
	switch {
	case s.Min != nil && s.Max != nil:
		return fmt.Sprintf("Number must be between %v and %v.", *s.Min, *s.Max)
	case s.Min != nil:
		return fmt.Sprintf("Number must be >= %v.", *s.Min)
	case s.Max != nil:
		return fmt.Sprintf("Number must be <= %v.", *s.Max)
	default:
		return "Number out of range."
	}
}
