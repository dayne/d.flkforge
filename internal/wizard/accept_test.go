package wizard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
)

// env is forge with flk when it's on PATH (as the CLI has it).
func env() *forge.Env {
	e := &forge.Env{Version: "0.1.0"}
	if x, err := flk.Find(); err == nil {
		e.Flk = flk.New(x)
	} else {
		e.FlkErr = err
	}
	return e
}

func writer(e *forge.Env, dir string, edit bool) func(*recipe.Recipe) (string, bool, error) {
	return func(r *recipe.Recipe) (string, bool, error) {
		var out *forge.Outcome
		var err error
		if edit {
			out, err = e.Render(dir, r, forge.RenderOpts{})
		} else {
			out, err = e.New(dir, r, forge.NewOpts{App: r.App})
		}
		if err != nil {
			return "", true, err
		}
		return strings.Join(out.FlkWarnings, "\n"), out.Refused != "", nil
	}
}

// TestAcceptance is PLAN M4's acceptance test, driven by key presses:
// pick steam-game, change the name, ports and a backup setting, write;
// the result passes check. Then edit, swap backup-pause for backup-stop,
// write again: only the backup lines change.
func TestAcceptance(t *testing.T) {
	e := env()
	dir := filepath.Join(t.TempDir(), "mygame.flk")
	cat, err := e.Catalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := sized(New(Options{Catalog: cat, Dir: dir, Write: writer(e, dir, false)}), 80, 24)

	pickTemplate(t, m, "core/steam-game")
	goStep(t, m, "Name")
	press(m, "ctrl+u", "flkt-forge-steam")
	setParam(t, m, "Software", "steamcmd", "app_id", "896660")
	setParam(t, m, "Process", "run-wrapper", "command", "/server/valheim_server.x86_64, -port, 2466")
	setParam(t, m, "Network", "service", "port", "2466")
	setParam(t, m, "Network", "service", "host_port", "2476")
	setParam(t, m, "Health", "check-log", "match", "Game server connected")
	setParam(t, m, "Backups", "backup-pause", "keep", "5")
	goStep(t, m, "Review")
	focusRow(t, m, func(r row) bool { return r.kind == rowWrite })
	press(m, "enter")
	if !m.Written() {
		t.Fatalf("not written: %q\n%s", m.report, m.Render())
	}
	toml := read(t, dir, "flk.toml")
	for _, want := range []string{`app = "flkt-forge-steam"`, "internal_port = 2466", "port = 2476", "keep = 5", `mode = "pause"`, "app_update 896660"} {
		if !strings.Contains(toml+read(t, dir, "Dockerfile"), want) {
			t.Errorf("missing %q", want)
		}
	}
	res := e.Check(dir)
	for _, l := range res.Lines {
		if l.Status == "FAIL" && l.Group != "tools" {
			t.Errorf("check: %s: %s", l.Group, l.Msg)
		}
	}
	if e.Flk != nil && res.Fail > 0 {
		t.Errorf("check failed: %+v", res.Lines)
	}

	// Edit: swap backup-pause for backup-stop.
	before := map[string]string{}
	for _, f := range []string{"Dockerfile", "flk.toml", "flk-run"} {
		before[f] = read(t, dir, f)
	}
	r, err := recipe.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	m = sized(New(Options{Catalog: cat, Dir: dir, Recipe: r, Edit: true, Write: writer(e, dir, true)}), 80, 24)
	pick(t, m, "Backups", "core/backup-stop")
	goStep(t, m, "Review")
	focusRow(t, m, func(r row) bool { return r.kind == rowWrite })
	press(m, "enter")
	if !m.Written() {
		t.Fatalf("edit not written: %q", m.report)
	}
	for _, f := range []string{"Dockerfile", "flk-run"} {
		if read(t, dir, f) != before[f] {
			t.Errorf("%s changed", f)
		}
	}
	gone, came := diffLines(before["flk.toml"], read(t, dir, "flk.toml"))
	// The pause comment and mode go; the stop comment and mode come; keep
	// stays (backup-stop has it too, so the swap carries it over).
	if !anyContains(gone, `mode = "pause"`) || !anyContains(came, `mode = "stop"`) {
		t.Errorf("mode didn't swap: -%v +%v", gone, came)
	}
	for _, l := range append(gone, came...) {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") && !strings.Contains(l, "mode =") {
			t.Errorf("unexpected change: %q", l)
		}
	}
	if !strings.Contains(read(t, dir, "flk.toml"), "keep = 5") {
		t.Error("keep wasn't carried over to backup-stop")
	}
	if res := e.Check(dir); hasFail(res, "files") || hasFail(res, "recipe") {
		t.Errorf("check after edit: %+v", res.Lines)
	}
}

func hasFail(r *forge.CheckResult, group string) bool {
	for _, l := range r.Lines {
		if l.Group == group && l.Status == "FAIL" {
			return true
		}
	}
	return false
}

func anyContains(l []string, s string) bool {
	for _, x := range l {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

// diffLines returns the lines only in a and only in b (as multisets).
func diffLines(a, b string) (gone, came []string) {
	count := map[string]int{}
	for _, l := range strings.Split(a, "\n") {
		count[l]++
	}
	for _, l := range strings.Split(b, "\n") {
		if count[l] > 0 {
			count[l]--
		} else {
			came = append(came, l)
		}
	}
	for l, n := range count {
		for ; n > 0; n-- {
			gone = append(gone, l)
		}
	}
	return
}

func read(t *testing.T, dir, f string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, f))
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(b))
}
