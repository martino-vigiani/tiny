package widgets

import (
	"context"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func TestEditorPipeModeFallsBackToReadLine(t *testing.T) {
	env, buf := newEnv("multi\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:", EditorCmd: "vim"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "multi" {
		t.Fatalf("Run = %v, want %q", got, "multi")
	}
	if !strings.Contains(buf.String(), "Editor not available") {
		t.Fatalf("output missing fallback message: %q", buf.String())
	}
}

func TestEditorFallbackWithEditorFromEnv(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim")
	env, buf := newEnv("line\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "line" {
		t.Fatalf("Run = %v, want %q", got, "line")
	}
	if !strings.Contains(buf.String(), "Editor not available") {
		t.Fatalf("output missing fallback message: %q", buf.String())
	}
}

func TestEditorFallbackWithNoEditorConfigured(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	env, _ := newEnv("plain\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "plain" {
		t.Fatalf("Run = %v, want %q", got, "plain")
	}
}
