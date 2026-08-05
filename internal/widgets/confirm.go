package widgets

import (
	"context"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeConfirm, confirmWidget{})
}

type confirmWidget struct{}

func (confirmWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	def, hasDef := confirmDefault(s)
	defStr := "n"
	if hasDef && def {
		defStr = "y"
	}
	PrintHeader(env, s.Prompt())
	suffix := "(y/n) " + Hint(env, defStr) + ": "
	PrintPrompt(env, suffix)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "true":
			return true, nil
		case "n", "no", "false":
			return false, nil
		case "":
			if hasDef {
				return def, nil
			}
			if eof && s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return false, nil
		default:
			if eof {
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return false, nil
			}
			PrintError(env, "Answer with y or n.")
			PrintPrompt(env, suffix)
		}
	}
}

// confirmDefault interprets a confirm default, mirroring the headless runner:
// bool values pass through, yes/y/true map to true, no/n/false map to false,
// and anything else is treated as absent.
func confirmDefault(s spec.Step) (bool, bool) {
	if s.Default == nil {
		return false, false
	}
	b, err := spec.ParseConfirmDefault(s.Default)
	if err != nil {
		return false, false
	}
	return b, true
}
