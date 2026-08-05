package spec

// Interpolation: a step can refer to the answers already given.
//
//	- type: input
//	  key: module
//	  title: "Go module path for ${project_name}"
//	  default: "github.com/me/${project_name}"
//
// The grammar is closed and will not grow: `${key}`, nothing else. No filters,
// no expressions, no defaults-within-defaults. It is the same argument that
// keeps `when:` to six forms — a wizard describes an interaction, and anything
// that needs computing belongs in the program that consumes the answers, where
// it can be tested.
//
// Interpolation reads only earlier answers, so it resolves identically in an
// interactive run and under -defaults.

import (
	"strconv"
	"strings"
)

// Interpolate returns a copy of the step with every `${key}` in its
// user-visible text replaced by the corresponding answer.
//
// Strings without a '$' are returned untouched and nothing is allocated, which
// is the overwhelmingly common case: a wizard that uses no interpolation pays
// nothing for the feature existing.
func (s Step) Interpolate(answers map[string]any) Step {
	if len(answers) == 0 {
		return s
	}
	s.Title = expand(s.Title, answers)
	s.PromptText = expand(s.PromptText, answers)
	s.Placeholder = expand(s.Placeholder, answers)
	s.Message = expand(s.Message, answers)
	if def, ok := s.Default.(string); ok {
		s.Default = expand(def, answers)
	}
	s.Options = expandAll(s.Options, answers)
	// Labels and descriptions are read by the same person as the title, so a
	// wizard that interpolates one and not the other would be showing an
	// earlier answer in half its text.
	s.Labels = expandAll(s.Labels, answers)
	s.Descriptions = expandAll(s.Descriptions, answers)
	return s
}

// expandAll interpolates a slice, sharing the original when nothing changes.
func expandAll(values []string, answers map[string]any) []string {
	if !anyNeedsExpanding(values) {
		return values
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = expand(v, answers)
	}
	return out
}

func anyNeedsExpanding(values []string) bool {
	for _, v := range values {
		if strings.IndexByte(v, '$') >= 0 {
			return true
		}
	}
	return false
}

// expand replaces every `${key}` with the answer for key, rendered as text.
//
// A reference to a key with no answer — a later step, or one that was skipped —
// is left verbatim rather than becoming an empty string: a visible `${db_name}`
// in a prompt is a bug report, a silent blank is a mystery.
func expand(text string, answers map[string]any) string {
	if strings.IndexByte(text, '$') < 0 {
		return text
	}
	var b strings.Builder
	for {
		i := strings.Index(text, "${")
		if i < 0 {
			break
		}
		end := strings.IndexByte(text[i:], '}')
		if end < 0 {
			break
		}
		end += i
		key := text[i+2 : end]
		value, ok := answers[key]
		if !ok {
			// Keep the reference and carry on past it.
			b.WriteString(text[:end+1])
			text = text[end+1:]
			continue
		}
		b.WriteString(text[:i])
		b.WriteString(AnswerText(value))
		text = text[end+1:]
	}
	if b.Len() == 0 {
		return text
	}
	b.WriteString(text)
	return b.String()
}

// AnswerText renders an answer the way a person would write it in a sentence.
// It is the same rendering the env output uses for scalars, so a value looks
// the same wherever it surfaces.
func AnswerText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case []string:
		return strings.Join(t, ", ")
	case float64:
		return formatFloat(t)
	case int:
		return formatInt(t)
	default:
		return ""
	}
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
func formatInt(n int) string       { return strconv.Itoa(n) }
