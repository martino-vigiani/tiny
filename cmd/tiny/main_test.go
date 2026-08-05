package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/theme"
)

func TestResolveThemePrecedence(t *testing.T) {
	inline := &theme.Theme{Symbols: theme.Symbols{Prompt: ptr("A ")}}

	// The wizard's own theme applies when nothing overrides it.
	got, err := resolveTheme(inline, "", false)
	if err != nil {
		t.Fatalf("resolveTheme error: %v", err)
	}
	if got.Prompt != "A " {
		t.Errorf("prompt = %q, want the wizard's own theme", got.Prompt)
	}

	// $TINY_THEME overrides the wizard.
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.yaml")
	writeFile(t, envFile, "symbols:\n  prompt: \"B \"\n")
	t.Setenv("TINY_THEME", envFile)
	got, err = resolveTheme(inline, "", false)
	if err != nil {
		t.Fatalf("resolveTheme error: %v", err)
	}
	if got.Prompt != "B " {
		t.Errorf("prompt = %q, want $TINY_THEME to win over the wizard", got.Prompt)
	}

	// -theme overrides $TINY_THEME.
	flagFile := filepath.Join(dir, "flag.yaml")
	writeFile(t, flagFile, "symbols:\n  prompt: \"C \"\n")
	got, err = resolveTheme(inline, flagFile, false)
	if err != nil {
		t.Fatalf("resolveTheme error: %v", err)
	}
	if got.Prompt != "C " {
		t.Errorf("prompt = %q, want -theme to win over $TINY_THEME", got.Prompt)
	}
}

func TestResolveThemeNoColor(t *testing.T) {
	always := &theme.Theme{Color: theme.ColorAlways}
	got, err := resolveTheme(always, "", true)
	if err != nil {
		t.Fatalf("resolveTheme error: %v", err)
	}
	if strings.Contains(got.HeaderOpen, "\x1b") {
		t.Errorf("header = %q, want -no-color to beat color: always", got.HeaderOpen)
	}

	t.Setenv("NO_COLOR", "1")
	got, err = resolveTheme(always, "", false)
	if err != nil {
		t.Fatalf("resolveTheme error: %v", err)
	}
	if strings.Contains(got.HeaderOpen, "\x1b") {
		t.Errorf("header = %q, want $NO_COLOR to beat color: always", got.HeaderOpen)
	}
}

func TestLoadTheme(t *testing.T) {
	if _, err := loadTheme("sleek"); err != nil {
		t.Errorf("loadTheme(preset) error: %v", err)
	}
	if _, err := loadTheme("nonesuch"); err == nil {
		t.Error("loadTheme(unknown name) = nil error, want a failure listing the presets")
	}
	if _, err := loadTheme("./missing.yaml"); err == nil {
		t.Error("loadTheme(missing file) = nil error, want a failure")
	}
}

func TestWriteAnswersToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := writeAnswers(path, "json", map[string]any{"name": "MyApp"}); err != nil {
		t.Fatalf("writeAnswers error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(data), `"name": "MyApp"`) {
		t.Errorf("output = %q, want the answers as JSON", data)
	}
}

// The core guarantee of the CLI: the wizard is drawn on stderr and stdout
// carries nothing but the payload, so `tiny w.yaml | jq` works while a human
// answers the questions.
func TestStdoutCarriesOnlyThePayload(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary; skipped under -short")
	}
	bin := filepath.Join(t.TempDir(), "tiny")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building tiny: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, filepath.Join("..", "..", "examples", "quickstart.yaml"))
	cmd.Stdin = strings.NewReader("2\nDemo\nn\n4\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running tiny: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Errorf("stdout = %q, want it to start with the JSON payload", stdout.String())
	}
	if strings.Contains(stdout.String(), "Language") {
		t.Errorf("stdout = %q, want the prompts kept off the payload stream", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Language") {
		t.Errorf("stderr = %q, want the wizard drawn there", stderr.String())
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func ptr(s string) *string { return &s }
