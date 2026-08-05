// Package theme defines how a tiny wizard looks: the symbols it draws, the
// colors it paints them with, and the built-in presets.
//
// A Theme is the declarative form (what users write in YAML). Resolve turns it
// into a Resolved, which holds every string a widget can print already
// concatenated with its ANSI escapes, so rendering a step costs no formatting
// and no allocation.
//
// The package is a leaf: it imports nothing from the rest of tiny.
package theme

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Theme is the user-facing, declarative description of a wizard's look.
//
// Every field is optional and every symbol/style is a pointer so that Merge can
// tell "not set" apart from "deliberately set to empty" — emptying a symbol is
// how a theme removes decoration.
type Theme struct {
	Name    string  `yaml:"name,omitempty"`
	Preset  string  `yaml:"preset,omitempty"`
	Color   string  `yaml:"color,omitempty"`
	Symbols Symbols `yaml:"symbols,omitempty"`
	Styles  Styles  `yaml:"styles,omitempty"`
}

// Symbols are the literal strings a wizard draws around its content.
type Symbols struct {
	HeaderPrefix  *string `yaml:"header_prefix,omitempty"`
	HeaderSuffix  *string `yaml:"header_suffix,omitempty"`
	Prompt        *string `yaml:"prompt,omitempty"`
	Indent        *string `yaml:"indent,omitempty"`
	DefaultMarker *string `yaml:"default_marker,omitempty"`
	ItemMarker    *string `yaml:"item_marker,omitempty"`
	Bullet        *string `yaml:"bullet,omitempty"`
	ErrorPrefix   *string `yaml:"error_prefix,omitempty"`
	NotePrefix    *string `yaml:"note_prefix,omitempty"`
	HintOpen      *string `yaml:"hint_open,omitempty"`
	HintClose     *string `yaml:"hint_close,omitempty"`
	TableRule     *string `yaml:"table_rule,omitempty"`

	// Interactive mode only: the row cursor, the checkbox marks, and the
	// caret drawn at the end of the filter line.
	Cursor      *string `yaml:"cursor,omitempty"`
	CursorBlank *string `yaml:"cursor_blank,omitempty"`
	Checked     *string `yaml:"checked,omitempty"`
	Unchecked   *string `yaml:"unchecked,omitempty"`
	Caret       *string `yaml:"caret,omitempty"`
	Ellipsis    *string `yaml:"ellipsis,omitempty"`
}

// Styles map each rendered element to a style expression (see ParseStyle).
type Styles struct {
	Header      *string `yaml:"header,omitempty"`
	Prompt      *string `yaml:"prompt,omitempty"`
	Option      *string `yaml:"option,omitempty"`
	Index       *string `yaml:"index,omitempty"`
	Marker      *string `yaml:"marker,omitempty"`
	Hint        *string `yaml:"hint,omitempty"`
	Error       *string `yaml:"error,omitempty"`
	Note        *string `yaml:"note,omitempty"`
	TableHeader *string `yaml:"table_header,omitempty"`

	// Interactive mode only.
	Cursor   *string `yaml:"cursor,omitempty"`
	Selected *string `yaml:"selected,omitempty"`
	Filter   *string `yaml:"filter,omitempty"`
}

// Color modes accepted by Theme.Color.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// Resolved is the compiled form of a Theme: every string a widget prints, with
// its ANSI escapes already attached. Widgets only concatenate and write.
type Resolved struct {
	Name string

	// Header: HeaderOpen + title + HeaderClose.
	HeaderOpen  string
	HeaderClose string

	// Prompt is the full input marker, colored, e.g. "\x1b[36m> \x1b[0m".
	Prompt string

	// List rendering: Indent + marker + Index + Bullet + " " + option.
	Indent        string
	DefaultMarker string
	ItemMarker    string
	Bullet        string

	// Wrappers. Each pair is empty when the element has no style.
	MarkerOpen, MarkerClose           string
	IndexOpen, IndexClose             string
	OptionOpen, OptionClose           string
	HintOpen, HintClose               string
	ErrorOpen, ErrorClose             string
	NoteOpen, NoteClose               string
	TableHeaderOpen, TableHeaderClose string

	// Prefixes printed before an error or a note line.
	ErrorPrefix string
	NotePrefix  string

	// TableRule is the unit repeated to draw the header separator.
	TableRule string

	// Interactive mode: the row cursor, the checkbox marks, the filter caret
	// and the marker for a line clipped to the terminal width.
	Cursor      string
	CursorBlank string
	Checked     string
	Unchecked   string
	Caret       string
	Ellipsis    string

	CursorOpen, CursorClose     string
	SelectedOpen, SelectedClose string
	FilterOpen, FilterClose     string
}

// reset is the ANSI sequence closing every style opened by ParseStyle.
const reset = "\x1b[0m"

func ptr(s string) *string { return &s }

// presets holds the built-in themes, addressable by name from YAML
// (`preset: minimal`), the -theme flag, or $TINY_THEME.
var presets = map[string]Theme{
	// default: the reference look — em-dash headers, "> " prompt, subtle color.
	"default": {
		Name: "default",
		Symbols: Symbols{
			HeaderPrefix:  ptr("\n— "),
			HeaderSuffix:  ptr(" —\n"),
			Prompt:        ptr("> "),
			Indent:        ptr("  "),
			DefaultMarker: ptr(">"),
			ItemMarker:    ptr(" "),
			Bullet:        ptr(")"),
			ErrorPrefix:   ptr(""),
			NotePrefix:    ptr(""),
			HintOpen:      ptr("["),
			HintClose:     ptr("]"),
			TableRule:     ptr("-"),
			Cursor:        ptr("> "),
			CursorBlank:   ptr("  "),
			Checked:       ptr("[x] "),
			Unchecked:     ptr("[ ] "),
			Caret:         ptr("_"),
			Ellipsis:      ptr("..."),
		},
		Styles: Styles{
			Header:      ptr("bold"),
			Prompt:      ptr(""),
			Option:      ptr(""),
			Index:       ptr("dim"),
			Marker:      ptr("cyan"),
			Hint:        ptr("dim"),
			Error:       ptr("red"),
			Note:        ptr("dim"),
			TableHeader: ptr("bold"),
			Cursor:      ptr("bold"),
			Selected:    ptr("green"),
			Filter:      ptr(""),
		},
	},
	// minimal: no decoration around titles, nothing but the words.
	"minimal": {
		Name:   "minimal",
		Preset: "default",
		Symbols: Symbols{
			HeaderPrefix: ptr("\n"),
			HeaderSuffix: ptr("\n"),
			Prompt:       ptr("> "),
		},
		Styles: Styles{
			Header: ptr("bold"),
			Marker: ptr(""),
			Index:  ptr("dim"),
		},
	},
	// sleek: pointed prompt and bullets, for modern terminals.
	"sleek": {
		Name:   "sleek",
		Preset: "default",
		Symbols: Symbols{
			HeaderPrefix:  ptr("\n"),
			HeaderSuffix:  ptr("\n"),
			Prompt:        ptr("❯ "),
			DefaultMarker: ptr("●"),
			ItemMarker:    ptr("○"),
			Bullet:        ptr("."),
			ErrorPrefix:   ptr("✗ "),
			NotePrefix:    ptr("· "),
			TableRule:     ptr("─"),
			Cursor:        ptr("❯ "),
			Checked:       ptr("◉ "),
			Unchecked:     ptr("◯ "),
			Caret:         ptr("▏"),
			Ellipsis:      ptr("…"),
		},
		Styles: Styles{
			Header: ptr("bold cyan"),
			Prompt: ptr("cyan"),
			Marker: ptr("green"),
			Error:  ptr("bold red"),
		},
	},
	// ascii: 7-bit output only, still colored. For terminals or log pipelines
	// that mangle UTF-8.
	"ascii": {
		Name:   "ascii",
		Preset: "default",
		Symbols: Symbols{
			HeaderPrefix: ptr("\n== "),
			HeaderSuffix: ptr(" ==\n"),
			TableRule:    ptr("-"),
		},
	},
	// plain: ASCII and never colored — byte-stable output for snapshot tests
	// and CI logs.
	"plain": {
		Name:   "plain",
		Preset: "ascii",
		Color:  ColorNever,
		Styles: Styles{
			Header:      ptr(""),
			Prompt:      ptr(""),
			Option:      ptr(""),
			Index:       ptr(""),
			Marker:      ptr(""),
			Hint:        ptr(""),
			Error:       ptr(""),
			Note:        ptr(""),
			TableHeader: ptr(""),
		},
	},
}

// PresetNames returns the built-in preset names, sorted.
func PresetNames() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Preset returns a copy of the built-in theme called name.
func Preset(name string) (Theme, bool) {
	t, ok := presets[name]
	return t, ok
}

// Parse decodes a theme from YAML bytes. The document may be a bare theme
// mapping or a wizard file with a top-level `theme:` key, so the same file
// works as a standalone theme and as an inline block.
func Parse(data []byte) (*Theme, error) {
	var wrapper struct {
		Theme *Theme `yaml:"theme"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err == nil && wrapper.Theme != nil {
		if err := wrapper.Theme.Validate(); err != nil {
			return nil, err
		}
		return wrapper.Theme, nil
	}
	var t Theme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing theme yaml: %w", err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// Validate reports whether the theme's enumerated fields are meaningful. It
// runs before anything is drawn so a typo fails fast instead of half-rendering.
func (t *Theme) Validate() error {
	if t == nil {
		return nil
	}
	switch t.Color {
	case "", ColorAuto, ColorAlways, ColorNever:
	default:
		return fmt.Errorf("theme: invalid color %q: must be auto, always or never", t.Color)
	}
	if t.Preset != "" {
		if _, ok := presets[t.Preset]; !ok {
			return fmt.Errorf("theme: unknown preset %q (available: %s)", t.Preset, strings.Join(PresetNames(), ", "))
		}
	}
	for name, expr := range t.Styles.all() {
		if expr == nil {
			continue
		}
		if _, err := ParseStyle(*expr); err != nil {
			return fmt.Errorf("theme: style %q: %w", name, err)
		}
	}
	return nil
}

func (s Styles) all() map[string]*string {
	return map[string]*string{
		"header":       s.Header,
		"prompt":       s.Prompt,
		"option":       s.Option,
		"index":        s.Index,
		"marker":       s.Marker,
		"hint":         s.Hint,
		"error":        s.Error,
		"note":         s.Note,
		"table_header": s.TableHeader,
		"cursor":       s.Cursor,
		"selected":     s.Selected,
		"filter":       s.Filter,
	}
}

// Merge returns a theme where every field set in over replaces the one in base.
// Fields left unset in over keep the base value, so overlays stay small.
func Merge(base, over Theme) Theme {
	out := base
	if over.Name != "" {
		out.Name = over.Name
	}
	if over.Color != "" {
		out.Color = over.Color
	}
	out.Symbols = mergeSymbols(base.Symbols, over.Symbols)
	out.Styles = mergeStyles(base.Styles, over.Styles)
	return out
}

func mergeSymbols(base, over Symbols) Symbols {
	pick(&base.HeaderPrefix, over.HeaderPrefix)
	pick(&base.HeaderSuffix, over.HeaderSuffix)
	pick(&base.Prompt, over.Prompt)
	pick(&base.Indent, over.Indent)
	pick(&base.DefaultMarker, over.DefaultMarker)
	pick(&base.ItemMarker, over.ItemMarker)
	pick(&base.Bullet, over.Bullet)
	pick(&base.ErrorPrefix, over.ErrorPrefix)
	pick(&base.NotePrefix, over.NotePrefix)
	pick(&base.HintOpen, over.HintOpen)
	pick(&base.HintClose, over.HintClose)
	pick(&base.TableRule, over.TableRule)
	pick(&base.Cursor, over.Cursor)
	pick(&base.CursorBlank, over.CursorBlank)
	pick(&base.Checked, over.Checked)
	pick(&base.Unchecked, over.Unchecked)
	pick(&base.Caret, over.Caret)
	pick(&base.Ellipsis, over.Ellipsis)
	return base
}

func mergeStyles(base, over Styles) Styles {
	pick(&base.Header, over.Header)
	pick(&base.Prompt, over.Prompt)
	pick(&base.Option, over.Option)
	pick(&base.Index, over.Index)
	pick(&base.Marker, over.Marker)
	pick(&base.Hint, over.Hint)
	pick(&base.Error, over.Error)
	pick(&base.Note, over.Note)
	pick(&base.TableHeader, over.TableHeader)
	pick(&base.Cursor, over.Cursor)
	pick(&base.Selected, over.Selected)
	pick(&base.Filter, over.Filter)
	return base
}

func pick(dst **string, over *string) {
	if over != nil {
		*dst = over
	}
}

// Expand resolves a theme's `preset:` chain into a single flat theme, with the
// theme's own fields layered on top of the preset it names. Cycles between
// presets are impossible: only built-ins can be referenced and their chains are
// fixed, but the depth is bounded anyway.
func Expand(t Theme) (Theme, error) {
	const maxDepth = 8
	chain := []Theme{t}
	cur := t
	for depth := 0; cur.Preset != ""; depth++ {
		if depth == maxDepth {
			return Theme{}, fmt.Errorf("theme: preset chain too deep at %q", cur.Preset)
		}
		base, ok := presets[cur.Preset]
		if !ok {
			return Theme{}, fmt.Errorf("theme: unknown preset %q (available: %s)", cur.Preset, strings.Join(PresetNames(), ", "))
		}
		chain = append(chain, base)
		cur = base
	}
	// The last element is the root preset; fold forwards so the caller's own
	// theme is applied last and wins.
	out := chain[len(chain)-1]
	for i := len(chain) - 2; i >= 0; i-- {
		out = Merge(out, chain[i])
	}
	out.Preset = ""
	return out, nil
}

// Default returns the built-in default theme, already expanded.
func Default() Theme {
	return presets["default"]
}

// Resolve compiles a theme into the strings widgets print.
//
// colorCapable reports whether the UI stream can display ANSI escapes (a TTY
// with a usable TERM); it only matters in "auto" mode. Any missing symbol or
// style falls back to the default theme's value, so a partial theme is valid.
func Resolve(t Theme, colorCapable bool) (*Resolved, error) {
	expanded, err := Expand(t)
	if err != nil {
		return nil, err
	}
	full := Merge(mustExpandDefault(), expanded)

	color := colorCapable
	switch full.Color {
	case ColorAlways:
		color = true
	case ColorNever:
		color = false
	}

	style := func(expr *string) (string, string) {
		if !color || expr == nil || *expr == "" {
			return "", ""
		}
		on, err := ParseStyle(*expr)
		if err != nil || on == "" {
			return "", ""
		}
		return on, reset
	}

	headerOn, headerOff := style(full.Styles.Header)
	promptOn, promptOff := style(full.Styles.Prompt)
	markerOn, markerOff := style(full.Styles.Marker)
	indexOn, indexOff := style(full.Styles.Index)
	optionOn, optionOff := style(full.Styles.Option)
	hintOn, hintOff := style(full.Styles.Hint)
	errorOn, errorOff := style(full.Styles.Error)
	noteOn, noteOff := style(full.Styles.Note)
	tableOn, tableOff := style(full.Styles.TableHeader)
	cursorOn, cursorOff := style(full.Styles.Cursor)
	selectedOn, selectedOff := style(full.Styles.Selected)
	filterOn, filterOff := style(full.Styles.Filter)

	sym := full.Symbols
	// The header's leading newlines must stay outside the escape sequence:
	// coloring a newline paints the rest of the line in some terminals.
	prefix := deref(sym.HeaderPrefix)
	lead := prefix[:len(prefix)-len(strings.TrimLeft(prefix, "\n"))]
	prefix = prefix[len(lead):]
	suffix := deref(sym.HeaderSuffix)
	trail := strings.TrimRight(suffix, "\n")
	nl := suffix[len(trail):]

	r := &Resolved{
		Name:             full.Name,
		HeaderOpen:       lead + headerOn + prefix,
		HeaderClose:      trail + headerOff + nl,
		Prompt:           promptOn + deref(sym.Prompt) + promptOff,
		Indent:           deref(sym.Indent),
		DefaultMarker:    deref(sym.DefaultMarker),
		ItemMarker:       deref(sym.ItemMarker),
		Bullet:           deref(sym.Bullet),
		MarkerOpen:       markerOn,
		MarkerClose:      markerOff,
		IndexOpen:        indexOn,
		IndexClose:       indexOff,
		OptionOpen:       optionOn,
		OptionClose:      optionOff,
		HintOpen:         hintOn + deref(sym.HintOpen),
		HintClose:        deref(sym.HintClose) + hintOff,
		ErrorOpen:        errorOn + deref(sym.ErrorPrefix),
		ErrorClose:       errorOff,
		NoteOpen:         noteOn + deref(sym.NotePrefix),
		NoteClose:        noteOff,
		TableHeaderOpen:  tableOn,
		TableHeaderClose: tableOff,
		ErrorPrefix:      deref(sym.ErrorPrefix),
		NotePrefix:       deref(sym.NotePrefix),
		TableRule:        deref(sym.TableRule),
		Cursor:           deref(sym.Cursor),
		CursorBlank:      deref(sym.CursorBlank),
		Checked:          deref(sym.Checked),
		Unchecked:        deref(sym.Unchecked),
		Caret:            deref(sym.Caret),
		Ellipsis:         deref(sym.Ellipsis),
		CursorOpen:       cursorOn,
		CursorClose:      cursorOff,
		SelectedOpen:     selectedOn,
		SelectedClose:    selectedOff,
		FilterOpen:       filterOn,
		FilterClose:      filterOff,
	}
	if r.TableRule == "" {
		r.TableRule = "-"
	}
	if r.Name == "" {
		r.Name = "custom"
	}
	return r, nil
}

// expandedDefault is the default theme flattened once at startup; Resolve uses
// it as the base every custom theme is layered onto.
var expandedDefault = func() Theme {
	t, err := Expand(presets["default"])
	if err != nil {
		panic("theme: built-in default is invalid: " + err.Error())
	}
	return t
}()

func mustExpandDefault() Theme { return expandedDefault }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ParseStyle turns a style expression into the ANSI sequence that opens it.
//
// An expression is a space-separated list of tokens, applied in order:
//
//	attributes   bold, dim, italic, underline, reverse
//	colors       black red green yellow blue magenta cyan white,
//	             and their bright- variants
//	backgrounds  bg-<color>
//	256-color    color:0..255, bg-color:0..255
//	truecolor    #rrggbb, bg-#rrggbb
//	none         explicitly no styling
//
// An empty expression means "no styling" and returns "".
func ParseStyle(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "none" {
		return "", nil
	}
	var codes []string
	for _, tok := range strings.Fields(expr) {
		code, err := styleToken(strings.ToLower(tok))
		if err != nil {
			return "", err
		}
		if code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return "", nil
	}
	return "\x1b[" + strings.Join(codes, ";") + "m", nil
}

var attrs = map[string]string{
	"bold":      "1",
	"dim":       "2",
	"italic":    "3",
	"underline": "4",
	"reverse":   "7",
}

var baseColors = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3,
	"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
}

func styleToken(tok string) (string, error) {
	if tok == "none" {
		return "", nil
	}
	if code, ok := attrs[tok]; ok {
		return code, nil
	}
	bg := false
	if rest, found := strings.CutPrefix(tok, "bg-"); found {
		bg = true
		tok = rest
	}
	if hex, found := strings.CutPrefix(tok, "#"); found {
		r, g, b, err := parseHex(hex)
		if err != nil {
			return "", err
		}
		prefix := "38"
		if bg {
			prefix = "48"
		}
		return prefix + ";2;" + itoa(r) + ";" + itoa(g) + ";" + itoa(b), nil
	}
	if num, found := strings.CutPrefix(tok, "color:"); found {
		n, err := strconv.Atoi(num)
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("invalid 256-color index %q (want color:0..255)", num)
		}
		prefix := "38"
		if bg {
			prefix = "48"
		}
		return prefix + ";5;" + itoa(n), nil
	}
	bright := false
	if rest, found := strings.CutPrefix(tok, "bright-"); found {
		bright = true
		tok = rest
	}
	n, ok := baseColors[tok]
	if !ok {
		return "", fmt.Errorf("unknown style token %q", tok)
	}
	base := 30
	if bg {
		base = 40
	}
	if bright {
		base += 60
	}
	return itoa(base + n), nil
}

func parseHex(hex string) (r, g, b int, err error) {
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex color %q (want #rrggbb)", "#"+hex)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid hex color %q (want #rrggbb)", "#"+hex)
	}
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff, nil
}

func itoa(n int) string { return strconv.Itoa(n) }
