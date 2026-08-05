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
	PrintNote(env, "Editor not available, type the content directly:")
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, "")
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		if line != "" {
			return line, nil
		}
		if def, ok := s.Default.(string); ok && def != "" {
			return def, nil
		}
		if eof {
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}
		if s.Required {
			PrintError(env, "This field is required.")
			continue
		}
		return "", nil
	}
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
