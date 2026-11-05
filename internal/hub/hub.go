// Package hub is flkforge's hub (SPEC 10): shared snippets and recipes in
// git repositories laid out like the built-in catalog
// (NAMESPACE/NAME/snippet.toml, NAMESPACE/_templates/NAME.toml). A hub is
// a checkout on disk: cloned from a URL into flkforge's data directory, or
// a clone the admin already has. Pulling is git clone / git pull
// --ff-only; publishing commits locally. flkforge never pushes.
package hub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Hub is one configured hub.
type Hub struct {
	Name string `toml:"name" json:"name"`
	URL  string `toml:"url,omitempty" json:"url,omitempty"`
	Path string `toml:"path,omitempty" json:"path"`
}

// Config is hubs.toml.
type Config struct {
	Hubs []Hub `toml:"hub"`
	file string
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ConfigFile is ~/.config/flkforge/hubs.toml.
func ConfigFile() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "flkforge", "hubs.toml")
}

// DataDir is where hubs cloned from a URL live.
func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "flkforge", "hubs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "flkforge", "hubs")
}

// Load reads the config (a missing file is no hubs).
func Load(file string) (*Config, error) {
	c := &Config{file: file}
	if file == "" {
		return c, nil
	}
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	md, err := toml.Decode(string(data), c)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", file, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, fmt.Errorf("%s: unknown key %s", file, und[0])
	}
	for i := range c.Hubs {
		if c.Hubs[i].Path == "" {
			c.Hubs[i].Path = filepath.Join(DataDir(), c.Hubs[i].Name)
		}
	}
	return c, nil
}

// Save writes the config.
func (c *Config) Save() error {
	var b bytes.Buffer
	b.WriteString("# flkforge's hubs: git repositories of shared snippets and recipes.\n# Change with flkforge hub add / rm.\n\n")
	for _, h := range c.Hubs {
		fmt.Fprintf(&b, "[[hub]]\n  name = %q\n", h.Name)
		if h.URL != "" {
			fmt.Fprintf(&b, "  url = %q\n", h.URL)
		}
		if h.Path != filepath.Join(DataDir(), h.Name) {
			fmt.Fprintf(&b, "  path = %q\n", h.Path)
		}
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(c.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.file, b.Bytes(), 0o644)
}

// Get finds a hub by name.
func (c *Config) Get(name string) (*Hub, error) {
	for i := range c.Hubs {
		if c.Hubs[i].Name == name {
			return &c.Hubs[i], nil
		}
	}
	var names []string
	for _, h := range c.Hubs {
		names = append(names, h.Name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no hub %q: no hubs yet (flkforge hub add NAME URL)", name)
	}
	return nil, fmt.Errorf("no hub %q (hubs: %s)", name, strings.Join(names, ", "))
}

// Add adds a hub: a URL to clone, or the path of an existing clone.
func (c *Config) Add(name, url, path string) (*Hub, error) {
	if !nameRE.MatchString(name) {
		return nil, fmt.Errorf("hub name %q: lowercase letters, digits and dashes", name)
	}
	if _, err := c.Get(name); err == nil {
		return nil, fmt.Errorf("there's already a hub %q (flkforge hub rm %s first)", name, name)
	}
	if (url == "") == (path == "") {
		return nil, fmt.Errorf("give a URL to clone, or --path to a clone you have (not both)")
	}
	h := Hub{Name: name, URL: url, Path: path}
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
			return nil, fmt.Errorf("%s isn't a git checkout", abs)
		}
		h.Path = abs
	} else {
		h.Path = filepath.Join(DataDir(), name)
	}
	c.Hubs = append(c.Hubs, h)
	return &c.Hubs[len(c.Hubs)-1], nil
}

// Remove drops a hub from the config; its checkout is left on disk.
func (c *Config) Remove(name string) (*Hub, error) {
	for i, h := range c.Hubs {
		if h.Name == name {
			c.Hubs = append(c.Hubs[:i], c.Hubs[i+1:]...)
			return &h, nil
		}
	}
	_, err := c.Get(name)
	return nil, err
}

// git runs git in dir.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Cloned says whether the hub's checkout exists.
func (h *Hub) Cloned() bool {
	_, err := os.Stat(filepath.Join(h.Path, ".git"))
	return err == nil
}

// Commit is the checkout's HEAD ("" if not cloned).
func (h *Hub) Commit() string {
	if !h.Cloned() {
		return ""
	}
	c, _ := git(h.Path, "rev-parse", "--short=12", "HEAD")
	return c
}

// Pull clones the hub, or brings its checkout up to date (fast-forward
// only: a checkout with commits of its own is left as it is).
func (h *Hub) Pull() (string, error) {
	if !h.Cloned() {
		if h.URL == "" {
			return "", fmt.Errorf("%s: %s isn't a git checkout any more", h.Name, h.Path)
		}
		if err := os.MkdirAll(filepath.Dir(h.Path), 0o755); err != nil {
			return "", err
		}
		if _, err := git(filepath.Dir(h.Path), "clone", "--quiet", h.URL, h.Path); err != nil {
			return "", err
		}
		return "cloned " + h.URL + " at " + h.Commit(), nil
	}
	if _, err := git(h.Path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
		return "no upstream to pull from: " + h.Path + " is used as it is, at " + h.Commit(), nil
	}
	before := h.Commit()
	if _, err := git(h.Path, "pull", "--quiet", "--ff-only"); err != nil {
		return "", fmt.Errorf("%v (the checkout has changes of its own? flkforge only fast-forwards)", err)
	}
	if after := h.Commit(); after != before {
		return "updated " + before + " -> " + after, nil
	}
	return "up to date at " + before, nil
}

// Hash is a snippet directory's content hash: every file's path and
// bytes, in path order. The lock records it (SPEC 10).
func Hash(fsys fs.FS, dir string) (string, error) {
	var paths []string
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return "", err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, dir), "/")
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(b))
		h.Write(b)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
