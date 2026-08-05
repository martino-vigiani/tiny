package widgets

import (
	"context"
	"strconv"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeAutocomplete, autocompleteWidget{})
}

type autocompleteWidget struct{}

func (autocompleteWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if env.rich() {
		return richFilter(ctx, s, env, matchPrefix)
	}
	match := matcherFor(s.Match, matchPrefix)
	PrintHeader(env, s.Prompt())
	if s.Placeholder != "" {
		PrintNotef(env, "Hint: %s", s.Placeholder)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		PrintPrompt(env, "")
		text, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if eof {
			if d, ok := autocompleteDefault(s); ok {
				return d, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		if text == "" {
			if d, ok := autocompleteDefault(s); ok {
				return d, nil
			}
			continue
		}
		matches := filterMatching(s.Options, text, match, s.Match == spec.MatchFuzzy)
		switch {
		case len(matches) == 1:
			return matches[0], nil
		case len(matches) == 0:
			if s.AllowCustom {
				return text, nil
			}
			PrintError(env, "No matches, try again.")
			continue
		}
		PrintListHeight(env, matches, -1, s.Height)
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
				if d, ok := autocompleteDefaultIn(matches, s); ok {
					return d, nil
				}
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return "", nil
			}
			if line == "" {
				return matches[0], nil
			}
			if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(matches) {
				return matches[n-1], nil
			}
			for _, m := range matches {
				if strings.EqualFold(m, line) {
					return m, nil
				}
			}
			PrintErrorf(env, "Invalid choice, pick a number between 1 and %d.", len(matches))
		}
	}
}

// autocompleteDefault resolves the default to its canonical option.
func autocompleteDefault(s spec.Step) (string, bool) {
	d, ok := s.Default.(string)
	if !ok {
		return "", false
	}
	if canon, ok := spec.CanonicalOption(d, s.Options); ok {
		return canon, true
	}
	return d, true
}

// autocompleteDefaultIn resolves the default if it is one of the given matches.
func autocompleteDefaultIn(matches []string, s spec.Step) (string, bool) {
	d, ok := s.Default.(string)
	if !ok {
		return "", false
	}
	for _, m := range matches {
		if strings.EqualFold(m, d) {
			return m, true
		}
	}
	return "", false
}
