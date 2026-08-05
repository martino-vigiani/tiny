package widgets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeEditor, editorWidget{})
}

type editorWidget struct{}

func (editorWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	PrintHeader(env, s.Prompt())
	editor := s.EditorCmd
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if env.IsTTY && editor != "" {
		if v, err := runEditor(ctx, s, editor); err == nil {
			return v, nil
		}
	}
	return readMultiline(ctx, s, env)
}

// endOfText is the sentinel that closes multi-line entry: a line containing
// nothing but a single dot, the convention SMTP and `ed` have used for decades.
// A blank line cannot serve as the terminator, because blank lines are exactly
// what multi-line text is made of.
const endOfText = "."

// readMultiline collects text a line at a time until the sentinel or EOF.
//
// This is the path taken whenever no editor can be launched — no $EDITOR, no
// terminal, a pipe, CI. Before, it read a single line and returned it, so
// multi-line content was simply impossible without an editor: the one thing the
// step exists for.
func readMultiline(ctx context.Context, s spec.Step, env *Env) (any, error) {
	PrintNote(env, "Type the content; finish with a line containing only "+endOfText)
	var lines []string
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, "")
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if line == endOfText {
			break
		}
		if eof {
			if len(lines) > 0 {
				break
			}
			if def, ok := s.Default.(string); ok && def != "" {
				return def, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		lines = append(lines, line)
	}

	// Trailing blank lines are an artefact of typing, not content.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		if def, ok := s.Default.(string); ok && def != "" {
			return def, nil
		}
		if s.Required {
			PrintError(env, "This field is required.")
			return readMultiline(ctx, s, env)
		}
		return "", nil
	}
	return strings.Join(lines, "\n"), nil
}

func runEditor(ctx context.Context, s spec.Step, editor string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "tiny-editor-*")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if def, ok := s.Default.(string); ok {
		if _, err := tmp.WriteString(def); err != nil {
			tmp.Close()
			return nil, err
		}
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty editor command")
	}
	args := append(append([]string{}, fields[1:]...), path)
	cmd := exec.CommandContext(ctx, fields[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.TrimRight(string(content), "\r\n"), nil
}
