package importer

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/source"
	"github.com/dayne/d.flkforge/snippets"
)

// Load reads an app directory: flk.toml, Dockerfile, and its files.
func load(t *testing.T, dir string) Input {
	t.Helper()
	ft, err := os.ReadFile(filepath.Join(dir, "flk.toml"))
	if err != nil {
		t.Skipf("no %s here", dir)
	}
	df, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	return Input{FlkToml: string(ft), Dockerfile: string(df), ReadFile: func(n string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, n))
	}}
}

// TestImportRealApps: each real app (read-only) imports to a recipe that
// renders, and, with flk on PATH, flk reads its flk.toml exactly as it
// reads the app's own.
func TestImportRealApps(t *testing.T) {
	home, _ := os.UserHomeDir()
	cat, err := source.Load(source.FS{Label: "built in", FS: snippets.FS})
	if err != nil {
		t.Fatal(err)
	}
	x, flkErr := flk.Find()
	for _, app := range []string{"valheim", "minecraft", "vintagestory", "enshrouded", "mqtt", "ncm", "opencode"} {
		t.Run(app, func(t *testing.T) {
			src := filepath.Join(home, app+".flk")
			res, err := Import(load(t, src), cat)
			if err != nil {
				t.Fatal(err)
			}
			if os.Getenv("SHOW") != "" {
				t.Logf("%s", res.Recipe.Format())
				for _, n := range res.Notes {
					t.Log("note: " + n)
				}
			}
			out, err := render.Render(res.Recipe, cat)
			if err != nil {
				t.Fatalf("doesn't render: %v\n%s", err, res.Recipe.Format())
			}
			for _, f := range out.Findings {
				if f.Level == "error" {
					t.Errorf("rule error: %s: %s", f.Rule, f.Msg)
				}
			}
			checkOrder(t, load(t, src).Dockerfile, string(out.File("Dockerfile").Content))
			if flkErr != nil {
				return
			}
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "flk.toml"), out.File("flk.toml").Content, 0o644)
			c := flk.New(x)
			got, err := c.ConfigShow(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			want, err := c.ConfigShow(src, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Warnings) > 0 {
				t.Errorf("flk warns: %v", got.Warnings)
			}
			for _, k := range keys(got.Parsed, want.Parsed) {
				if !reflect.DeepEqual(got.Parsed[k], want.Parsed[k]) {
					t.Errorf("parsed.%s:\n got %v\nwant %v", k, got.Parsed[k], want.Parsed[k])
				}
			}
			t.Logf("%d notes; kept as is: %d flk.toml keys, %d Dockerfile instructions", len(res.Notes), res.KeptToml, res.KeptDocker)
		})
	}
}

func keys(a, b map[string]any) []string {
	s := map[string]bool{}
	for k := range a {
		s[k] = true
	}
	for k := range b {
		s[k] = true
	}
	var out []string
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// checkOrder: the rebuilt Dockerfile keeps what the original's build
// depends on: the final WORKDIR and USER, and every COPY whose destination
// a RUN uses comes before that RUN.
func checkOrder(t *testing.T, orig, got string) {
	t.Helper()
	last := func(text, cmd string) string {
		v := ""
		for _, st := range parseDockerfile(text) {
			for _, in := range st.ins {
				if in.cmd == cmd {
					v = in.args
				}
			}
		}
		return v
	}
	for _, cmd := range []string{"WORKDIR", "USER"} {
		if a, b := last(orig, cmd), last(got, cmd); a != b {
			t.Errorf("final %s: original %q, imported %q", cmd, a, b)
		}
	}
	stages := parseDockerfile(got)
	ins := stages[len(stages)-1].ins
	for i, in := range ins {
		if in.cmd != "COPY" || strings.Contains(in.args, "--from") {
			continue
		}
		f := strings.Fields(in.args)
		dest := f[len(f)-1]
		for j, other := range ins {
			if other.cmd == "RUN" && strings.Contains(other.args, dest) && j < i {
				t.Errorf("RUN %q uses %s before COPY %q puts it there", short(other.args), dest, in.args)
			}
		}
	}
}

func short(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
