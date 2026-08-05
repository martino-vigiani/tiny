package widgets

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// fileTree builds a small directory to browse.
func fileTree(t *testing.T) string {
	t.Helper()
	// Resolved, because the widget resolves symlinks to keep its containment
	// check honest — on macOS /var is a link to /private/var, so an unresolved
	// temp path would never equal what the widget answers.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving the temp dir: %v", err)
	}
	for _, name := range []string{"alpha.txt", "beta.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("making sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "inner.txt"), []byte("y"), 0o644); err != nil {
		t.Fatalf("writing inner: %v", err)
	}
	return root
}

func fileEnv(input string) (*Env, *bytes.Buffer) {
	var out bytes.Buffer
	return &Env{In: bufio.NewReader(strings.NewReader(input)), Out: &out}, &out
}

func TestFilePicksAFile(t *testing.T) {
	root := fileTree(t)
	// At the root there is no "../", so the listing is "sub/", "alpha.txt",
	// "beta.txt" — directories first, each group sorted.
	env, _ := fileEnv("2\n")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root}
	got, err := fileWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != filepath.Join(root, "alpha.txt") {
		t.Errorf("answer = %v, want the absolute path to alpha.txt", got)
	}
}

// Choosing a directory descends into it, so a file two levels down is
// reachable without any special syntax.
func TestFileDescendsIntoDirectories(t *testing.T) {
	root := fileTree(t)
	// "sub/" is first, then inside it "../" and "inner.txt".
	env, _ := fileEnv("1\n2\n")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root}
	got, err := fileWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != filepath.Join(root, "sub", "inner.txt") {
		t.Errorf("answer = %v, want the file inside sub/", got)
	}
}

// With dirs: true a directory is the answer rather than somewhere to go, since
// otherwise there would be no way to choose one at all.
func TestFileDirsModeAnswersWithTheDirectory(t *testing.T) {
	root := fileTree(t)
	env, _ := fileEnv("1\n")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "Dir", Root: root, Dirs: true}
	got, err := fileWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != filepath.Join(root, "sub") {
		t.Errorf("answer = %v, want the directory itself", got)
	}
}

func TestFileHidesDotFilesUnlessAsked(t *testing.T) {
	root := fileTree(t)
	env, out := fileEnv("\n")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root}
	if _, err := (fileWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.Contains(out.String(), ".hidden") {
		t.Errorf("listing showed a dot-file without hidden: true:\n%s", out.String())
	}

	s.Hidden = true
	env, out = fileEnv("\n")
	if _, err := (fileWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(out.String(), ".hidden") {
		t.Errorf("hidden: true did not show the dot-file:\n%s", out.String())
	}
}

// Nothing to read means the default, the same policy every other widget has.
func TestFileEOFUsesDefault(t *testing.T) {
	root := fileTree(t)
	env, _ := fileEnv("")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root, Default: "/etc/hosts"}
	got, err := fileWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "/etc/hosts" {
		t.Errorf("answer = %v, want the default", got)
	}
}

// The answer is always an absolute path, so a caller does not have to know what
// tiny's working directory was.
func TestFileAnswersAbsolutePaths(t *testing.T) {
	root := fileTree(t)
	env, _ := fileEnv("2\n")
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root}
	got, err := fileWidget{}.Run(context.Background(), s, env)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	path, _ := got.(string)
	if !filepath.IsAbs(path) {
		t.Errorf("answer = %q, want an absolute path", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("answer %q does not exist: %v", path, err)
	}
}

// The listing is confined to where it started. Without that, a spec you were
// sent could walk `../` to anywhere on the filesystem, and "a tiny spec cannot
// read a file" would be false a second way.
func TestFileCannotEscapeItsRoot(t *testing.T) {
	root := fileTree(t)
	s := spec.Step{Type: spec.TypeFile, Key: "f", Title: "File", Root: root}

	// At the root there is nowhere above to go, so no "../" is offered.
	env, out := fileEnv("\n")
	if _, err := (fileWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.Contains(out.String(), "../") {
		t.Errorf("the root listing offered a way out of itself:\n%s", out.String())
	}

	// One level down it is offered, and taking it returns to the root — which
	// again has no way up.
	env, out = fileEnv("1\n1\n\n")
	if _, err := (fileWidget{}).Run(context.Background(), s, env); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.Count(out.String(), "../") != 1 {
		t.Errorf("expected exactly one listing to offer ../ (the one below the root):\n%s",
			out.String())
	}
}
