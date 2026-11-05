package wizard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/dayne/d.flkforge/internal/recipe"
)

func screen(m *Model) string { return ansi.Strip(m.Render()) }

func steam(t *testing.T) *Model {
	t.Helper()
	m := sized(newWizard(t, Options{}), 80, 24)
	pickTemplate(t, m, "core/steam-game")
	return m
}

func TestQuitNeedsTwiceOnceChanged(t *testing.T) {
	m := sized(newWizard(t, Options{}), 80, 24)
	press(m, "esc")
	if !m.Quitting() {
		t.Error("an untouched wizard should quit on the first esc")
	}
	m = steam(t)
	press(m, "esc")
	if m.Quitting() || !strings.Contains(screen(m), "again quits anyway") {
		t.Errorf("first esc after changes: quitting=%v\n%s", m.Quitting(), screen(m))
	}
	press(m, "esc")
	if !m.Quitting() {
		t.Error("second esc should quit")
	}
}

func TestEnteringAStepFocusesWhatsMissing(t *testing.T) {
	m := steam(t)
	goStep(t, m, "Software")
	if rw := m.rows[m.focus]; rw.use != "steamcmd" || rw.param != "app_id" {
		t.Errorf("focus on %+v, want steamcmd.app_id", rw)
	}
}

func TestBadValueShowsAndRendersWithout(t *testing.T) {
	m := steam(t)
	setParam(t, m, "Network", "service", "port", "lots")
	if !strings.Contains(screen(m), `✗ "lots" isn't a whole number`) {
		t.Errorf("no error shown:\n%s", screen(m))
	}
	u, _ := m.snippetOf("service")
	if _, ok := u.Params["port"]; ok {
		t.Error("a bad value reached the recipe")
	}
	press(m, "ctrl+u", "2456")
	if u.Params["port"] != int64(2456) {
		t.Errorf("port = %v", u.Params["port"])
	}
}

func TestEnumCycles(t *testing.T) {
	m := steam(t)
	goStep(t, m, "Network")
	focusParam(t, m, "service", "protocol")
	// udp → (the default, tcp) → tcp → udp
	press(m, "right")
	u, _ := m.snippetOf("service")
	if v, ok := u.Params["protocol"]; ok {
		t.Errorf("protocol = %v after → from udp, want unset (the default)", v)
	}
	press(m, "right")
	if u.Params["protocol"] != "tcp" {
		t.Errorf("protocol = %v, want tcp", u.Params["protocol"])
	}
	press(m, "right")
	if u.Params["protocol"] != "udp" {
		t.Errorf("protocol = %v, want udp", u.Params["protocol"])
	}
}

func TestChangeBaseKeepsOneBase(t *testing.T) {
	m := steam(t)
	pick(t, m, "Base", "core/base-debian")
	n := 0
	for _, u := range m.r.Uses {
		if strings.HasPrefix(u.Snippet, "core/base-") {
			n++
		}
	}
	if n != 1 || m.r.Uses[0].Snippet != "core/base-debian" {
		t.Errorf("uses after changing the base: %+v", m.r.Uses)
	}
}

func TestRemove(t *testing.T) {
	m := steam(t)
	goStep(t, m, "Process")
	focusRow(t, m, func(r row) bool { return r.kind == rowUse && r.use == "restart-outer-net" })
	press(m, "ctrl+d")
	for _, u := range m.r.Uses {
		if u.Key() == "restart-outer-net" {
			t.Fatal("not removed")
		}
	}
	if !strings.Contains(screen(m), "removed restart-outer-net") {
		t.Errorf("no status:\n%s", screen(m))
	}
	goStep(t, m, "Base")
	focusRow(t, m, func(r row) bool { return r.kind == rowUse })
	press(m, "ctrl+d")
	if !strings.Contains(screen(m), "a recipe needs a base") {
		t.Error("removing the base should be refused")
	}
}

func TestWriteNeedsAWholeRecipe(t *testing.T) {
	wrote := false
	m := sized(newWizard(t, Options{Write: func(*recipe.Recipe) (string, bool, error) { wrote = true; return "", false, nil }}), 80, 24)
	pickTemplate(t, m, "core/steam-game")
	goStep(t, m, "Review")
	press(m, "enter")
	if wrote || !strings.Contains(screen(m), "fix the problems first") {
		t.Errorf("wrote=%v\n%s", wrote, screen(m))
	}
}

func TestHelpAndHints(t *testing.T) {
	m := steam(t)
	goStep(t, m, "Backups")
	focusRow(t, m, func(r row) bool { return r.kind == rowUse })
	press(m, "?")
	if !strings.Contains(screen(m), "flkforge: keys") {
		t.Errorf("no help:\n%s", screen(m))
	}
	press(m, "x")
	if strings.Contains(screen(m), "flkforge: keys") {
		t.Error("help stayed up")
	}
	press(m, "ctrl+t", "ctrl+t")
	if m.hint != 2 {
		t.Errorf("hint level %d", m.hint)
	}
}

func TestMultilineIsReadOnly(t *testing.T) {
	m := sized(newWizard(t, Options{}), 80, 24)
	pickTemplate(t, m, "core/web-service")
	goStep(t, m, "Process")
	focusParam(t, m, "start-script", "body")
	press(m, "x")
	u, _ := m.snippetOf("start-script")
	if _, ok := u.Params["body"]; ok {
		t.Error("typing changed a multi-line param")
	}
	if !strings.Contains(screen(m), "lines: edit the file after writing") {
		t.Errorf("not shown as multi-line:\n%s", screen(m))
	}
}

func TestTakenName(t *testing.T) {
	m := sized(newWizard(t, Options{Taken: func(app string) string {
		if app == "valheim-flk" {
			return "/home/x/valheim.flk/flk.toml"
		}
		return ""
	}}), 80, 24)
	pickTemplate(t, m, "core/blank")
	press(m, "ctrl+u", "valheim-flk", "ctrl+n", "ctrl+p")
	if !strings.Contains(screen(m), "already registered") {
		t.Errorf("no warning:\n%s", screen(m))
	}
}

func TestEveryScreenFits(t *testing.T) {
	for _, sz := range [][2]int{{60, 16}, {80, 24}, {100, 30}, {119, 40}, {120, 40}, {200, 60}} {
		m := sized(newWizard(t, Options{}), sz[0], sz[1])
		checkSize(t, m.Render(), sz[0], sz[1])
		pickTemplate(t, m, "core/dev-sandbox")
		for range m.steps {
			checkSize(t, m.Render(), sz[0], sz[1])
			press(m, "ctrl+n")
		}
		press(m, "?")
		checkSize(t, m.Render(), sz[0], sz[1])
	}
}
