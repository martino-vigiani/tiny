package spec

// Declarative validation for the text steps.
//
// The point is not that a wizard can reject input — `required:` already did a
// little of that. The point is that the SAME rule is checked in two places: at
// the prompt, and against the step's own default when the spec is loaded. A
// generated spec whose default violates its own pattern therefore fails on
// `tiny -defaults`, in the dry run, instead of in front of a person.

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// TextRule is the compiled form of a step's validation fields.
type TextRule struct {
	pattern   *regexp.Regexp
	minLength int
	maxLength int
	message   string
}

// compileRule builds the step's rule, or nil when it has no validation fields.
func compileRule(s Step) (*TextRule, error) {
	if s.Pattern == "" && s.MinLength == 0 && s.MaxLength == 0 {
		return nil, nil
	}
	r := &TextRule{
		minLength: s.MinLength,
		maxLength: s.MaxLength,
		message:   s.Message,
	}
	if s.Pattern != "" {
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			// Go uses RE2, which has no lookahead or backreferences. That is
			// the usual reason a pattern copied from elsewhere fails, so say so
			// rather than passing the raw error through.
			return nil, fmt.Errorf("invalid pattern %q: %v "+
				"(Go regexps are RE2: no lookahead (?=, no backreferences \\1)", s.Pattern, err)
		}
		r.pattern = re
	}
	return r, nil
}

// Check reports why value is unacceptable, or "" when it passes.
//
// A step's own `message:` replaces every generated message: an author who
// writes one has said how they want the rule explained.
func (r *TextRule) Check(value string) string {
	if r == nil {
		return ""
	}
	n := utf8.RuneCountInString(value)
	switch {
	case r.minLength > 0 && n < r.minLength:
		return r.explain(fmt.Sprintf("Must be at least %d characters.", r.minLength))
	case r.maxLength > 0 && n > r.maxLength:
		return r.explain(fmt.Sprintf("Must be at most %d characters.", r.maxLength))
	case r.pattern != nil && !r.pattern.MatchString(value):
		return r.explain(fmt.Sprintf("Must match %s.", r.pattern.String()))
	default:
		return ""
	}
}

func (r *TextRule) explain(generated string) string {
	if r.message != "" {
		return r.message
	}
	return generated
}

// Rule returns the step's compiled validation rule, or nil. It is populated by
// Validate, so it is only meaningful on a wizard that has been parsed.
func (s Step) Rule() *TextRule { return s.rule }

// validateText checks the step's validation fields and its default against
// them. An empty default is exempt: it means "no default", not "the empty
// string is acceptable".
func (s *Step) validateText() error {
	if s.MinLength < 0 || s.MaxLength < 0 {
		return fmt.Errorf("step %q: lengths cannot be negative", s.Key)
	}
	if s.MinLength > 0 && s.MaxLength > 0 && s.MinLength > s.MaxLength {
		return fmt.Errorf("step %q: min_length (%d) cannot exceed max_length (%d)", s.Key, s.MinLength, s.MaxLength)
	}
	rule, err := compileRule(*s)
	if err != nil {
		return fmt.Errorf("step %q: %v", s.Key, err)
	}
	s.rule = rule
	if def, ok := s.Default.(string); ok && def != "" {
		if problem := rule.Check(def); problem != "" {
			return fmt.Errorf("step %q: default %q fails its own validation: %s", s.Key, def, problem)
		}
	}
	return nil
}
