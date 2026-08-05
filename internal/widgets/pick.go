package widgets

// The interactive picker: arrow keys, type-to-filter, space to toggle.
//
// One engine drives select, multiselect, search and autocomplete, because from
// the terminal's point of view they are the same thing — a list, a cursor, a
// filter and a set of marks — differing only in how a match is decided and what
// Enter returns.
//
// It runs only when there is a real terminal. With a pipe, in CI, or inside an
// agent's shell, the widgets take exactly the line-based path they always have,
// which is what keeps the headless/interactive parity intact.

import (
	"context"
	"slices"
	"strings"

	"github.com/martino-vigiani/tiny/internal/keys"
	"github.com/martino-vigiani/tiny/internal/spec"
)

// matchFunc decides whether an option matches the typed filter.
type matchFunc func(option, filter string) bool

// picker is the live state of an interactive list.
type picker struct {
	options []string
	filter  string
	// shown holds the indices of options passing the filter, in order. It is
	// reused across keystrokes so filtering allocates nothing after the first.
	shown  []int
	cursor int // index into shown
	marks  []bool
	multi  bool
	match  matchFunc
	rank   bool
	custom bool
	height int
	title  string
	hint   string
}

// rich reports whether this Env should draw a live prompt: a real terminal, and
// the caller has not opted out.
func (e *Env) rich() bool {
	if !e.Rich {
		return false
	}
	// A scripted key source means a test is driving the widget; a real terminal
	// is required otherwise.
	return e.Keys != nil || e.IsTTY
}

// runPicker draws a live list and returns the chosen indices.
//
// It returns the indices rather than the values so each caller can build its
// own canonical answer type — a string for select, a []string for multiselect —
// exactly as the line-based path does.
func runPicker(ctx context.Context, env *Env, p *picker) ([]int, error) {
	source, cleanup, err := env.keySource()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	p.refilter()
	env.reset()
	env.hideCursor()
	defer env.showCursor()
	defer func() {
		// Leave the transcript on its own line, so the committed answer the
		// caller prints next does not land inside the frame — and forget this
		// frame, or the next widget would wind back into it and eat the
		// questions already answered.
		WriteUI(env, "\r\n")
		env.reset()
	}()

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.draw(env)

		key, err := source.Next()
		if err != nil {
			if keys.IsEOF(err) {
				return p.commit(), nil
			}
			return nil, err
		}

		switch key.Type {
		case keys.CtrlC:
			return nil, context.Canceled
		case keys.CtrlD:
			return nil, errAbandoned
		case keys.Enter:
			// With allow_custom the typed filter is itself an answer, so a
			// filter matching nothing still commits. Without this the live
			// prompt silently returned the default where the line-based path
			// returned the typed text — the same spec answering differently
			// depending on how it was drawn.
			if len(p.shown) == 0 && p.custom && p.filter != "" {
				return nil, errCustomAnswer
			}
			return p.commit(), nil
		case keys.Up:
			p.move(-1)
		case keys.Down:
			p.move(1)
		case keys.PageUp:
			p.move(-p.visible(env))
		case keys.PageDown:
			p.move(p.visible(env))
		case keys.Home:
			p.cursor = 0
		case keys.End:
			p.cursor = len(p.shown) - 1
		case keys.Space:
			if p.multi {
				p.toggle()
			} else {
				p.typeRune(' ')
			}
		case keys.Tab:
			if p.multi {
				p.toggle()
				p.move(1)
			}
		case keys.CtrlA:
			if p.multi {
				p.toggleAll()
			}
		case keys.Backspace:
			p.backspace()
		case keys.CtrlU:
			p.setFilter("")
		case keys.CtrlW:
			p.deleteWord()
		case keys.Esc:
			if p.filter == "" {
				return nil, context.Canceled
			}
			p.setFilter("")
		case keys.Rune:
			p.typeRune(key.Rune)
		}
	}
}

// keySource returns the keystroke source, entering raw mode when it is a real
// terminal. A test injecting keys never touches the terminal.
func (e *Env) keySource() (keys.Source, func(), error) {
	if e.Keys != nil {
		return e.Keys, func() {}, nil
	}
	session, err := keys.EnterRaw(e.StdinFd)
	if err != nil {
		return nil, nil, err
	}
	return keys.NewDecoder(e.In), func() { _ = session.Close() }, nil
}

// visible is how many options the frame may draw.
//
// The chrome it has to leave room for is everything the frame emits besides the
// options: the header, whose themed decoration carries its own newlines and by
// default opens with a blank one; the filter line; the two "..." markers that
// appear when the list is windowed; and the final line, which is not newline
// terminated. Under-counting any of these makes the frame taller than the
// screen, and once the terminal scrolls the redraw never recovers.
func (p *picker) visible(env *Env) int {
	t := env.look()
	chrome := countNewlines(t.HeaderOpen) + countNewlines(t.HeaderClose) // header
	chrome++                                                             // filter line
	chrome += 2                                                          // the "..." markers, when windowed
	chrome++                                                             // the frame's unterminated last line
	return env.visibleRows(p.height, chrome)
}

// refilter recomputes the visible options, keeping the cursor on the option it
// was on when that option survives the filter. A fuzzy step also ranks them, so
// the closest match is under the cursor rather than merely present somewhere in
// the list — which is the whole point of fuzzy matching.
func (p *picker) refilter() {
	var was int = -1
	if p.cursor >= 0 && p.cursor < len(p.shown) {
		was = p.shown[p.cursor]
	}
	p.shown = p.shown[:0]
	for i, opt := range p.options {
		if p.filter == "" || p.match(opt, p.filter) {
			p.shown = append(p.shown, i)
		}
	}
	if p.rank && p.filter != "" {
		p.sortByScore()
	}
	p.cursor = 0
	if was >= 0 {
		for i, idx := range p.shown {
			if idx == was {
				p.cursor = i
				break
			}
		}
	}
}

// sortByScore orders the visible options best-match first. It sorts the
// existing []int in place with slices.SortFunc, so ranking costs no allocation
// on a path that runs once per keystroke.
func (p *picker) sortByScore() {
	slices.SortStableFunc(p.shown, func(a, b int) int {
		sa, _ := fuzzyScore(p.options[a], p.filter)
		sb, _ := fuzzyScore(p.options[b], p.filter)
		return sb - sa
	})
}

func (p *picker) setFilter(f string) {
	p.filter = f
	p.refilter()
}

func (p *picker) typeRune(r rune) {
	p.setFilter(p.filter + string(r))
}

func (p *picker) backspace() {
	if p.filter == "" {
		return
	}
	runes := []rune(p.filter)
	p.setFilter(string(runes[:len(runes)-1]))
}

func (p *picker) deleteWord() {
	trimmed := strings.TrimRight(p.filter, " ")
	if i := strings.LastIndexByte(trimmed, ' '); i >= 0 {
		p.setFilter(trimmed[:i+1])
		return
	}
	p.setFilter("")
}

func (p *picker) move(by int) {
	if len(p.shown) == 0 {
		return
	}
	p.cursor += by
	// Wrapping only for single steps: paging past the end should stop there,
	// not teleport to the other end of a long list.
	switch {
	case by == 1 && p.cursor >= len(p.shown):
		p.cursor = 0
	case by == -1 && p.cursor < 0:
		p.cursor = len(p.shown) - 1
	case p.cursor < 0:
		p.cursor = 0
	case p.cursor >= len(p.shown):
		p.cursor = len(p.shown) - 1
	}
}

func (p *picker) toggle() {
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return
	}
	i := p.shown[p.cursor]
	p.marks[i] = !p.marks[i]
}

// toggleAll selects everything currently visible, or clears it when everything
// visible is already selected.
func (p *picker) toggleAll() {
	all := true
	for _, i := range p.shown {
		if !p.marks[i] {
			all = false
			break
		}
	}
	for _, i := range p.shown {
		p.marks[i] = !all
	}
}

// commit returns the chosen option indices.
func (p *picker) commit() []int {
	if p.multi {
		var out []int
		for i, m := range p.marks {
			if m {
				out = append(out, i)
			}
		}
		return out
	}
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return nil
	}
	return []int{p.shown[p.cursor]}
}

// draw renders one frame: the title, the filter line, the visible window of
// options, and the "N more" markers standing in for what is off-screen.
func (p *picker) draw(env *Env) {
	t := env.look()
	env.begin()

	// Title. The theme's header decoration carries its own newlines — the
	// default one opens with a blank line — so the frame counts the newlines it
	// actually emitted rather than assuming one. Getting this wrong by a single
	// line makes every later frame overwrite the wrong part of the screen.
	env.appendFramed(t.HeaderOpen)
	env.appendClipped(p.title, 0)
	env.appendFramed(t.HeaderClose)
	env.frame.rows += countNewlines(t.HeaderOpen) + countNewlines(t.HeaderClose)

	// Filter line: the prompt marker, what has been typed, and the caret.
	env.buf = append(env.buf, t.Prompt...)
	env.buf = append(env.buf, t.FilterOpen...)
	env.appendClipped(p.filter, 2)
	env.buf = append(env.buf, t.FilterClose...)
	env.buf = append(env.buf, t.Caret...)
	if p.hint != "" && p.filter == "" {
		env.buf = append(env.buf, ' ')
		env.buf = append(env.buf, t.NoteOpen...)
		env.appendClipped(p.hint, 4)
		env.buf = append(env.buf, t.NoteClose...)
	}
	env.endLine()

	if len(p.shown) == 0 {
		env.buf = append(env.buf, t.ErrorOpen...)
		env.buf = append(env.buf, "No matches"...)
		env.buf = append(env.buf, t.ErrorClose...)
		env.end()
		return
	}

	// The viewport is the same window() the line-based path uses, with the
	// cursor as the focus instead of the default.
	from, to := window(len(p.shown), p.cursor, p.visible(env))
	if from > 0 {
		env.buf = append(env.buf, t.Indent...)
		env.buf = append(env.buf, t.NoteOpen...)
		env.buf = append(env.buf, "..."...)
		env.buf = append(env.buf, t.NoteClose...)
		env.endLine()
	}
	for i := from; i < to; i++ {
		opt := p.options[p.shown[i]]
		active := i == p.cursor
		used := 0
		if active {
			env.buf = append(env.buf, t.CursorOpen...)
			env.buf = append(env.buf, t.Cursor...)
			used += len(t.Cursor)
		} else {
			env.buf = append(env.buf, t.CursorBlank...)
			used += len(t.CursorBlank)
		}
		if p.multi {
			env.buf = append(env.buf, t.SelectedOpen...)
			if p.marks[p.shown[i]] {
				env.buf = append(env.buf, t.Checked...)
				used += len(t.Checked)
			} else {
				env.buf = append(env.buf, t.Unchecked...)
				used += len(t.Unchecked)
			}
			env.buf = append(env.buf, t.SelectedClose...)
		}
		env.appendClipped(opt, used)
		if active {
			env.buf = append(env.buf, t.CursorClose...)
		}
		env.endLine()
	}
	if to < len(p.shown) {
		env.buf = append(env.buf, t.Indent...)
		env.buf = append(env.buf, t.NoteOpen...)
		env.buf = append(env.buf, "..."...)
		env.buf = append(env.buf, t.NoteClose...)
		env.endLine()
	}
	env.end()
}

// countNewlines reports how many lines a themed fragment occupies.
func countNewlines(s string) int {
	return strings.Count(s, "\n")
}

// matchSubstring and matchPrefix are the two filter behaviours, shared with the
// line-based path so a filter means the same thing in both.
func matchSubstring(option, filter string) bool {
	return strings.Contains(strings.ToLower(option), strings.ToLower(filter))
}

func matchPrefix(option, filter string) bool {
	return strings.HasPrefix(strings.ToLower(option), strings.ToLower(filter))
}

// matcherFor resolves a step's `match:` to its comparison, falling back to the
// widget's own default when the step does not say.
func matcherFor(mode string, fallback matchFunc) matchFunc {
	switch mode {
	case spec.MatchSubstring:
		return matchSubstring
	case spec.MatchPrefix:
		return matchPrefix
	case spec.MatchFuzzy:
		return fuzzyMatch
	default:
		return fallback
	}
}

// newPicker builds the picker for a step, with the marks a multiselect default
// preselects.
func newPicker(s spec.Step, marks []bool, multi bool, match matchFunc, hint string) *picker {
	if marks == nil {
		marks = make([]bool, len(s.Options))
	}
	if s.Match != "" {
		match = matcherFor(s.Match, match)
	}
	return &picker{
		rank:    s.Match == spec.MatchFuzzy,
		custom:  s.AllowCustom,
		options: s.Options,
		shown:   make([]int, 0, len(s.Options)),
		marks:   marks,
		multi:   multi,
		match:   match,
		height:  s.Height,
		title:   s.Prompt(),
		hint:    hint,
	}
}
