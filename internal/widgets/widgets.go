// Package widgets defines the widget contract: every wizard step type is
// rendered by exactly one widget, dispatched through a global registry.
package widgets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/martino-vigiani/tiny/internal/keys"
	"github.com/martino-vigiani/tiny/internal/spec"
	"github.com/martino-vigiani/tiny/internal/theme"
)

// Env carries the IO environment shared by all widgets.
//
// In is the buffered reader for interactive input. Out is the *UI stream*: every
// prompt, list and message goes there and nothing else does — the answers are
// serialised by the runner to a separate writer, so a caller can pipe the
// payload while the human still sees the wizard. StdinFd is the file descriptor
// of the real stdin (used only when IsTTY is true, e.g. by password widgets),
// and IsTTY tells widgets whether they are talking to a terminal or to a pipe.
//
// Theme holds the compiled look; when nil, widgets fall back to the built-in
// default theme so a zero Env still renders.
type Env struct {
	In      *bufio.Reader
	Out     io.Writer
	StdinFd int
	IsTTY   bool
	Theme   *theme.Resolved

	// Rich turns on the live, redrawing prompt: arrow keys, type-to-filter,
	// space to toggle. It is false by default, so every path that does not
	// explicitly ask for it — pipes, CI, tests — behaves exactly as before.
	Rich bool

	// Keys, when set, supplies keystrokes instead of the terminal. It is the
	// seam that lets an interactive widget be tested without a terminal, and
	// setting it also means raw mode is never entered.
	Keys keys.Source

	// Size overrides terminal size detection, for tests.
	Size func() (width, height int)

	// buf is a reusable line buffer: rendering a step appends into it and
	// writes once, which keeps drawing allocation-free after the first step.
	buf   []byte
	frame frame
}

// errAbandoned is returned when the user closes the input stream (Ctrl-D) at a
// live prompt: the step is over, with whatever the default provides.
var errAbandoned = errors.New("input closed")

// errCustomAnswer signals that the live prompt committed the typed filter
// rather than an option, which only an allow_custom step permits.
var errCustomAnswer = errors.New("custom answer")

// defaultTheme is the compiled fallback for an Env with no theme. It is built
// once, on first use, and never mutated.
var defaultTheme = sync.OnceValue(func() *theme.Resolved {
	r, err := theme.Resolve(theme.Default(), false)
	if err != nil {
		panic("widgets: built-in default theme is invalid: " + err.Error())
	}
	return r
})

// look returns the env's compiled theme, or the default one when unset.
func (e *Env) look() *theme.Resolved {
	if e.Theme != nil {
		return e.Theme
	}
	return defaultTheme()
}

// maxRetainedBuf caps how much scratch space an Env keeps between steps. The
// buffer is reused so ordinary rendering never allocates, but one huge table
// should not pin its own size for the rest of the run.
const maxRetainedBuf = 8 << 10

// emit writes the pending render to the UI stream and resets the buffer,
// keeping its capacity for the next one unless it grew past maxRetainedBuf.
func (e *Env) emit() {
	if len(e.buf) == 0 {
		return
	}
	_, _ = e.Out.Write(e.buf)
	// The UI stream is buffered, and a live prompt has no natural moment when
	// the buffer fills: a frame must reach the terminal now or the user watches
	// a blank screen until something else happens to flush it.
	_ = flushOut(e)
	if cap(e.buf) > maxRetainedBuf {
		e.buf = nil
		return
	}
	e.buf = e.buf[:0]
}

// Input is a single widget: it renders one step and returns the answer.
//
// The returned value is stored in the answers map under the step's key and is
// later serialised by the runner (json/yaml/env), so widgets must return the
// canonical Go type for their step kind: string for select/input/password/
// search/editor/autocomplete, bool for confirm, float64 for number, []string
// for multiselect, map[string]string for keyvalue.
type Input interface {
	Run(ctx context.Context, s spec.Step, env *Env) (any, error)
}

// registry maps each step type to its widget. Populated by the widget
// packages through init() + Register.
var registry = map[spec.StepType]Input{}

// Register associates a widget with a step type. It panics if the type is
// already registered.
func Register(t spec.StepType, w Input) {
	if _, ok := registry[t]; ok {
		panic(fmt.Sprintf("widgets: step type %q already registered", t))
	}
	registry[t] = w
}

// savedTestWidgets remembers the production widget each type had before it was
// overridden by RegisterTest, so RestoreTestRegistry can put them back.
var savedTestWidgets = map[spec.StepType]Input{}

// RegisterTest replaces the widget registered for t with w. It panics if t is
// not registered. It is a test-only hook that lets the runner tests simulate
// real widgets while the production widgets are still stubs; do not use it in
// production code.
func RegisterTest(t spec.StepType, w Input) {
	if _, ok := registry[t]; !ok {
		panic(fmt.Sprintf("widgets: no widget registered for type %q", t))
	}
	if _, saved := savedTestWidgets[t]; !saved {
		savedTestWidgets[t] = registry[t]
	}
	registry[t] = w
}

// RestoreTestRegistry reinstalls the production widgets overridden by
// RegisterTest. It is a test-only helper.
func RestoreTestRegistry() {
	for t, w := range savedTestWidgets {
		registry[t] = w
	}
	savedTestWidgets = map[spec.StepType]Input{}
}

// RunStep dispatches a step to its registered widget.
func RunStep(ctx context.Context, s spec.Step, env *Env) (any, error) {
	w, ok := registry[s.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported step type %q", s.Type)
	}
	return w.Run(ctx, s, env)
}

// flushOut flushes env.Out when it is a *bufio.Writer so prompts appear before
// the widget blocks on input in buffered mode.
func flushOut(env *Env) error {
	if env.Out != nil {
		if bw, ok := env.Out.(*bufio.Writer); ok {
			if err := bw.Flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadLine reads a line from env.In, trimming the trailing newline. It returns
// ("", nil) when EOF is reached with no pending data; a final partial line is
// returned as-is. Any other read error is propagated.
func ReadLine(env *Env) (string, error) {
	line, err := env.In.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			if line == "" {
				return "", nil
			}
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ReadInput reads a line and tells the caller whether the stream hit EOF with
// no pending data. When eof is true, value is the empty string and the caller
// should apply the widget EOF policy instead of parsing value. The output
// buffer is flushed before reading so prompts show up in buffered mode.
func ReadInput(env *Env) (value string, eof bool, err error) {
	if err := flushOut(env); err != nil {
		return "", false, err
	}
	if _, err := env.In.Peek(1); err == io.EOF {
		return "", true, nil
	}
	line, err := ReadLine(env)
	if err != nil {
		return "", false, err
	}
	echoNewline(env)
	return line, false, nil
}

// echoNewline closes the prompt line when the input is not a terminal. On a TTY
// the user's own Enter does it; on a pipe nothing does, and everything the
// widget prints next would run into the prompt.
func echoNewline(env *Env) {
	if !env.IsTTY {
		WriteUI(env, "\n")
	}
}

// ReadPassword reads a line of input without echoing it back. When env.IsTTY
// is true it uses terminal mode and prints a trailing newline; otherwise it
// falls back to the pipe path. The output buffer is flushed first.
func ReadPassword(env *Env) (string, error) {
	value, _, err := readPasswordInput(env)
	return value, err
}

func readPasswordInput(env *Env) (value string, eof bool, err error) {
	if err := flushOut(env); err != nil {
		return "", false, err
	}
	if env.IsTTY {
		b, err := term.ReadPassword(env.StdinFd)
		WriteUI(env, "\n")
		if err != nil {
			return "", false, err
		}
		return string(b), false, nil
	}
	if _, err := env.In.Peek(1); err == io.EOF {
		return "", true, nil
	}
	line, err := ReadLine(env)
	if err != nil {
		return "", false, err
	}
	echoNewline(env)
	return line, false, nil
}

// unexpectedEOF returns the error used when a required step runs out of input.
func unexpectedEOF(key string) error {
	return fmt.Errorf("unexpected end of input for required step %q", key)
}

// appendText appends text from a spec — a title, an option, a hint — with the
// control characters removed.
//
// A spec is data from an untrusted source: it can be mailed to you, generated
// from an issue, or piped in with `curl`. Written verbatim, an option
// containing an escape sequence could retitle the window, clear the screen,
// write the clipboard (OSC 52) or forge a hyperlink. Theme escapes are appended
// separately and are unaffected — the theme comes from the person running the
// wizard, not from the spec.
func (e *Env) appendText(text string) {
	start := 0
	for i := 0; i < len(text); i++ {
		if c := text[i]; c >= 0x20 && c != 0x7f {
			continue
		}
		e.buf = append(e.buf, text[start:i]...)
		start = i + 1
	}
	e.buf = append(e.buf, text[start:]...)
}

// PrintHeader renders a step's section title through the theme.
func PrintHeader(env *Env, title string) {
	t := env.look()
	env.buf = append(env.buf, t.HeaderOpen...)
	env.appendText(title)
	env.buf = append(env.buf, t.HeaderClose...)
	env.emit()
}

// PrintList renders a numbered option list; the item at defaultIdx carries the
// theme's default marker. Pass defaultIdx = -1 for no default marker.
func PrintList(env *Env, items []string, defaultIdx int) {
	PrintListHeight(env, items, defaultIdx, 0)
}

// PrintListHeight is PrintList with a cap on how many options are drawn.
//
// A list longer than the cap is windowed around the marked item and followed by
// a count of what was left out, so the question itself does not scroll off the
// top of the terminal. Every option keeps its own number, so what the user
// types still means the same thing whether or not the list was truncated.
func PrintListHeight(env *Env, items []string, defaultIdx, height int) {
	from, to := window(len(items), defaultIdx, height)
	printList(env, items, from, to, func(i int) bool { return i == defaultIdx })
}

// PrintListMarked renders a numbered option list where every item whose entry
// in marked is true carries the theme's default marker. It is what multiselect
// uses to show which options are preselected.
func PrintListMarked(env *Env, items []string, marked []bool) {
	PrintListMarkedHeight(env, items, marked, 0)
}

// PrintListMarkedHeight is PrintListMarked with the same height cap as
// PrintListHeight.
func PrintListMarkedHeight(env *Env, items []string, marked []bool, height int) {
	first := -1
	for i, m := range marked {
		if m {
			first = i
			break
		}
	}
	from, to := window(len(items), first, height)
	printList(env, items, from, to, func(i int) bool { return i < len(marked) && marked[i] })
}

// window returns the slice of a list to draw: everything when height is zero or
// the list already fits, otherwise a run of `height` items containing focus.
func window(count, focus, height int) (from, to int) {
	if height <= 0 || count <= height {
		return 0, count
	}
	if focus < 0 {
		return 0, height
	}
	// Keep the focused item roughly centred, then clamp to the ends.
	from = focus - height/2
	if from < 0 {
		from = 0
	}
	if from+height > count {
		from = count - height
	}
	return from, from + height
}

func printList(env *Env, items []string, from, to int, isMarked func(int) bool) {
	t := env.look()
	if from > 0 {
		appendOmitted(env, t, from, "above")
	}
	for i := from; i < to; i++ {
		item := items[i]
		env.buf = append(env.buf, t.Indent...)
		if t.MarkerOpen != "" {
			env.buf = append(env.buf, t.MarkerOpen...)
		}
		if isMarked(i) {
			env.buf = append(env.buf, t.DefaultMarker...)
		} else {
			env.buf = append(env.buf, t.ItemMarker...)
		}
		if t.MarkerClose != "" {
			env.buf = append(env.buf, t.MarkerClose...)
		}
		env.buf = append(env.buf, ' ')
		env.buf = append(env.buf, t.IndexOpen...)
		env.buf = strconv.AppendInt(env.buf, int64(i+1), 10)
		env.buf = append(env.buf, t.Bullet...)
		env.buf = append(env.buf, t.IndexClose...)
		env.buf = append(env.buf, ' ')
		env.buf = append(env.buf, t.OptionOpen...)
		env.appendText(item)
		env.buf = append(env.buf, t.OptionClose...)
		env.buf = append(env.buf, '\n')
	}
	if to < len(items) {
		appendOmitted(env, t, len(items)-to, "below")
	}
	env.emit()
}

// appendOmitted draws the "N more" marker that stands in for the options a
// height cap left out.
func appendOmitted(env *Env, t *theme.Resolved, n int, where string) {
	env.buf = append(env.buf, t.Indent...)
	env.buf = append(env.buf, t.NoteOpen...)
	env.buf = append(env.buf, "… "...)
	env.buf = strconv.AppendInt(env.buf, int64(n), 10)
	env.buf = append(env.buf, " more "...)
	env.buf = append(env.buf, where...)
	env.buf = append(env.buf, t.NoteClose...)
	env.buf = append(env.buf, '\n')
}

// PrintPrompt writes the theme's input marker, optionally followed by extra
// text (a range hint, a date format, the confirm suffix) shown before the
// caret. It does not end the line: the user types right after it.
func PrintPrompt(env *Env, extra string) {
	env.buf = append(env.buf, env.look().Prompt...)
	env.buf = append(env.buf, extra...)
	env.emit()
}

// PrintError renders a validation message: the theme's error prefix and style,
// then msg on its own line.
func PrintError(env *Env, msg string) {
	t := env.look()
	env.buf = append(env.buf, t.ErrorOpen...)
	env.appendText(msg)
	env.buf = append(env.buf, t.ErrorClose...)
	env.buf = append(env.buf, '\n')
	env.emit()
}

// PrintErrorf is PrintError with formatting. Prefer PrintError on hot paths.
func PrintErrorf(env *Env, format string, a ...any) {
	PrintError(env, fmt.Sprintf(format, a...))
}

// PrintNote renders an instruction line (how to answer, what a widget expects)
// in the theme's note style.
func PrintNote(env *Env, msg string) {
	t := env.look()
	env.buf = append(env.buf, t.NoteOpen...)
	env.appendText(msg)
	env.buf = append(env.buf, t.NoteClose...)
	env.buf = append(env.buf, '\n')
	env.emit()
}

// PrintNotef is PrintNote with formatting.
func PrintNotef(env *Env, format string, a ...any) {
	PrintNote(env, fmt.Sprintf(format, a...))
}

// Hint wraps text in the theme's hint delimiters and style, for the inline
// "[default]" markers widgets append to their prompt.
func Hint(env *Env, text string) string {
	t := env.look()
	return t.HintOpen + text + t.HintClose
}

// indexFold returns the position of the first option equal to value, ignoring
// case, or -1. Widgets scan instead of indexing a lowercased map: an option
// list is a handful of entries typed once, so building the map costs more than
// the lookups ever save.
func indexFold(options []string, value string) int {
	for i, opt := range options {
		if strings.EqualFold(opt, value) {
			return i
		}
	}
	return -1
}

// WriteUI writes a raw string to the UI stream. Widgets that need a literal
// newline or an editor's passthrough output use it; everything themable should
// go through the Print* helpers instead.
func WriteUI(env *Env, s string) {
	_, _ = io.WriteString(env.Out, s)
}
