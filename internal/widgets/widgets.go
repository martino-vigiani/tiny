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

	// buf is a reusable line buffer: rendering a step appends into it and
	// writes once, which keeps drawing allocation-free after the first step.
	buf []byte
}

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

// PrintHeader renders a step's section title through the theme.
func PrintHeader(env *Env, title string) {
	t := env.look()
	env.buf = append(env.buf, t.HeaderOpen...)
	env.buf = append(env.buf, title...)
	env.buf = append(env.buf, t.HeaderClose...)
	env.emit()
}

// PrintList renders a numbered option list; the item at defaultIdx carries the
// theme's default marker. Pass defaultIdx = -1 for no default marker.
func PrintList(env *Env, items []string, defaultIdx int) {
	printList(env, items, func(i int) bool { return i == defaultIdx })
}

// PrintListMarked renders a numbered option list where every item whose entry
// in marked is true carries the theme's default marker. It is what multiselect
// uses to show which options are preselected.
func PrintListMarked(env *Env, items []string, marked []bool) {
	printList(env, items, func(i int) bool { return i < len(marked) && marked[i] })
}

func printList(env *Env, items []string, isMarked func(int) bool) {
	t := env.look()
	for i, item := range items {
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
		env.buf = append(env.buf, item...)
		env.buf = append(env.buf, t.OptionClose...)
		env.buf = append(env.buf, '\n')
	}
	env.emit()
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
	env.buf = append(env.buf, msg...)
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
	env.buf = append(env.buf, msg...)
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

// WriteUI writes a raw string to the UI stream. Widgets that need a literal
// newline or an editor's passthrough output use it; everything themable should
// go through the Print* helpers instead.
func WriteUI(env *Env, s string) {
	_, _ = io.WriteString(env.Out, s)
}
