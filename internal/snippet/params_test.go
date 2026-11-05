package snippet

import "testing"

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
