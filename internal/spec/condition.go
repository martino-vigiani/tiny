package spec

// Conditional steps: a step carrying `when:` runs only if its condition holds
// against the answers collected before it.
//
// The condition language is deliberately tiny — six forms, no parser, no
// precedence, no arithmetic. It exists so a wizard can branch, not so a wizard
// can compute. Anything more expressive belongs in the program that consumes
// the answers, where it can be tested.
//
// Because a condition reads only earlier answers, it evaluates identically in
// interactive and headless runs: the same spec still produces the same answers,
// which is the invariant the whole tool rests on.

import (
	"fmt"
	"strconv"
	"strings"
)

// Condition is a parsed `when:` expression.
type Condition struct {
	// Raw is the expression as written, kept for error messages.
	Raw string

	Key      string
	Operator ConditionOp
	// Value is the right-hand side for the comparison operators, and Values
	// holds the alternatives for `in`.
	Value  string
	Values []string
}

// ConditionOp enumerates the supported comparisons.
type ConditionOp string

const (
	// OpTruthy is a bare key: true for a true bool, a non-empty string or
	// collection, and a non-zero number.
	OpTruthy ConditionOp = "truthy"
	// OpFalsy is `!key`.
	OpFalsy ConditionOp = "falsy"
	// OpEqual is `key == value`, compared case-insensitively as text.
	OpEqual ConditionOp = "=="
	// OpNotEqual is `key != value`.
	OpNotEqual ConditionOp = "!="
	// OpIn is `key in a|b|c`.
	OpIn ConditionOp = "in"
	// OpContains is `key contains value`, for the collection answers
	// (multiselect, keyvalue).
	OpContains ConditionOp = "contains"
)

// ParseCondition parses a `when:` expression.
func ParseCondition(expr string) (*Condition, error) {
	raw := strings.TrimSpace(expr)
	if raw == "" {
		return nil, fmt.Errorf("empty condition")
	}
	c := &Condition{Raw: raw}

	if rest, found := strings.CutPrefix(raw, "!"); found {
		key := strings.TrimSpace(rest)
		if !isConditionKey(key) {
			return nil, fmt.Errorf("invalid condition %q: %q is not a key", raw, key)
		}
		c.Key, c.Operator = key, OpFalsy
		return c, nil
	}

	for _, op := range []ConditionOp{OpEqual, OpNotEqual} {
		if lhs, rhs, found := strings.Cut(raw, string(op)); found {
			key := strings.TrimSpace(lhs)
			if !isConditionKey(key) {
				return nil, fmt.Errorf("invalid condition %q: %q is not a key", raw, key)
			}
			c.Key, c.Operator, c.Value = key, op, unquote(strings.TrimSpace(rhs))
			return c, nil
		}
	}

	// Word operators are matched on whitespace boundaries so a key or value
	// containing "in" is not mistaken for the operator.
	if key, rhs, ok := cutWord(raw, "in"); ok {
		if !isConditionKey(key) {
			return nil, fmt.Errorf("invalid condition %q: %q is not a key", raw, key)
		}
		for _, part := range strings.Split(rhs, "|") {
			if p := unquote(strings.TrimSpace(part)); p != "" {
				c.Values = append(c.Values, p)
			}
		}
		if len(c.Values) == 0 {
			return nil, fmt.Errorf("invalid condition %q: `in` needs at least one value, e.g. `key in a|b`", raw)
		}
		c.Key, c.Operator = key, OpIn
		return c, nil
	}
	if key, rhs, ok := cutWord(raw, "contains"); ok {
		if !isConditionKey(key) {
			return nil, fmt.Errorf("invalid condition %q: %q is not a key", raw, key)
		}
		value := unquote(rhs)
		if value == "" {
			return nil, fmt.Errorf("invalid condition %q: `contains` needs a value", raw)
		}
		c.Key, c.Operator, c.Value = key, OpContains, value
		return c, nil
	}

	if !isConditionKey(raw) {
		return nil, fmt.Errorf("invalid condition %q: expected `key`, `!key`, `key == value`, "+
			"`key != value`, `key in a|b` or `key contains value`", raw)
	}
	c.Key, c.Operator = raw, OpTruthy
	return c, nil
}

// cutWord splits raw around a whitespace-delimited operator word.
func cutWord(raw, word string) (lhs, rhs string, ok bool) {
	needle := " " + word + " "
	i := strings.Index(raw, needle)
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(raw[:i]), strings.TrimSpace(raw[i+len(needle):]), true
}

// unquote strips one layer of matching quotes, so a value with spaces can be
// written `key == "two words"`.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func isConditionKey(k string) bool {
	return k != "" && keyRe.MatchString(k)
}

// Eval reports whether the condition holds for the answers collected so far.
//
// A condition naming a key that has not been answered — a later step, or a step
// that was itself skipped — is false. That makes skipping transitive: a step
// depending on a skipped step is skipped too, rather than silently reappearing.
func (c *Condition) Eval(answers map[string]any) bool {
	if c == nil {
		return true
	}
	value, answered := answers[c.Key]
	if !answered {
		return false
	}
	switch c.Operator {
	case OpTruthy:
		return truthy(value)
	case OpFalsy:
		return !truthy(value)
	case OpEqual:
		return matchesText(value, c.Value)
	case OpNotEqual:
		return !matchesText(value, c.Value)
	case OpIn:
		for _, candidate := range c.Values {
			if matchesText(value, candidate) {
				return true
			}
		}
		return false
	case OpContains:
		return contains(value, c.Value)
	default:
		return false
	}
}

// truthy mirrors what a person would mean by "they answered this": a true bool,
// a non-empty string, a non-empty collection, a non-zero number.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case []string:
		return len(t) > 0
	case map[string]string:
		return len(t) > 0
	default:
		return true
	}
}

// matchesText compares an answer to a written value. Scalars compare as text,
// case-insensitively; a collection matches when any of its elements does, which
// makes `multi == iOS` mean "iOS is among the chosen options".
func matchesText(v any, want string) bool {
	switch t := v.(type) {
	case nil:
		return want == ""
	case bool:
		b, err := strconv.ParseBool(strings.ToLower(want))
		if err != nil {
			// Accept the yes/no spelling the confirm step uses.
			switch strings.ToLower(want) {
			case "yes", "y":
				return t
			case "no", "n":
				return !t
			}
			return false
		}
		return t == b
	case string:
		return strings.EqualFold(t, want)
	case float64:
		f, err := strconv.ParseFloat(strings.TrimSpace(want), 64)
		return err == nil && t == f
	case int:
		n, err := strconv.Atoi(strings.TrimSpace(want))
		return err == nil && t == n
	case []string:
		for _, item := range t {
			if strings.EqualFold(item, want) {
				return true
			}
		}
		return false
	case map[string]string:
		_, ok := t[want]
		return ok
	default:
		return false
	}
}

// contains is `in` read the other way round: does this collection (or string)
// hold the given value?
func contains(v any, want string) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(strings.ToLower(t), strings.ToLower(want))
	case []string:
		for _, item := range t {
			if strings.EqualFold(item, want) {
				return true
			}
		}
		return false
	case map[string]string:
		_, ok := t[want]
		return ok
	default:
		return matchesText(v, want)
	}
}
