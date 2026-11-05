package cli

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
	"github.com/dayne/d.flkforge/internal/source"
)

// cmdSnippets: `snippets [list]` or `snippets show ID`.
func cmdSnippets(c *ctx) int {
	sub := c.arg(0, "list")
	switch sub {
	case "list":
		if len(c.pos) > 1 {
			return c.usageError("snippets list takes no argument")
		}
		return listSnippets(c)
	case "show":
		if len(c.pos) != 2 {
			return c.usageError("snippets show ID")
		}
		return showSnippet(c, c.pos[1])
	}
	return c.usageError(fmt.Sprintf("unknown subcommand %q (list, show)", sub))
}

func (c *ctx) catalog() (*source.Catalog, bool) {
	dir := c.opt("dir")
	if dir == "" {
		dir = "."
	}
	cat, err := c.env().Catalog(dir)
	if err != nil {
		c.errorf("%v", err)
		return nil, false
	}
	return cat, true
}

func listSnippets(c *ctx) int {
	cat, ok := c.catalog()
	if !ok {
		return exitFail
	}
	want := c.opt("category")
	if want != "" && !slices.Contains(snippet.Categories, want) {
		return c.usageError(fmt.Sprintf("category %q isn't one of %s", want, strings.Join(snippet.Categories, ", ")))
	}
	var list []*snippet.Snippet
	for _, s := range cat.All() {
		if want == "" || s.Category == want {
			list = append(list, s)
		}
	}
	slices.SortStableFunc(list, func(a, b *snippet.Snippet) int {
		return slices.Index(snippet.Categories, a.Category) - slices.Index(snippet.Categories, b.Category)
	})
	if c.has("json") {
		out := []snippetJSON{}
		for _, s := range list {
			out = append(out, toJSON(s))
		}
		c.printJSON(out)
		return exitOK
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCATEGORY\tSUMMARY")
	for _, s := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.Category, s.Summary)
	}
	tw.Flush()
	fmt.Fprintln(c.out, "\nMore: flkforge explain ID (what it does), flkforge snippets show ID (its params)")
	return exitOK
}

type paramJSON struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Default  any      `json:"default,omitempty"`
	Required bool     `json:"required"`
	Values   []string `json:"values,omitempty"`
	Validate string   `json:"validate,omitempty"`
	Min      *int64   `json:"min,omitempty"`
	Max      *int64   `json:"max,omitempty"`
	Prompt   string   `json:"prompt"`
	Hint     string   `json:"hint,omitempty"`
}

type snippetJSON struct {
	ID        string        `json:"id"`
	Version   string        `json:"version"`
	Title     string        `json:"title"`
	Category  string        `json:"category"`
	Summary   string        `json:"summary"`
	Multi     bool          `json:"multi"`
	Requires  []string      `json:"requires,omitempty"`
	Conflicts []string      `json:"conflicts,omitempty"`
	Suggests  []string      `json:"suggests,omitempty"`
	Source    string        `json:"source,omitempty"`
	Origin    string        `json:"origin"`
	Params    []paramJSON   `json:"params"`
	Hints     snippet.Hints `json:"hints"`
}

func toJSON(s *snippet.Snippet) snippetJSON {
	j := snippetJSON{ID: s.ID, Version: s.Version, Title: s.Title, Category: s.Category, Summary: s.Summary,
		Multi: s.Multi, Requires: s.Requires, Conflicts: s.Conflicts, Suggests: s.Suggests, Source: s.Source,
		Origin: s.Origin, Params: []paramJSON{}, Hints: s.Hints}
	for _, n := range s.ParamOrder {
		p := s.Params[n]
		j.Params = append(j.Params, paramJSON{n, p.Type, p.Default, p.Required, p.Values, p.Validate, p.Min, p.Max, p.Prompt, p.Hint})
	}
	return j
}

func showSnippet(c *ctx, id string) int {
	cat, ok := c.catalog()
	if !ok {
		return exitFail
	}
	s, err := cat.Get(id)
	if err != nil {
		return c.errorf("%v", err)
	}
	if c.has("json") {
		c.printJSON(toJSON(s))
		return exitOK
	}
	w := c.out
	fmt.Fprintf(w, "%s %s: %s (%s)\n%s\n", s.ID, s.Version, s.Title, s.Category, s.Summary)
	if s.Multi {
		fmt.Fprintln(w, "Can be used more than once: give each use an id (flkforge add "+s.Name()+" --id NAME).")
	}
	for _, rel := range []struct {
		name string
		l    []string
	}{{"Requires", s.Requires}, {"Conflicts with", s.Conflicts}, {"Goes well with", s.Suggests}} {
		if len(rel.l) > 0 {
			fmt.Fprintf(w, "%s: %s\n", rel.name, strings.Join(rel.l, ", "))
		}
	}
	if len(s.ParamOrder) == 0 {
		fmt.Fprintln(w, "\nNo params.")
	} else {
		fmt.Fprintln(w, "\nParams (flkforge add "+s.Name()+" --set NAME=VALUE; flkforge set USE NAME=VALUE):")
		for _, n := range s.ParamOrder {
			p := s.Params[n]
			ty := p.Type
			if len(p.Values) > 0 {
				ty += " (" + strings.Join(p.Values, "|") + ")"
			}
			def := ""
			switch {
			case p.Required:
				def = "required"
			case p.Default != nil:
				def = "default " + fmt.Sprint(p.Default)
			default:
				def = "optional"
			}
			fmt.Fprintf(w, "  %s  %s, %s\n      %s\n", n, ty, def, p.Prompt)
			if p.Hint != "" {
				fmt.Fprintf(w, "      %s\n", p.Hint)
			}
		}
	}
	fmt.Fprintf(w, "\nflkforge explain %s: what it does under the hood.\n", s.Name())
	return exitOK
}

func cmdTemplates(c *ctx) int {
	cat, ok := c.catalog()
	if !ok {
		return exitFail
	}
	type tj struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Summary  string   `json:"summary"`
		Snippets []string `json:"snippets"`
		Origin   string   `json:"origin"`
	}
	out := []tj{}
	for _, t := range cat.Templates() {
		j := tj{ID: t.ID, Title: t.Recipe.Title, Summary: t.Recipe.Summary, Origin: t.Origin}
		for _, u := range t.Recipe.Uses {
			j.Snippets = append(j.Snippets, u.Key())
		}
		out = append(out, j)
	}
	if c.has("json") {
		c.printJSON(out)
		return exitOK
	}
	for _, t := range cat.Templates() {
		fmt.Fprintf(c.out, "%s: %s\n  %s\n  snippets:\n", t.ID, t.Recipe.Title, t.Recipe.Summary)
		for _, u := range t.Recipe.Uses {
			var ps []string
			for _, k := range sortedParamKeys(u.Params) {
				ps = append(ps, k+"="+fmt.Sprint(u.Params[k]))
			}
			line := "    " + u.Key()
			if len(ps) > 0 {
				line += "  (" + strings.Join(ps, ", ") + ")"
			}
			fmt.Fprintln(c.out, line)
		}
		fmt.Fprintln(c.out)
	}
	fmt.Fprintln(c.out, "Start one: flkforge new DIR --template ID --yes [--set USE.PARAM=VALUE ...]")
	return exitOK
}

// cmdExplain: a snippet, a rule, a limit (an ask that isn't a snippet), or
// a flk.toml key.
func cmdExplain(c *ctx) int {
	what := c.pos[0]
	cat, ok := c.catalog()
	if !ok {
		return exitFail
	}
	if s, err := cat.Get(what); err == nil {
		return explainSnippet(c, s)
	}
	for _, r := range render.Rules {
		if r.ID == what {
			if c.has("json") {
				c.printJSON(map[string]any{"kind": "rule", "rule": r})
				return exitOK
			}
			fmt.Fprintf(c.out, "rule %s (%s)\n%s\n\nWhy: %s\n", r.ID, r.Level, r.Summary, r.Why)
			return exitOK
		}
	}
	if l, ok := cat.Limit(what); ok {
		if c.has("json") {
			c.printJSON(map[string]any{"kind": "limit", "limit": l})
			return exitOK
		}
		fmt.Fprintf(c.out, "%s: %s\n%s\n\nWhy: %s\nInstead: %s\n", l.ID, l.Ask, limitGroup(l.Group), l.Why, l.Instead)
		return exitOK
	}
	return explainKey(c, cat, what)
}

func limitGroup(g string) string {
	return map[string]string{
		"flk":        "flk does this already: no snippet needed.",
		"root":       "Needs a root step on the host (flkforge shows it, never runs it).",
		"flk-change": "Needs a change in flk.",
		"impossible": "Not possible on Firecracker.",
	}[g]
}

func explainSnippet(c *ctx, s *snippet.Snippet) int {
	if c.has("json") {
		c.printJSON(map[string]any{"kind": "snippet", "snippet": toJSON(s), "readme": s.Readme})
		return exitOK
	}
	w := c.out
	h := s.Hints
	fmt.Fprintf(w, "%s: %s\n\n%s\n", s.ID, s.Title, h.What)
	if h.UnderTheHood != "" {
		fmt.Fprintf(w, "\nUnder the hood:\n%s\n", indent(strings.TrimSpace(h.UnderTheHood), "  "))
	}
	if len(h.Gotchas) > 0 {
		fmt.Fprintln(w, "\nGotchas:")
		for _, g := range h.Gotchas {
			fmt.Fprintln(w, "  - "+g)
		}
	}
	if len(h.Docs) > 0 {
		fmt.Fprintln(w, "\nRead more:")
		for _, d := range h.Docs {
			fmt.Fprintln(w, "  "+d)
		}
	}
	if s.Source != "" {
		fmt.Fprintf(w, "\nLearned from (apps on flkforge's reference host): %s\n", s.Source)
	}
	if s.Readme != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(s.Readme))
	}
	fmt.Fprintf(w, "\nParams: flkforge snippets show %s\n", s.Name())
	return exitOK
}

// explainKey: what flk says about a flk.toml key (its own reference, read
// live so it can't go stale), and which snippets set it.
func explainKey(c *ctx, cat *source.Catalog, key string) int {
	key = strings.Trim(key, "[]")
	last := key[strings.LastIndex(key, ".")+1:]
	var snippets []string
	setRE := regexp.MustCompile(`(?m)^\s*(` + regexp.QuoteMeta(last) + `\s*=|\[\[?[a-z_.]*\b` + regexp.QuoteMeta(last) + `\b)`)
	for _, s := range cat.All() {
		for _, f := range s.FlkToml {
			if setRE.MatchString(f.Text) && !slices.Contains(snippets, s.ID) {
				snippets = append(snippets, s.ID)
			}
		}
	}
	ref, refErr := flkConfigReference(c, key, last)
	if len(ref) == 0 && len(snippets) == 0 {
		if hits := searchHints(cat, key); len(hits) > 0 {
			if c.has("json") {
				c.printJSON(map[string]any{"kind": "search", "query": key, "mentions": hits})
				return exitOK
			}
			fmt.Fprintf(c.out, "%q isn't a snippet, rule, limit or flk.toml key; it's mentioned in:\n", key)
			for _, h := range hits {
				fmt.Fprintln(c.out, "  "+h)
			}
			return exitOK
		}
		if refErr != nil {
			return c.errorf("nothing called %q: not a snippet, rule or limit, and flk's reference isn't readable (%v)", key, refErr)
		}
		return c.errorf("nothing called %q: not a snippet, rule, limit or flk.toml key (flkforge snippets, flk help --all)", key)
	}
	if c.has("json") {
		c.printJSON(map[string]any{"kind": "key", "key": key, "flk_reference": ref, "snippets": snippets})
		return exitOK
	}
	fmt.Fprintf(c.out, "flk.toml key %s\n", key)
	if len(ref) > 0 {
		fmt.Fprintln(c.out, "\nflk's reference (flk help --all, Config):")
		for _, r := range ref {
			fmt.Fprintln(c.out, indent(r, "  "))
		}
	} else if refErr != nil {
		fmt.Fprintf(c.out, "\n(flk's reference isn't readable: %v)\n", refErr)
	}
	if len(snippets) > 0 {
		fmt.Fprintf(c.out, "\nSet by: %s\n", strings.Join(snippets, ", "))
	}
	return exitOK
}

// flkConfigReference returns the bullets of `flk help --all`'s Config
// section that mention the key.
func flkConfigReference(c *ctx, key, last string) ([]string, error) {
	e := c.env()
	if e.Flk == nil {
		return nil, e.FlkErr
	}
	text, err := e.Flk.HelpAllText()
	if err != nil {
		return nil, err
	}
	start := strings.Index(text, "\n## Config")
	if start < 0 {
		return nil, fmt.Errorf("no Config section in flk help --all")
	}
	section := text[start+1:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	var bullets []string
	var cur bytes.Buffer
	flush := func() {
		if cur.Len() > 0 {
			bullets = append(bullets, strings.TrimRight(cur.String(), "\n"))
			cur.Reset()
		}
	}
	for _, l := range strings.Split(section, "\n") {
		switch {
		case strings.HasPrefix(l, "- "):
			flush()
			cur.WriteString(l + "\n")
		case strings.HasPrefix(l, "  ") && cur.Len() > 0:
			cur.WriteString(l + "\n")
		default:
			flush()
		}
	}
	flush()
	var match []string
	for _, b := range bullets {
		for _, k := range []string{"`" + key + "`", "`" + last + "`", "`[" + key + "]`", "`[[" + key + "]]`", "[" + key + "]"} {
			if strings.Contains(b, k) {
				match = append(match, b)
				break
			}
		}
	}
	return match, nil
}

func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func sortedParamKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// searchHints finds snippets and limits whose text mentions q (a word such
// as "daemonize" also finds "daemon").
func searchHints(cat *source.Catalog, q string) []string {
	stem := strings.ToLower(q)
	if len(stem) > 5 {
		stem = stem[:len(stem)-3]
	}
	var hits []string
	for _, s := range cat.All() {
		texts := append([]string{s.Summary, s.Hints.What, s.Hints.UnderTheHood}, s.Hints.Gotchas...)
		for _, n := range s.ParamOrder {
			texts = append(texts, s.Params[n].Hint)
		}
		for _, t := range texts {
			if strings.Contains(strings.ToLower(t), stem) {
				hits = append(hits, s.ID+": "+sentence(t, stem))
				break
			}
		}
	}
	for _, l := range cat.Limits() {
		for _, t := range []string{l.Ask, l.Why, l.Instead} {
			if strings.Contains(strings.ToLower(t), stem) {
				hits = append(hits, l.ID+" (flkforge explain "+l.ID+"): "+l.Ask)
				break
			}
		}
	}
	return hits
}

// sentence returns the sentence of t that mentions stem.
func sentence(t, stem string) string {
	t = strings.Join(strings.Fields(t), " ")
	for _, s := range strings.SplitAfter(t, ". ") {
		if strings.Contains(strings.ToLower(s), stem) {
			return strings.TrimSpace(s)
		}
	}
	return t
}
