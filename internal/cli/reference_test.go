package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/recipe"
)

// TestReferenceApps: each recipe in testdata/real renders a flk.toml that
// flk reads like the real app's (flk config show --json -a APP, read-only):
// the same parsed config and machine_config. The real apps are reference
// material (PLAN): the only differences allowed are the run engine's own
// settings (env RUN_*), and each recipe's notes explain the Dockerfile.
func TestReferenceApps(t *testing.T) {
	x, err := flk.Find()
	if err != nil {
		t.Skip("no flk on PATH")
	}
	c := flk.New(x)
	recipes, _ := filepath.Glob("../../testdata/real/*.toml")
	if len(recipes) == 0 {
		t.Fatal("no reference recipes")
	}
	for _, path := range recipes {
		name := strings.TrimSuffix(filepath.Base(path), ".toml")
		t.Run(name, func(t *testing.T) {
			r, err := recipe.Read(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(strings.TrimSuffix(path, ".toml") + ".notes.md"); err != nil {
				t.Errorf("no %s.notes.md explaining the differences", name)
			}
			want, err := c.ConfigShow("", r.App)
			if err != nil {
				t.Skipf("%s isn't registered here: %v", r.App, err)
			}
			dir := filepath.Join(t.TempDir(), name)
			if out, errs, code := run(t, noFlk, false, "new", dir, "--recipe", path, "--yes"); code != 0 {
				t.Fatalf("exit %d: %s%s", code, out, errs)
			}
			got, err := c.ConfigShow(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Warnings) > 0 {
				t.Errorf("flk config show warns: %v", got.Warnings)
			}
			for _, m := range []map[string]any{got.Parsed, got.MachineConfig} {
				if env, ok := m["env"].(map[string]any); ok {
					for k := range env {
						if strings.HasPrefix(k, "RUN_") {
							delete(env, k)
						}
					}
				}
			}
			diff(t, "parsed", got.Parsed, want.Parsed)
			diff(t, "machine_config", got.MachineConfig, want.MachineConfig)
		})
	}
}

// diff reports differing top-level keys, readably.
func diff(t *testing.T, what string, got, want map[string]any) {
	t.Helper()
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		if !reflect.DeepEqual(got[k], want[k]) {
			t.Errorf("%s.%s:\n got %v\nwant %v", what, k, got[k], want[k])
		}
	}
}
