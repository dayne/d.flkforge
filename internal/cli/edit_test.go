package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/forge"
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

// templateSets fills each template's open params with real-looking values.
var templateSets = map[string][]string{
	"core/blank":       nil,
	"core/web-service": nil,
	"core/dev-sandbox": nil,
	"core/daemon":      {"packages.list=redis", "service.port=6390", "check-tcp.port=6390"},
	"core/steam-game":  {"steamcmd.app_id=896660", "run-wrapper.command=/server/valheim_server.x86_64", "service.port=2456", "check-log.match=Game server connected"},
	"core/steam-game-windows": {"steamcmd.app_id=2278520", "run-wrapper.command=wine64,/server/enshrouded_server.exe",
		"service.port=15637", "check-log.match=Switching state from LoadEcsScene to Run"},
	"core/java-server": {"service.port=25565", "check-log.match=Done \\(", "check-tcp.port=25565"},
	"core/dotnet-server": {"download-sha256.name=VS", "download-sha256.version=1.22.7",
		"download-sha256.sha256=a7d4a520604590a96de812ee6c6baf4404d43d86cf1ec4fbac1b3a580b52c847",
		"download-sha256.file=dist/vs_server_linux-x64_${VS_VERSION}.tar.gz", "run-wrapper.command=dotnet,VintagestoryServer.dll",
		"service.port=42420", "check-log.match=Dedicated Server now running"},
	"core/python-workbench": {"git-pinned.url=https://github.com/HazyResearch/ncm.git",
		"git-pinned.ref=c70274273f489abc73832b2b98584a8483b6b203", "git-pinned.dest=/opt/ncm"},
}

// TestTemplatesGolden: PLAN M3. Every template renders, with no rule
// findings, to the files in testdata/golden/templates/NAME; with flk on
// PATH, flk config show reads each flk.toml with no warnings.
func TestTemplatesGolden(t *testing.T) {
	out, _, _ := run(t, noFlk, false, "templates", "--json")
	var list []struct{ ID string }
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != len(templateSets) {
		t.Errorf("%d templates, %d in templateSets: add the new ones", len(list), len(templateSets))
	}
	env := noFlk
	if x, err := flk.Find(); err == nil {
		env = func() *forge.Env { return &forge.Env{Version: Version, Flk: flk.New(x)} }
	}
	for _, tpl := range list {
		t.Run(tpl.ID, func(t *testing.T) {
			sets, ok := templateSets[tpl.ID]
			if !ok {
				t.Fatalf("no values for %s in templateSets", tpl.ID)
			}
			dir := filepath.Join(t.TempDir(), "app")
			args := []string{"new", dir, "--template", tpl.ID, "--yes", "--app", "flkt-template"}
			for _, s := range sets {
				args = append(args, "--set", s)
			}
			out, errs, code := run(t, env, false, args...)
			if code != 0 {
				t.Fatalf("exit %d\n%s%s", code, out, errs)
			}
			if strings.Contains(out, "Rules:") {
				t.Errorf("rule findings:\n%s", out)
			}
			if strings.Contains(out, "flk config show warns") {
				t.Errorf("flk warns:\n%s", out)
			}
			for _, f := range []string{"Dockerfile", "flk.toml"} {
				got := read(t, dir, f)
				path := filepath.Join("../../testdata/golden/templates", strings.TrimPrefix(tpl.ID, "core/"), f)
				if *update {
					os.MkdirAll(filepath.Dir(path), 0o755)
					os.WriteFile(path, []byte(got), 0o644)
					continue
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (make golden writes it)", err)
				}
				if got != string(want) {
					t.Errorf("%s differs from %s (make golden; review with git diff):\n%s", f, path, got)
				}
			}
		})
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
