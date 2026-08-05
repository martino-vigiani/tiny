package widgets

import (
	"context"
	"strconv"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeSearch, searchWidget{})
}

type searchWidget struct{}

func (searchWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	PrintHeader(env, s.Prompt())
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		PrintPrompt(env, "")
		query, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if eof {
			if d := searchDefault(s.Default, s.Options); d != "" {
				return d, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		matches := filterSubstring(s.Options, query)
		if len(matches) == 0 {
			PrintError(env, "No matches, try again.")
			continue
		}
		PrintList(env, matches, -1)
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
				if d := searchDefault(s.Default, matches); d != "" {
					return d, nil
				}
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return "", nil
			}
			if line == "" {
				if d := searchDefault(s.Default, matches); d != "" {
					return d, nil
				}
				return matches[0], nil
			}
			if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(matches) {
				return matches[n-1], nil
			}
			PrintErrorf(env, "Invalid choice, pick a number between 1 and %d.", len(matches))
		}
	}
}

// filterSubstring returns the options containing query, case-insensitive.
func filterSubstring(options []string, query string) []string {
	q := strings.ToLower(query)
	var out []string
	for _, opt := range options {
		if strings.Contains(strings.ToLower(opt), q) {
			out = append(out, opt)
		}
	}
	return out
}

// searchDefault returns the canonical option matching the default if it is
// among options, or "" otherwise.
func searchDefault(def any, options []string) string {
	d, ok := def.(string)
	if !ok {
		return ""
	}
	if canon, ok := spec.CanonicalOption(d, options); ok {
		return canon
	}
	return ""
}
