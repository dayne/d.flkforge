package forge

import (
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	a := "a\nb\nc\nd\ne\nf\ng\nh\n"
	b := "a\nb\nC\nd\ne\nf\ng\nh\ni\n"
	got := strings.Join(Diff("old", "new", a, b, 1, 0), "\n")
	want := `--- old
+++ new
@@ -2,3 +2,3 @@
 b
-c
+C
 d
@@ -8,1 +8,2 @@
 h
+i`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if d := Diff("a", "b", a, a, 3, 0); len(d) != 0 {
		t.Errorf("no change, got %v", d)
	}
	if d := Diff("a", "b", "", strings.Repeat("x\n", 50), 3, 10); len(d) != 11 || !strings.Contains(d[10], "more lines") {
		t.Errorf("limit: %v", d)
	}
}
