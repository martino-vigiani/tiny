package spec

// The short form: a wizard written as plain argv words.
//
//	tiny select framework SwiftUI UIKit AppKit
//	tiny select framework SwiftUI UIKit + input name MyApp + confirm git yes
//	tiny number port 8080 --min 1 --max 65535
//
// The shape is chosen for token cost, which is not the same as character cost.
// Under BPE tokenizers a space is free — it is absorbed into the token that
// follows — while every `,` `:` `=` `|` is a token of its own. So
// `SwiftUI UIKit AppKit` is 5 tokens where `SwiftUI,UIKit,AppKit` is 7, and a
// compact punctuated mini-language would cost MORE than plain words. Bare argv
// words are both the cheapest and the most readable option, which is rare
// enough to be worth stating.
//
// This is sugar and nothing else: ParseShort builds a *Wizard and hands it to
// the same Validate everything else uses. There is no second execution path,
// and TestShortFormEquivalence pins each short form against the YAML it is
// meant to be shorthand for.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// stepSeparator divides steps in the short form. It is a bare word so no shell
// quoting is ever needed: `;` and `|` would both need quotes, and `|` unquoted
// would be a pipe — the caller who forgets would get "command not found"
// instead of an error from tiny.
const stepSeparator = "+"

// shortTypes are the step types expressible as argv. `table` is deliberately
// absent: its rows are two-dimensional and any argv encoding needs a second
// delimiter, at which point the YAML form is both shorter and legible.
var shortTypes = map[string]StepType{
	"select":       TypeSelect,
	"multiselect":  TypeMultiselect,
	"confirm":      TypeConfirm,
	"input":        TypeInput,
	"password":     TypePassword,
	"search":       TypeSearch,
	"number":       TypeNumber,
	"date":         TypeDate,
	"keyvalue":     TypeKeyValue,
	"editor":       TypeEditor,
	"autocomplete": TypeAutocomplete,
	"file":         TypeFile,
}

// IsShortType reports whether word starts a short-form wizard rather than
// naming a spec file. A file that happens to be called `select` is still
// reachable as `./select`.
func IsShortType(word string) bool {
	_, ok := shortTypes[word]
	return ok
}

// ShortTypeNames lists the expressible types, sorted, for error messages.
func ShortTypeNames() string {
	return "select|multiselect|confirm|input|password|search|number|date|keyvalue|editor|autocomplete|file"
}

// ParseShort builds a wizard from argv words.
func ParseShort(args []string) (*Wizard, error) {
	groups := splitSteps(args)
	w := &Wizard{Steps: make([]Step, 0, len(groups))}
	for i, group := range groups {
		step, err := parseShortStep(group, i+1)
		if err != nil {
			return nil, err
		}
		w.Steps = append(w.Steps, step)
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return w, nil
}

func splitSteps(args []string) [][]string {
	var groups [][]string
	start := 0
	for i, a := range args {
		if a == stepSeparator {
			groups = append(groups, args[start:i])
			start = i + 1
		}
	}
	return append(groups, args[start:])
}

// attrArity says how many words each attribute consumes after its own name.
var attrArity = map[string]int{
	"--title": 1, "--prompt": 1, "--default": 1, "--placeholder": 1,
	"--min": 1, "--max": 1, "--increment": 1, "--format": 1, "--editor": 1,
	"--delimiter": 1, "--height": 1, "--when": 1, "--pattern": 1,
	"--message": 1, "--min-length": 1, "--max-length": 1,
	"--match": 1, "--min-select": 1, "--max-select": 1, "--root": 1,
	"--return-column": 1,
	"--required":      0, "--allow-custom": 0, "--dirs": 0, "--hidden": 0,
}

func parseShortStep(words []string, n int) (Step, error) {
	if len(words) == 0 {
		return Step{}, fmt.Errorf("step %d: empty step; steps are separated by %q and each starts with a type", n, stepSeparator)
	}
	typ, ok := shortTypes[words[0]]
	if !ok {
		return Step{}, fmt.Errorf("step %d: unknown type %q; use one of %s (table: YAML only)", n, words[0], ShortTypeNames())
	}
	if len(words) < 2 {
		return Step{}, fmt.Errorf("step %d (%s): missing key; usage: tiny %s <key> <value>...", n, typ, typ)
	}
	s := Step{Type: typ, Key: unescapeWord(words[1])}

	var values []string
	for i := 2; i < len(words); i++ {
		word := words[i]
		if !strings.HasPrefix(word, "--") {
			values = append(values, unescapeWord(word))
			continue
		}
		arity, known := attrArity[word]
		if !known {
			return s, fmt.Errorf("step %d %q: unknown attribute %s; valid: %s",
				n, s.Key, word, validAttrs(typ))
		}
		if arity == 0 {
			if err := setFlagAttr(&s, word); err != nil {
				return s, fmt.Errorf("step %d %q: %v", n, s.Key, err)
			}
			continue
		}
		if i+1 >= len(words) {
			return s, fmt.Errorf("step %d %q: %s needs a value", n, s.Key, word)
		}
		i++
		if err := setValueAttr(&s, word, unescapeWord(words[i])); err != nil {
			return s, fmt.Errorf("step %d %q: %v", n, s.Key, err)
		}
	}

	if err := applyPositional(&s, values, n); err != nil {
		return s, err
	}
	if s.Title == "" && s.PromptText == "" {
		// The key already names the thing being asked, so deriving the title
		// from it costs no tokens at all — which is what lets a short-form
		// question match gum's word count.
		s.Title = titleFromKey(s.Key)
	}
	return s, nil
}

// applyPositional maps the bare words after the key onto the step, which means
// something different for each family of type.
func applyPositional(s *Step, values []string, n int) error {
	switch s.Type {
	case TypeSelect, TypeMultiselect, TypeSearch, TypeAutocomplete:
		if len(values) == 0 {
			return fmt.Errorf("step %d %q: %s needs at least one option; usage: tiny %s %s first second",
				n, s.Key, s.Type, s.Type, s.Key)
		}
		s.Options = values
		// A single-choice step defaults to its first option, which costs no
		// tokens and makes every short wizard headless-safe by construction.
		// It is the one place the short form is not a literal transcription of
		// the minimal YAML.
		if s.Default == nil && s.Type != TypeMultiselect {
			s.Default = values[0]
		}
	case TypeKeyValue:
		if len(values) == 0 {
			return nil
		}
		pairs := map[string]any{}
		for _, v := range values {
			k, val, found := strings.Cut(v, "=")
			if !found {
				return fmt.Errorf("step %d %q: keyvalue takes key=value words, got %q", n, s.Key, v)
			}
			pairs[k] = val
		}
		if s.Default == nil {
			s.Default = pairs
		}
	default:
		if len(values) == 0 {
			return nil
		}
		if len(values) > 1 {
			return fmt.Errorf("step %d %q: %s takes at most one value (the default), got %d; "+
				"did you mean: tiny select %s %s", n, s.Key, s.Type, len(values), s.Key, strings.Join(values, " "))
		}
		if s.Default == nil {
			s.Default = values[0]
		}
	}
	return nil
}

func setFlagAttr(s *Step, name string) error {
	switch name {
	case "--required":
		s.Required = true
		return nil
	case "--allow-custom":
		s.AllowCustom = true
		return nil
	case "--dirs":
		s.Dirs = true
		return nil
	case "--hidden":
		s.Hidden = true
		return nil
	default:
		return fmt.Errorf("unknown flag %s", name)
	}
}

func setValueAttr(s *Step, name, value string) error {
	switch name {
	case "--title":
		s.Title = value
	case "--prompt":
		s.PromptText = value
	case "--default":
		s.Default = value
	case "--placeholder":
		s.Placeholder = value
	case "--format":
		s.Format = value
	case "--editor":
		s.EditorCmd = value
	case "--delimiter":
		s.Delimiter = value
	case "--when":
		s.When = value
	case "--pattern":
		s.Pattern = value
	case "--match":
		s.Match = value
	case "--root":
		s.Root = value
	case "--message":
		s.Message = value
	case "--min":
		f, err := parseNumberAttr(name, value)
		if err != nil {
			return err
		}
		s.Min = &f
	case "--max":
		f, err := parseNumberAttr(name, value)
		if err != nil {
			return err
		}
		s.Max = &f
	case "--increment":
		f, err := parseNumberAttr(name, value)
		if err != nil {
			return err
		}
		s.Increment = f
	case "--height":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.Height = n
	case "--min-length":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.MinLength = n
	case "--max-length":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.MaxLength = n
	case "--min-select":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.MinSelect = n
	case "--max-select":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.MaxSelect = n
	case "--return-column":
		n, err := parseIntAttr(name, value)
		if err != nil {
			return err
		}
		s.ReturnColumn = n
	default:
		return fmt.Errorf("unknown attribute %s", name)
	}
	return nil
}

func parseNumberAttr(name, value string) (float64, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s wants a number, got %q", name, value)
	}
	return f, nil
}

func parseIntAttr(name, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s wants a whole number, got %q", name, value)
	}
	return n, nil
}

// validAttrs lists the attributes worth suggesting for a type, so a typo comes
// back with the alternatives rather than just a rejection.
func validAttrs(t StepType) string {
	common := "--title --prompt --default --required --when"
	switch t {
	case TypeSelect, TypeSearch, TypeAutocomplete, TypeMultiselect:
		return common + " --height --match --allow-custom --min-select --max-select"
	case TypeFile:
		return common + " --root --dirs --hidden --height"
	case TypeNumber:
		return common + " --min --max --increment"
	case TypeDate:
		return common + " --format"
	case TypeInput, TypePassword, TypeEditor:
		return common + " --placeholder --pattern --message --min-length --max-length"
	default:
		return common
	}
}

// unescapeWord lets a value start with the characters the grammar reserves:
// `\--foo` is the literal `--foo`, `\+` is a literal plus.
func unescapeWord(w string) string {
	if strings.HasPrefix(w, `\-`) || w == `\+` {
		return w[1:]
	}
	return w
}

// titleFromKey turns db_host into "Db host": the key is already a description,
// so re-typing it as a title would be paying for the same words twice.
func titleFromKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for i, r := range key {
		switch {
		case r == '_' || r == '-' || r == '.':
			b.WriteByte(' ')
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
