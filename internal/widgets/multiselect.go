package widgets

import (
	"context"
	"strconv"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeMultiselect, multiselectWidget{})
}

type multiselectWidget struct{}

// defaultMarks reports, per option, which ones the step's default selects, so
// the list can show what pressing enter would answer — and so Run can turn the
// same mask into the answer without resolving the default twice.
func defaultMarks(s spec.Step) ([]bool, error) {
	items, err := spec.SplitMultiDefault(s.Default)
	if err != nil {
		return nil, err
	}
	return spec.MarkOptions(items, s.Options), nil
}

func (multiselectWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	PrintHeader(env, s.Prompt())
	// The default is resolved once: it decides both what the list shows as
	// preselected and what an empty answer returns.
	marks, defErr := defaultMarks(s)
	if env.rich() {
		if defErr != nil {
			return nil, defErr
		}
		return richMultiselect(ctx, s, env, marks)
	}
	PrintStepListMarked(env, s, marks, s.Height)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		PrintPrompt(env, "")
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if eof || line == "" {
			if defErr != nil {
				return nil, defErr
			}
			return spec.OptionsFor(marks, s.Options), nil
		}
		selected := make([]bool, len(s.Options))
		all := false
		valid := true
		for _, tok := range strings.Split(line, ",") {
			t := strings.TrimSpace(tok)
			if t == "" {
				continue
			}
			lt := strings.ToLower(t)
			if lt == "all" {
				all = true
				continue
			}
			if lt == "none" {
				all = false
				clear(selected)
				continue
			}
			if n, err := strconv.Atoi(t); err == nil && n >= 1 && n <= len(s.Options) {
				selected[n-1] = true
				continue
			}
			if i := s.IndexOfOption(t); i >= 0 {
				selected[i] = true
				continue
			}
			valid = false
		}
		if !valid {
			PrintError(env, "Invalid choice, try again.")
			continue
		}
		if all {
			for i := range selected {
				selected[i] = true
			}
		}
		if problem := checkSelectionCount(s, selected); problem != "" {
			PrintError(env, problem)
			continue
		}
		return spec.OptionsFor(selected, s.Options), nil
	}
}

// richMultiselect runs the live picker with the default's marks preselected.
// The answer is built exactly as the line-based path builds it, so both return
// the options in spec order.
func richMultiselect(ctx context.Context, s spec.Step, env *Env, marks []bool) (any, error) {
	if marks == nil {
		marks = make([]bool, len(s.Options))
	}
	p := newPicker(s, marks, true, matchSubstring, "space to toggle, enter to confirm")
	chosen, err := runPicker(ctx, env, p)
	if err != nil {
		if err == errAbandoned {
			return spec.OptionsFor(marks, s.Options), nil
		}
		return nil, err
	}
	picked := make([]bool, len(s.Options))
	for _, i := range chosen {
		picked[i] = true
	}
	if problem := checkSelectionCount(s, picked); problem != "" {
		PrintError(env, problem)
		return richMultiselect(ctx, s, env, picked)
	}
	return spec.OptionsFor(picked, s.Options), nil
}

// checkSelectionCount reports why a selection is unacceptable, or "" when it
// passes. Bounds are checked at the moment of answering rather than while
// toggling: forbidding a keystroke that would exceed the maximum leaves the
// user unable to swap one choice for another.
func checkSelectionCount(s spec.Step, marks []bool) string {
	n := 0
	for _, m := range marks {
		if m {
			n++
		}
	}
	return spec.CheckSelectionCount(s, n)
}
