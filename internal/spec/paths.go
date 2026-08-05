package spec

// Paths: every shape a branching wizard can take.
//
// A wizard with conditions does not ask one set of questions, it asks one of
// several, and which ones is decided by answers nobody has given yet. That is
// hard to review by reading, and it is the thing that goes wrong: a branch
// nobody reaches, a required step that a plausible answer skips, a payload key
// a consumer assumes is always present.
//
// Paths enumerates the distinct sets of steps the wizard can run, without
// running it and without asking anything. Each comes with the answers that
// reach it, so a reviewer — or a test — can check the list rather than
// imagine it.
//
// # What it explores
//
// Only the keys some condition actually reads are branched on, and each over a
// small, declared domain:
//
//   - confirm: yes and no;
//   - select, search, autocomplete: every option;
//   - multiselect: nothing, each single option, and all of them — enough to
//     decide any `contains`, without the 2^n an exhaustive walk would cost;
//   - anything else: its default, plus every literal a condition compares it
//     against, since those are the only values that can change an outcome.
//
// This is a bounded exploration, not a proof. It cannot invent the one input
// string that satisfies a `==` nothing in the spec mentions. What it does
// guarantee is that every path it prints is reachable by the answers printed
// with it.

import (
	"sort"
	"strconv"
	"strings"
)

// Path is one reachable run of the wizard.
type Path struct {
	// Steps are the keys asked, in order.
	Steps []string
	// Answers are the branch answers that reach it, as text.
	Answers map[string]string
}

// DefaultPathLimit bounds how many distinct paths are enumerated. A wizard
// with more shapes than this is a wizard no reviewer was going to read anyway,
// and the caller is told the list was cut.
const DefaultPathLimit = 500

// Paths returns the distinct step sequences the wizard can produce, and
// whether the enumeration hit the limit.
func Paths(w *Wizard, limit int) ([]Path, bool) {
	if limit <= 0 {
		limit = DefaultPathLimit
	}
	branching := branchKeys(w)
	e := &pathEnum{
		wiz:     w,
		branch:  branching,
		limit:   limit,
		seen:    map[string]bool{},
		answers: map[string]any{},
	}
	e.walk(0, nil)
	return e.out, e.truncated
}

// branchKeys maps each key a condition reads to the candidate values worth
// trying for it. A key nothing conditions on is not branched: it cannot change
// which questions get asked, so exploring it would only multiply identical
// paths.
func branchKeys(w *Wizard) map[string][]any {
	// Which keys are read, and with what literals.
	literals := map[string][]string{}
	for _, s := range w.Steps {
		c := s.Condition()
		if c == nil {
			continue
		}
		if _, ok := literals[c.Key]; !ok {
			literals[c.Key] = nil
		}
		switch c.Operator {
		case OpEqual, OpNotEqual, OpContains:
			literals[c.Key] = append(literals[c.Key], c.Value)
		case OpIn:
			literals[c.Key] = append(literals[c.Key], c.Values...)
		}
	}
	out := make(map[string][]any, len(literals))
	for _, s := range w.Steps {
		lits, read := literals[s.Key]
		if !read {
			continue
		}
		out[s.Key] = candidates(s, lits)
	}
	return out
}

// candidates is the domain explored for one step.
func candidates(s Step, literals []string) []any {
	switch s.Type {
	case TypeConfirm:
		return []any{true, false}

	case TypeSelect, TypeSearch, TypeAutocomplete:
		out := make([]any, 0, len(s.Options))
		for _, o := range s.Options {
			out = append(out, o)
		}
		return out

	case TypeMultiselect:
		// Nothing, each option alone, and everything. Any `contains` is
		// decided by this, and the empty and full sets are where the bounds
		// (min_select, max_select) show up.
		out := make([]any, 0, len(s.Options)+2)
		out = append(out, []string{})
		for _, o := range s.Options {
			out = append(out, []string{o})
		}
		if len(s.Options) > 1 {
			all := make([]string, len(s.Options))
			copy(all, s.Options)
			out = append(out, all)
		}
		return out

	case TypeNumber:
		out := []any{}
		if s.Default != nil {
			if f, err := ToFloat(s.Default); err == nil {
				out = append(out, f)
			}
		}
		for _, l := range literals {
			if f, err := strconv.ParseFloat(strings.TrimSpace(l), 64); err == nil {
				out = appendUnique(out, f)
			}
		}
		if len(out) == 0 {
			out = append(out, float64(0))
		}
		return out

	default:
		// A free-text step can hold anything, so the only values that can
		// change an outcome are its default and whatever the conditions
		// compare it against.
		out := []any{}
		if d, ok := s.Default.(string); ok {
			out = append(out, d)
		}
		for _, l := range literals {
			out = appendUnique(out, l)
		}
		if len(out) == 0 {
			out = append(out, "")
		}
		return out
	}
}

func appendUnique(out []any, v any) []any {
	for _, existing := range out {
		if existing == v {
			return out
		}
	}
	return append(out, v)
}

// pathEnum carries the walk.
type pathEnum struct {
	wiz       *Wizard
	branch    map[string][]any
	limit     int
	seen      map[string]bool
	answers   map[string]any
	out       []Path
	truncated bool
}

// walk runs the wizard symbolically from step i, forking wherever an answer
// can change what comes next.
func (e *pathEnum) walk(i int, taken []string) {
	if e.truncated {
		return
	}
	if i == len(e.wiz.Steps) {
		e.record(taken)
		return
	}
	s := e.wiz.Steps[i]
	if c := s.Condition(); c != nil && !c.Eval(e.answers) {
		// Not asked, and it contributes no answer — exactly what a real run
		// does, so a later condition on this key sees the same absence.
		e.walk(i+1, taken)
		return
	}
	// A fresh slice per step: sharing one array across the walk works only
	// while nothing keeps a reference, and record does.
	next := append(append(make([]string, 0, len(taken)+1), taken...), s.Key)

	values, forks := e.branch[s.Key]
	if !forks {
		// Nothing reads this answer, so its value cannot change the shape of
		// the run. Record it anyway: an interpolation may read it, and it
		// costs one map entry.
		previous, had := e.answers[s.Key]
		e.answers[s.Key] = defaultAnswer(s)
		e.walk(i+1, next)
		e.restore(s.Key, previous, had)
		return
	}
	for _, v := range values {
		if e.truncated {
			return
		}
		previous, had := e.answers[s.Key]
		e.answers[s.Key] = v
		e.walk(i+1, next)
		e.restore(s.Key, previous, had)
	}
}

func (e *pathEnum) restore(key string, previous any, had bool) {
	if had {
		e.answers[key] = previous
		return
	}
	delete(e.answers, key)
}

// record keeps one path, the first time its shape is seen.
func (e *pathEnum) record(taken []string) {
	signature := strings.Join(taken, "\x00")
	if e.seen[signature] {
		return
	}
	if len(e.out) >= e.limit {
		e.truncated = true
		return
	}
	e.seen[signature] = true

	steps := make([]string, len(taken))
	copy(steps, taken)
	// Only the branch answers: the rest are defaults, and printing them would
	// bury the two or three values that actually decided the path.
	answers := make(map[string]string, len(e.branch))
	for key := range e.branch {
		if v, ok := e.answers[key]; ok {
			answers[key] = AnswerText(v)
		}
	}
	e.out = append(e.out, Path{Steps: steps, Answers: answers})
}

// defaultAnswer is a plausible value for a step nothing branches on, used only
// so interpolation and truthiness see something of the right type.
func defaultAnswer(s Step) any {
	switch s.Type {
	case TypeConfirm:
		if s.Default != nil {
			if b, err := ParseConfirmDefault(s.Default); err == nil {
				return b
			}
		}
		return false
	case TypeNumber:
		if s.Default != nil {
			if f, err := ToFloat(s.Default); err == nil {
				return f
			}
		}
		return float64(0)
	case TypeMultiselect:
		items, err := SplitMultiDefault(s.Default)
		if err != nil {
			return []string{}
		}
		return OrderByOptions(items, s.Options)
	default:
		if d, ok := s.Default.(string); ok {
			return d
		}
		return ""
	}
}

// SortPaths orders paths by length and then by content, so two runs of the
// same spec print the same list — a diff of the two is then a real change.
func SortPaths(paths []Path) {
	sort.SliceStable(paths, func(a, b int) bool {
		if len(paths[a].Steps) != len(paths[b].Steps) {
			return len(paths[a].Steps) < len(paths[b].Steps)
		}
		return strings.Join(paths[a].Steps, ",") < strings.Join(paths[b].Steps, ",")
	})
}
