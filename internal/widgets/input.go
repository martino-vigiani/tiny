package widgets

import (
	"context"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeInput, inputWidget{})
}

type inputWidget struct{}

func (inputWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	PrintHeader(env, s.Prompt())
	// The hints depend only on the step and the theme, so they are built once
	// rather than on every retry.
	hints := inputHints(env, s)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, hints)
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) != "" {
			if problem := s.Rule().Check(line); problem != "" {
				PrintError(env, problem)
				continue
			}
			return line, nil
		}
		if def, ok := s.Default.(string); ok && def != "" {
			return def, nil
		}
		if eof {
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		if s.Required {
			PrintError(env, "This field is required.")
			continue
		}
		return "", nil
	}
}

// inputHints renders the placeholder and default markers shown after the
// prompt, wrapped in the theme's hint delimiters.
func inputHints(env *Env, s spec.Step) string {
	var hints []string
	if s.Placeholder != "" {
		hints = append(hints, Hint(env, s.Placeholder))
	}
	if def, ok := s.Default.(string); ok && def != "" {
		hints = append(hints, Hint(env, def))
	}
	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, " ") + " "
}
