package widgets

import (
	"context"
	"strconv"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeSelect, selectWidget{})
}

type selectWidget struct{}

func (selectWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	def, hasDef := selectDefault(s)
	if env.rich() {
		return richSelect(ctx, s, env, def, hasDef)
	}
	PrintHeader(env, s.Prompt())
	defIdx := -1
	if hasDef {
		for i, opt := range s.Options {
			if opt == def {
				defIdx = i
				break
			}
		}
	}
	PrintListHeight(env, s.Options, defIdx, s.Height)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		PrintPrompt(env, "")
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if eof {
			if hasDef {
				return def, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		if line == "" {
			if hasDef {
				return def, nil
			}
			continue
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(s.Options) {
			return s.Options[n-1], nil
		}
		if i := indexFold(s.Options, line); i >= 0 {
			return s.Options[i], nil
		}
		PrintErrorf(env, "Invalid choice, pick a number between 1 and %d.", len(s.Options))
	}
}

// selectDefault resolves the step default to its canonical option, falling
// back to the raw default when it is not an option (invalid spec).
func selectDefault(s spec.Step) (string, bool) {
	d, ok := s.Default.(string)
	if !ok {
		return "", false
	}
	if canon, ok := spec.CanonicalOption(d, s.Options); ok {
		return canon, true
	}
	return d, true
}

// richSelect runs the live picker, starting the cursor on the default so
// pressing enter immediately answers the same thing the line-based path would.
func richSelect(ctx context.Context, s spec.Step, env *Env, def string, hasDef bool) (any, error) {
	marks := make([]bool, len(s.Options))
	if hasDef {
		if i := indexFold(s.Options, def); i >= 0 {
			marks[i] = true
		}
	}
	p := newPicker(s, marks, false, matchSubstring, "type to filter")
	p.refilter()
	if hasDef {
		for i, idx := range p.shown {
			if marks[idx] {
				p.cursor = i
				break
			}
		}
	}
	chosen, err := runPicker(ctx, env, p)
	if err != nil {
		if err == errAbandoned {
			if hasDef {
				return def, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		return nil, err
	}
	if len(chosen) == 0 {
		if hasDef {
			return def, nil
		}
		return "", nil
	}
	return s.Options[chosen[0]], nil
}
