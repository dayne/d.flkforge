package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// rules checks across snippets after the merge (SPEC 6.4). Each finding
// has an id, a level (error stops a render; warn and hint don't) and a
// fix. They read the merged flk.toml the way flk will.
func rules(uses []*use, base *use, app App, res *Result) []Finding {
	var out []Finding
	add := func(rule, level, msg, fix string) {
		out = append(out, Finding{Rule: rule, Level: level, Msg: msg, Fix: fix})
	}
	if !fullImageName(app.Image) {
		add("image-full-name", "error",
			"base image "+app.Image+" is a short name: podman without search registries fails with \"short-name did not resolve\"",
			"write the registry too, e.g. docker.io/library/"+app.Image)
	}

	var cfg map[string]any
	if _, err := toml.Decode(string(res.File("flk.toml").Content), &cfg); err != nil {
		return out // render already refused invalid TOML
	}
	table := func(m map[string]any, k string) map[string]any { t, _ := m[k].(map[string]any); return t }
	env := table(cfg, "env")
	flk := table(cfg, "flk")
	backup := table(flk, "backup")
	checks := table(cfg, "checks")

	// kill_timeout: at most 300 (flk's limit), and above the wrapper's
	// STOP_TIMEOUT with a margin, or flk kills the VM mid-save.
	kill := 10.0
	if v, ok := cfg["kill_timeout"]; ok {
		if s, ok := seconds(v); ok {
			kill = s
		}
	}
	if kill > 300 {
		add("kill-timeout-max", "error", fmt.Sprintf("kill_timeout is %gs; flk allows at most 300", kill), "lower kill_timeout (and STOP_TIMEOUT with it)")
	}
	if st, ok := env["STOP_TIMEOUT"].(string); ok {
		if n, err := strconv.Atoi(st); err == nil && kill < float64(n)+10 {
			add("kill-timeout-margin", "error",
				fmt.Sprintf("kill_timeout (%gs) must be at least 10s above the wrapper's STOP_TIMEOUT (%ds): flk kills the VM after kill_timeout, before the server has saved", kill, n),
				fmt.Sprintf("set kill_timeout to %d or more (core/kill-timeout), or lower stop_timeout", n+30))
		}
	}

	// A wrapper with its own restart loop goes with [[restart]] retries = 1.
	if hasUse(uses, "run-wrapper") && !usesExec(uses) {
		retries := int64(10)
		if rs, ok := cfg["restart"].([]map[string]any); ok && len(rs) > 0 {
			if r, ok := rs[0]["retries"].(int64); ok {
				retries = r
			}
		}
		if retries > 1 {
			add("wrapper-restart", "warn",
				fmt.Sprintf("flk-run restarts the server itself, and flk's [[restart]] retries it %d more times on top: a crash loop gets multiplied", retries),
				"add core/restart-outer-net ([[restart]] retries = 1)")
		}
	}

	// before_deploy is only useful with a check: it's what makes a failed
	// release roll back with its data.
	if bd, _ := backup["before_deploy"].(bool); bd && len(checks) == 0 && !hasHTTPChecks(cfg) {
		add("before-deploy-check", "warn",
			"[flk.backup] before_deploy = true but there's no health check: no deploy is ever judged failed, so the backup is never used to roll back",
			"add a check (core/check-log or core/check-http)")
	}

	// UDP-only apps have no TCP port to wait on: deploy needs a log check.
	if services, ok := cfg["services"].([]map[string]any); ok && len(services) > 0 && len(checks) == 0 && table(cfg, "http_service") == nil {
		udpOnly := true
		for _, s := range services {
			if p, _ := s["protocol"].(string); p != "udp" {
				udpOnly = false
			}
		}
		if udpOnly {
			add("udp-only-check", "warn",
				"every service is UDP and there's no check: deploy waits for a TCP port that never answers",
				"add core/check-log with the server's ready line")
		}
	}

	// flk matches log checks against the raw console text, where lines end
	// in "\r\r\n": a regex anchored with $ never matches (BUGS.flk.md #2).
	for name, c := range checks {
		ct, _ := c.(map[string]any)
		if t, _ := ct["type"].(string); t != "log" {
			continue
		}
		if m, _ := ct["match"].(string); strings.HasSuffix(m, "$") && !strings.HasSuffix(m, `\$`) {
			add("log-check-dollar", "warn",
				fmt.Sprintf("[checks.%s] match %q ends in $, but console lines end in \\r\\r: flk never matches it and the deploy fails its check", name, m),
				"drop the $ (or end with \\r*$)")
		}
	}

	// A non-root user on a busybox base: init needs setpriv, or it runs the
	// app as root and only logs a warning.
	if base.s.Base.Busybox && app.User != "" && app.User != "root" {
		hasSetpriv := false
		for _, u := range uses {
			if strings.Contains(u.s.Packages, "setpriv") {
				hasSetpriv = true
			}
		}
		if !hasSetpriv {
			add("busybox-user", "error",
				"the app runs as "+app.User+" on a busybox base with no setpriv: flk's init can't switch users there and runs it as root",
				"add the setpriv package (core/packages)")
		}
	}

	// [flk] stdin = "null" with a console stop: the stop still works (it
	// goes through flk-run's own FIFO), but flk console can't type to it.
	if s, _ := flk["stdin"].(string); s == "null" {
		if st, _ := env["RUN_STOP"].(string); strings.HasPrefix(st, "console:") {
			add("stdin-null-console", "hint",
				"stdin is null, so flk console can't type into the server; flk-run's console stop still works",
				"drop core/stdin-null if you want flk console")
		}
	}
	return out
}

func hasUse(uses []*use, name string) bool {
	for _, u := range uses {
		if u.s.Name() == name {
			return true
		}
	}
	return false
}

func usesExec(uses []*use) bool {
	for _, u := range uses {
		if u.s.Name() == "run-wrapper" {
			if b, _ := u.p["exec"].(bool); b {
				return true
			}
		}
	}
	return false
}

func hasHTTPChecks(cfg map[string]any) bool {
	hs, _ := cfg["http_service"].(map[string]any)
	_, ok := hs["checks"]
	return ok
}

// seconds reads a flk duration: a number, or a string such as "30s" or "2m".
func seconds(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case string:
		if n, err := strconv.ParseFloat(x, 64); err == nil {
			return n, true
		}
		if d, err := time.ParseDuration(x); err == nil {
			return d.Seconds(), true
		}
	}
	return 0, false
}

// fullImageName: the first part of the name is a registry host.
func fullImageName(img string) bool {
	first, _, ok := strings.Cut(img, "/")
	return ok && (strings.ContainsAny(first, ".:") || first == "localhost")
}
