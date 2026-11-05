package hub

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/snippet"
)

// Published says what publish did.
type Published struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"` // snippet or recipe
	Path   string   `json:"path"` // in the hub's checkout
	Branch string   `json:"branch"`
	Commit string   `json:"commit"`
	Notes  []string `json:"notes"`
}

var ipRE = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// PublishOpts: where in the hub, and on which branch.
type PublishOpts struct {
	As      string // namespace/name; default: the hub's name / the source's name
	Branch  string // default flkforge/publish-NAME (a new branch, for review)
	Current bool   // commit on the checked-out branch instead
	Title   string // for a recipe
	Summary string
}

// Publish copies a snippet directory, or an app's recipe as a template,
// into the hub's checkout and commits it there. It never pushes.
func (h *Hub) Publish(src string, o PublishOpts) (*Published, error) {
	if !h.Cloned() {
		return nil, fmt.Errorf("%s isn't cloned yet: flkforge hub pull %s", h.Name, h.Name)
	}
	if out, _ := git(h.Path, "status", "--porcelain"); out != "" {
		return nil, fmt.Errorf("%s has uncommitted changes: commit or stash them first (flkforge commits only its own)", h.Path)
	}
	p := &Published{}
	var files map[string][]byte // path in the hub -> content
	switch {
	case exists(filepath.Join(src, "snippet.toml")):
		p.Kind = "snippet"
		data, err := os.ReadFile(filepath.Join(src, "snippet.toml"))
		if err != nil {
			return nil, err
		}
		s, err := snippet.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", src, err)
		}
		p.ID = s.ID
		if o.As != "" {
			p.ID = o.As
		}
		if err := checkID(p.ID); err != nil {
			return nil, err
		}
		if p.ID != s.ID {
			data = []byte(strings.Replace(string(data), fmt.Sprintf("id = %q", s.ID), fmt.Sprintf("id = %q", p.ID), 1))
			p.Notes = append(p.Notes, "id "+s.ID+" -> "+p.ID)
		}
		files = map[string][]byte{"snippet.toml": data}
		root := os.DirFS(src)
		err = fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || path == "snippet.toml" {
				return err
			}
			b, err := fs.ReadFile(root, path)
			files[path] = b
			return err
		})
		if err != nil {
			return nil, err
		}
		p.Path = filepath.Join(h.Path, filepath.FromSlash(p.ID))
	case exists(filepath.Join(src, recipe.File)):
		p.Kind = "recipe"
		r, err := recipe.Read(src)
		if err != nil {
			return nil, err
		}
		if o.As == "" {
			o.As = h.Name + "/" + strings.TrimSuffix(strings.TrimSuffix(r.App, "-flk"), "-")
		}
		p.ID = o.As
		if err := checkID(p.ID); err != nil {
			return nil, err
		}
		// What belongs to this host or this app, not to the pattern.
		p.Notes = append(p.Notes, "app name "+r.App+" removed (each user picks theirs)")
		r.App, r.Render, r.Seeded, r.Lock, r.Template = "", nil, nil, nil, ""
		var keep []recipe.Use
		for _, u := range r.Uses {
			if u.Snippet == "core/bind" {
				p.Notes = append(p.Notes, "core/bind removed (a host address)")
				continue
			}
			for k, v := range u.Params {
				if s, ok := v.(string); ok && ipRE.MatchString(s) && !strings.Contains(s, "\n") {
					p.Notes = append(p.Notes, fmt.Sprintf("%s.%s = %q looks like an address: check it belongs in a shared recipe", u.Key(), k, s))
				}
			}
			keep = append(keep, u)
		}
		r.Uses = keep
		r.Title, r.Summary = o.Title, o.Summary
		if r.Title == "" || r.Summary == "" {
			return nil, fmt.Errorf("a published recipe needs --title and --summary (they're what hub search shows)")
		}
		files = map[string][]byte{"": []byte(r.Format())}
		ns, name, _ := strings.Cut(p.ID, "/")
		p.Path = filepath.Join(h.Path, ns, "_templates", name+".toml")
	default:
		return nil, fmt.Errorf("%s has neither a snippet.toml nor a %s", src, recipe.File)
	}

	name := strings.ReplaceAll(p.ID, "/", "-")
	p.Branch = o.Branch
	if o.Current {
		cur, err := git(h.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return nil, err
		}
		p.Branch = cur
	} else {
		if p.Branch == "" {
			p.Branch = "flkforge/publish-" + name
		}
		if _, err := git(h.Path, "rev-parse", "--verify", "--quiet", "refs/heads/"+p.Branch); err == nil {
			if _, err := git(h.Path, "checkout", "--quiet", p.Branch); err != nil {
				return nil, err
			}
		} else if _, err := git(h.Path, "checkout", "--quiet", "-b", p.Branch); err != nil {
			return nil, err
		}
	}
	// Write the files (a republished snippet replaces its directory).
	if p.Kind == "snippet" {
		if err := os.RemoveAll(p.Path); err != nil {
			return nil, err
		}
		for rel, b := range files {
			dst := filepath.Join(p.Path, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			mode := fs.FileMode(0o644)
			if st, err := os.Stat(filepath.Join(src, rel)); err == nil {
				mode = st.Mode().Perm()
			}
			if err := os.WriteFile(dst, b, mode); err != nil {
				return nil, err
			}
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p.Path, files[""], 0o644); err != nil {
			return nil, err
		}
	}
	rel, _ := filepath.Rel(h.Path, p.Path)
	if _, err := git(h.Path, "add", "--", rel); err != nil {
		return nil, err
	}
	if out, _ := git(h.Path, "status", "--porcelain"); out == "" {
		p.Commit = h.Commit()
		p.Notes = append(p.Notes, "nothing changed: the hub already has this version")
		return p, nil
	}
	msg := fmt.Sprintf("Publish %s %s (flkforge hub publish)", p.Kind, p.ID)
	if _, err := git(h.Path, "commit", "--quiet", "-m", msg); err != nil {
		return nil, err
	}
	p.Commit = h.Commit()
	return p, nil
}

func checkID(id string) error {
	ns, name, ok := strings.Cut(id, "/")
	if !ok || !nameRE.MatchString(ns) || !nameRE.MatchString(name) {
		return fmt.Errorf("%q: give namespace/name (lowercase letters, digits, dashes)", id)
	}
	if ns == "core" {
		return fmt.Errorf("namespace core is flkforge's own: publish under yours (--as NAMESPACE/NAME)")
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
