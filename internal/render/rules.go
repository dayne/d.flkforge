package render

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Rule is one check across snippets (SPEC 6.4): `flkforge explain RULE`
// shows it.
type Rule struct {
	ID      string `json:"id"`
	Level   string `json:"level"` // error stops a render; warn and hint don't
	Summary string `json:"summary"`
	Why     string `json:"why"`
}

// Rules is the catalog, in the order they run.
var Rules = []Rule{
	{"image-full-name", "error", "Base images are written with their registry (docker.io/library/...).",
		"podman on this host has no search registries, so a short name such as alpine:3.20 fails with \"short-name did not resolve\"."},
	{"no-shell", "warn", "A base without /bin/sh (scratch, distroless) boots only on flk's static agent init.",
		"flk's init is a shell script; for images without a shell it uses its Go agent as init, which flk has only if Go was on the host when it built the agent (flk doctor says)."},
	{"kill-timeout-max", "error", "kill_timeout is at most 300 seconds.",
		"flk refuses longer ones."},
	{"kill-timeout-margin", "error", "kill_timeout is at least 10s above the run wrapper's STOP_TIMEOUT.",
		"flk kills the VM after kill_timeout. If that comes before the wrapper's own deadline, the server is killed mid-save and the wrapper never escalates."},
	{"wrapper-restart", "warn", "A run wrapper with its own restart loop goes with [[restart]] retries = 1.",
		"flk-run already restarts the server and gives up on a crash loop; flk's default 10 retries would multiply the loop."},
	{"before-deploy-check", "warn", "[flk.backup] before_deploy needs a health check.",
		"The before-deploy backup is for rolling back a release that failed its checks; without a check, no release fails."},
	{"udp-only-check", "warn", "An app whose services are all UDP needs a log (or other) check.",
		"Without checks, deploy waits for the first internal_port over TCP, which a UDP server never answers."},
	{"log-check-dollar", "warn", "A log check's regex doesn't end in $.",
		"flk 0.1.0 matches it against the raw console, whose lines end in \\r\\r, so $ never matches (BUGS.flk.md #2; fixed on flk's r1-reliability branch)."},
	{"busybox-user", "error", "A non-root user on a busybox base needs util-linux's setpriv.",
		"flk's init switches users with setpriv --reuid or chroot --userspec. Busybox's setpriv has no --reuid, so without the setpriv package init runs the app as root and only logs a warning."},
	{"live-backup-console", "error", "A live backup with console hooks needs stdin = \"console\".",
		"Console hooks type into the app's stdin (the serial console); with [flk] stdin = \"null\" nothing reaches the app and the backup copies a world that's still being written."},
	{"stdin-null-console", "hint", "stdin = \"null\" with a console stop: flk console can't type to the server.",
		"flk-run's console stop uses its own FIFO, so stopping still works; only typing in flk console is lost."},
	{"mount-owner", "warn", "A non-root app's mount points are created in the image, owned by its user.",
		"A volume created without --owner takes the owner of its mount point in the image on first use. If the image doesn't have it, it's root's and the app can't write."},
	{"mount-hides", "warn", "A volume mounted over a directory the image fills hides what's there.",
		"The mount covers the image's files: mounting over /home hides the home directories the image made. Install system-wide, mount somewhere else, or seed the volume at first boot."},
	{"secret-in-env", "error", "Secrets go in flk secrets, never in [env].",
		"flk.toml is meant for git. A variable named like a password or token with a value in [env] is a secret in the open: core/secret records only its name."},
	{"private-key", "error", "No private key is ever written into the app (the build context).",
		"Anything in the build context can end up in an image layer. Public keys only; private ones stay with their owner."},
	{"guest-init", "warn", "Nothing in the image expects systemd or manages the network, hostname, /etc/hosts or /etc/resolv.conf.",
		"flk's init is PID 1: there's no systemd, and init writes the hostname, /etc/hosts and /etc/resolv.conf at boot. Daemons run in the foreground, started by the app or its wrapper."},
	{"jvm-heap", "warn", "The JVM heap fits inside the VM's memory, with at least 1 GB to spare.",
		"The JVM uses memory beyond its heap (metaspace, threads, GC, direct buffers), and the kernel and init need some too. A heap too close to [[vm]] memory gets the server killed by the kernel's OOM killer under load."},
	{"huge-pages", "warn", "huge_pages needs memory in whole 2 MB pages and a host pool.",
		"flk takes the VM's memory from a hugetlbfs pool: the host needs vm.nr_hugepages large enough (a root step)."},
}

func ruleLevel(id string) string {
	for _, r := range Rules {
		if r.ID == id {
			return r.Level
		}
	}
	return "warn"
}

// rules runs the catalog on a render, reading the merged flk.toml the way
// flk will.
func rules(uses []*use, base *use, app App, res *Result) []Finding {
	var out []Finding
	add := func(rule, msg, fix string) {
		out = append(out, Finding{Rule: rule, Level: ruleLevel(rule), Msg: msg, Fix: fix})
	}
	dockerfile := string(res.File("Dockerfile").Content)

	if !fullImageName(app.Image) {
		add("image-full-name", "base image "+app.Image+" is a short name: podman without search registries fails with \"short-name did not resolve\"",
			"write the registry too, e.g. docker.io/library/"+app.Image)
	}
	if app.NoShell {
		add("no-shell", app.Image+" has no /bin/sh: flk boots it with its static agent init, which exists only if Go was on the host when flk built its agent",
			"check flk doctor before the first deploy")
	}

	var cfg map[string]any
	if _, err := toml.Decode(string(res.File("flk.toml").Content), &cfg); err != nil {
		return out // render already refused invalid TOML
	}
	table := func(m map[string]any, k string) map[string]any { t, _ := m[k].(map[string]any); return t }
	list := func(m map[string]any, k string) []map[string]any { l, _ := m[k].([]map[string]any); return l }
	env := table(cfg, "env")
	flk := table(cfg, "flk")
	backup := table(flk, "backup")
	checks := table(cfg, "checks")

	kill := 10.0
	if v, ok := cfg["kill_timeout"]; ok {
		if s, ok := seconds(v); ok {
			kill = s
		}
	}
	if kill > 300 {
		add("kill-timeout-max", fmt.Sprintf("kill_timeout is %gs; flk allows at most 300", kill), "lower kill_timeout (and STOP_TIMEOUT with it)")
	}
	if st, ok := env["STOP_TIMEOUT"].(string); ok {
		if n, err := strconv.Atoi(st); err == nil && kill < float64(n)+10 {
			add("kill-timeout-margin",
				fmt.Sprintf("kill_timeout (%gs) must be at least 10s above the wrapper's STOP_TIMEOUT (%ds): flk kills the VM after kill_timeout, before the server has saved", kill, n),
				fmt.Sprintf("set kill_timeout to %d or more (core/kill-timeout), or lower stop_timeout", n+30))
		}
	}

	if hasUse(uses, "run-wrapper") && !usesExec(uses) {
		retries := int64(10)
		if rs := list(cfg, "restart"); len(rs) > 0 {
			if r, ok := rs[0]["retries"].(int64); ok {
				retries = r
			}
		}
		if retries > 1 {
			add("wrapper-restart",
				fmt.Sprintf("flk-run restarts the server itself, and flk's [[restart]] retries it %d more times on top: a crash loop gets multiplied", retries),
				"add core/restart-outer-net ([[restart]] retries = 1)")
		}
	}

	if bd, _ := backup["before_deploy"].(bool); bd && len(checks) == 0 && !hasHTTPChecks(cfg) {
		add("before-deploy-check",
			"[flk.backup] before_deploy = true but there's no health check: no deploy is ever judged failed, so the backup is never used to roll back",
			"add a check (core/check-log, core/check-tcp or core/check-http)")
	}

	if services := list(cfg, "services"); len(services) > 0 && len(checks) == 0 && table(cfg, "http_service") == nil {
		udpOnly := true
		for _, s := range services {
			if p, _ := s["protocol"].(string); p != "udp" {
				udpOnly = false
			}
		}
		if udpOnly {
			add("udp-only-check", "every service is UDP and there's no check: deploy waits for a TCP port that never answers",
				"add core/check-log with the server's ready line")
		}
	}

	for _, name := range sortedKeys(checks) {
		ct, _ := checks[name].(map[string]any)
		if t, _ := ct["type"].(string); t != "log" {
			continue
		}
		if m, _ := ct["match"].(string); strings.HasSuffix(m, "$") && !strings.HasSuffix(m, `\$`) {
			add("log-check-dollar",
				fmt.Sprintf("[checks.%s] match %q ends in $, but console lines end in \\r\\r: flk never matches it and the deploy fails its check", name, m),
				"drop the $ (or end with \\r*$)")
		}
	}

	if app.Busybox && app.User != "" && app.User != "root" {
		hasSetpriv := false
		for _, u := range uses {
			if strings.Contains(u.s.Packages, "setpriv") {
				hasSetpriv = true
			}
		}
		if !hasSetpriv {
			add("busybox-user", "the app runs as "+app.User+" on a busybox base with no setpriv: flk's init can't switch users there and runs it as root",
				"add the setpriv package (core/packages)")
		}
	}

	stdin, _ := flk["stdin"].(string)
	if mode, _ := backup["mode"].(string); mode == "live" && stdin == "null" {
		for _, k := range []string{"pre", "post"} {
			if hooks, ok := backup[k].([]map[string]any); ok {
				for _, h := range hooks {
					if _, console := h["console"]; console {
						add("live-backup-console", "[flk.backup] "+k+" types into the console, but [flk] stdin = \"null\": the hooks never reach the app",
							"drop core/stdin-null, or use host-command hooks")
					}
				}
			}
		}
	}
	if stdin == "null" {
		if st, _ := env["RUN_STOP"].(string); strings.HasPrefix(st, "console:") {
			add("stdin-null-console", "stdin is null, so flk console can't type into the server; flk-run's console stop still works",
				"drop core/stdin-null if you want flk console")
		}
	}

	home := "/root"
	if app.User != "" && app.User != "root" {
		home = "/home/" + app.User
	}
	for _, m := range list(cfg, "mounts") {
		dest, _ := m["destination"].(string)
		dest = path.Clean(dest)
		seeded := dest == home && strings.Contains(dockerfile, "flk-seed-home")
		if app.User != "" && app.User != "root" && dest != home && !ownsDir(dockerfile, dest, app.User) {
			add("mount-owner", dest+" isn't created in the image owned by "+app.User+": a volume made without --owner stays root's, and the app can't write it",
				"use core/volume (it does this), or RUN install -d -o "+app.User+" -g "+app.User+" "+dest)
		}
		if !seeded && slices.Contains([]string{"/", "/home", "/root", "/usr", "/etc", "/opt", "/var", "/srv", home}, dest) {
			add("mount-hides", "a volume at "+dest+" hides what the image put there (home directories, installed tools, config)",
				"mount it at a directory of its own (/data), or seed it at first boot")
		}
	}

	secretName := regexp.MustCompile(`(?i)(PASSWORD|PASSWD|TOKEN|SECRET|API_?KEY|PRIVATE_?KEY|WEBHOOK)`)
	for _, k := range sortedKeys(env) {
		if v, _ := env[k].(string); v != "" && secretName.MatchString(k) {
			add("secret-in-env", "[env] "+k+" has a value and looks like a secret: flk.toml is meant for git",
				"remove it from [env]; add core/secret with name = \""+k+"\" (flk secrets set "+k+"=- reads it from stdin)")
		}
	}
	privateKey := regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	for _, f := range res.Files {
		b := path.Base(f.Path)
		if privateKey.Match(f.Content) || slices.Contains([]string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"}, b) {
			add("private-key", f.Path+" is (or holds) a private key: it would go into the image",
				"keep private keys out of the app; use the public key (.pub), or a secret")
		}
	}

	guest := regexp.MustCompile(`(?m)^\s*(RUN|CMD|ENTRYPOINT).*(systemctl|/sbin/init\b|/lib/systemd|>\s*/etc/(resolv\.conf|hosts|hostname)\b)`)
	if m := guest.FindString(dockerfile); m != "" {
		add("guest-init", "the Dockerfile has "+strings.TrimSpace(m)+": there's no systemd, and flk's init writes the network files",
			"run daemons in the foreground (the app or its run wrapper), and leave /etc/hosts, resolv.conf and the hostname to init")
	}

	if heap, src := jvmHeap(env, dockerfile); heap > 0 {
		mem := vmMemory(list(cfg, "vm"))
		if heap+1024 > mem {
			add("jvm-heap", fmt.Sprintf("the JVM heap (%s: %d MB) leaves less than 1 GB of the VM's %d MB for everything else", src, heap, mem),
				fmt.Sprintf("raise [[vm]] memory to %d MB or more (core/vm), or lower the heap", heap+1536))
		}
	}

	for _, v := range list(cfg, "vm") {
		if hp, ok := v["huge_pages"]; ok && hp != nil && hp != false && hp != "" {
			mem, _ := v["memory"].(string)
			add("huge-pages", fmt.Sprintf("huge_pages = %v: the VM's memory (%s) comes from the host's hugetlbfs pool", hp, mem),
				"make memory a multiple of 2 MB, and as root size the pool: sysctl vm.nr_hugepages=N (N x 2 MB >= the VM's memory)")
		}
	}
	return out
}

// ownsDir: the Dockerfile creates dir owned by user (install -d -o, or chown).
func ownsDir(dockerfile, dir, user string) bool {
	for _, l := range strings.Split(strings.ReplaceAll(dockerfile, "\\\n", " "), "\n") {
		if !strings.Contains(l, dir) {
			continue
		}
		if strings.Contains(l, "install -d") && strings.Contains(l, "-o "+user) || strings.Contains(l, "chown") && strings.Contains(l, user) {
			return true
		}
	}
	return false
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

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var (
	xmxRE  = regexp.MustCompile(`-Xmx([0-9]+)([kKmMgG]?)`)
	sizeRE = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(k|kb|m|mb|g|gb|t|tb)?\s*$`)
)

// jvmHeap finds the JVM's maximum heap in MB: -Xmx in [env] values or the
// Dockerfile, or an env setting named *_HEAP (MC_HEAP = "8G").
func jvmHeap(env map[string]any, dockerfile string) (int, string) {
	for _, k := range sortedKeys(env) {
		v, _ := env[k].(string)
		if m := xmxRE.FindStringSubmatch(v); m != nil {
			return sizeMB(m[1]+m[2], "m"), "[env] " + k + " -Xmx" + m[1] + m[2]
		}
		if strings.HasSuffix(k, "_HEAP") {
			if mb := sizeMB(v, "m"); mb > 0 {
				return mb, "[env] " + k + " = " + v
			}
		}
	}
	if m := xmxRE.FindStringSubmatch(dockerfile); m != nil {
		return sizeMB(m[1]+m[2], "m"), "-Xmx" + m[1] + m[2] + " in the Dockerfile"
	}
	return 0, ""
}

// vmMemory is the first [[vm]]'s memory in MB (flk's default is 256).
func vmMemory(vms []map[string]any) int {
	if len(vms) == 0 {
		return 256
	}
	v := vms[0]
	if n, ok := v["memory_mb"].(int64); ok {
		return int(n)
	}
	if s, ok := v["memory"].(string); ok {
		return sizeMB(s, "m")
	}
	if n, ok := v["memory"].(int64); ok {
		return int(n)
	}
	if s, ok := v["size"].(string); ok {
		// Fly's sizes: shared-cpu-Nx and performance-Nx; flk maps them to
		// memory (flk config show's machine_config has the exact number).
		if strings.HasPrefix(s, "performance") {
			return 2048
		}
	}
	return 256
}

// sizeMB reads "8G", "11gb", "2048mb" or "512" (in defUnit) as MB.
func sizeMB(s, defUnit string) int {
	m := sizeRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	unit := strings.ToLower(m[2])
	if unit == "" {
		unit = defUnit
	}
	switch unit[0] {
	case 'k':
		f /= 1024
	case 'g':
		f *= 1024
	case 't':
		f *= 1024 * 1024
	}
	return int(f)
}
