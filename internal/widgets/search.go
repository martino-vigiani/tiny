package widgets

import (
	"context"
	"slices"
	"strconv"

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
	if env.rich() {
		return richFilter(ctx, s, env, matchSubstring)
	}
	match := matcherFor(s.Match, matchSubstring)
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
		matches := filterMatching(s.Options, query, match, s.Match == spec.MatchFuzzy)
		if len(matches) == 0 {
			// With allow_custom the typed text is itself a valid answer, which
			// is what makes a search step usable for values that are suggested
			// rather than enumerated.
			if s.AllowCustom && query != "" {
				return query, nil
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

// filterMatching returns the options a filter accepts.
//
// A fuzzy step gets them ranked best-match first, exactly as the live prompt
// ranks them. Without that the two ways of drawing the same step would present
// the matches in different orders, and "the second one" would mean different
// things — which is the presentation guarantee broken.
func filterMatching(options []string, query string, match matchFunc, rank bool) []string {
	out := make([]string, 0, len(options))
	for _, opt := range options {
		if match(opt, query) {
			out = append(out, opt)
		}
	}
	if rank && query != "" {
		slices.SortStableFunc(out, func(a, b string) int {
			sa, _ := fuzzyScore(a, query)
			sb, _ := fuzzyScore(b, query)
			return sb - sa
		})
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

// richFilter is the live prompt behind both search and autocomplete: the same
// list and cursor, differing only in how a match is decided.
func richFilter(ctx context.Context, s spec.Step, env *Env, match matchFunc) (any, error) {
	def := searchDefault(s.Default, s.Options)
	marks := make([]bool, len(s.Options))
	if def != "" {
		if i := indexFold(s.Options, def); i >= 0 {
			marks[i] = true
		}
	}
	p := newPicker(s, marks, false, match, "type to filter")
	p.refilter()
	if def != "" {
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
			if def != "" {
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
		if def != "" {
			return def, nil
		}
		return "", nil
	}
	return s.Options[chosen[0]], nil
}
