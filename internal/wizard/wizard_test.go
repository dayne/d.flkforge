package wizard

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dayne/d.flkforge/internal/source"
	"github.com/dayne/d.flkforge/snippets"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func catalog(t *testing.T) *source.Catalog {
	t.Helper()
	c, err := source.Load(source.FS{Label: "built in", FS: snippets.FS})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// runCmd runs a command and everything it batches, returning the messages.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, runCmd(c)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(2 * time.Second):
		return nil
	}
}

// send delivers msgs and the results of the commands they start, until
// the model settles.
func send(m *Model, msgs ...tea.Msg) {
	for len(msgs) > 0 {
		msg := msgs[0]
		msgs = msgs[1:]
		_, cmd := m.Update(msg)
		if _, quit := msg.(tea.QuitMsg); quit {
			continue
		}
		for _, r := range runCmd(cmd) {
			if _, q := r.(tea.QuitMsg); !q {
				msgs = append(msgs, r)
			}
		}
	}
}

func key(s string) tea.KeyPressMsg {
	codes := map[string]tea.Key{
		"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
		"tab": {Code: tea.KeyTab}, "shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"pgdown": {Code: tea.KeyPgDown}, "pgup": {Code: tea.KeyPgUp},
		"esc": {Code: tea.KeyEscape}, "enter": {Code: tea.KeyEnter}, "backspace": {Code: tea.KeyBackspace},
		"space": {Code: tea.KeySpace, Text: " "},
	}
	if k, ok := codes[s]; ok {
		return tea.KeyPressMsg(k)
	}
	if c, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(c)[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

// press sends keys; a key longer than one rune that isn't a named key is
// typed as text.
func press(m *Model, keys ...string) {
	named := map[string]bool{"up": true, "down": true, "left": true, "right": true, "tab": true, "shift+tab": true,
		"pgdown": true, "pgup": true, "esc": true, "enter": true, "backspace": true, "space": true}
	for _, k := range keys {
		if named[k] || strings.HasPrefix(k, "ctrl+") || len([]rune(k)) == 1 {
			send(m, key(k))
			continue
		}
		for _, r := range k {
			send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
}

func sized(m *Model, w, h int) *Model {
	send(m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func checkSize(t *testing.T, screen string, w, h int) {
	t.Helper()
	lines := strings.Split(screen, "\n")
	if len(lines) != h {
		t.Errorf("%d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if n := ansi.StringWidth(l); n != w {
			t.Errorf("line %d is %d wide, want %d: %q", i, n, w, ansi.Strip(l))
		}
	}
}

// snap checks the screen's size and compares it with testdata/NAME-WxH.golden.
func snap(t *testing.T, m *Model, name string) {
	t.Helper()
	s := m.Render()
	checkSize(t, s, m.width, m.height)
	got := ansi.Strip(s) + "\n"
	path := filepath.Join("testdata", fmt.Sprintf("%s-%dx%d.golden", name, m.width, m.height))
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/wizard -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from %s:\n%s", name, path, got)
	}
}

func newWizard(t *testing.T, o Options) *Model {
	t.Helper()
	if o.Catalog == nil {
		o.Catalog = catalog(t)
	}
	if o.Dir == "" {
		o.Dir = "/tmp/mygame.flk"
	}
	return New(o)
}

// goStep presses ctrl+n / ctrl+p until the wizard is on the named step.
func goStep(t *testing.T, m *Model, name string) {
	t.Helper()
	want := -1
	for i, st := range m.steps {
		if st.name == name {
			want = i
		}
	}
	if want < 0 {
		t.Fatalf("no step %q", name)
	}
	for m.step < want {
		press(m, "ctrl+n")
	}
	for m.step > want {
		press(m, "ctrl+p")
	}
}

// focusRow presses down/up until the focused row matches.
func focusRow(t *testing.T, m *Model, match func(row) bool) {
	t.Helper()
	target := -1
	for i, rw := range m.rows {
		if match(rw) {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("no such row on step %s", m.cur().name)
	}
	for m.focus < target {
		press(m, "down")
	}
	for m.focus > target {
		press(m, "up")
	}
}

func focusParam(t *testing.T, m *Model, use, param string) {
	t.Helper()
	focusRow(t, m, func(r row) bool { return r.kind == rowParam && r.use == use && r.param == param })
}

// setParam types a value into a param's field (clearing it first).
func setParam(t *testing.T, m *Model, step, use, param, value string) {
	t.Helper()
	goStep(t, m, step)
	focusParam(t, m, use, param)
	press(m, "ctrl+u", value)
}

// pick adds a snippet through the step's add row.
func pick(t *testing.T, m *Model, step, id string) {
	t.Helper()
	goStep(t, m, step)
	focusRow(t, m, func(r row) bool { return r.kind == rowAdd })
	press(m, "enter")
	if m.picker == nil {
		t.Fatalf("no picker on %s: %s", step, m.status)
	}
	for m.picker.ids[m.picker.focus] != id {
		if m.picker.focus == len(m.picker.ids)-1 {
			t.Fatalf("%s isn't offered on %s: %v", id, step, m.picker.ids)
		}
		press(m, "down")
	}
	press(m, "enter")
}

func pickTemplate(t *testing.T, m *Model, id string) {
	t.Helper()
	goStep(t, m, "Start")
	focusRow(t, m, func(r row) bool { return r.kind == rowTemplate && r.tpl == id })
	press(m, "enter")
}

func TestPeek(t *testing.T) {
	if os.Getenv("PEEK") == "" {
		t.Skip()
	}
	m := sized(newWizard(t, Options{}), 160, 48)
	pickTemplate(t, m, "core/steam-game")
	setParam(t, m, "Process", "run-wrapper", "command", "/server/x")
	fmt.Println(ansi.Strip(m.Render()))
	sized(m, 80, 24)
	pick(t, m, "Backups", "core/backup-stop")
	fmt.Println(ansi.Strip(m.Render()))
}

// Every kind of screen, at both reference sizes.
func TestSnapshots(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {160, 48}} {
		t.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(t *testing.T) {
			m := sized(newWizard(t, Options{Validate: func([]byte) ([]string, error) { return nil, nil }}), sz[0], sz[1])
			snap(t, m, "start")
			pickTemplate(t, m, "core/steam-game")
			snap(t, m, "name")
			setParam(t, m, "Process", "run-wrapper", "command", "/server/valheim_server.x86_64")
			snap(t, m, "param")
			goStep(t, m, "Backups")
			focusRow(t, m, func(r row) bool { return r.kind == rowAdd })
			press(m, "enter")
			snap(t, m, "picker")
			press(m, "esc")
			setParam(t, m, "Software", "steamcmd", "app_id", "896660")
			setParam(t, m, "Network", "service", "port", "2456")
			setParam(t, m, "Health", "check-log", "match", "Game server connected")
			goStep(t, m, "Review")
			snap(t, m, "review")
			press(m, "ctrl+t", "ctrl+t")
			goStep(t, m, "Backups")
			snap(t, m, "hints-minimal")
		})
	}
}
