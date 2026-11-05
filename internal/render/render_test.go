package render

import (
	"os"
	"testing"

	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/source"
	"github.com/dayne/d.flkforge/snippets"
)

func catalog(t *testing.T) *source.Catalog {
	t.Helper()
	c, err := source.Load(source.FS{Label: "built in", FS: snippets.FS})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRenderHelloPrint(t *testing.T) {
	r, err := recipe.Read("../../testdata/hello.toml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Render(r, catalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SHOW") != "" {
		for _, f := range res.Files {
			t.Logf("=== %s (%s %o)\n%s", f.Path, f.Own, f.Mode, f.Content)
		}
		t.Logf("steps: %+v\nfindings: %+v", res.Steps, res.Findings)
	}
}
