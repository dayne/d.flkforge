package cli

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dayne/d.flkforge/internal/hub"
	"github.com/dayne/d.flkforge/internal/snippet"
)

// cmdHub: hub add|rm|list|pull|search|show|publish (SPEC 10).
func cmdHub(c *ctx) int {
	sub := c.arg(0, "list")
	args := []string{}
	if len(c.pos) > 1 {
		args = c.pos[1:]
	}
	need := func(n int, usage string) bool {
		if len(args) != n {
			c.usageError("hub " + usage)
			return false
		}
		return true
	}
	env := c.env()
	cfg, err := env.HubConfig()
	if err != nil {
		return c.errorf("%v", err)
	}
	switch sub {
	case "list":
		if !need(0, "list") {
			return exitUsage
		}
		return hubList(c, cfg)
	case "add":
		if !c.has("path") && !need(2, "add NAME URL (or NAME --path DIR)") || c.has("path") && !need(1, "add NAME --path DIR") {
			return exitUsage
		}
		url := ""
		if len(args) == 2 {
			url = args[1]
		}
		h, err := cfg.Add(args[0], url, c.opt("path"))
		if err != nil {
			return c.errorf("%v", err)
		}
		if err := cfg.Save(); err != nil {
			return c.errorf("%v", err)
		}
		fmt.Fprintf(c.out, "Added hub %s (%s).\n", h.Name, h.Path)
		if !h.Cloned() {
			fmt.Fprintf(c.out, "Next: flkforge hub pull %s\n", h.Name)
		}
		return exitOK
	case "rm":
		if !need(1, "rm NAME") {
			return exitUsage
		}
		h, err := cfg.Remove(args[0])
		if err != nil {
			return c.errorf("%v", err)
		}
		if err := cfg.Save(); err != nil {
			return c.errorf("%v", err)
		}
		fmt.Fprintf(c.out, "Removed hub %s from %s; its checkout is still at %s (delete it yourself if you don't need it).\n", h.Name, hub.ConfigFile(), h.Path)
		return exitOK
	case "pull":
		var hubs []hub.Hub
		if len(args) == 1 {
			h, err := cfg.Get(args[0])
			if err != nil {
				return c.errorf("%v", err)
			}
			hubs = []hub.Hub{*h}
		} else if len(args) == 0 {
			hubs = cfg.Hubs
		} else {
			return c.usageError("hub pull [NAME]")
		}
		if len(hubs) == 0 {
			return c.errorf("no hubs yet: flkforge hub add NAME URL")
		}
		code := exitOK
		for _, h := range hubs {
			msg, err := h.Pull()
			if err != nil {
				fmt.Fprintf(c.errw, "%s: %v\n", h.Name, err)
				code = exitFail
				continue
			}
			fmt.Fprintf(c.out, "%s: %s\n", h.Name, msg)
		}
		if code == exitOK {
			if _, err := env.Catalog("."); err != nil {
				return c.errorf("the hubs pulled, but the catalog doesn't load: %v", err)
			}
		}
		return code
	case "search":
		if len(args) > 1 {
			return c.usageError("hub search [TERM]")
		}
		return hubSearch(c, strings.Join(args, " "))
	case "show":
		if !need(1, "show ID") {
			return exitUsage
		}
		return hubShow(c, args[0])
	case "publish":
		if !need(1, "publish SRC --hub NAME") {
			return exitUsage
		}
		if !c.has("hub") {
			return c.usageError("publish needs --hub NAME")
		}
		h, err := cfg.Get(c.opt("hub"))
		if err != nil {
			return c.errorf("%v", err)
		}
		p, err := h.Publish(args[0], hub.PublishOpts{As: c.opt("as"), Branch: c.opt("branch"), Current: c.has("current"), Title: c.opt("title"), Summary: c.opt("summary")})
		if err != nil {
			return c.errorf("%v", err)
		}
		if c.has("json") {
			c.printJSON(p)
			return exitOK
		}
		fmt.Fprintf(c.out, "Published %s %s to %s, committed %s on branch %s.\n", p.Kind, p.ID, p.Path, p.Commit, p.Branch)
		for _, n := range p.Notes {
			fmt.Fprintln(c.out, "  - "+n)
		}
		fmt.Fprintf(c.out, "Nothing is pushed. Next: review it (git -C %s show), then merge and push it yourself.\n", h.Path)
		return exitOK
	}
	return c.usageError(fmt.Sprintf("unknown hub subcommand %q (add, rm, list, pull, search, show, publish)", sub))
}

func hubList(c *ctx, cfg *hub.Config) int {
	type hj struct {
		hub.Hub
		Cloned bool   `json:"cloned"`
		Commit string `json:"commit,omitempty"`
	}
	out := []hj{}
	for _, h := range cfg.Hubs {
		out = append(out, hj{h, h.Cloned(), h.Commit()})
	}
	if c.has("json") {
		c.printJSON(out)
		return exitOK
	}
	if len(out) == 0 {
		fmt.Fprintln(c.out, "No hubs yet. Add one: flkforge hub add NAME URL (or NAME --path DIR for a clone you have).")
		return exitOK
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCOMMIT\tPATH\tURL")
	for _, h := range out {
		commit := h.Commit
		if !h.Cloned {
			commit = "(not pulled)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", h.Name, commit, h.Path, h.URL)
	}
	tw.Flush()
	return exitOK
}

func hubSearch(c *ctx, term string) int {
	cat, err := c.env().Catalog(".")
	if err != nil {
		return c.errorf("%v", err)
	}
	type hit struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Origin  string `json:"origin"`
	}
	hits := []hit{}
	t := strings.ToLower(term)
	match := func(fields ...string) bool {
		if t == "" {
			return true
		}
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f), t) {
				return true
			}
		}
		return false
	}
	for _, s := range cat.All() {
		if strings.HasPrefix(s.Origin, "hub ") && match(s.ID, s.Title, s.Summary, strings.Join(s.Tags, " ")) {
			hits = append(hits, hit{s.ID, "snippet", s.Title, s.Summary, s.Origin})
		}
	}
	for _, tp := range cat.Templates() {
		if strings.HasPrefix(tp.Origin, "hub ") && match(tp.ID, tp.Recipe.Title, tp.Recipe.Summary) {
			hits = append(hits, hit{tp.ID, "recipe", tp.Recipe.Title, tp.Recipe.Summary, tp.Origin})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID < hits[j].ID })
	if c.has("json") {
		c.printJSON(hits)
		return exitOK
	}
	if len(hits) == 0 {
		fmt.Fprintln(c.out, "Nothing found in the hubs (flkforge hub list; flkforge hub pull brings them up to date).")
		return exitOK
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	for _, h := range hits {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", h.ID, h.Kind, h.Origin, h.Summary)
	}
	tw.Flush()
	fmt.Fprintln(c.out, "\nRead one in full: flkforge hub show ID. A recipe starts an app: flkforge new DIR --template ID; a snippet joins one: flkforge add ID --trust.")
	return exitOK
}

// hubShow prints a hub snippet's every file, or a hub recipe, in full:
// what you're trusting when you add it.
func hubShow(c *ctx, id string) int {
	cat, err := c.env().Catalog(".")
	if err != nil {
		return c.errorf("%v", err)
	}
	if s, err := cat.Get(id); err == nil {
		if c.has("json") {
			files := map[string]string{}
			fs.WalkDir(s.FS, s.Dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					b, _ := fs.ReadFile(s.FS, p)
					files[strings.TrimPrefix(p, s.Dir+"/")] = string(b)
				}
				return nil
			})
			sum, _ := hub.Hash(s.FS, s.Dir)
			c.printJSON(map[string]any{"kind": "snippet", "id": s.ID, "version": s.Version, "origin": s.Origin, "sha256": sum, "files": files})
			return exitOK
		}
		showSnippetFiles(c, s)
		return exitOK
	}
	t, err := cat.Template(id)
	if err != nil {
		return c.errorf("nothing called %q in the catalog or the hubs", id)
	}
	if c.has("json") {
		c.printJSON(map[string]any{"kind": "recipe", "id": t.ID, "origin": t.Origin, "recipe": t.Recipe.Format()})
		return exitOK
	}
	fmt.Fprintf(c.out, "%s (recipe, from %s)\n\n%s", t.ID, t.Origin, t.Recipe.Format())
	return exitOK
}

func showSnippetFiles(c *ctx, s *snippet.Snippet) {
	sum, _ := hub.Hash(s.FS, s.Dir)
	fmt.Fprintf(c.out, "%s %s, from %s (%s)\n", s.ID, s.Version, s.Origin, sum)
	fs.WalkDir(s.FS, s.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := fs.ReadFile(s.FS, p)
		fmt.Fprintf(c.out, "\n=== %s\n%s", path.Join(s.ID, strings.TrimPrefix(p, s.Dir+"/")), b)
		if len(b) > 0 && b[len(b)-1] != '\n' {
			fmt.Fprintln(c.out)
		}
		return nil
	})
}
