package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/forge"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// noFlk is an environment without flk, so output doesn't depend on the host.
func noFlk() *forge.Env {
	return &forge.Env{Version: Version, FlkErr: os.ErrNotExist}
}

func run(t *testing.T, env func() *forge.Env, tty bool, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, Options{Stdout: &out, Stderr: &errb, IsTTY: func() bool { return tty }, Env: env})
	return out.String(), errb.String(), code
}

// TestGoldenHello: PLAN M0's acceptance test, the files half. The recipe
// for flk's examples/hello renders the files in testdata/golden/hello.
func TestGoldenHello(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hello")
	out, errs, code := run(t, noFlk, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errs)
	}
	golden := "../../testdata/golden/hello"
	files := map[string]string{"report.txt": strings.ReplaceAll(out, dir, "DIR")}
	for _, f := range []string{"Dockerfile", "flk.toml", "start.sh", "flkforge.toml"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		files[f] = string(b)
	}
	for name, got := range files {
		path := filepath.Join(golden, name)
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (make golden writes it)", err)
		}
		if got != string(want) {
			t.Errorf("%s differs from %s (make golden rewrites it; review with git diff):\n%s", name, path, got)
		}
	}
	if st, err := os.Stat(filepath.Join(dir, "start.sh")); err != nil || st.Mode().Perm() != 0o755 {
		t.Errorf("start.sh should be 0755: %v %v", st.Mode(), err)
	}

	// A second render changes nothing.
	out, _, code = run(t, noFlk, false, "render", dir)
	if code != 0 || strings.Contains(out, " written  base") {
		t.Errorf("second render: exit %d\n%s", code, out)
	}
}

// TestHelloMatchesFlkExample: PLAN M0's acceptance test, the flk half.
// flk reads the rendered flk.toml exactly as it reads examples/hello's:
// the same parsed config and machine_config, and no warnings.
func TestHelloMatchesFlkExample(t *testing.T) {
	x, err := flk.Find()
	if err != nil {
		t.Skip("no flk on PATH")
	}
	src := os.Getenv("FLK_SRC")
	if src == "" {
		home, _ := os.UserHomeDir()
		src = filepath.Join(home, "flk")
	}
	example := filepath.Join(src, "examples", "hello")
	if _, err := os.Stat(filepath.Join(example, "flk.toml")); err != nil {
		t.Skip("no flk checkout (set FLK_SRC)")
	}
	dir := filepath.Join(t.TempDir(), "hello")
	env := func() *forge.Env { return &forge.Env{Version: Version, Flk: flk.New(x)} }
	if out, errs, code := run(t, env, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errs)
	}
	c := flk.New(x)
	got, err := c.ConfigShow(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.ConfigShow(example, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) > 0 {
		t.Errorf("flk config show warns: %v", got.Warnings)
	}
	if !reflect.DeepEqual(got.Parsed, want.Parsed) {
		t.Errorf("parsed differs:\n got %v\nwant %v", got.Parsed, want.Parsed)
	}
	if !reflect.DeepEqual(got.MachineConfig, want.MachineConfig) {
		t.Errorf("machine_config differs:\n got %v\nwant %v", got.MachineConfig, want.MachineConfig)
	}
}

func TestStrictArguments(t *testing.T) {
	for _, args := range [][]string{
		{"bogus"},
		{"check", "--bogus"},
		{"check", "a", "b"},
		{"render", "--force=yes"},
		{"new", "--recipe"},
		{"help", "--json"},
	} {
		if _, errs, code := run(t, noFlk, false, args...); code != exitUsage || !strings.HasPrefix(errs, "Error:") {
			t.Errorf("%v: exit %d, %q; want exit 2 with an error", args, code, errs)
		}
	}
}

func TestNoTerminal(t *testing.T) {
	_, errs, code := run(t, noFlk, false, "new", t.TempDir())
	if code != exitUsage || !strings.Contains(errs, "--recipe FILE --yes") {
		t.Errorf("no TTY: exit %d, %q; want a refusal naming the non-interactive form", code, errs)
	}
	_, errs, code = run(t, noFlk, false, "new", t.TempDir(), "--recipe", "../../testdata/hello.toml")
	if code != exitUsage || !strings.Contains(errs, "--yes") {
		t.Errorf("no TTY, no --yes: exit %d, %q", code, errs)
	}
}

func TestNewRefusesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "flk.toml"), []byte("app = \"mine\"\n"), 0o644)
	_, errs, code := run(t, noFlk, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes")
	if code != exitFail || !strings.Contains(errs, "already has a flk.toml") {
		t.Errorf("exit %d, %q", code, errs)
	}
	// --force writes next to it, but never over it.
	out, _, code := run(t, noFlk, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes", "--force")
	if code != exitFail || !strings.Contains(out, "flkforge didn't write it") {
		t.Errorf("--force: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "flk.toml")); string(b) != "app = \"mine\"\n" {
		t.Errorf("flk.toml was overwritten: %s", b)
	}
}

func TestDrift(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	if _, _, code := run(t, noFlk, false, "new", dir, "--recipe", "../../testdata/hello.toml", "--yes", "--app", "flkt-forge-drift"); code != 0 {
		t.Fatal("new failed")
	}
	path := filepath.Join(dir, "flk.toml")
	b, _ := os.ReadFile(path)
	edited := strings.Replace(string(b), "hi from flk", "edited", 1)
	os.WriteFile(path, []byte(edited), 0o644)

	out, _, code := run(t, noFlk, false, "render", dir, "--json")
	var res struct {
		Refused string
		Files   []struct{ Path, Status string }
		Next    string
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err, out)
	}
	if code != exitFail || res.Refused == "" || !strings.Contains(res.Next, "diff -u flk.toml flk.toml.flkforge-new") {
		t.Errorf("render after a hand edit: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(path); string(b) != edited {
		t.Error("the hand-edited file was overwritten")
	}
	if _, err := os.Stat(path + ".flkforge-new"); err != nil {
		t.Error("no flk.toml.flkforge-new")
	}
	out, _, code = run(t, noFlk, false, "check", dir)
	if code != exitFail || !strings.Contains(out, "flk.toml was changed by hand") {
		t.Errorf("check: exit %d\n%s", code, out)
	}
	if _, _, code = run(t, noFlk, false, "render", dir, "--force"); code != 0 {
		t.Error("render --force failed")
	}
	if b, _ := os.ReadFile(path); string(b) == edited {
		t.Error("--force didn't overwrite")
	}
	if _, err := os.Stat(path + ".flkforge-new"); err == nil {
		t.Error("flk.toml.flkforge-new left behind after --force")
	}
}

func TestHelpAllJSON(t *testing.T) {
	out, _, code := run(t, noFlk, false, "help", "--all", "--json")
	var h struct {
		Schema   int
		Version  string
		Commands []struct{ Name string }
	}
	if err := json.Unmarshal([]byte(out), &h); err != nil || code != 0 {
		t.Fatalf("exit %d, %v", code, err)
	}
	if h.Schema != HelpSchema || h.Version != Version || len(h.Commands) != len(commands) {
		t.Errorf("got %+v", h)
	}
}
