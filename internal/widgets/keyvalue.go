package widgets

import (
	"context"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeKeyValue, keyvalueWidget{})
}

type keyvalueWidget struct{}

func (keyvalueWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	PrintHeader(env, s.Prompt())
	PrintNote(env, "Enter key=value pairs, one per line; empty line to finish:")
	out := map[string]string{}
	entered := false
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if eof || line == "" {
			break
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			PrintErrorf(env, "Expected key=value, got %q.", line)
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		out[key] = value
		entered = true
	}
	if !entered {
		def, err := spec.ToStringMap(s.Default)
		if err != nil {
			return nil, err
		}
		return def, nil
	}
	return out, nil
}
