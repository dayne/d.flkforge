package render

import (
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/recipe"
)

func TestRenderRefuses(t *testing.T) {
	cat := catalog(t)
	base := "[[use]]\nsnippet = \"base-alpine\"\n"
	for name, tc := range map[string]struct{ recipe, want string }{
		"no base":        {"[[use]]\nsnippet = \"volume\"\n", "no base snippet"},
		"two bases":      {base + base[:0] + "[[use]]\nsnippet = \"core/base-alpine\"\nid = \"other\"\n", "used 2 times"},
		"unknown":        {base + "[[use]]\nsnippet = \"nope\"\n", "no snippet \"nope\""},
		"bad param":      {base + "[[use]]\nsnippet = \"volume\"\nparams = { size = \"big\" }\n", "size: want a whole number"},
		"unknown param":  {base + "[[use]]\nsnippet = \"volume\"\nparams = { sise = 2 }\n", "sise: no such param"},
		"required":       {base + "[[use]]\nsnippet = \"packages\"\n", "list is required"},
		"enum/validate":  {base + "[[use]]\nsnippet = \"volume\"\nparams = { name = \"My Data\" }\n", "doesn't match"},
		"merge conflict": {base + "[[use]]\nsnippet = \"env\"\nid = \"a\"\nparams = { vars = { X = \"1\" } }\n[[use]]\nsnippet = \"env\"\nid = \"b\"\nparams = { vars = { X = \"2\" } }\n", "env.X is set twice"},
		"needs base":     {"[[use]]\nsnippet = \"packages\"\nparams = { list = [\"x\"] }\n", "needs a snippet of category base"},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := recipe.Parse([]byte("app = \"flkt-x\"\n" + tc.recipe))
			if err == nil {
				_, err = Render(r, cat)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestRecipeRefusesDuplicateIDs(t *testing.T) {
	_, err := recipe.Parse([]byte("app = \"x\"\n[[use]]\nsnippet = \"core/volume\"\n[[use]]\nsnippet = \"core/volume\"\n"))
	if err == nil || !strings.Contains(err.Error(), "its own id") {
		t.Errorf("got %v", err)
	}
}

func TestShortImageName(t *testing.T) {
	for img, ok := range map[string]bool{
		"docker.io/library/alpine:3.20": true, "localhost/x": true, "registry:5000/x": true,
		"alpine:3.20": false, "library/alpine": false,
	} {
		if fullImageName(img) != ok {
			t.Errorf("fullImageName(%q) = %v", img, !ok)
		}
	}
}

// The same recipe renders byte-identical files every time (SPEC 2.6), even
// with map params, whose order Go randomizes.
func TestDeterministic(t *testing.T) {
	r, err := recipe.Parse([]byte(`app = "flkt-x"
[[use]]
snippet = "base-alpine"
[[use]]
snippet = "env"
params = { vars = { Z = "1", A = "2", M = "3", B = "4" } }
`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := Render(r, catalog(t))
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		again, _ := Render(r, catalog(t))
		for i, f := range again.Files {
			if string(f.Content) != string(first.Files[i].Content) {
				t.Fatalf("%s differs between renders", f.Path)
			}
		}
	}
	if !strings.Contains(string(first.File("flk.toml").Content), "A = \"2\"\n  B = \"4\"\n  M = \"3\"\n  Z = \"1\"") {
		t.Errorf("env not sorted:\n%s", first.File("flk.toml").Content)
	}
}
