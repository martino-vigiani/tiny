package widgets

import (
	"context"
	"strings"
	"time"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeDate, dateWidget{})
}

type dateWidget struct{}

func (dateWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	layout := "2006-01-02"
	expected := "YYYY-MM-DD"
	hint := "(format: YYYY-MM-DD)"
	if s.Format == "datetime" {
		layout = "2006-01-02 15:04:05"
		expected = "YYYY-MM-DD HH:MM:SS"
		hint = "(format: YYYY-MM-DD HH:MM:SS)"
	}
	defStr, hasDef := s.Default.(string)
	PrintHeader(env, s.Prompt())
	suffix := hint + ": "
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, suffix)
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		v := strings.TrimSpace(line)
		if v == "" {
			if hasDef {
				v = defStr
			} else if eof {
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return "", nil
			} else if s.Required {
				PrintErrorf(env, "Invalid date, expected %s.", expected)
				continue
			} else {
				return "", nil
			}
		}
		if _, err := time.Parse(layout, v); err != nil {
			if eof {
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return "", nil
			}
			PrintErrorf(env, "Invalid date, expected %s.", expected)
			continue
		}
		return v, nil
	}
}
