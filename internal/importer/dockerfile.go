package importer

import (
	"strings"
)

// instr is one Dockerfile instruction, continuation lines joined.
type instr struct {
	idx  int    // position, for keeping instructions in order
	cmd  string // FROM, RUN, COPY... upper case
	args string // the rest, as written (continuations joined with " ")
	raw  string // the instruction as written, continuation lines kept
	user string // the USER in effect when it runs ("" = root)
}

// stage is one FROM ... section.
type stage struct {
	image string
	name  string // AS name
	ins   []instr
}

// parseDockerfile splits a Dockerfile into stages of instructions.
// Comments and blank lines are dropped.
func parseDockerfile(text string) []stage {
	var stages []stage
	var cur *stage
	user := ""
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		raw := line
		joined := strings.TrimSuffix(trim, "\\")
		for strings.HasSuffix(strings.TrimSpace(raw), "\\") && i+1 < len(lines) {
			i++
			next := strings.TrimRight(lines[i], " \t\r")
			if strings.HasPrefix(strings.TrimSpace(next), "#") {
				continue
			}
			raw += "\n" + next
			joined += " " + strings.TrimSuffix(strings.TrimSpace(next), "\\")
		}
		cmd, args, _ := strings.Cut(joined, " ")
		cmd = strings.ToUpper(cmd)
		args = strings.Join(strings.Fields(args), " ")
		if cmd == "FROM" {
			f := strings.Fields(args)
			s := stage{}
			for j := 0; j < len(f); j++ {
				switch {
				case strings.HasPrefix(f[j], "--"):
				case strings.EqualFold(f[j], "AS") && j+1 < len(f):
					s.name = f[j+1]
					j++
				case s.image == "":
					s.image = f[j]
				}
			}
			stages = append(stages, s)
			cur = &stages[len(stages)-1]
			user = ""
			continue
		}
		if cur == nil {
			continue // ARG before FROM
		}
		if cmd == "USER" {
			user = strings.Fields(args + " ")[0]
			if user == "root" || user == "0" {
				user = ""
			}
		}
		cur.ins = append(cur.ins, instr{cmd: cmd, args: args, raw: strings.TrimLeft(raw, " \t"), user: user})
	}
	return stages
}

// execForm reads CMD/ENTRYPOINT's JSON array form, or a shell form as one item.
func execForm(args string) ([]string, bool) {
	a := strings.TrimSpace(args)
	if !strings.HasPrefix(a, "[") {
		return []string{"/bin/sh", "-c", a}, false
	}
	var out []string
	inner := strings.TrimSuffix(strings.TrimPrefix(a, "["), "]")
	var cur strings.Builder
	in, esc := false, false
	for _, r := range inner {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && in:
			esc = true
		case r == '"':
			if in {
				out = append(out, cur.String())
				cur.Reset()
			}
			in = !in
		case in:
			cur.WriteRune(r)
		}
	}
	return out, true
}

// packagesIn finds an install-only RUN line's packages: apt-get install,
// apk add or dnf install, with only updates and clean-ups around them.
func packagesIn(args string) ([]string, string, bool) {
	var pkgs []string
	family := ""
	for _, part := range strings.Split(args, "&&") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		cmdline := strings.Join(f, " ")
		switch {
		case f[0] == "apt-get" && len(f) > 1 && f[1] == "update", f[0] == "apt" && len(f) > 1 && f[1] == "update":
			family = "apt"
		case (f[0] == "apt-get" || f[0] == "apt") && len(f) > 1 && f[1] == "install":
			family = "apt"
			pkgs = append(pkgs, nonFlags(f[2:])...)
		case f[0] == "apk" && len(f) > 1 && f[1] == "add":
			family = "apk"
			pkgs = append(pkgs, nonFlags(f[2:])...)
		case f[0] == "dnf" && len(f) > 1 && f[1] == "install":
			family = "dnf"
			pkgs = append(pkgs, nonFlags(f[2:])...)
		case strings.HasPrefix(cmdline, "rm -rf /var/lib/apt/lists"), strings.HasPrefix(cmdline, "dnf clean"),
			strings.HasPrefix(cmdline, "apt-get clean"), strings.HasPrefix(cmdline, "rm -rf /var/cache/apk"):
		case len(f) >= 2 && f[0] == "DEBIAN_FRONTEND=noninteractive" && (f[1] == "apt-get" || f[1] == "apt"):
			sub, fam, ok := packagesIn(strings.Join(f[1:], " "))
			if !ok {
				return nil, "", false
			}
			pkgs, family = append(pkgs, sub...), fam
		default:
			return nil, "", false
		}
	}
	return pkgs, family, len(pkgs) > 0
}

func nonFlags(f []string) []string {
	var out []string
	for _, w := range f {
		if !strings.HasPrefix(w, "-") {
			out = append(out, w)
		}
	}
	return out
}
