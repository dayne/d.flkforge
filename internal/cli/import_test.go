package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A small hand-written app to import.
func handApp(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hand.flk")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "flk.toml"), []byte(`app = "hand-flk"
kill_timeout = 30

[build]
  dockerfile = "Dockerfile"

[env]
  GREETING = "hi"
  TZ = "UTC"

[[mounts]]
  source = "data"
  destination = "/data"

[http_service]
  internal_port = 8080

[checks.web]
  type = "http"
  port = 8080
  path = "/"
  grace_period = "20s"

[metrics]
  port = 9090
`), 0o644)
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM alpine:3.20
RUN apk add --no-cache busybox-extras
COPY site.conf /etc/site.conf
COPY start.sh /start.sh
RUN chmod +x /start.sh
RUN echo built > /etc/built
CMD ["/start.sh"]
`), 0o644)
	os.WriteFile(filepath.Join(dir, "start.sh"), []byte("#!/bin/sh\nexec httpd -f -p 8080\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "site.conf"), []byte("x\n"), 0o644)
	return dir
}

func TestImport(t *testing.T) {
	src := handApp(t)
	before, _ := os.ReadFile(filepath.Join(src, "flk.toml"))

	// Default: the recipe on stdout, notes on stderr, nothing written.
	out, errs, code := run(t, noFlk, false, "import", src)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{`snippet = "core/base-alpine"`, `snippet = "core/start-script"`, `snippet = "core/check-http"`, `snippet = "core/timezone"`, `[metrics]`} {
		if !strings.Contains(out, want) {
			t.Errorf("recipe lacks %s:\n%s", want, out)
		}
	}
	if !strings.Contains(errs, "short name") || !strings.Contains(errs, "kept as is") {
		t.Errorf("notes: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(src, "flkforge.toml")); err == nil {
		t.Error("import wrote into SRC without --in-place")
	}

	// --to: a new app directory that renders.
	dir := filepath.Join(t.TempDir(), "out")
	out, errs, code = run(t, noFlk, false, "import", src, "--to", dir, "--json")
	var res struct {
		Written string
		Missing []string `json:"to_copy"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || code != 0 {
		t.Fatalf("exit %d %v: %s%s", code, err, out, errs)
	}
	if res.Written != dir || len(res.Missing) != 1 || res.Missing[0] != "site.conf" {
		t.Errorf("--to: %+v", res)
	}
	df := read(t, dir, "Dockerfile")
	if !strings.Contains(df, "RUN echo built > /etc/built") || !strings.Contains(read(t, dir, "start.sh"), "httpd") {
		t.Errorf("Dockerfile:\n%s", df)
	}

	// --in-place: only flkforge.toml, and not twice.
	if _, errs, code = run(t, noFlk, false, "import", src, "--in-place"); code != 0 {
		t.Fatalf("--in-place: %s", errs)
	}
	if after, _ := os.ReadFile(filepath.Join(src, "flk.toml")); string(after) != string(before) {
		t.Error("--in-place changed flk.toml")
	}
	if _, errs, code = run(t, noFlk, false, "import", src, "--in-place"); code == 0 || !strings.Contains(errs, "already has a recipe") {
		t.Errorf("second --in-place: %d %s", code, errs)
	}
	// Adopting it: render shows the drift of the hand-written files.
	out, _, code = run(t, noFlk, false, "render", src, "--dry-run")
	if code != exitFail || !strings.Contains(out, "flkforge didn't write it") {
		t.Errorf("render after --in-place: %d\n%s", code, out)
	}
}
