package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/recipe"
)

// The agent path, start to finish, without flk: a template with its open
// params set, then add, set and rm.
func TestTemplateAddSetRm(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	_, errs, code := run(t, noFlk, false, "new", dir, "--template", "daemon", "--yes")
	if code != exitFail || !strings.Contains(errs, "--set service.port=VALUE") || !strings.Contains(errs, "--set packages.list=VALUE") {
		t.Fatalf("open params: exit %d\n%s", code, errs)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a failed new wrote something")
	}
	out, errs, code := run(t, noFlk, false, "new", dir, "--template", "daemon", "--yes", "--app", "flkt-forge-t",
		"--set", "packages.list=redis", "--set", "service.port=6390", "--set", "check-tcp.port=6390")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	r, _ := recipe.Read(dir)
	if r.Template != "core/daemon" || r.App != "flkt-forge-t" || r.Title != "" {
		t.Errorf("recipe: %+v", r)
	}

	if _, errs, code = run(t, noFlk, false, "set", "vm", "memory=512mb", "cpus=2", dir); code != 0 {
		t.Fatalf("set: %s", errs)
	}
	toml := read(t, dir, "flk.toml")
	if !strings.Contains(toml, `memory = "512mb"`) || !strings.Contains(toml, "cpus = 2") {
		t.Errorf("set didn't reach flk.toml:\n%s", toml)
	}
	if _, errs, code = run(t, noFlk, false, "add", "secret", "--set", "name=REDIS_PASSWORD", dir); code != 0 {
		t.Fatalf("add: %s", errs)
	}
	out, _, _ = run(t, noFlk, false, "render", dir, "--json")
	var res struct {
		Report struct{ Run []struct{ Text string } }
	}
	json.Unmarshal([]byte(out), &res)
	found := false
	for _, s := range res.Report.Run {
		found = found || s.Text == "flk secrets set REDIS_PASSWORD=- --stage"
	}
	if !found {
		t.Errorf("no secrets step: %s", out)
	}
	if _, errs, code = run(t, noFlk, false, "rm", "check-tcp", dir); code != 0 {
		t.Fatalf("rm: %s", errs)
	}
	if strings.Contains(read(t, dir, "flk.toml"), "[checks") {
		t.Error("rm left the check")
	}

	// Mistakes change nothing.
	before := read(t, dir, "flkforge.toml")
	for _, args := range [][]string{
		{"set", "vm", "cpus=many", dir},
		{"set", "nope", "x=1", dir},
		{"add", "secret", "--set", "name=bad name", dir},
		{"add", "volume", dir}, // already used: needs --id
		{"rm", "nope", dir},
	} {
		if _, _, code := run(t, noFlk, false, args...); code != exitFail {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
	if read(t, dir, "flkforge.toml") != before {
		t.Error("a failed edit changed the recipe")
	}

	// A second volume with its own id is fine.
	if _, errs, code = run(t, noFlk, false, "add", "volume", "--id", "logs", "--set", "name=logs", "--set", "path=/logs", dir); code != 0 {
		t.Fatalf("add volume --id: %s", errs)
	}
	if !strings.Contains(read(t, dir, "flk.toml"), `source = "logs"`) {
		t.Error("second volume missing")
	}
}

// A seed file the recipe's version moved past is kept, with a note.
func TestSeedKeptWithNote(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	if _, _, code := run(t, noFlk, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes"); code != 0 {
		t.Fatal("new")
	}
	out, _, code := run(t, noFlk, false, "set", "start-script", "body=#!/bin/sh\nexec sleep 1\n", dir)
	if code != 0 || !strings.Contains(out, "the recipe's version changed since it was written") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if !strings.Contains(read(t, dir, "start.sh"), "Hello from") {
		t.Error("the seed file was overwritten")
	}
}

func TestExplain(t *testing.T) {
	for what, want := range map[string]string{
		"run-wrapper":         "Under the hood",
		"kill-timeout-margin": "rule kill-timeout-margin (error)",
		"gpu":                 "Not possible on Firecracker",
		"low-port":            "root step",
	} {
		out, errs, code := run(t, noFlk, false, "explain", what)
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("explain %s: exit %d %s\n%s", what, code, errs, out)
		}
	}
	// A key, without flk: the snippets that set it.
	out, _, code := run(t, noFlk, false, "explain", "kill_timeout")
	if code != 0 || !strings.Contains(out, "core/kill-timeout") {
		t.Errorf("explain kill_timeout: %d\n%s", code, out)
	}
	if _, _, code := run(t, noFlk, false, "explain", "no_such_thing_at_all"); code != exitFail {
		t.Errorf("explain unknown: exit %d", code)
	}
}

func TestTemplatesRender(t *testing.T) {
	out, _, _ := run(t, noFlk, false, "templates", "--json")
	var list []struct{ ID string }
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) < 4 {
		t.Fatalf("%v %s", err, out)
	}
	sets := map[string][]string{
		"core/daemon":      {"packages.list=redis", "service.port=6390", "check-tcp.port=6390"},
		"core/steam-game":  {"steamcmd.app_id=896660", "run-wrapper.command=/server/srv", "service.port=2456", "check-log.match=ready"},
		"core/web-service": nil,
		"core/blank":       nil,
	}
	for _, tpl := range list {
		args := []string{"new", filepath.Join(t.TempDir(), "app"), "--template", tpl.ID, "--yes", "--app", "flkt-x"}
		for _, s := range sets[tpl.ID] {
			args = append(args, "--set", s)
		}
		if out, errs, code := run(t, noFlk, false, args...); code != 0 {
			t.Errorf("%s: exit %d\n%s%s", tpl.ID, code, out, errs)
		} else if strings.Contains(out, "Rules:") {
			t.Errorf("%s renders with rule findings:\n%s", tpl.ID, out)
		}
	}
}

func TestSnippetsShow(t *testing.T) {
	out, _, code := run(t, noFlk, false, "snippets", "show", "volume")
	if code != 0 || !strings.Contains(out, "name  string, default data") {
		t.Errorf("exit %d\n%s", code, out)
	}
	out, _, _ = run(t, noFlk, false, "snippets", "show", "volume", "--json")
	var s struct {
		ID     string
		Params []struct{ Name string }
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil || s.ID != "core/volume" || len(s.Params) < 3 {
		t.Errorf("%v %s", err, out)
	}
}

func read(t *testing.T, dir, f string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, f))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
