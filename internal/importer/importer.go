// Package importer builds a recipe from a hand-written flk app (PLAN,
// Later): its flk.toml, its Dockerfile, and the files they copy. Best
// effort: what matches a known snippet becomes that snippet with its
// params; what doesn't is kept as is, in core/flk-toml and
// core/dockerfile-lines, so nothing is lost; every decision is a note.
package importer

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/snippet"
	"github.com/dayne/d.flkforge/internal/source"
)

// Input is the app to import.
type Input struct {
	FlkToml    string
	Dockerfile string // "" if the app uses [build] image
	// ReadFile reads a file of the app's directory (a script the
	// Dockerfile copies), or fails.
	ReadFile func(name string) ([]byte, error)
}

// Result is the imported recipe and what happened to each part.
type Result struct {
	Recipe *recipe.Recipe
	// Notes: what matched which snippet, and what was kept as is.
	Notes []string
	// Kept counts the flk.toml keys and Dockerfile instructions kept as is.
	KeptToml, KeptDocker int
}

type importer struct {
	cat   *source.Catalog
	in    Input
	r     *recipe.Recipe
	notes []string
	cfg   map[string]any
	// what the Dockerfile told us
	baseUser   string
	appUser    string
	mounts     []string
	steamDir   string
	startDest  string
	keptToml   int
	keptDocker int
}

// Import builds a recipe from an app.
func Import(in Input, cat *source.Catalog) (*Result, error) {
	im := &importer{cat: cat, in: in, r: &recipe.Recipe{}}
	if _, err := toml.Decode(in.FlkToml, &im.cfg); err != nil {
		return nil, fmt.Errorf("flk.toml: %v", err)
	}
	if app, ok := im.cfg["app"].(string); ok {
		im.r.App = app
		delete(im.cfg, "app")
	}
	for _, m := range list(im.cfg, "mounts") {
		if d, ok := m["destination"].(string); ok {
			im.mounts = append(im.mounts, path.Clean(d))
		}
	}
	if in.Dockerfile != "" {
		if err := im.dockerfile(); err != nil {
			return nil, err
		}
	}
	im.flkToml()
	im.sortUses()
	return &Result{Recipe: im.r, Notes: im.notes, KeptToml: im.keptToml, KeptDocker: im.keptDocker}, nil
}

func (im *importer) note(format string, a ...any) {
	im.notes = append(im.notes, fmt.Sprintf(format, a...))
}

// use adds a use of a snippet with params; id when the snippet is used
// more than once.
func (im *importer) use(id, snip string, params map[string]any) {
	u := recipe.Use{Snippet: snip, Params: params}
	name := snip[strings.LastIndex(snip, "/")+1:]
	if id != "" && id != name {
		u.ID = id
	}
	for slices.ContainsFunc(im.r.Uses, func(o recipe.Use) bool { return o.Key() == u.Key() }) {
		u.ID = u.Key() + "-2"
	}
	im.r.Uses = append(im.r.Uses, u)
}

func (im *importer) find(snip string) *recipe.Use {
	for i := range im.r.Uses {
		if im.r.Uses[i].Snippet == snip {
			return &im.r.Uses[i]
		}
	}
	return nil
}

// sortUses puts the uses in category order (base first), as a recipe
// written by hand would be.
func (im *importer) sortUses() {
	rank := func(u recipe.Use) int {
		s, err := im.cat.Get(u.Snippet)
		if err != nil {
			return len(snippet.Categories)
		}
		return slices.Index(snippet.Categories, s.Category)
	}
	sort.SliceStable(im.r.Uses, func(i, j int) bool { return rank(im.r.Uses[i]) < rank(im.r.Uses[j]) })
}

func table(m map[string]any, k string) map[string]any { t, _ := m[k].(map[string]any); return t }

func list(m map[string]any, k string) []map[string]any { return maps(m[k]) }

// maps reads an array of tables, written either way ([[x]] or x = [{...}]).
func maps(v any) []map[string]any {
	switch l := v.(type) {
	case []map[string]any:
		return l
	case []any:
		out := make([]map[string]any, 0, len(l))
		for _, e := range l {
			t, ok := e.(map[string]any)
			if !ok {
				return nil
			}
			out = append(out, t)
		}
		return out
	}
	return nil
}

// --- Dockerfile ---------------------------------------------------------------

var (
	appUpdateRE  = regexp.MustCompile(`\+app_update (\d+)`)
	installDirRE = regexp.MustCompile(`\+force_install_dir (\S+)`)
	checkFileRE  = regexp.MustCompile(`test -[efx] (\S+)/(\S+)`)
)

func (im *importer) dockerfile() error {
	stages := parseDockerfile(im.in.Dockerfile)
	if len(stages) == 0 {
		return fmt.Errorf("the Dockerfile has no FROM")
	}
	final := stages[len(stages)-1]
	// Earlier stages: a steamcmd download is core/steamcmd; others can't
	// be imported (a recipe's stages come from snippets).
	var steamStage string
	for _, st := range stages[:len(stages)-1] {
		if st.image == final.image || stageIsBase(st, final) {
			continue
		}
		if im.steamcmd(st.ins, false) {
			steamStage = st.name
			continue
		}
		im.note("build stage %q (FROM %s) isn't imported: no snippet makes it; add it with core/dockerfile-lines by hand", st.name, st.image)
		im.keptDocker++
	}
	// A multi-stage file whose final stage is FROM an earlier stage
	// (enshrouded's `FROM wine AS server`): walk the earlier one first.
	var ins []instr
	image := final.image
	for _, st := range stages[:len(stages)-1] {
		if stageIsBase(st, final) {
			image = st.image
			ins = append(ins, st.ins...)
		}
	}
	ins = append(ins, final.ins...)
	im.base(image, ins)
	im.instructions(ins, steamStage)
	return nil
}

func stageIsBase(st, final stage) bool { return st.name != "" && final.image == st.name }

// base matches the final stage's image to a base snippet.
func (im *importer) base(image string, ins []instr) {
	full := image
	first, _, hasSlash := strings.Cut(image, "/")
	registry := hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost")
	if !registry {
		if hasSlash {
			full = "docker.io/" + image
		} else {
			full = "docker.io/library/" + image
		}
		im.note("FROM %s is a short name: written in full as %s (podman here has no search registries)", image, full)
	}
	repo, tag, _ := strings.Cut(full, ":")
	hasWine := false
	for _, in := range ins {
		if in.cmd == "RUN" && strings.Contains(in.args, "wine64") {
			hasWine = true
		}
	}
	switch {
	case repo == "docker.io/library/ubuntu":
		im.use("", "core/base-ubuntu", map[string]any{"version": tag})
		im.baseUser = "ubuntu"
	case repo == "docker.io/library/debian":
		im.use("", "core/base-debian", map[string]any{"version": tag})
	case repo == "docker.io/library/alpine":
		im.use("", "core/base-alpine", map[string]any{"version": tag})
	case repo == "docker.io/library/eclipse-temurin":
		v, kind, _ := strings.Cut(tag, "-")
		im.use("", "core/base-temurin", map[string]any{"version": v, "jdk": kind == "jdk"})
		im.baseUser = "ubuntu"
	case repo == "mcr.microsoft.com/dotnet/runtime":
		im.use("", "core/base-dotnet", map[string]any{"version": tag})
		im.baseUser = "ubuntu"
	case repo == "docker.io/library/python" && strings.HasSuffix(tag, "-slim"):
		im.use("", "core/base-python", map[string]any{"version": strings.TrimSuffix(tag, "-slim")})
	case repo == "docker.io/steamcmd/steamcmd" && hasWine:
		im.use("", "core/base-wine", map[string]any{"version": tag})
		im.baseUser = "ubuntu"
	default:
		family := "apt"
		for _, in := range ins {
			if in.cmd == "RUN" {
				if _, f, ok := packagesIn(in.args); ok {
					family = f
				}
			}
		}
		p := map[string]any{"image": full, "family": family}
		if family == "apk" {
			p["busybox"] = true
		}
		if strings.Contains(repo, "steamcmd") {
			im.baseUser = "ubuntu"
			p["user"] = "ubuntu"
		}
		im.use("", "core/base-image", p)
		im.note("base %s: core/base-image with family %s; check its user (uid 1000) and entrypoint", full, family)
		return
	}
	im.note("FROM %s: %s", full, im.r.Uses[len(im.r.Uses)-1].Snippet)
}

// steamcmd recognizes a steamcmd download in a list of instructions.
func (im *importer) steamcmd(ins []instr, single bool) bool {
	for _, in := range ins {
		if in.cmd != "RUN" {
			continue
		}
		m := appUpdateRE.FindStringSubmatch(in.args)
		if m == nil {
			continue
		}
		id, _ := strconv.ParseInt(m[1], 10, 64)
		p := map[string]any{"app_id": id}
		if strings.Contains(in.args, "ForcePlatformType windows") {
			p["platform"] = "windows"
		}
		if d := installDirRE.FindStringSubmatch(in.args); d != nil {
			im.steamDir = strings.TrimSuffix(d[1], "\"")
			if im.steamDir != "/server" {
				p["dir"] = im.steamDir
			}
		}
		if c := checkFileRE.FindStringSubmatch(in.args); c != nil && c[1] == im.steamDir {
			p["check_file"] = c[2]
		}
		if single {
			p["single_stage"] = true
		}
		im.use("", "core/steamcmd", p)
		im.note("steamcmd download of app %d: core/steamcmd (the pinned build comes from [build] args)", id)
		return true
	}
	return false
}

// instructions matches the final stage's instructions.
func (im *importer) instructions(ins []instr, steamStage string) {
	var pkgs []string
	var copied = map[string]string{} // dest -> source file
	var kept []instr
	wine := im.find("core/base-wine") != nil
	finalUser, workdir := "", ""
	homeEnv := ""
	var imageEnv = map[string]any{}
	var copyFiles []instr
	var workdirs []instr
	entryClear := false
	var cmd []string
	renamed, sudo := "", false
	repoAdded := false // after add-apt-repository, package lines stay where they are

	for i := range ins {
		ins[i].idx = i
	}
	for i := 0; i < len(ins); i++ {
		in := ins[i]
		switch in.cmd {
		case "RUN":
			if p, _, ok := packagesIn(in.args); ok && !repoAdded {
				pkgs = append(pkgs, p...)
				continue
			}
			if strings.Contains(in.args, "add-apt-repository") || strings.Contains(in.args, "/etc/apt/sources.list") {
				repoAdded = true
			}
			if appUpdateRE.MatchString(in.args) && im.find("core/steamcmd") == nil {
				im.steamcmd([]instr{in}, true)
				continue
			}
			if appUpdateRE.MatchString(in.args) {
				continue
			}
			if wine && strings.Contains(in.args, "wineboot") {
				continue // base-wine makes the prefix
			}
			if m := regexp.MustCompile(`^usermod -l (\S+) (\S+)`).FindStringSubmatch(in.args); m != nil {
				renamed = m[1]
				sudo = strings.Contains(in.args, "NOPASSWD")
				continue
			}
			if im.onlyDirs(in.args) {
				continue
			}
			if dest, ok := chmodOf(in.args); ok && copied[dest] != "" {
				continue
			}
			kept = append(kept, in)
		case "COPY":
			f := strings.Fields(in.args)
			var flags, rest []string
			for _, w := range f {
				if strings.HasPrefix(w, "--") {
					flags = append(flags, w)
				} else {
					rest = append(rest, w)
				}
			}
			if len(flags) > 0 && strings.HasPrefix(flags[0], "--from=") {
				if strings.TrimPrefix(flags[0], "--from=") == steamStage {
					continue // core/steamcmd copies it
				}
				kept = append(kept, in)
				continue
			}
			if len(rest) == 2 && !strings.HasSuffix(rest[1], "/") && im.in.ReadFile != nil {
				if _, err := im.in.ReadFile(rest[0]); err == nil {
					copied[rest[1]] = rest[0]
					copyFiles = append(copyFiles, in)
					continue
				}
			}
			copyFiles = append(copyFiles, in)
		case "ENV":
			kv := envPairs(in.args)
			if wine && kv["WINEPREFIX"] != "" {
				continue
			}
			for k, v := range kv {
				if k == "HOME" {
					homeEnv = v
					continue
				}
				if k == "DEBIAN_FRONTEND" {
					continue
				}
				imageEnv[k] = v
			}
		case "USER":
			finalUser = in.user // a WORKDIR carries over a USER switch
		case "WORKDIR":
			workdir = in.args
			workdirs = append(workdirs, in)
		case "ENTRYPOINT":
			if strings.TrimSpace(in.args) == "[]" {
				entryClear = true
				continue
			}
			kept = append(kept, in)
		case "CMD":
			cmd, _ = execForm(in.args)
		case "ARG":
			name, _, _ := strings.Cut(in.args, "=")
			if name == "STEAM_BUILDID" && im.find("core/steamcmd") != nil {
				continue
			}
			kept = append(kept, in)
		default:
			kept = append(kept, in)
		}
	}
	// A COPY or WORKDIR before a kept RUN stays in place: the RUN may use
	// it (vintagestory's tarball in /tmp, ncm's WORKDIR for pip), and the
	// snippets' slots would move it after.
	lastRun := -1
	for _, k := range kept {
		if k.cmd == "RUN" {
			lastRun = max(lastRun, k.idx)
		}
	}
	startCmd := ""
	if len(cmd) == 1 {
		startCmd = cmd[0]
	}
	var laterCopies []instr
	for _, c := range copyFiles {
		f := strings.Fields(c.args)
		if c.idx < lastRun && f[len(f)-1] != startCmd {
			kept = append(kept, c)
		} else {
			laterCopies = append(laterCopies, c)
		}
	}
	copyFiles = laterCopies
	lastKeptWorkdir := ""
	for _, w := range workdirs {
		if w.idx < lastRun {
			kept = append(kept, w)
			lastKeptWorkdir = w.args
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].idx < kept[j].idx })
	// Packages the snippets add themselves.
	own := map[string]bool{}
	if wine {
		own["wine64"], own["procps"] = true, true
	}
	if _, ok := table(im.cfg, "env")["TZ"]; ok {
		own["tzdata"] = true
	}
	var rest []string
	for _, p := range pkgs {
		if !own[p] && !slices.Contains(rest, p) {
			rest = append(rest, p)
		}
	}
	if renamed != "" && sudo {
		rest = slices.DeleteFunc(rest, func(p string) bool { return p == "sudo" })
	}
	if len(rest) > 0 {
		im.use("", "core/packages", map[string]any{"list": rest})
		im.note("packages: core/packages (%s)", strings.Join(rest, ", "))
	}
	// The user.
	switch {
	case renamed != "":
		p := map[string]any{"name": renamed, "sudo": sudo}
		if workdir != "" && workdir != "/home/"+renamed {
			p["workdir"] = workdir
		}
		im.use("", "core/user-rename", p)
		im.appUser = renamed
		im.note("usermod -l %s: core/user-rename (sudo %v)", renamed, sudo)
	case finalUser != "":
		p := map[string]any{}
		if im.baseUser == "" || finalUser != im.baseUser {
			p["name"] = finalUser
		}
		if workdir != "" && workdir != "/home/"+finalUser {
			p["workdir"] = workdir
		}
		im.use("", "core/user-1000", p)
		im.appUser = finalUser
		im.note("USER %s: core/user-1000", finalUser)
		if homeEnv != "" && homeEnv != "/home/"+finalUser {
			im.note("ENV HOME=%s differs from /home/%s (core/user-1000's): check it", homeEnv, finalUser)
		}
	case workdir != "" && (len(workdirs) == 0 || workdirs[len(workdirs)-1].idx > lastRun):
		kept = append(kept, instr{cmd: "WORKDIR", raw: "WORKDIR " + workdir, idx: len(ins)})
	}
	if len(imageEnv) > 0 {
		im.use("", "core/image-env", map[string]any{"vars": imageEnv})
		im.note("ENV: core/image-env")
	}
	// The command.
	startSrc := ""
	if len(cmd) > 0 {
		startSrc = copied[cmd[0]]
	}
	switch {
	case startSrc != "" && len(cmd) == 1:
		body, _ := im.in.ReadFile(startSrc)
		p := map[string]any{"body": string(body)}
		if startSrc != "start.sh" {
			p["file"] = startSrc
		}
		if cmd[0] != "/start.sh" {
			p["dest"] = cmd[0]
		}
		im.use("", "core/start-script", p)
		im.startDest = cmd[0]
		delete(copied, cmd[0])
		im.note("CMD %s: core/start-script, with %s as its body (written once; yours to edit)", cmd[0], startSrc)
	case len(cmd) == 3 && strings.Contains(cmd[2], "while true") && strings.Contains(cmd[2], "sleep") && strings.Contains(cmd[2], "trap"):
		im.use("", "core/idle-cmd", map[string]any{})
		im.note("CMD (an idle loop): core/idle-cmd")
	case len(cmd) > 0:
		kept = append(kept, instr{cmd: "CMD", raw: "CMD " + jsonList(cmd), user: finalUser})
	}
	if entryClear && !wine {
		if b := im.find("core/base-image"); b != nil {
			b.Params["clear_entrypoint"] = true
		} else {
			im.use("", "core/entrypoint-clear", map[string]any{})
		}
	}
	// A kept WORKDIR comes after the user snippet's: restate the final one.
	if lastKeptWorkdir != "" && workdir != "" && workdir != lastKeptWorkdir {
		kept = append(kept, instr{cmd: "WORKDIR", raw: "WORKDIR " + workdir, user: finalUser, idx: len(ins) + 1})
	}
	// Other copies.
	for _, in := range copyFiles {
		f := strings.Fields(in.args)
		var files []string
		mode := ""
		for _, w := range f {
			if m, ok := strings.CutPrefix(w, "--chmod="); ok {
				mode = m
			} else if !strings.HasPrefix(w, "--") {
				files = append(files, w)
			}
		}
		if len(files) == 2 && files[1] == im.startDest {
			continue
		}
		dest := files[len(files)-1]
		p := map[string]any{"files": files[:len(files)-1], "dest": dest}
		if mode != "" {
			p["mode"] = mode
		}
		im.use(path.Base(strings.TrimSuffix(files[0], "/")), "core/copy-files", p)
		im.note("COPY %s: core/copy-files (the files stay yours)", strings.Join(files, " "))
	}
	// What's left, as is: before the switch to the app's user it runs as
	// root (install slot); after it, as the user (user slot). Root steps
	// that came after a COPY a snippet now makes (the copy slot) go after
	// it (dirs slot): they may use what it copied (mqtt's chmod).
	firstCopy := len(ins) + 2
	for _, c := range copyFiles {
		firstCopy = min(firstCopy, c.idx)
	}
	for _, in := range ins {
		if in.cmd == "COPY" && startCmd != "" && strings.HasSuffix(in.args, " "+startCmd) {
			firstCopy = min(firstCopy, in.idx)
		}
	}
	var buf []string
	slot := ""
	flush := func() {
		if len(buf) > 0 {
			im.use("kept-"+slot, "core/dockerfile-lines", map[string]any{"slot": slot, "text": strings.Join(buf, "\n")})
			buf = nil
		}
	}
	for _, in := range kept {
		s := "install"
		switch {
		case in.user != "":
			s = "user"
		case in.idx > firstCopy:
			s = "dirs"
		}
		if in.cmd == "CMD" || in.cmd == "ENTRYPOINT" {
			s = "entry"
		}
		if s != slot {
			flush()
			slot = s
		}
		buf = append(buf, in.raw)
		im.keptDocker++
	}
	flush()
	if im.keptDocker > 0 {
		im.note("%d Dockerfile instructions kept as is (core/dockerfile-lines): read them; a snippet may fit better", im.keptDocker)
	}
}

// onlyDirs: a RUN that only makes mount points (or the Steam dir) owned by
// the app's user, which core/volume and core/steamcmd do.
func (im *importer) onlyDirs(args string) bool {
	for _, part := range strings.Split(args, "&&") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		var paths []string
		switch {
		case f[0] == "install" && slices.Contains(f, "-d"):
			for i := 1; i < len(f); i++ {
				if f[i] == "-o" || f[i] == "-g" || f[i] == "-m" {
					i++
					continue
				}
				if !strings.HasPrefix(f[i], "-") {
					paths = append(paths, f[i])
				}
			}
		case f[0] == "mkdir" || f[0] == "chown":
			for _, w := range f[1:] {
				if strings.HasPrefix(w, "/") {
					paths = append(paths, w)
				}
			}
		default:
			return false
		}
		for _, p := range paths {
			p = path.Clean(p)
			if !slices.Contains(im.mounts, p) && p != im.steamDir {
				return false
			}
		}
	}
	return true
}

func chmodOf(args string) (string, bool) {
	f := strings.Fields(args)
	if len(f) == 3 && f[0] == "chmod" {
		return f[2], true
	}
	return "", false
}

// envPairs reads ENV KEY=VALUE KEY2="VALUE 2" (or the old ENV KEY VALUE).
func envPairs(args string) map[string]string {
	out := map[string]string{}
	if !strings.Contains(strings.Fields(args + " =")[0], "=") {
		k, v, _ := strings.Cut(args, " ")
		out[k] = strings.Trim(v, `"`)
		return out
	}
	re := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=("(?:[^"\\]|\\.)*"|\S*)`)
	for _, m := range re.FindAllStringSubmatch(args, -1) {
		v := m[2]
		if strings.HasPrefix(v, `"`) {
			if u, err := strconv.Unquote(v); err == nil {
				v = u
			}
		}
		out[m[1]] = v
	}
	return out
}

func jsonList(l []string) string {
	q := make([]string, len(l))
	for i, s := range l {
		q[i] = strconv.Quote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// --- flk.toml -------------------------------------------------------------------

func (im *importer) flkToml() {
	cfg := im.cfg
	build := table(cfg, "build")
	if d, ok := build["dockerfile"].(string); ok && d == "Dockerfile" {
		delete(build, "dockerfile")
	}
	if args := table(build, "args"); args != nil {
		if b, ok := args["STEAM_BUILDID"].(string); ok {
			if s := im.find("core/steamcmd"); s != nil {
				s.Params["build_id"] = b
				delete(args, "STEAM_BUILDID")
			}
		}
		if len(args) == 0 {
			delete(build, "args")
		}
	}
	if len(build) == 0 {
		delete(cfg, "build")
	}
	if kt, ok := cfg["kill_timeout"]; ok {
		if s, ok := seconds(kt); ok {
			im.use("", "core/kill-timeout", map[string]any{"seconds": s})
			im.note("kill_timeout: core/kill-timeout")
			delete(cfg, "kill_timeout")
		}
	}
	if env := table(cfg, "env"); env != nil {
		if tz, ok := env["TZ"].(string); ok {
			im.use("", "core/timezone", map[string]any{"zone": tz})
			delete(env, "TZ")
		}
		vars := map[string]any{}
		for k, v := range env {
			if s, ok := v.(string); ok {
				vars[k] = s
				delete(env, k)
			}
		}
		if len(vars) > 0 {
			im.use("", "core/env", map[string]any{"vars": vars})
			im.note("[env]: core/env (%d variables)", len(vars))
		}
		if len(env) == 0 {
			delete(cfg, "env")
		}
	}
	if rs := list(cfg, "restart"); len(rs) == 1 {
		r := rs[0]
		policy, _ := r["policy"].(string)
		retries, hasRetries := r["retries"].(int64)
		switch {
		case policy == "on-failure" && hasRetries && retries == 1 && len(r) == 2:
			im.use("", "core/restart-outer-net", map[string]any{})
		case policy == "always":
			p := map[string]any{}
			if hasRetries {
				p["retries"] = retries
			}
			im.use("", "core/restart-always", p)
		default:
			p := map[string]any{}
			if policy != "" {
				p["policy"] = policy
			}
			if hasRetries {
				p["retries"] = retries
			}
			im.use("", "core/restart", p)
		}
		delete(cfg, "restart")
	}
	flk := table(cfg, "flk")
	backup := table(flk, "backup")
	beforeDeploy, _ := backup["before_deploy"].(bool)
	if flk != nil {
		if k, ok := flk["keep_releases"].(int64); ok {
			if beforeDeploy {
				im.use("", "core/before-deploy", map[string]any{"keep_releases": k})
				delete(backup, "before_deploy")
			} else {
				im.use("", "core/keep-releases", map[string]any{"count": k})
			}
			delete(flk, "keep_releases")
		}
		if s, ok := flk["stdin"].(string); ok && s == "null" {
			im.use("", "core/stdin-null", map[string]any{})
			delete(flk, "stdin")
		}
		if b, ok := flk["bind"].(string); ok {
			im.use("", "core/bind", map[string]any{"address": b})
			delete(flk, "bind")
		}
		if n, ok := flk["rootfs_extra_mb"].(int64); ok {
			im.use("", "core/rootfs-extra", map[string]any{"mb": n})
			delete(flk, "rootfs_extra_mb")
		}
		if b, ok := flk["internal_hosts"].(bool); ok && b {
			im.use("", "core/internal-hosts", map[string]any{})
			delete(flk, "internal_hosts")
		}
	}
	if backup != nil {
		im.backup(backup)
		if beforeDeploy && backup["before_deploy"] != nil {
			im.use("", "core/before-deploy", map[string]any{})
			delete(backup, "before_deploy")
		}
		if len(backup) == 0 {
			delete(flk, "backup")
		}
	}
	if flk != nil && len(flk) == 0 {
		delete(cfg, "flk")
	}
	if checks := table(cfg, "checks"); checks != nil {
		for _, name := range sortedKeys(checks) {
			c, _ := checks[name].(map[string]any)
			if im.check(name, c) {
				delete(checks, name)
			}
		}
		if len(checks) == 0 {
			delete(cfg, "checks")
		}
	}
	if vms := list(cfg, "vm"); len(vms) == 1 {
		v := vms[0]
		p := map[string]any{}
		ok := true
		for k, val := range v {
			switch k {
			case "cpus":
				p["cpus"] = val
			case "memory", "size":
				p[k] = fmt.Sprint(val)
			case "huge_pages":
				p["huge_pages"] = true
			default:
				ok = false
			}
		}
		if ok {
			im.use("", "core/vm", p)
			delete(cfg, "vm")
		}
	}
	if mounts := list(cfg, "mounts"); mounts != nil {
		var keep []map[string]any
		for _, m := range mounts {
			src, _ := m["source"].(string)
			dst, _ := m["destination"].(string)
			if len(m) != 2 || src == "" || dst == "" {
				keep = append(keep, m)
				continue
			}
			id := ""
			if len(mounts) > 1 {
				id = src
			}
			im.use(id, "core/volume", map[string]any{"name": src, "path": dst})
		}
		setList(cfg, "mounts", keep)
		im.note("[[mounts]]: core/volume for each (sizes aren't in flk.toml: check them in the next steps)")
	}
	if services := list(cfg, "services"); services != nil {
		var keep []map[string]any
		for i, s := range services {
			port, _ := s["internal_port"].(int64)
			proto, _ := s["protocol"].(string)
			ports := maps(s["ports"])
			if port == 0 || len(ports) != 1 || len(s) > 3 || len(ports[0]) != 1 {
				keep = append(keep, s)
				continue
			}
			host, _ := ports[0]["port"].(int64)
			p := map[string]any{"port": port}
			if proto != "" {
				p["protocol"] = proto
			}
			if host != port {
				p["host_port"] = host
			}
			id := ""
			if len(services) > 1 {
				id = fmt.Sprintf("service-%d", i+1)
				if proto != "" {
					id = fmt.Sprintf("%s-%d", proto, port)
				}
			}
			im.use(id, "core/service", p)
		}
		setList(cfg, "services", keep)
	}
	if hs := table(cfg, "http_service"); hs != nil {
		if port, ok := hs["internal_port"].(int64); ok && len(hs) == 1 {
			p := map[string]any{"port": port}
			if hp, ok := table(cfg, "flk")["http_port"].(int64); ok {
				p["host_port"] = hp
				delete(table(cfg, "flk"), "http_port")
				if len(table(cfg, "flk")) == 0 {
					delete(cfg, "flk")
				}
			}
			im.use("", "core/http-service", p)
			delete(cfg, "http_service")
		}
	}
	// What's left goes in as is.
	if len(cfg) > 0 {
		var b bytes.Buffer
		toml.NewEncoder(&b).Encode(cfg)
		text := strings.TrimSpace(b.String())
		im.use("kept", "core/flk-toml", map[string]any{"text": text + "\n"})
		im.keptToml = countKeys(cfg)
		im.note("flk.toml keys kept as is (core/flk-toml): %s", strings.Join(sortedKeys(cfg), ", "))
	}
}

func (im *importer) backup(b map[string]any) {
	mode, _ := b["mode"].(string)
	verify, hasVerify := b["verify"].(string)
	keep, hasKeep := b["keep"].(int64)
	p := map[string]any{}
	if hasVerify {
		p["verify"] = verify
	}
	if hasKeep {
		p["keep"] = keep
	}
	switch mode {
	case "pause", "stop":
		for k := range b {
			if k != "mode" && k != "verify" && k != "keep" && k != "before_deploy" {
				return
			}
		}
		im.use("", "core/backup-"+mode, p)
		delete(b, "mode")
		delete(b, "verify")
		delete(b, "keep")
	case "live":
		pre := maps(b["pre"])
		post := maps(b["post"])
		var preLines, postLines []string
		wait, timeout := "", int64(0)
		for i, h := range pre {
			c, ok := h["console"].(string)
			if !ok {
				return
			}
			preLines = append(preLines, c)
			if w, ok := h["wait"].(string); ok {
				if i != len(pre)-1 {
					return
				}
				wait = w
				timeout, _ = h["timeout"].(int64)
			} else if len(h) != 1 {
				return
			}
		}
		for _, h := range post {
			c, ok := h["console"].(string)
			if !ok || len(h) != 1 {
				return
			}
			postLines = append(postLines, c)
		}
		if wait == "" || hasKeep {
			return
		}
		p := map[string]any{"pre": preLines, "wait": wait, "post": postLines}
		if timeout > 0 {
			p["timeout"] = timeout
		}
		if hasVerify {
			p["verify"] = verify
		}
		im.use("", "core/backup-live-console", p)
		for _, k := range []string{"mode", "pre", "post", "verify"} {
			delete(b, k)
		}
	case "":
		if hasVerify && !hasKeep {
			im.use("", "core/backup-verify", map[string]any{"file": verify})
			delete(b, "verify")
			im.note("[flk.backup] verify: core/backup-verify (your %s is kept: flkforge only writes it when it's missing)", verify)
		}
	}
}

func (im *importer) check(name string, c map[string]any) bool {
	typ, _ := c["type"].(string)
	grace, _ := c["grace_period"].(string)
	p := map[string]any{}
	if name != map[string]string{"log": "ready", "tcp": "tcp", "http": "http"}[typ] {
		p["name"] = name
	}
	if grace != "" {
		p["grace"] = grace
	}
	switch typ {
	case "log":
		m, ok := c["match"].(string)
		if !ok || len(c) > 3 {
			return false
		}
		p["match"] = m
		im.use(name, "core/check-log", p)
	case "tcp":
		port, ok := c["port"].(int64)
		if !ok || len(c) > 3 {
			return false
		}
		p["port"] = port
		im.use(name, "core/check-tcp", p)
	case "http":
		port, ok := c["port"].(int64)
		if !ok || len(c) > 4 {
			return false
		}
		p["port"] = port
		if pa, ok := c["path"].(string); ok {
			p["path"] = pa
		}
		im.use(name, "core/check-http", p)
	default:
		return false
	}
	if grace == "" {
		im.note("[checks.%s] has no grace_period: the snippet writes its default", name)
	}
	return true
}

func setList(cfg map[string]any, k string, keep []map[string]any) {
	if len(keep) == 0 {
		delete(cfg, k)
	} else {
		cfg[k] = keep
	}
}

func countKeys(m map[string]any) int {
	n := 0
	for _, v := range m {
		if t, ok := v.(map[string]any); ok {
			n += countKeys(t)
		} else {
			n++
		}
	}
	return n
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func seconds(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n, true
		}
		if d, err := time.ParseDuration(x); err == nil && d%time.Second == 0 {
			return int64(d.Seconds()), true
		}
	}
	return 0, false
}
