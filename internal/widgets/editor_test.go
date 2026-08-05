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
	if !strings.Contains(buf.String(), "finish with a line containing only .") {
		t.Fatalf("output missing the multi-line instructions: %q", buf.String())
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
	if !strings.Contains(buf.String(), "finish with a line containing only .") {
		t.Fatalf("output missing the multi-line instructions: %q", buf.String())
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

// The whole point of the step is text that spans lines. Without an editor it
// used to read exactly one line, which made that impossible.
func TestEditorCollectsMultipleLines(t *testing.T) {
	env, _ := newEnv("first\nsecond\n\nfourth\n.\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "first\nsecond\n\nfourth"
	if got != want {
		t.Errorf("Run = %q, want %q — blank lines inside the text must survive", got, want)
	}
}

// EOF closes the entry just like the sentinel does, so a heredoc works.
func TestEditorEndsAtEOF(t *testing.T) {
	env, _ := newEnv("alpha\nbeta\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "alpha\nbeta" {
		t.Errorf("Run = %q, want both lines", got)
	}
}

// Nothing typed falls back to the default, as every other widget does.
func TestEditorEmptyUsesDefault(t *testing.T) {
	env, _ := newEnv(".\n")
	s := spec.Step{Type: spec.TypeEditor, Key: "notes", Title: "Notes:", Default: "fallback"}
	got, err := editorWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("Run = %q, want the default", got)
	}
}
