package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

const hubSnippet = `id = "dayne/motd"
version = "0.1.0"
title = "A message of the day"
category = "misc"
summary = "Writes /etc/motd in the image."
requires = ["category:base"]

[params.text]
type = "string"
default = "hello"
prompt = "The message"

[[dockerfile]]
slot = "install"
text = "RUN echo {{sh .P.text}} > /etc/motd"

[hints]
what = "Writes /etc/motd."
docs = ["man motd"]
`

// The hub end to end, on a local git repository: add, pull, search, show,
// trust, the lock, a change upstream, relock, publish, rm.
func TestHub(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@x", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@x"} {
		t.Setenv(k, v)
	}
	origin := filepath.Join(tmp, "hub-origin")
	os.MkdirAll(filepath.Join(origin, "dayne", "motd"), 0o755)
	os.WriteFile(filepath.Join(origin, "dayne", "motd", "snippet.toml"), []byte(hubSnippet), 0o644)
	gitIn(t, origin, "init", "-q", "-b", "main")
	gitIn(t, origin, "add", ".")
	gitIn(t, origin, "commit", "-q", "-m", "motd")

	cfgFile := filepath.Join(tmp, "config", "hubs.toml")
	env := func() *forge.Env { return &forge.Env{Version: Version, FlkErr: os.ErrNotExist, Hubs: cfgFile} }
	ok := func(args ...string) string {
		t.Helper()
		out, errs, code := run(t, env, false, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, code, out, errs)
		}
		return out
	}

	ok("hub", "add", "dayne", origin)
	if out := ok("hub", "pull"); !strings.Contains(out, "cloned") {
		t.Errorf("pull: %s", out)
	}
	if out := ok("hub", "search", "motd"); !strings.Contains(out, "dayne/motd") {
		t.Errorf("search: %s", out)
	}
	if out := ok("hub", "show", "dayne/motd"); !strings.Contains(out, "RUN echo") || !strings.Contains(out, "sha256:") {
		t.Errorf("show: %s", out)
	}

	app := filepath.Join(tmp, "app")
	ok("new", app, "--template", "blank", "--yes", "--app", "flkt-hub")
	out, errs, code := run(t, env, false, "add", "dayne/motd", app)
	if code == 0 || !strings.Contains(errs, "add --trust") || !strings.Contains(out, "RUN echo") {
		t.Fatalf("add without --trust: %d\n%s%s", code, out, errs)
	}
	ok("add", "dayne/motd", "--trust", "--set", "text=hi there", app)
	r, err := recipe.Read(app)
	if err != nil {
		t.Fatalf("%v\n%s", err, read(t, app, "flkforge.toml"))
	}
	lock, _ := r.Lock["dayne/motd"].(map[string]any)
	if lock["version"] != "0.1.0" || !strings.HasPrefix(lock["sha256"].(string), "sha256:") || lock["origin"] != "hub dayne" {
		t.Errorf("lock: %v", r.Lock)
	}
	if !strings.Contains(read(t, app, "Dockerfile"), "RUN echo 'hi there' > /etc/motd") {
		t.Error("the hub snippet didn't render")
	}
	ok("render", app) // locked: no --trust needed again

	// A change upstream: pulled, then refused until --relock.
	os.WriteFile(filepath.Join(origin, "dayne", "motd", "snippet.toml"), []byte(strings.Replace(hubSnippet, `version = "0.1.0"`, `version = "0.2.0"`, 1)), 0o644)
	gitIn(t, origin, "commit", "-q", "-am", "0.2.0")
	if out := ok("hub", "pull", "dayne"); !strings.Contains(out, "updated") {
		t.Errorf("pull: %s", out)
	}
	out, _, code = run(t, env, false, "render", app)
	if code != exitFail || !strings.Contains(out, "changed since the recipe locked them") {
		t.Errorf("render after an upstream change: %d\n%s", code, out)
	}
	ok("render", app, "--relock")
	r, _ = recipe.Read(app)
	if r.Lock["dayne/motd"].(map[string]any)["version"] != "0.2.0" {
		t.Errorf("relock: %v", r.Lock)
	}

	// Publish the app's recipe: on a new branch, app name stripped.
	ok("add", "bind", "--set", "address=10.1.2.3", app)
	out = ok("hub", "publish", app, "--hub", "dayne", "--title", "MOTD box", "--summary", "A blank box with a message")
	if !strings.Contains(out, "on branch flkforge/publish-dayne-flkt-hub") || !strings.Contains(out, "core/bind removed") {
		t.Errorf("publish: %s", out)
	}
	clone := filepath.Join(tmp, "data", "flkforge", "hubs", "dayne")
	pub := read(t, clone, "dayne/_templates/flkt-hub.toml")
	if strings.Contains(pub, "flkt-hub\"") || strings.Contains(pub, "core/bind") || !strings.Contains(pub, `title = "MOTD box"`) {
		t.Errorf("published recipe:\n%s", pub)
	}
	if log := gitIn(t, clone, "log", "--oneline", "-1"); !strings.Contains(log, "Publish recipe dayne/flkt-hub") {
		t.Errorf("commit: %s", log)
	}
	if out := ok("hub", "search", "box"); !strings.Contains(out, "dayne/flkt-hub") {
		t.Errorf("the published recipe isn't found: %s", out)
	}
	// Publishing into core is refused; so is a hub that has core snippets.
	if _, errs, code := run(t, env, false, "hub", "publish", app, "--hub", "dayne", "--as", "core/x", "--title", "x", "--summary", "y"); code == 0 || !strings.Contains(errs, "core is flkforge's own") {
		t.Errorf("publish to core: %d %s", code, errs)
	}

	ok("hub", "rm", "dayne")
	if out := ok("hub", "list"); !strings.Contains(out, "No hubs yet") {
		t.Errorf("list after rm: %s", out)
	}
}
