package flktoml

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// The real apps' flk.toml files (copied into testdata) survive a round
// trip with their meaning intact, and in the house order with no change
// in the text apart from spacing.
func TestRoundTripRealFiles(t *testing.T) {
	for _, name := range []string{"valheim", "mqtt", "minecraft"} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + name + ".toml")
			if err != nil {
				t.Fatal(err)
			}
			tab, err := Parse(string(src))
			if err != nil {
				t.Fatal(err)
			}
			out := Format(tab, nil)
			var a, b map[string]any
			if _, err := toml.Decode(string(src), &a); err != nil {
				t.Fatal(err)
			}
			if _, err := toml.Decode(out, &b); err != nil {
				t.Fatalf("output isn't TOML: %v\n%s", err, out)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("meaning changed:\n%s", out)
			}
			// every comment survives
			for _, l := range strings.Split(string(src), "\n") {
				l = strings.TrimSpace(l)
				if i := strings.Index(l, "#"); i >= 0 && !strings.Contains(l[:i], `"`) {
					if !strings.Contains(out, strings.TrimSpace(l[i:])) {
						t.Errorf("comment lost: %q", l[i:])
					}
				}
			}
		})
	}
}

func TestFormatHouseStyle(t *testing.T) {
	in := `# top
app = "x"

# Give it time.
kill_timeout = 120

[build]
  dockerfile = "Dockerfile"

[env]
  A = "1"     # one
  LONGER = "2"  # two

  # group
  B = "3"

[flk.backup]
  mode = "pause"

[checks.ready]
  type = "log"

# Game
[[services]]
  internal_port = 2456
  protocol = "udp"
  [[services.ports]]
    port = 2456
`
	tab, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `# top
app = "x"

# Give it time.
kill_timeout = 120

[build]
  dockerfile = "Dockerfile"

[env]
  A = "1"       # one
  LONGER = "2"  # two

  # group
  B = "3"

[flk.backup]
  mode = "pause"

[checks.ready]
  type = "log"

# Game
[[services]]
  internal_port = 2456
  protocol = "udp"
  [[services.ports]]
    port = 2456
`
	if got := Format(tab, HouseOrder); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMerge(t *testing.T) {
	a, _ := Parse("kill_timeout = 30\n[env]\n  A = \"1\"\n[[mounts]]\n  source = \"a\"\n")
	b, _ := Parse("kill_timeout = 120\n[env]\n  B = \"2\"\n[[mounts]]\n  source = \"b\"\n")
	if err := Merge(a, b, nil); err == nil || !strings.Contains(err.Error(), "kill_timeout is set twice") {
		t.Fatalf("want a conflict, got %v", err)
	}
	a, _ = Parse("kill_timeout = 30\n[env]\n  A = \"1\"\n[[mounts]]\n  source = \"a\"\n")
	if err := Merge(a, b, map[string]string{"kill_timeout": "max"}); err != nil {
		t.Fatal(err)
	}
	got := Format(a, HouseOrder)
	for _, s := range []string{"kill_timeout = 120", `A = "1"`, `B = "2"`, `source = "a"`, `source = "b"`} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %s in\n%s", s, got)
		}
	}
	c, _ := Parse("[env]\n  A = \"1\"\n")
	if err := Merge(a, c, nil); err != nil {
		t.Errorf("the same value twice isn't a conflict: %v", err)
	}
}

func TestParseRefuses(t *testing.T) {
	for _, in := range []string{"a.b = 1", "x = '''\nhi'''", "x = ", "[a\n", "x = 1\nx = 2"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}
