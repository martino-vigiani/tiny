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

// preselected reports, per option, whether the step's default selects it, so
// the list can show what pressing enter would answer. An unparsable default
// marks nothing rather than failing: Run resolves it properly later.
func preselected(s spec.Step) []bool {
	items, err := spec.SplitMultiDefault(s.Default)
	if err != nil || len(items) == 0 {
		return nil
	}
	chosen := spec.OrderByOptions(items, s.Options)
	if len(chosen) == 0 {
		return nil
	}
	set := make(map[string]bool, len(chosen))
	for _, c := range chosen {
		set[c] = true
	}
	marks := make([]bool, len(s.Options))
	for i, opt := range s.Options {
		marks[i] = set[opt]
	}
	return marks
}

func (multiselectWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	PrintHeader(env, s.Prompt())
	PrintListMarked(env, s.Options, preselected(s))
	lower := make(map[string]string, len(s.Options))
	for _, opt := range s.Options {
		lower[strings.ToLower(opt)] = opt
	}
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
			items, err := spec.SplitMultiDefault(s.Default)
			if err != nil {
				return nil, err
			}
			return spec.OrderByOptions(items, s.Options), nil
		}
		selected := map[string]bool{}
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
				selected = map[string]bool{}
				continue
			}
			if n, err := strconv.Atoi(t); err == nil && n >= 1 && n <= len(s.Options) {
				selected[s.Options[n-1]] = true
				continue
			}
			if canon, ok := lower[lt]; ok {
				selected[canon] = true
				continue
			}
			valid = false
		}
		if !valid {
			PrintError(env, "Invalid choice, try again.")
			continue
		}
		if all {
			for _, opt := range s.Options {
				selected[opt] = true
			}
		}
		out := make([]string, 0, len(s.Options))
		for _, opt := range s.Options {
			if selected[opt] {
				out = append(out, opt)
			}
		}
		return out, nil
	}
}
