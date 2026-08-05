package widgets

import (
	"context"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypePassword, passwordWidget{})
}

type passwordWidget struct{}

func (passwordWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	prompt := s.Prompt()
	if prompt == "" {
		prompt = "Password:"
	}
	PrintHeader(env, prompt)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, "")
		value, eof, err := readPasswordInput(env)
		if err != nil {
			return nil, err
		}
		if value != "" {
			return value, nil
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
