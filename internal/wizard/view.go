package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dayne/d.flkforge/internal/snippet"
)

var (
	stBold    = lipgloss.NewStyle().Bold(true)
	stDim     = lipgloss.NewStyle().Faint(true)
	stFocus   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stHi      = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	stCursor  = lipgloss.NewStyle().Reverse(true)
	stTab     = lipgloss.NewStyle().Reverse(true).Bold(true)
	stHeader  = lipgloss.NewStyle().Bold(true).Reverse(true)
	stStatus  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	cursorFn  = func(s string) string { return stCursor.Render(s) }
	wideWidth = 120
)

// View is the screen, in the alternate screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.Render())
	v.AltScreen = true
	return v
}

// Render draws the whole screen: exactly width x height cells.
func (m *Model) Render() string {
	w, h := m.width, m.height
	if w < 60 || h < 16 {
		return strings.Join(fit([]string{"flkforge needs a terminal of at least 60x16 (80x24 or more is better)."}, w, h), "\n")
	}
	if m.phase != editing {
		return m.renderReport(w, h)
	}
	if m.help {
		return m.renderHelp(w, h)
	}
	body := h - 2
	var lines []string
	lines = append(lines, m.header(w))
	hintW := w
	hintH := min(len(m.hintLines(hintW, 0)), []int{max(4, body/3), 4, 2}[m.hint])
	hintH = max(hintH, 2)
	if w >= wideWidth {
		topH := body - hintH
		stepsW := 20
		formW := (w - stepsW) * 40 / 100
		prevW := w - stepsW - formW
		steps := fit(m.stepLines(), stepsW, topH)
		form := fit(m.formLines(formW-1, topH), formW, topH)
		prev := fit(m.previewLines(prevW-2, topH), prevW-2, topH)
		for i := 0; i < topH; i++ {
			lines = append(lines, steps[i]+form[i]+stDim.Render("│ ")+prev[i])
		}
	} else {
		lines = append(lines, fit([]string{m.stepBar(w)}, w, 1)...)
		rest := body - 1 - hintH
		formH := rest * 55 / 100
		prevH := rest - formH
		lines = append(lines, fit(m.formLines(w, formH), w, formH)...)
		lines = append(lines, fit(m.previewLines(w, prevH), w, prevH)...)
	}
	lines = append(lines, fit(m.hintLines(w, hintH), w, hintH)...)
	lines = append(lines, fit([]string{m.footer()}, w, 1)...)
	return strings.Join(fit(lines, w, h), "\n")
}

// fit makes exactly h lines of exactly w cells: truncated or padded.
func fit(lines []string, w, h int) []string {
	out := make([]string, 0, h)
	for _, l := range lines {
		if len(out) == h {
			break
		}
		if ansi.StringWidth(l) > w {
			l = ansi.Truncate(l, w, "…")
		}
		out = append(out, l+strings.Repeat(" ", max(0, w-ansi.StringWidth(l))))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

// wrap breaks text into lines of at most w cells, at spaces.
func wrap(text string, w int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= w:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}

func (m *Model) header(w int) string {
	mode := "new"
	if m.o.Edit {
		mode = "edit"
	}
	left := fmt.Sprintf(" flkforge %s · %s · %s ", mode, m.r.App, shortDir(m.o.Dir, max(12, w/3)))
	right := " " + m.verdict() + " "
	pad := max(1, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	return stHeader.Render(left + strings.Repeat(" ", pad) + right)
}

// verdict is the recipe's state, for the header.
func (m *Model) verdict() string {
	if m.rerr != nil {
		if n := len(m.paramErrors()); n > 0 {
			return fmt.Sprintf("✗ %d to fill in", n)
		}
		return "✗ doesn't render"
	}
	errs, warns := 0, 0
	for _, f := range m.res.Findings {
		switch f.Level {
		case "error":
			errs++
		case "warn":
			warns++
		}
	}
	switch {
	case errs > 0:
		return fmt.Sprintf("✗ %d rule errors", errs)
	case warns > 0:
		return fmt.Sprintf("! %d warnings", warns)
	}
	return "✓ renders"
}

func (m *Model) stepLines() []string {
	probs := m.stepProblems()
	out := []string{stBold.Render(" Steps"), ""}
	for i, st := range m.steps {
		mark := " "
		if probs[st.name] > 0 {
			mark = stErr.Render("!")
		}
		name := fmt.Sprintf("%2d %s", i+1, st.name)
		if i == m.step {
			out = append(out, mark+stFocus.Render("›"+name))
		} else {
			out = append(out, mark+" "+name)
		}
	}
	return append(out, "", stDim.Render(" ctrl+n next"), stDim.Render(" ctrl+p back"))
}

func (m *Model) stepBar(w int) string {
	st := m.cur()
	s := fmt.Sprintf(" Step %d/%d: %s", m.step+1, len(m.steps), stFocus.Render(st.name))
	if m.step > 0 {
		s += stDim.Render("   ‹ " + m.steps[m.step-1].name)
	}
	if m.step < len(m.steps)-1 {
		s += stDim.Render("   " + m.steps[m.step+1].name + " ›")
	}
	if n := len(m.paramErrors()); n > 0 {
		var where []string
		for _, s := range m.steps {
			if m.stepProblems()[s.name] > 0 {
				where = append(where, s.name)
			}
		}
		s += stErr.Render("   ! " + strings.Join(where, ", "))
	}
	return s
}

// formLines draws the current step's form (or the picker), scrolled so the
// focused row shows.
func (m *Model) formLines(w, h int) []string {
	if m.picker != nil {
		return m.pickerLines(w, h)
	}
	st := m.cur()
	head := []string{" " + stBold.Render(st.name) + stDim.Render("  "+stepIntro[st.name]), ""}
	if st.name == "Review" {
		// The button first, so it shows however much there is to review.
		out := append(head, m.rowLine(m.rows[0], m.focus == 0, 0, nil))
		return append(out, m.reviewLines(w)...)
	}
	errs := m.paramErrors()
	label := 0
	for _, rw := range m.rows {
		if rw.kind == rowParam {
			label = max(label, len(rw.param))
		}
	}
	label = min(label, 18)
	var rows []string
	for i, rw := range m.rows {
		rows = append(rows, m.rowLine(rw, i == m.focus, label, errs))
	}
	avail := max(1, h-len(head))
	if m.focus < m.offset {
		m.offset = m.focus
	}
	if m.focus >= m.offset+avail {
		m.offset = m.focus - avail + 1
	}
	end := min(len(rows), m.offset+avail)
	return append(head, rows[m.offset:end]...)
}

var stepIntro = map[string]string{
	"Start":         "a template to start from",
	"Name":          "the app's name",
	"Base":          "the image it starts from",
	"Software":      "how the program gets in",
	"Process":       "how it runs, stops and restarts",
	"User & data":   "who runs it; what survives a deploy",
	"Network":       "ports",
	"Health":        "what a deploy waits for",
	"Backups":       "how a backup keeps it from writing",
	"Resources":     "CPUs, memory, disk, misc",
	"Env & secrets": "settings and secret names",
	"Review":        "the result, then write",
}

func (m *Model) rowLine(rw row, focused bool, label int, errs map[string]string) string {
	pre := "  "
	if focused {
		pre = stFocus.Render("› ")
	}
	switch rw.kind {
	case rowTemplate:
		t, err := m.o.Catalog.Template(rw.tpl)
		if err != nil {
			return pre + rw.tpl
		}
		dot := "○"
		if m.r.Template == t.ID {
			dot = "●"
		}
		name := strings.TrimPrefix(t.ID, "core/")
		if focused {
			name = stFocus.Render(name)
		}
		return pre + dot + " " + name + stDim.Render("  "+t.Recipe.Title)
	case rowApp:
		v := m.app.view(focused, cursorFn)
		line := pre + "app name  " + v
		if m.app.err != "" {
			line += stErr.Render("  ✗ " + m.app.err)
		} else if where := m.taken[m.r.App]; where != "" {
			line += stWarn.Render("  ! already registered, from " + where)
		}
		return line
	case rowUse:
		u, s := m.snippetOf(rw.use)
		if u == nil || s == nil {
			return pre + "■ " + rw.use
		}
		name := rw.use
		if name != s.Name() {
			name += " (" + s.ID + ")"
		}
		line := pre + stBold.Render("■ "+name) + stDim.Render("  "+s.Title)
		if focused && s.Category != "base" {
			line += stDim.Render("  · ctrl+d removes")
		}
		return line
	case rowParam:
		return pre + "  " + fmt.Sprintf("%-*s", label, rw.param) + "  " + m.paramValue(rw, focused, errs)
	case rowAdd:
		t := "+ " + m.cur().add
		if focused {
			t = stFocus.Render(t)
		}
		return pre + t
	case rowWrite:
		t := "[ Write the files to " + shortDir(m.o.Dir, 32) + " ]"
		if ok, _ := m.canWrite(); !ok {
			return pre + stDim.Render(t+"  (not yet)")
		}
		if focused {
			t = stTab.Render(t)
		}
		return pre + t
	}
	return pre
}

// paramValue shows a param's value, its default, or that it's needed.
func (m *Model) paramValue(rw row, focused bool, errs map[string]string) string {
	_, s := m.snippetOf(rw.use)
	if s == nil {
		return ""
	}
	p := s.Params[rw.param]
	f := m.paramField(rw.use, rw.param)
	var v string
	switch {
	case multiline(m, rw):
		u, _ := m.snippetOf(rw.use)
		text, _ := u.Params[rw.param].(string)
		if text == "" {
			text, _ = p.Default.(string)
		}
		v = stDim.Render(fmt.Sprintf("(%d lines: ctrl+o edits them in your editor)", strings.Count(strings.TrimRight(text, "\n"), "\n")+1))
	case p.Type == "bool" || p.Type == "enum":
		cur := f.String()
		shown := cur
		if cur == "" {
			if p.Default == nil {
				shown = stDim.Render("(optional)")
			} else {
				shown = stDim.Render("default: " + fmt.Sprint(p.Default))
			}
		}
		if focused {
			v = stFocus.Render("‹ ") + shown + stFocus.Render(" ›")
		} else {
			v = shown
		}
	case f.String() == "" && !focused:
		switch {
		case p.Required:
			v = stErr.Render("(required)")
		case p.Default != nil && fmt.Sprint(p.Default) != "" && fmt.Sprint(p.Default) != "[]":
			v = stDim.Render("default: " + snippet.FormatValue(p, p.Default))
		default:
			v = stDim.Render("(optional)")
		}
	default:
		v = f.view(focused, cursorFn)
	}
	if e := f.err; e != "" {
		v += stErr.Render("  ✗ " + e)
	} else if e := errs[fieldKey(rw.use, rw.param)]; e != "" && f.String() != "" {
		v += stErr.Render("  ✗ " + e)
	}
	return v
}

// reviewLines: what the files will do, the rules' findings, flk's verdict.
func (m *Model) reviewLines(w int) []string {
	var out []string
	if m.rerr != nil {
		out = append(out, "", stErr.Render(" Not ready:"))
		for _, l := range strings.Split(m.rerr.Error(), "\n") {
			for _, wl := range wrap(l, w-3) {
				out = append(out, "   "+wl)
			}
		}
		return out
	}
	out = append(out, "", " Files: "+fileList(m))
	if where := m.taken[m.r.App]; where != "" {
		for _, wl := range wrap("App name: "+m.r.App+" is already registered, from "+where+": flk launch will refuse it (change it on the Name step)", w-2) {
			out = append(out, " "+stWarn.Render(wl))
		}
	}
	if len(m.res.Findings) == 0 {
		out = append(out, " Rules: "+stOK.Render("no findings"))
	} else {
		out = append(out, " Rules:")
		for _, f := range m.res.Findings {
			st := stWarn
			if f.Level == "error" {
				st = stErr
			}
			for i, wl := range wrap(f.Rule+": "+f.Msg, w-5) {
				if i == 0 {
					out = append(out, "   "+st.Render(f.Level)+" "+wl)
				} else {
					out = append(out, "     "+wl)
				}
			}
		}
	}
	switch {
	case m.o.Validate == nil:
		out = append(out, " flk config show: "+stDim.Render("not available"))
	case m.validating:
		out = append(out, " flk config show: "+stDim.Render("checking…"))
	case m.flkErr != nil:
		out = append(out, " flk config show: "+stErr.Render(m.flkErr.Error()))
	case !m.validated:
		out = append(out, " flk config show: "+stDim.Render("not run"))
	case len(m.flkWarn) == 0:
		out = append(out, " flk config show: "+stOK.Render("no warnings"))
	default:
		out = append(out, " flk config show: "+stWarn.Render(fmt.Sprintf("%d warnings", len(m.flkWarn))))
		for _, x := range m.flkWarn {
			out = append(out, "   "+x)
		}
	}
	var run []string
	for _, s := range m.res.Steps {
		if s.Kind == snippet.NextRun {
			run = append(run, s.Text)
		}
	}
	if len(run) > 0 {
		for i, wl := range wrap("Then: "+strings.Join(run, "; "), w-2) {
			if i > 0 {
				wl = "  " + wl
			}
			out = append(out, " "+wl)
		}
	}
	for _, s := range m.res.Steps {
		if s.Kind == snippet.NextPending || s.Kind == snippet.NextRoot {
			label := "To do: "
			if s.Kind == snippet.NextRoot {
				label = "As root: "
			}
			for i, wl := range wrap(label+s.Text, w-2) {
				if i > 0 {
					wl = "  " + wl
				}
				out = append(out, " "+stWarn.Render(wl))
			}
		}
	}
	return out
}

func fileList(m *Model) string {
	var names []string
	for _, f := range m.res.Files {
		n := f.Path
		if f.Own == snippet.Seed {
			n += " (yours)"
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

func (m *Model) pickerLines(w, h int) []string {
	p := m.picker
	out := []string{" " + stBold.Render("Add to "+m.cur().name) + stDim.Render("  enter adds, esc cancels"), ""}
	avail := max(1, h-2)
	start := 0
	if p.focus >= avail {
		start = p.focus - avail + 1
	}
	for i := start; i < len(p.ids) && i < start+avail; i++ {
		s, _ := m.o.Catalog.Get(p.ids[i])
		name := strings.TrimPrefix(s.ID, "core/")
		line := "  " + fmt.Sprintf("%-20s", name) + " " + stDim.Render(s.Summary)
		if i == p.focus {
			line = stFocus.Render("› "+fmt.Sprintf("%-20s", name)) + " " + s.Summary
		}
		out = append(out, line)
	}
	return out
}

// previewLines: file tabs, then the file with the focused use's lines
// marked, scrolled to them.
func (m *Model) previewLines(w, h int) []string {
	res := m.preview
	if res == nil {
		out := []string{stBold.Render("Preview"), ""}
		if m.rerr != nil {
			for _, wl := range wrap("Nothing to show until it renders: "+m.rerr.Error(), w) {
				out = append(out, stDim.Render(wl))
			}
		}
		return out
	}
	if m.file >= len(res.Files) {
		m.file = 0
	}
	var tabs []string
	for i, f := range res.Files {
		if i == m.file {
			tabs = append(tabs, stTab.Render(" "+f.Path+" "))
		} else {
			tabs = append(tabs, " "+f.Path+" ")
		}
	}
	note := stDim.Render("  ctrl+f")
	if m.placeholder {
		note = stErr.Render("  TODO: to fill in")
	}
	out := []string{strings.Join(tabs, "") + note}
	f := res.Files[m.file]
	lines := strings.Split(strings.TrimRight(string(f.Content), "\n"), "\n")
	hi := m.highlight[f.Path]
	first := -1
	for i := range lines {
		if hi[i] {
			first = i
			break
		}
	}
	avail := max(1, h-1)
	start := 0
	if first >= avail-2 {
		start = min(first-2, max(0, len(lines)-avail))
	}
	for i := start; i < len(lines) && i < start+avail; i++ {
		if hi[i] {
			out = append(out, stHi.Render("▌"+lines[i]))
		} else {
			out = append(out, " "+lines[i])
		}
	}
	return out
}

// hintLines: what the focused thing is and does, at the hint level.
func (m *Model) hintLines(w, h int) []string {
	rule := stDim.Render("─ hint " + strings.Repeat("─", max(0, w-7)))
	var text []string
	add := func(s string) { text = append(text, wrap(s, w-2)...) }
	snippetHints := func(s *snippet.Snippet) {
		if m.hint == 0 {
			add(s.Hints.What)
			if s.Hints.UnderTheHood != "" {
				add(strings.Join(strings.Fields(s.Hints.UnderTheHood), " "))
			}
			for _, g := range s.Hints.Gotchas {
				add("! " + g)
			}
			if len(s.Hints.Docs) > 0 {
				add("More: " + strings.Join(s.Hints.Docs, "; "))
			}
		} else {
			add(s.Summary)
		}
	}
	switch {
	case m.picker != nil:
		s, _ := m.o.Catalog.Get(m.picker.ids[m.picker.focus])
		add(stBold.Render(s.ID) + ": " + s.Title)
		snippetHints(s)
	case len(m.rows) == 0:
	default:
		rw := m.rows[m.focus]
		switch rw.kind {
		case rowTemplate:
			if t, err := m.o.Catalog.Template(rw.tpl); err == nil {
				add(stBold.Render(t.ID) + ": " + t.Recipe.Summary)
				var keys []string
				for _, u := range t.Recipe.Uses {
					keys = append(keys, u.Key())
				}
				add("Snippets: " + strings.Join(keys, ", ") + ". enter starts from it; the next steps change anything.")
			}
		case rowApp:
			add(stBold.Render("App name") + ": lowercase letters, digits and dashes (flk's rule). Start experiments with flkt- so they're easy to tell from real apps.")
			add("The machines' names are flk's to make up. The directory: " + m.o.Dir)
		case rowUse:
			if _, s := m.snippetOf(rw.use); s != nil {
				add(stBold.Render(s.ID) + ": " + s.Title)
				snippetHints(s)
			}
		case rowParam:
			if _, s := m.snippetOf(rw.use); s != nil {
				p := s.Params[rw.param]
				kind := p.Type
				if len(p.Values) > 0 {
					kind += " " + strings.Join(p.Values, "|")
				}
				switch {
				case p.Required:
					kind += ", required"
				case p.Default != nil:
					if d, ok := p.Default.(string); ok && strings.Contains(d, "\n") {
						kind += fmt.Sprintf(", default: %d lines", strings.Count(strings.TrimRight(d, "\n"), "\n")+1)
					} else {
						kind += ", default " + snippet.FormatValue(p, p.Default)
					}
				}
				add(stBold.Render(rw.use+"."+rw.param) + " (" + kind + "): " + p.Prompt)
				if p.Hint != "" {
					add(p.Hint)
				}
				switch p.Type {
				case "enum", "bool":
					add(stDim.Render("←/→ or space changes it."))
				case "list":
					add(stDim.Render("A list: a, b, c (or a TOML array for items with commas)."))
				case "map":
					add(stDim.Render(`A table: { NAME = "value", OTHER = "x" }`))
				}
				if m.hint == 0 {
					add(s.Hints.What)
				}
			}
		case rowAdd:
			add(stBold.Render("Add") + ": enter lists the snippets for this step. One that conflicts with a snippet already here replaces it (a backup mode, a base, a restart policy).")
		case rowWrite:
			ok, why := m.canWrite()
			if !ok {
				add(stErr.Render("Not ready: ") + why)
			} else {
				add(stBold.Render("Write") + ": the Dockerfile, flk.toml, the files the snippets add, and flkforge.toml (the recipe). Nothing is deployed: the report lists the flk commands to run.")
			}
		}
	}
	if m.hint == 2 && len(text) > 1 {
		text = text[:1]
	}
	out := []string{rule}
	for _, t := range text {
		out = append(out, " "+t)
	}
	return out
}

func (m *Model) footer() string {
	if m.status != "" {
		return stStatus.Render(" " + m.status)
	}
	keys := " tab/↑↓ move · ctrl+n/ctrl+p step · ctrl+d remove · ctrl+f file · ctrl+t hints · F1 help · esc quit"
	if m.picker != nil {
		keys = " ↑↓ choose · enter add · esc cancel"
	}
	return stDim.Render(keys)
}

func (m *Model) renderReport(w, h int) string {
	title := " flkforge: writing…"
	foot := ""
	var body []string
	if m.phase == done {
		title = " flkforge: written"
		foot = " q or enter quits"
		if m.refused {
			title = " flkforge: not written"
			foot = " b goes back to the wizard · q quits"
		}
		body = strings.Split(strings.TrimRight(m.report, "\n"), "\n")
	}
	lines := []string{stHeader.Render(title + strings.Repeat(" ", max(0, w-ansi.StringWidth(title))))}
	lines = append(lines, fit(body, w, h-2)...)
	lines = append(lines, stDim.Render(foot))
	return strings.Join(fit(lines, w, h), "\n")
}

// shortDir shows a directory in at most n cells: ~ for the home
// directory, and the end of a long path rather than its start.
func shortDir(dir string, n int) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if home, err := os.UserHomeDir(); err == nil && home != "/" {
		if dir == home {
			dir = "~"
		} else if rest, ok := strings.CutPrefix(dir, home+"/"); ok {
			dir = "~/" + rest
		}
	}
	if r := []rune(dir); len(r) > n {
		dir = "…" + string(r[len(r)-n+1:])
	}
	return dir
}

var helpText = `Moving
  tab, ↓ / shift+tab, ↑    next / previous field
  ctrl+n, pgdown, ]        next step (] and [ outside a text field)
  ctrl+p, pgup, [          previous step
  enter                    next field; on the last one, the next step

Editing
  type                     edit a text field (ctrl+u clears it, ←/→ move)
  ←/→, space               change a choice or a yes/no
  enter on "+ add…"        add a snippet; one that conflicts with a snippet
                           already there replaces it (a backup mode, a base)
  ctrl+o                   edit a text value in your editor ($VISUAL,
                           $EDITOR or vi): the way to change a multi-line
                           one, such as a start script's body
  ctrl+d                   remove the focused snippet

Seeing
  ctrl+f                   next file in the preview (it follows the focused
                           snippet otherwise; its lines are marked ▌)
  ctrl+t                   hints: full, summary, minimal
  ?, F1                    this help (outside a text field)

Leaving
  Review, then "Write"     write the files (nothing is deployed)
  esc, ctrl+c              quit (twice if nothing is written yet)

The command line does all of this too: flkforge help --all.
Press any key to go back.`

func (m *Model) renderHelp(w, h int) string {
	title := " flkforge: keys"
	lines := []string{stHeader.Render(title + strings.Repeat(" ", max(0, w-ansi.StringWidth(title))))}
	for _, l := range strings.Split(helpText, "\n") {
		lines = append(lines, " "+l)
	}
	return strings.Join(fit(lines, w, h), "\n")
}
