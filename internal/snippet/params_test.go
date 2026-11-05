package snippet

import (
	"fmt"
	"testing"
)

func TestCoerce(t *testing.T) {
	i := func(n int64) *int64 { return &n }
	for _, tc := range []struct {
		p  Param
		v  any
		ok bool
	}{
		{Param{Type: "port"}, int64(8080), true},
		{Param{Type: "port"}, int64(70000), false},
		{Param{Type: "port"}, "8080", false},
		{Param{Type: "int", Min: i(1)}, int64(0), false},
		{Param{Type: "size"}, "6gb", true},
		{Param{Type: "size"}, "lots", false},
		{Param{Type: "duration"}, "5m", true},
		{Param{Type: "duration"}, int64(30), true},
		{Param{Type: "duration"}, "soon", false},
		{Param{Type: "enum", Values: []string{"stop", "live"}}, "pause", false},
		{Param{Type: "secret"}, "API_TOKEN", true},
		{Param{Type: "secret"}, "hunter2 value", false},
		{Param{Type: "regex"}, "a(", false},
		{Param{Type: "list"}, []any{"a", "b"}, true},
		{Param{Type: "list"}, []any{"a", int64(1)}, false},
		{Param{Type: "map"}, map[string]any{"A": "1"}, true},
		{Param{Type: "map"}, map[string]any{"A": int64(1)}, false},
		{Param{Type: "string", Validate: "^/"}, "data", false},
	} {
		if _, err := tc.p.Coerce(tc.v); (err == nil) != tc.ok {
			t.Errorf("%s %v: err = %v, want ok=%v", tc.p.Type, tc.v, err, tc.ok)
		}
	}
}

func TestParseRefusesUnknownKeys(t *testing.T) {
	_, err := Parse([]byte("id = \"core/x\"\nversion = \"1\"\ntitle = \"x\"\ncategory = \"misc\"\nsummary = \"x\"\ntypo = 1\n"))
	if err == nil {
		t.Error("an unknown key should fail")
	}
}

// What FormatValue shows, ParseValue reads back.
func TestValueRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		p Param
		v any
	}{
		{Param{Type: "string"}, "3.20"},
		{Param{Type: "int"}, int64(42)},
		{Param{Type: "bool"}, true},
		{Param{Type: "list"}, []string{"a", "b"}},
		{Param{Type: "list"}, []string{"bash", "-c", "exec java -jar \"x\", y"}},
		{Param{Type: "map"}, map[string]any{"A": "1", "B c": "two"}},
		{Param{Type: "duration"}, "30s"},
	} {
		s := FormatValue(&tc.p, tc.v)
		back, err := ParseValue(&tc.p, s)
		if err != nil {
			t.Errorf("%v -> %q: %v", tc.v, s, err)
			continue
		}
		got, _ := tc.p.Coerce(back)
		want, _ := tc.p.Coerce(tc.v)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%v -> %q -> %v", tc.v, s, got)
		}
	}
}
