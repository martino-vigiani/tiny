package widgets

// The file step: pick a path from the filesystem.
//
// It is the one kind of interaction gum has that tiny had no answer for. The
// implementation is deliberately a list of entries rather than a browser: the
// picker already knows how to show a list, filter it and return a choice, so a
// file step is that list plus directory traversal.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeFile, fileWidget{})
}

type fileWidget struct{}

// dirSuffix marks a directory in the listing, so choosing one is visibly
// different from choosing a file even before you press enter.
const dirSuffix = "/"

// parentEntry is how the listing offers a way back up.
const parentEntry = "../"

func (fileWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := startDir(s)
	if err != nil {
		return nil, err
	}

	// The listing never rises above where it started. Without that, `../` walks
	// a spec anywhere on the filesystem — a mailed spec could enumerate a home
	// directory, and "a spec cannot read a file" would be false a second way.
	root := dir
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := listDir(dir, root, s)
		if err != nil {
			return nil, err
		}

		// The step is rewritten as an ordinary option list, so both the live
		// picker and the line-based path render it with no special handling.
		listing := s
		listing.Options = entries
		listing.Title = s.Prompt() + " — " + dir
		// The suggested path is marked when the listing is showing the
		// directory that holds it, so pressing enter accepts the suggestion —
		// otherwise a step's default was reachable headless but invisible to
		// the person the suggestion was for.
		listing.Default = suggestionIn(dir, s, entries)

		choice, err := chooseEntry(ctx, listing, env)
		if err != nil {
			return nil, err
		}
		if choice == "" {
			// Nothing chosen: fall back the way every other widget does.
			if def, ok := s.Default.(string); ok && def != "" {
				return def, nil
			}
			if s.Required {
				return nil, unexpectedEOF(s.Key)
			}
			return "", nil
		}

		switch {
		case choice == parentEntry:
			if parent := filepath.Dir(dir); within(parent, root) {
				dir = parent
			}
		case strings.HasSuffix(choice, dirSuffix):
			child := resolve(dir, strings.TrimSuffix(choice, dirSuffix))
			if !within(child, root) {
				// A symlink pointing outside: cleaning the path lexically would
				// not have caught it.
				PrintError(env, "That directory is outside the listing root.")
				continue
			}
			// A step configured to pick directories answers with the directory
			// rather than descending into it; there would otherwise be no way
			// to choose one.
			if s.Dirs {
				return child, nil
			}
			dir = child
		default:
			chosen := resolve(dir, choice)
			if !within(chosen, root) {
				PrintError(env, "That path is outside the listing root.")
				continue
			}
			return chosen, nil
		}
	}
}

// suggestionIn returns the listing entry matching the step's default, when the
// listing is showing the directory that default lives in.
func suggestionIn(dir string, s spec.Step, entries []string) any {
	def, ok := s.Default.(string)
	if !ok || def == "" {
		return nil
	}
	abs, err := filepath.Abs(def)
	if err != nil || filepath.Dir(abs) != dir {
		return nil
	}
	name := filepath.Base(abs)
	for _, e := range entries {
		if e == name || e == name+dirSuffix {
			return e
		}
	}
	return nil
}

// chooseEntry runs one directory listing through the ordinary select machinery.
func chooseEntry(ctx context.Context, listing spec.Step, env *Env) (string, error) {
	listing.Type = spec.TypeSelect
	value, err := RunStep(ctx, listing, env)
	if err != nil {
		return "", err
	}
	choice, _ := value.(string)
	return choice, nil
}

// startDir resolves where the listing begins: the step's root, or the working
// directory. A default path starts the listing in the directory holding it, so
// a step suggesting a file opens where that file lives.
func startDir(s spec.Step) (string, error) {
	root := s.Root
	if root == "" {
		if def, ok := s.Default.(string); ok && def != "" {
			root = filepath.Dir(def)
		}
	}
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}

// resolve joins a listing entry to its directory and follows symlinks, so a
// link out of the tree is visible to the containment check rather than hidden
// by lexical cleaning.
func resolve(dir, name string) string {
	joined := filepath.Join(dir, name)
	if real, err := filepath.EvalSymlinks(joined); err == nil {
		return real
	}
	return joined
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

// listDir returns the entries to offer, directories first and each sorted by
// name — the order a person expects, and stable across filesystems, which the
// order os.ReadDir happens to return is not. `../` appears only below the root,
// so the listing cannot walk out of where it started.
func listDir(dir, root string, s spec.Step) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(entries))
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !s.Hidden && strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, name+dirSuffix)
			continue
		}
		// A step picking directories still shows files as context would be
		// misleading: it would offer choices it cannot answer with.
		if !s.Dirs {
			files = append(files, name)
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)

	out := make([]string, 0, len(dirs)+len(files)+1)
	if dir != root {
		out = append(out, parentEntry)
	}
	out = append(out, dirs...)
	out = append(out, files...)
	return out, nil
}
