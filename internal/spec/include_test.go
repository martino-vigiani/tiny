package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree writes files under a fresh directory and returns it, symlinks resolved
// because the includes resolve theirs before checking containment.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving the temp dir: %v", err)
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return root
}

func keysOf(w *Wizard) []string {
	out := make([]string, len(w.Steps))
	for i, s := range w.Steps {
		out[i] = s.Key
	}
	return out
}

func TestIncludedStepsComeFirst(t *testing.T) {
	root := tree(t, map[string]string{
		"fragments/who.yaml": `
steps:
  - type: input
    key: owner
    title: Owner
    default: platform
`,
		"deploy.yaml": `
include:
  - fragments/who.yaml
steps:
  - type: confirm
    key: apply
    title: Apply?
    default: "no"
`,
	})
	w, err := ParseFile(filepath.Join(root, "deploy.yaml"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if got := strings.Join(keysOf(w), ","); got != "owner,apply" {
		t.Errorf("steps = %s, want owner,apply", got)
	}
	if len(w.Include) != 0 {
		t.Errorf("Include survived resolution: %v", w.Include)
	}
}

// A fragment's step is answered before the including file's, so a condition
// may refer to it — which is the whole reason includes come first.
func TestIncludedAnswersAreVisibleToLaterConditions(t *testing.T) {
	root := tree(t, map[string]string{
		"frag.yaml": `
steps:
  - type: confirm
    key: db
    title: Database?
    default: "yes"
`,
		"w.yaml": `
include: [frag.yaml]
steps:
  - type: select
    key: engine
    title: Engine
    options: [postgres]
    default: postgres
    when: "db"
`,
	})
	if _, err := ParseFile(filepath.Join(root, "w.yaml")); err != nil {
		t.Fatalf("a condition on an included key was rejected: %v", err)
	}
}

// A fragment is not a wizard on its own: validating it standalone would reject
// a condition on a key the including file answers.
func TestFragmentIsValidatedOnlyOnceAssembled(t *testing.T) {
	root := tree(t, map[string]string{
		"first.yaml": `
steps:
  - type: confirm
    key: db
    title: Database?
    default: "yes"
`,
		"second.yaml": `
steps:
  - type: select
    key: engine
    title: Engine
    options: [postgres]
    default: postgres
    when: "db"
`,
		"w.yaml": `
include: [first.yaml, second.yaml]
steps:
  - type: confirm
    key: apply
    title: Apply?
    default: "no"
`,
	})
	w, err := ParseFile(filepath.Join(root, "w.yaml"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if got := strings.Join(keysOf(w), ","); got != "db,engine,apply" {
		t.Errorf("steps = %s", got)
	}
}

func TestNestedIncludes(t *testing.T) {
	root := tree(t, map[string]string{
		"a.yaml": "steps:\n  - {type: input, key: a, title: A, default: x}\n",
		"b.yaml": "include: [a.yaml]\nsteps:\n  - {type: input, key: b, title: B, default: x}\n",
		"w.yaml": "include: [b.yaml]\nsteps:\n  - {type: input, key: c, title: C, default: x}\n",
	})
	w, err := ParseFile(filepath.Join(root, "w.yaml"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if got := strings.Join(keysOf(w), ","); got != "a,b,c" {
		t.Errorf("steps = %s, want a,b,c", got)
	}
}

func TestIncludeCycleIsRejected(t *testing.T) {
	root := tree(t, map[string]string{
		"a.yaml": "include: [b.yaml]\nsteps:\n  - {type: input, key: a, title: A, default: x}\n",
		"b.yaml": "include: [a.yaml]\nsteps:\n  - {type: input, key: b, title: B, default: x}\n",
	})
	_, err := ParseFile(filepath.Join(root, "a.yaml"))
	if err == nil {
		t.Fatal("a cycle was accepted")
	}
	if !strings.Contains(err.Error(), "include") {
		t.Errorf("error does not name the feature: %v", err)
	}
}

// The whole point of the confinement: a spec you were sent can pull in files
// that were already next to it, and nothing else.
func TestIncludeCannotEscapeTheWizardDirectory(t *testing.T) {
	outside := tree(t, map[string]string{
		"secret.yaml": "steps:\n  - {type: input, key: s, title: S, default: x}\n",
	})
	root := tree(t, map[string]string{
		"w.yaml": "include: ['../escape.yaml']\nsteps:\n  - {type: input, key: a, title: A, default: x}\n",
	})
	if _, err := ParseFile(filepath.Join(root, "w.yaml")); err == nil {
		t.Fatal("../ escaped the wizard directory")
	}

	// An absolute path is refused outright, before anything is read.
	abs := tree(t, map[string]string{
		"w.yaml": "include: ['" + filepath.Join(outside, "secret.yaml") + "']\nsteps:\n  - {type: input, key: a, title: A, default: x}\n",
	})
	if _, err := ParseFile(filepath.Join(abs, "w.yaml")); err == nil {
		t.Fatal("an absolute include path was accepted")
	}

	// A symlink is the interesting case: the path is lexically inside the
	// tree and still points out of it.
	linked := tree(t, map[string]string{
		"w.yaml": "include: [link.yaml]\nsteps:\n  - {type: input, key: a, title: A, default: x}\n",
	})
	if err := os.Symlink(filepath.Join(outside, "secret.yaml"), filepath.Join(linked, "link.yaml")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if _, err := ParseFile(filepath.Join(linked, "w.yaml")); err == nil {
		t.Fatal("a symlink pointing outside the tree was followed")
	}
}

// A spec with no directory has nothing to resolve against, and guessing the
// working directory would make the same bytes mean different things.
func TestIncludeIsRefusedWithoutAFile(t *testing.T) {
	_, err := Parse([]byte("include: [other.yaml]\nsteps:\n  - {type: input, key: a, title: A}\n"))
	if err == nil {
		t.Fatal("a spec parsed from bytes was allowed to include a file")
	}
	if !strings.Contains(err.Error(), "other.yaml") {
		t.Errorf("the error does not name what it refused: %v", err)
	}
}

// Duplicate keys across files are still duplicates.
func TestIncludeCannotSmuggleADuplicateKey(t *testing.T) {
	root := tree(t, map[string]string{
		"frag.yaml": "steps:\n  - {type: input, key: a, title: A, default: x}\n",
		"w.yaml":    "include: [frag.yaml]\nsteps:\n  - {type: input, key: a, title: A2, default: x}\n",
	})
	if _, err := ParseFile(filepath.Join(root, "w.yaml")); err == nil {
		t.Fatal("a key duplicated across two files was accepted")
	}
}

// A missing include names the file the way the author wrote it.
func TestMissingIncludeIsReported(t *testing.T) {
	root := tree(t, map[string]string{
		"w.yaml": "include: [nope.yaml]\nsteps:\n  - {type: input, key: a, title: A, default: x}\n",
	})
	_, err := ParseFile(filepath.Join(root, "w.yaml"))
	if err == nil {
		t.Fatal("a missing include was accepted")
	}
	if !strings.Contains(err.Error(), "nope.yaml") {
		t.Errorf("the error does not name the missing file: %v", err)
	}
}

// The including file wins over the fragment, the way every other theme source
// layers in tiny.
func TestIncludedThemeIsTheBase(t *testing.T) {
	root := tree(t, map[string]string{
		"frag.yaml": `
theme:
  preset: minimal
  symbols:
    prompt: "frag> "
    bullet: "!"
steps:
  - {type: input, key: a, title: A, default: x}
`,
		"w.yaml": `
include: [frag.yaml]
theme:
  symbols:
    prompt: "own> "
steps:
  - {type: input, key: b, title: B, default: x}
`,
	})
	w, err := ParseFile(filepath.Join(root, "w.yaml"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if w.Theme == nil || w.Theme.Symbols.Prompt == nil || *w.Theme.Symbols.Prompt != "own> " {
		t.Errorf("the including file did not win the prompt symbol: %+v", w.Theme)
	}
	if w.Theme.Symbols.Bullet == nil || *w.Theme.Symbols.Bullet != "!" {
		t.Errorf("the fragment's theme was dropped instead of used as the base: %+v", w.Theme)
	}
}
