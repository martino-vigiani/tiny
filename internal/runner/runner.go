// Package runner executes a wizard against an input stream and serialises the
// collected answers in json/yaml/env format.
package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
	"github.com/martino-vigiani/tiny/internal/widgets"
)

// Runner executes wizard steps against an input stream and collects answers.
type Runner struct {
	env *widgets.Env
}

// New creates a Runner reading from r and drawing the wizard on ui.
//
// ui is the *UI stream* and never carries answers: callers pass stderr there
// and serialise the payload to stdout, so `tiny w.yaml | jq` works while the
// human still sees the prompts. stdinFd is used for terminal-mode operations
// (only consulted when isTTY is true).
func New(r io.Reader, ui io.Writer, stdinFd int, isTTY bool) *Runner {
	return &Runner{
		env: &widgets.Env{
			In:      bufio.NewReader(r),
			Out:     ui,
			StdinFd: stdinFd,
			IsTTY:   isTTY,
		},
	}
}

// SetTheme installs the compiled theme used to draw every subsequent step.
// A Runner with no theme draws with the built-in default.
func (r *Runner) SetTheme(t *theme.Resolved) {
	r.env.Theme = t
}

// Run executes every step of the wizard in order and returns the answers map.
// Each step is dispatched to its registered widget; errors are wrapped with
// the step key.
// A step whose `when:` condition does not hold is skipped, and a skipped step
// contributes no key to the answers — the caller sees the question was never
// asked rather than a value nobody chose.
func (r *Runner) Run(ctx context.Context, wiz *spec.Wizard) (map[string]any, error) {
	answers := make(map[string]any, len(wiz.Steps))
	for _, step := range wiz.Steps {
		if !step.Condition().Eval(answers) {
			continue
		}
		value, err := widgets.RunStep(ctx, step, r.env)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", step.Key, err)
		}
		answers[step.Key] = value
	}
	return answers, nil
}

// PrintYAML serialises answers as YAML.
func PrintYAML(w io.Writer, answers map[string]any) error {
	data, err := yaml.Marshal(answers)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// PrintEnv serialises answers as KEY=VALUE lines, one per sorted key.
//
// Values are shell-quoted: anything outside the safe set is wrapped in single
// quotes with embedded quotes escaped, so `eval "$(tiny -format env w.yaml)"`
// is safe even when an answer contains spaces, quotes, `$` or `;`. Keys are
// normalised to valid shell identifiers ([A-Za-z0-9_], never leading with a
// digit); two keys that normalise to the same identifier are an error rather
// than a silently dropped answer.
//
// Booleans render as true/false, []string values are joined with ", ", numbers
// as their literal value. map[string]string values render one line per entry
// as KEY_mapkey=value with the map keys sorted; nil values are omitted.
func PrintEnv(w io.Writer, answers map[string]any) error {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := make(map[string]string, len(keys))
	for _, k := range keys {
		name := envKey(k)
		if prev, dup := seen[name]; dup {
			return fmt.Errorf("keys %q and %q both map to env name %q", prev, k, name)
		}
		seen[name] = k
		if err := writeEnvValue(w, name, answers[k]); err != nil {
			return err
		}
	}
	return nil
}

// envKey turns a wizard key into a valid shell identifier. Wizard keys may
// contain "." and "-", which no shell accepts in a variable name.
func envKey(key string) string {
	var b strings.Builder
	b.Grow(len(key) + 1)
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// shellQuote returns v as a single shell word. Values made only of characters
// that no shell treats specially are emitted bare; everything else is
// single-quoted, with each embedded quote closed, escaped and reopened.
func shellQuote(v string) string {
	if v == "" {
		return ""
	}
	if !needsQuoting(v) {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for {
		i := strings.IndexByte(v, '\'')
		if i < 0 {
			break
		}
		b.WriteString(v[:i])
		b.WriteString(`'\''`)
		v = v[i+1:]
	}
	b.WriteString(v)
	b.WriteByte('\'')
	return b.String()
}

func needsQuoting(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_', c == '-', c == '.', c == '/', c == ':', c == '@', c == '%', c == '+', c == ',', c == '=':
		default:
			return true
		}
	}
	return false
}

func writeEnvValue(w io.Writer, key string, v any) error {
	switch t := v.(type) {
	case nil:
		return nil
	case bool:
		_, err := fmt.Fprintf(w, "%s=%s\n", key, strconv.FormatBool(t))
		return err
	case string:
		_, err := fmt.Fprintf(w, "%s=%s\n", key, shellQuote(t))
		return err
	case []string:
		_, err := fmt.Fprintf(w, "%s=%s\n", key, shellQuote(strings.Join(t, ", ")))
		return err
	case map[string]string:
		mk := make([]string, 0, len(t))
		for k := range t {
			mk = append(mk, k)
		}
		sort.Strings(mk)
		for _, k := range mk {
			if _, err := fmt.Fprintf(w, "%s_%s=%s\n", key, envKey(k), shellQuote(t[k])); err != nil {
				return err
			}
		}
		return nil
	case float64:
		_, err := fmt.Fprintf(w, "%s=%s\n", key, strconv.FormatFloat(t, 'g', -1, 64))
		return err
	case int:
		_, err := fmt.Fprintf(w, "%s=%d\n", key, t)
		return err
	default:
		_, err := fmt.Fprintf(w, "%s=%s\n", key, shellQuote(fmt.Sprintf("%v", t)))
		return err
	}
}
