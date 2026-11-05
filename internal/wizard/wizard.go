// Package wizard is flkforge's terminal UI (SPEC 8): steps down the left,
// the current step's form in the middle, a live preview of the files on
// the right, and a hint panel below. It edits a recipe and renders it with
// the same pure renderer as the CLI; writing and flk's validation are
// callbacks (Options), so the model does no I/O itself and tests drive it
// with key presses.
package wizard

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
	"github.com/dayne/d.flkforge/internal/source"
)

// Options configure a wizard.
type Options struct {
	Catalog *source.Catalog
	Dir     string
	// Recipe to start from: an existing one (Edit), or nil for a new app
	// (the Start step offers the templates).
	Recipe *recipe.Recipe
	// Template to start a new app from (skips the Start step).
	Template string
	Edit     bool
	// Hints: the starting hint level: full, summary or minimal.
	Hints string
	// Write writes the recipe's files and returns the report to show.
	// refused: nothing was written (drift, a flk warning).
	Write func(r *recipe.Recipe) (report string, refused bool, err error)
	// Validate runs flk config show on a flk.toml (nil: no flk).
	Validate func(flkToml []byte) (warnings []string, err error)
	// Taken says where an app name is already registered with flk ("" if
	// it isn't), nil: unknown.
	Taken func(app string) string
}

// A step of the wizard (SPEC 8). Category steps edit the recipe's uses of
// those snippet categories.
type step struct {
	name string
	cats []string
	add  string // the add row's label
}

var allSteps = []step{
	{name: "Start"},
	{name: "Name"},
	{name: "Base", cats: []string{"base"}, add: "change the base…"},
	{name: "Software", cats: []string{"fetch"}, add: "add a way to get software in…"},
	{name: "Process", cats: []string{"wrapper", "restart"}, add: "add a process or restart snippet…"},
	{name: "User & data", cats: []string{"user", "data"}, add: "add a user or data snippet…"},
	{name: "Network", cats: []string{"network"}, add: "add a port or network snippet…"},
	{name: "Health", cats: []string{"check"}, add: "add a health check…"},
	{name: "Backups", cats: []string{"backup"}, add: "add a backup snippet…"},
	{name: "Resources", cats: []string{"vm", "misc"}, add: "add a resource or misc snippet…"},
	{name: "Env & secrets", cats: []string{"env"}, add: "add env vars or a secret…"},
	{name: "Review"},
}

type rowKind int

const (
	rowTemplate rowKind = iota
	rowApp
	rowUse
	rowParam
	rowAdd
	rowWrite
)

type row struct {
	kind  rowKind
	use   string // the use's key
	param string
	tpl   string // template id
}

type phase int

const (
	editing phase = iota
	writing
	done
)

// Model is the wizard.
type Model struct {
	o      Options
	r      *recipe.Recipe
	steps  []step
	step   int
	rows   []row
	focus  int
	offset int // first form row shown
	fields map[string]*field
	app    *field

	res  *render.Result
	rerr error
	// preview is res, or when only params are missing, a render with
	// placeholders in their place (so the preview always shows something).
	preview     *render.Result
	placeholder bool
	highlight   map[string]map[int]bool // file -> highlighted line numbers

	hint int // 0 full, 1 summary, 2 minimal
	file int // preview file
	// fileChosen: the admin picked the preview file (ctrl+f); otherwise it
	// follows the focused use to the first file it changes.
	fileChosen bool
	lastUse    string
	picker     *picker
	dirty      bool
	armQuit    bool
	status     string

	validated  bool
	validating bool
	flkWarn    []string
	flkErr     error

	phase   phase
	report  string
	refused bool

	width, height int
	quitting      bool
	help          bool
	taken         map[string]string // app name -> where it's registered ("": free)
}

type picker struct {
	ids   []string
	focus int
}

// New makes a wizard.
func New(o Options) *Model {
	m := &Model{o: o, fields: map[string]*field{}, width: 80, height: 24, taken: map[string]string{}}
	switch o.Hints {
	case "summary":
		m.hint = 1
	case "minimal", "off":
		m.hint = 2
	}
	m.steps = allSteps
	if o.Edit {
		m.steps = allSteps[1:]
	}
	switch {
	case o.Recipe != nil:
		m.r = cloneRecipe(o.Recipe)
	default:
		m.r = &recipe.Recipe{}
	}
	if m.r.App == "" {
		m.r.App = recipe.DefaultAppName(o.Dir)
	}
	m.app = newField(m.r.App)
	if o.Template != "" {
		if t, err := o.Catalog.Template(o.Template); err == nil {
			m.applyTemplate(t)
			m.step = 1 // Name
		}
	}
	m.build()
	m.rerender()
	return m
}

// Init does nothing: the first render is done in New.
func (m *Model) Init() tea.Cmd { return nil }

// Recipe is the recipe as edited so far.
func (m *Model) Recipe() *recipe.Recipe { return m.r }

// Quitting says whether the wizard has finished.
func (m *Model) Quitting() bool { return m.quitting }

// Written says whether the files were written.
func (m *Model) Written() bool { return m.phase == done && !m.refused }

func cloneRecipe(r *recipe.Recipe) *recipe.Recipe {
	n := *r
	n.Uses = nil
	for _, u := range r.Uses {
		p := map[string]any{}
		for k, v := range u.Params {
			p[k] = v
		}
		u.Params = p
		n.Uses = append(n.Uses, u)
	}
	return &n
}

func (m *Model) cur() step { return m.steps[m.step] }

func (m *Model) applyTemplate(t *source.Template) {
	app := m.r.App
	r := cloneRecipe(t.Recipe)
	r.Template, r.Title, r.Summary = t.ID, "", ""
	r.App = app
	r.Render, r.Seeded, r.Lock = m.r.Render, m.r.Seeded, m.r.Lock
	m.r = r
	m.fields = map[string]*field{}
	m.dirty = true
}

// snippetOf returns the snippet behind a use.
func (m *Model) snippetOf(key string) (*recipe.Use, *snippet.Snippet) {
	for i := range m.r.Uses {
		if m.r.Uses[i].Key() == key {
			s, err := m.o.Catalog.Get(m.r.Uses[i].Snippet)
			if err != nil {
				return &m.r.Uses[i], nil
			}
			return &m.r.Uses[i], s
		}
	}
	return nil, nil
}

// build lists the current step's rows.
func (m *Model) build() {
	st := m.cur()
	var rows []row
	switch st.name {
	case "Start":
		for _, t := range m.o.Catalog.Templates() {
			rows = append(rows, row{kind: rowTemplate, tpl: t.ID})
		}
	case "Name":
		rows = append(rows, row{kind: rowApp})
	case "Review":
		rows = append(rows, row{kind: rowWrite})
	default:
		for _, u := range m.r.Uses {
			s, err := m.o.Catalog.Get(u.Snippet)
			if err != nil || !slices.Contains(st.cats, s.Category) {
				continue
			}
			rows = append(rows, row{kind: rowUse, use: u.Key()})
			for _, p := range s.ParamOrder {
				rows = append(rows, row{kind: rowParam, use: u.Key(), param: p})
			}
		}
		rows = append(rows, row{kind: rowAdd})
	}
	m.rows = rows
	if m.focus >= len(rows) {
		m.focus = len(rows) - 1
	}
	if m.focus < 0 {
		m.focus = 0
	}
}

func fieldKey(use, param string) string { return use + "\x00" + param }

// paramField returns the text field of a param, made from its value.
func (m *Model) paramField(use, param string) *field {
	k := fieldKey(use, param)
	if f, ok := m.fields[k]; ok {
		return f
	}
	u, s := m.snippetOf(use)
	f := newField("")
	if u != nil && s != nil {
		if v, ok := u.Params[param]; ok {
			f.set(snippet.FormatValue(s.Params[param], v))
		}
	}
	m.fields[k] = f
	return f
}

// setParam applies a field's text to the recipe.
func (m *Model) setParam(use, param string) {
	u, s := m.snippetOf(use)
	if u == nil || s == nil {
		return
	}
	f := m.paramField(use, param)
	p := s.Params[param]
	raw := f.String()
	f.err = ""
	if u.Params == nil {
		u.Params = map[string]any{}
	}
	if raw == "" {
		delete(u.Params, param)
	} else if v, err := snippet.ParseValue(p, raw); err != nil {
		f.err = err.Error()
		delete(u.Params, param)
	} else if _, err := p.Coerce(v); err != nil {
		f.err = err.Error()
		delete(u.Params, param)
	} else {
		u.Params[param] = v
	}
	m.dirty = true
	m.rerender()
}

// rerender renders the recipe, and again without the focused use to find
// the lines it contributes.
func (m *Model) rerender() {
	m.validated, m.flkWarn, m.flkErr = false, nil, nil
	m.res, m.rerr = render.Render(m.r, m.o.Catalog)
	m.preview, m.placeholder = m.res, false
	base := m.r
	if m.res == nil {
		if filled := m.withPlaceholders(); filled != nil {
			if res, err := render.Render(filled, m.o.Catalog); err == nil {
				m.preview, m.placeholder, base = res, true, filled
			}
		}
	}
	m.highlight = nil
	if m.preview == nil || m.focus >= len(m.rows) {
		return
	}
	rw := m.rows[m.focus]
	if rw.use == "" {
		return
	}
	without := cloneRecipe(base)
	i := slices.IndexFunc(without.Uses, func(u recipe.Use) bool { return u.Key() == rw.use })
	if i < 0 {
		return
	}
	without.Uses = slices.Delete(without.Uses, i, i+1)
	other, err := render.Render(without, m.o.Catalog)
	if err != nil {
		return
	}
	m.highlight = map[string]map[int]bool{}
	for _, f := range m.preview.Files {
		var before []string
		if o := other.File(f.Path); o != nil {
			before = strings.Split(string(o.Content), "\n")
		}
		m.highlight[f.Path] = added(before, strings.Split(string(f.Content), "\n"))
	}
	if rw.use != m.lastUse {
		m.lastUse, m.fileChosen = rw.use, false
	}
	if !m.fileChosen {
		for i, f := range m.preview.Files {
			if len(m.highlight[f.Path]) > 0 {
				m.file = i
				break
			}
		}
	}
}

// added marks the lines of after that aren't in before (by a longest
// common subsequence).
func added(before, after []string) map[int]bool {
	n, k := len(before), len(after)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, k+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := k - 1; j >= 0; j-- {
			if before[i] == after[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	out := map[int]bool{}
	i, j := 0, 0
	for j < k {
		switch {
		case i < n && before[i] == after[j]:
			i++
			j++
		case i < n && lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			out[j] = true
			j++
		}
	}
	return out
}

// paramErrors are the render's param problems, by use and param.
func (m *Model) paramErrors() map[string]string {
	out := map[string]string{}
	var pes render.ParamErrors
	if !errors.As(m.rerr, &pes) {
		return out
	}
	for _, pe := range pes {
		var probs snippet.ParamErrors
		if errors.As(pe.Err, &probs) {
			for _, p := range probs {
				out[fieldKey(pe.Use, p.Param)] = p.Msg
			}
		}
	}
	return out
}

// stepProblems counts the param problems in each step.
func (m *Model) stepProblems() map[string]int {
	out := map[string]int{}
	for k := range m.paramErrors() {
		use := strings.SplitN(k, "\x00", 2)[0]
		if _, s := m.snippetOf(use); s != nil {
			for _, st := range m.steps {
				if slices.Contains(st.cats, s.Category) {
					out[st.name]++
				}
			}
		}
	}
	return out
}

// pickable lists the snippets the add row offers in this step.
func (m *Model) pickable() []string {
	st := m.cur()
	var ids []string
	for _, s := range m.o.Catalog.All() {
		if !slices.Contains(st.cats, s.Category) {
			continue
		}
		used := slices.ContainsFunc(m.r.Uses, func(u recipe.Use) bool { return u.Snippet == s.ID })
		if used && !s.Multi {
			continue
		}
		ids = append(ids, s.ID)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, _ := m.o.Catalog.Get(ids[i])
		b, _ := m.o.Catalog.Get(ids[j])
		return slices.Index(st.cats, a.Category) < slices.Index(st.cats, b.Category)
	})
	return ids
}

// add adds a snippet to the recipe. One that conflicts with uses already
// there replaces them (a new base replaces the old one), keeping the
// params they share.
func (m *Model) add(id string) {
	s, err := m.o.Catalog.Get(id)
	if err != nil {
		m.status = err.Error()
		return
	}
	nu := recipe.Use{Snippet: s.ID, Params: map[string]any{}}
	var replaced []string
	var keep []recipe.Use
	pos := -1 // where the first replaced use was: the new one takes its place
	for _, u := range m.r.Uses {
		o, err := m.o.Catalog.Get(u.Snippet)
		clash := err == nil && (o.Category == "base" && s.Category == "base" ||
			slices.Contains(s.Conflicts, o.ID) || slices.Contains(o.Conflicts, s.ID) ||
			o.ID == s.ID && !s.Multi)
		if !clash {
			keep = append(keep, u)
			continue
		}
		if pos < 0 {
			pos = len(keep)
		}
		replaced = append(replaced, u.Key())
		for k, v := range u.Params {
			if _, ok := s.Params[k]; ok {
				nu.Params[k] = v
			}
		}
	}
	if pos < 0 {
		pos = len(keep)
	}
	key := s.Name()
	if slices.ContainsFunc(keep, func(u recipe.Use) bool { return u.Key() == key }) {
		for n := 2; ; n++ {
			cand := fmt.Sprintf("%s-%d", s.Name(), n)
			if !slices.ContainsFunc(keep, func(u recipe.Use) bool { return u.Key() == cand }) {
				nu.ID = cand
				break
			}
		}
	}
	m.r.Uses = keep
	m.r.Uses = slices.Insert(m.r.Uses, pos, nu)
	for k := range m.fields {
		for _, r := range replaced {
			if strings.HasPrefix(k, r+"\x00") {
				delete(m.fields, k)
			}
		}
	}
	m.dirty = true
	if len(replaced) > 0 {
		m.status = "added " + nu.Key() + ", replacing " + strings.Join(replaced, ", ")
	} else {
		m.status = "added " + nu.Key()
	}
	m.build()
	for i, rw := range m.rows {
		if rw.kind == rowUse && rw.use == nu.Key() {
			m.focus = i
		}
	}
	m.rerender()
}

// remove drops a use from the recipe.
func (m *Model) remove(key string) {
	_, s := m.snippetOf(key)
	if s != nil && s.Category == "base" {
		m.status = "a recipe needs a base: change it with the add row instead"
		return
	}
	m.r.Uses = slices.DeleteFunc(m.r.Uses, func(u recipe.Use) bool { return u.Key() == key })
	for k := range m.fields {
		if strings.HasPrefix(k, key+"\x00") {
			delete(m.fields, k)
		}
	}
	m.dirty = true
	m.status = "removed " + key
	m.build()
	m.rerender()
}

// checkTaken asks flk (through Options.Taken) whether the app name is
// registered, in the background.
func (m *Model) checkTaken() tea.Cmd {
	if m.o.Taken == nil || m.o.Edit {
		return nil
	}
	name := m.r.App
	if _, ok := m.taken[name]; ok {
		return nil
	}
	return func() tea.Msg { return takenMsg{app: name, where: m.o.Taken(name)} }
}

type takenMsg struct{ app, where string }

// goStep moves to another step.
func (m *Model) goStep(i int) tea.Cmd {
	if i < 0 || i >= len(m.steps) || i == m.step {
		return nil
	}
	var taken tea.Cmd
	if m.cur().name == "Name" {
		taken = m.checkTaken()
	}
	m.step, m.focus, m.offset, m.file = i, 0, 0, 0
	m.build()
	// Start on the first param that still needs a value, if any.
	errs := m.paramErrors()
	for j, rw := range m.rows {
		if rw.kind == rowParam && errs[fieldKey(rw.use, rw.param)] != "" {
			m.focus = j
			break
		}
	}
	m.rerender()
	if m.cur().name == "Review" {
		return tea.Batch(taken, m.validate(), m.checkTaken())
	}
	return taken
}

// validate runs flk config show on the rendered flk.toml.
func (m *Model) validate() tea.Cmd {
	if m.o.Validate == nil || m.res == nil {
		return nil
	}
	m.validating = true
	content := m.res.File("flk.toml").Content
	return func() tea.Msg {
		w, err := m.o.Validate(content)
		return validatedMsg{content: string(content), warnings: w, err: err}
	}
}

type validatedMsg struct {
	content  string
	warnings []string
	err      error
}

type writtenMsg struct {
	report  string
	refused bool
	err     error
}

func (m *Model) write() tea.Cmd {
	if m.o.Write == nil {
		return nil
	}
	m.phase = writing
	r := cloneRecipe(m.r)
	return func() tea.Msg {
		rep, refused, err := m.o.Write(r)
		return writtenMsg{report: rep, refused: refused, err: err}
	}
}

// canWrite: the recipe renders, with no rule errors.
func (m *Model) canWrite() (bool, string) {
	if m.rerr != nil {
		return false, "fix the problems first (the steps marked !)"
	}
	if errs := m.res.Errors(); len(errs) > 0 {
		return false, "a rule fails: " + errs[0].Msg
	}
	if err := recipe.CheckAppName(m.r.App); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// Update handles a message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case takenMsg:
		m.taken[msg.app] = msg.where
	case validatedMsg:
		m.validating = false
		if m.res != nil && string(m.res.File("flk.toml").Content) == msg.content {
			m.validated, m.flkWarn, m.flkErr = true, msg.warnings, msg.err
		}
	case writtenMsg:
		m.phase = done
		m.report, m.refused = msg.report, msg.refused
		if msg.err != nil {
			m.report = "Error: " + msg.err.Error() + "\n\nNothing was written."
			m.refused = true
		}
		if !m.refused {
			m.dirty = false
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *Model) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.phase == writing {
		return nil
	}
	if m.phase == done {
		switch k {
		case "q", "enter", "esc", "ctrl+c":
			return m.quit()
		case "b":
			if m.refused {
				m.phase = editing
			}
		}
		return nil
	}
	if k != "esc" && k != "ctrl+c" {
		m.armQuit = false
	}
	if m.picker != nil {
		return m.pickerKey(k)
	}
	if m.help {
		m.help = false
		return nil
	}
	m.status = ""
	switch k {
	case "ctrl+c", "esc":
		if !m.dirty || m.armQuit {
			return m.quit()
		}
		m.armQuit = true
		m.status = "nothing written yet: " + k + " again quits anyway"
		return nil
	case "ctrl+n", "pgdown":
		return m.goStep(m.step + 1)
	case "ctrl+p", "pgup":
		return m.goStep(m.step - 1)
	case "ctrl+t":
		m.hint = (m.hint + 1) % 3
		return nil
	case "f1":
		m.help = true
		return nil
	case "ctrl+f":
		if m.preview != nil && len(m.preview.Files) > 0 {
			m.file = (m.file + 1) % len(m.preview.Files)
			m.fileChosen = true
		}
		return nil
	case "up", "shift+tab":
		m.move(-1)
		return nil
	case "down", "tab":
		m.move(1)
		return nil
	case "ctrl+d":
		if rw := m.rows[m.focus]; rw.use != "" {
			m.remove(rw.use)
		}
		return nil
	}
	rw := m.rows[m.focus]
	switch rw.kind {
	case rowApp:
		if k == "enter" {
			return m.goStep(m.step + 1)
		}
		if changed, handled := m.app.key(k, msg.Text); handled {
			if changed {
				m.r.App = m.app.String()
				m.app.err = ""
				if err := recipe.CheckAppName(m.r.App); err != nil {
					m.app.err = err.Error()
				}
				m.dirty = true
				m.rerender()
			}
			return nil
		}
	case rowParam:
		return m.paramKey(rw, k, msg.Text)
	case rowTemplate:
		if k == "enter" || k == "space" {
			t, err := m.o.Catalog.Template(rw.tpl)
			if err == nil {
				m.applyTemplate(t)
				m.status = "started from " + t.ID
				return m.goStep(m.step + 1)
			}
		}
	case rowAdd:
		if k == "enter" || k == "space" {
			if ids := m.pickable(); len(ids) > 0 {
				m.picker = &picker{ids: ids}
			} else {
				m.status = "nothing more to add here"
			}
			return nil
		}
	case rowUse:
		if k == "enter" {
			m.move(1)
			return nil
		}
	case rowWrite:
		if k == "enter" || k == "space" {
			if ok, why := m.canWrite(); !ok {
				m.status = why
				return nil
			}
			return m.write()
		}
	}
	switch k {
	case "enter":
		if m.focus == len(m.rows)-1 {
			return m.goStep(m.step + 1)
		}
		m.move(1)
	case "q":
		if !m.dirty || m.armQuit {
			return m.quit()
		}
		m.armQuit = true
		m.status = "nothing written yet: q again quits anyway"
	case "?", "f1":
		m.help = true
	case "]":
		return m.goStep(m.step + 1)
	case "[":
		return m.goStep(m.step - 1)
	}
	return nil
}

func (m *Model) move(d int) {
	n := m.focus + d
	if n < 0 || n >= len(m.rows) {
		return
	}
	m.focus = n
	m.rerender()
}

// paramKey edits a param: text fields take typing; enums and bools cycle.
func (m *Model) paramKey(rw row, k, text string) tea.Cmd {
	_, s := m.snippetOf(rw.use)
	if s == nil {
		return nil
	}
	p := s.Params[rw.param]
	f := m.paramField(rw.use, rw.param)
	switch p.Type {
	case "enum", "bool":
		var opts []string
		if p.Type == "bool" {
			opts = []string{"false", "true"}
		} else {
			opts = append([]string(nil), p.Values...)
		}
		if !p.Required {
			opts = append([]string{""}, opts...) // "": the default
		}
		i := slices.Index(opts, f.String())
		switch k {
		case "right", "space", "l":
			f.set(opts[(i+1)%len(opts)])
		case "left", "h":
			f.set(opts[(i-1+len(opts))%len(opts)])
		case "enter":
			m.move(1)
			return nil
		default:
			return nil
		}
		m.setParam(rw.use, rw.param)
		return nil
	}
	if multiline(m, rw) {
		if k == "enter" {
			m.move(1)
		}
		return nil
	}
	if k == "enter" {
		if m.focus == len(m.rows)-1 {
			return m.goStep(m.step + 1)
		}
		m.move(1)
		return nil
	}
	if changed, _ := f.key(k, text); changed {
		m.setParam(rw.use, rw.param)
	}
	return nil
}

// multiline: a param whose value has more than one line can't be edited in
// a one-line field (a start script's body): it's edited in the file.
func multiline(m *Model, rw row) bool {
	u, _ := m.snippetOf(rw.use)
	if u == nil {
		return false
	}
	if v, ok := u.Params[rw.param].(string); ok && strings.Contains(v, "\n") {
		return true
	}
	if _, ok := u.Params[rw.param]; !ok {
		_, s := m.snippetOf(rw.use)
		if d, ok := s.Params[rw.param].Default.(string); ok && strings.Contains(d, "\n") {
			return true
		}
	}
	return false
}

func (m *Model) pickerKey(k string) tea.Cmd {
	p := m.picker
	switch k {
	case "esc", "q", "ctrl+c":
		m.picker = nil
	case "up", "shift+tab", "k":
		if p.focus > 0 {
			p.focus--
		}
	case "down", "tab", "j":
		if p.focus < len(p.ids)-1 {
			p.focus++
		}
	case "enter", "space":
		id := p.ids[p.focus]
		m.picker = nil
		m.add(id)
	}
	return nil
}

// withPlaceholders fills every missing required param with a placeholder
// its type accepts, or returns nil if anything else is wrong.
func (m *Model) withPlaceholders() *recipe.Recipe {
	var pes render.ParamErrors
	if !errors.As(m.rerr, &pes) {
		return nil
	}
	filled := cloneRecipe(m.r)
	for _, pe := range pes {
		var probs snippet.ParamErrors
		if !errors.As(pe.Err, &probs) {
			return nil
		}
		i := slices.IndexFunc(filled.Uses, func(u recipe.Use) bool { return u.Key() == pe.Use })
		if i < 0 {
			return nil
		}
		s, err := m.o.Catalog.Get(filled.Uses[i].Snippet)
		if err != nil {
			return nil
		}
		for _, p := range probs {
			if !p.Missing {
				return nil
			}
			v := placeholder(s.Params[p.Param])
			if v == nil {
				return nil
			}
			filled.Uses[i].Params[p.Param] = v
		}
	}
	return filled
}

// placeholder is a value of the param's type that says TODO.
func placeholder(p *snippet.Param) any {
	var cands []any
	switch p.Type {
	case "int", "port":
		n := int64(1)
		if p.Min != nil {
			n = *p.Min
		}
		cands = []any{n}
	case "bool":
		cands = []any{false}
	case "enum":
		cands = []any{p.Values[0]}
	case "list":
		cands = []any{[]any{"TODO"}, []any{"/TODO"}}
	case "map":
		cands = []any{map[string]any{"TODO": "TODO"}}
	case "duration":
		cands = []any{"60s"}
	case "size":
		cands = []any{"1gb"}
	case "secret":
		cands = []any{"TODO"}
	default:
		cands = []any{"TODO", "/TODO", "https://TODO", "TODO=TODO", "0000000000000000000000000000000000000000000000000000000000000000"}
	}
	for _, c := range cands {
		if _, err := p.Coerce(c); err == nil {
			return c
		}
	}
	return nil
}
