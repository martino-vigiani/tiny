package spec

// include: composing a wizard out of files.
//
//	include:
//	  - fragments/identity.yaml
//	  - fragments/database.yaml
//	steps:
//	  - type: confirm
//	    key: apply
//	    title: "Apply?"
//
// The questions an organisation asks the same way every time — who owns this,
// which environment, which region — are written once and included, instead of
// being copied into every wizard and drifting apart one paste at a time.
//
// Included steps land before the including file's own, in the order listed, so
// a fragment's answers are available to the `when:` conditions that follow it.
// Themes layer the same way the rest of tiny layers them: a fragment's theme is
// the base, the including file wins over it, and the person running the wizard
// still overrides both.
//
// # Why this is confined
//
// tiny's central claim is that a spec is inert: reading one cannot run a
// command, reach the network or open a file you did not point it at. `include:`
// is the one feature that reads a second file, so it is bounded on every side
// that matters:
//
//   - only relative paths, resolved against the including file's directory;
//   - never outside the directory tree of the top-level spec, symlinks
//     resolved before the check, so a link cannot smuggle a path out;
//   - no cycles, and a bounded depth and file count;
//   - unavailable at all to a spec read from stdin or written as argv, which
//     has no directory to be relative to.
//
// A spec mailed to you can therefore pull in files that were already sitting
// next to it, and nothing else.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// The bounds. They are deliberately small: a wizard that needs more than this
// is a wizard nobody can read.
const (
	maxIncludeDepth = 8
	maxIncludeFiles = 64
)

// ParseFile reads a wizard from disk, resolving any `include:` against the
// directory the file lives in, and validates the assembled result.
//
// Validation happens once, at the end. A fragment is not a wizard: its steps
// may condition on keys the including file answers earlier, so validating it
// on its own would reject specs that are perfectly well-formed once assembled.
func ParseFile(path string) (*Wizard, error) {
	w, _, err := ParseFileSource(path)
	return w, err
}

// ParseFileSource is ParseFile, also returning every byte it read, in the order
// it read them.
//
// It is what -meta hashes. Hashing only the file named on the command line
// would give two runs the same fingerprint when a fragment changed underneath
// them, which is precisely the case a fingerprint exists to catch.
func ParseFileSource(path string) (*Wizard, []byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot resolve %s: %w", path, err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	root := filepath.Dir(abs)

	state := &includeState{root: root, seen: map[string]bool{}}
	w, err := loadWizard(abs, state, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := w.Validate(); err != nil {
		return nil, nil, err
	}
	return w, state.raw, nil
}

// includeState is what the recursion has to carry: where the tree is rooted,
// and which files have already been pulled in.
type includeState struct {
	root  string
	seen  map[string]bool
	count int
	// raw is every byte read, in read order: the fingerprint of the whole
	// assembly rather than of its first file.
	raw []byte
}

// loadWizard reads one file and splices in everything it includes.
func loadWizard(path string, state *includeState, depth int) (*Wizard, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("include: nested more than %d deep at %s", maxIncludeDepth, state.rel(path))
	}
	if state.seen[path] {
		return nil, fmt.Errorf("include: %s is included twice, or includes itself", state.rel(path))
	}
	state.seen[path] = true
	state.count++
	if state.count > maxIncludeFiles {
		return nil, fmt.Errorf("include: more than %d files in one wizard", maxIncludeFiles)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", state.rel(path), err)
	}
	state.raw = append(state.raw, data...)
	var w Wizard
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", state.rel(path), err)
	}
	if len(w.Include) == 0 {
		return &w, nil
	}

	dir := filepath.Dir(path)
	steps := make([]Step, 0, len(w.Steps))
	var merged *theme.Theme
	for _, ref := range w.Include {
		target, err := state.resolve(dir, ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", state.rel(path), err)
		}
		part, err := loadWizard(target, state, depth+1)
		if err != nil {
			return nil, err
		}
		steps = append(steps, part.Steps...)
		merged = layerTheme(merged, part.Theme)
	}
	w.Steps = append(steps, w.Steps...)
	// The including file wins: it is the one the person chose to run.
	w.Theme = layerTheme(merged, w.Theme)
	w.Include = nil
	return &w, nil
}

// layerTheme puts over on top of base, tolerating either being absent.
func layerTheme(base, over *theme.Theme) *theme.Theme {
	switch {
	case over == nil:
		return base
	case base == nil:
		return over
	default:
		out := theme.Merge(*base, *over)
		return &out
	}
}

// resolve turns an include reference into an absolute path, refusing every
// shape that would let a spec read a file outside the tree it came from.
func (s *includeState) resolve(dir, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("include: empty path")
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("include %q: absolute paths are not allowed; includes are relative to the including file", ref)
	}
	target := filepath.Join(dir, ref)
	// Symlinks are resolved before the containment check, since a link is
	// exactly how a lexically innocent path leaves the tree.
	if real, err := filepath.EvalSymlinks(target); err == nil {
		target = real
	}
	if !within(target, s.root) {
		return "", fmt.Errorf("include %q: outside %s, the directory the wizard was loaded from", ref, s.root)
	}
	return target, nil
}

// within reports whether path is root or sits inside it.
func within(path, root string) bool {
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// rel names a file the way the author wrote it, so an error points at
// something recognisable rather than at an absolute path.
func (s *includeState) rel(path string) string {
	if r, err := filepath.Rel(s.root, path); err == nil {
		return r
	}
	return path
}
