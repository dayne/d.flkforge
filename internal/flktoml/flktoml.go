// Package flktoml reads and writes the subset of TOML that flk.toml files
// use, keeping comments (SPEC 6.2). A generic TOML library drops comments,
// and the comments are what make a generated flk.toml look hand-written.
//
// The subset: comment lines, [table] and [[array]] headers (dotted paths),
// and `key = value` lines with an optional trailing comment. A value is
// any TOML value that fits on one line, or an array spread over several.
// Dotted keys (a.b = 1) and multi-line strings are refused. Every value is
// checked with a real TOML parser, so the output is always valid TOML.
package flktoml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// Table is a TOML table: its key/values and child tables, in order.
type Table struct {
	Comment  []string // comment lines above the header ("# ..."), without the newline
	Trailing string   // comment after the header, "# ..."
	Items    []Item
	Tail     []string // comment lines at the end of the table, as written (indent kept)
}

// Item is one of *KV, *Sub or *Arr.
type Item interface{ itemName() string }

// KV is `key = value`. Value is the TOML text of the value.
type KV struct {
	Key      string
	Value    string
	Comment  []string
	Trailing string
	// Blank: a blank line goes before it (as in the source text).
	Blank bool
	// Origin names the fragment it came from; a commented key from a new
	// origin gets a blank line before it, so each snippet's group stands apart.
	Origin string
}

// Sub is a child table, [parent.name].
type Sub struct {
	Name string
	T    *Table
}

// Arr is an array of tables, [[parent.name]].
type Arr struct {
	Name string
	Ts   []*Table
}

func (k *KV) itemName() string  { return k.Key }
func (s *Sub) itemName() string { return s.Name }
func (a *Arr) itemName() string { return a.Name }

// Get returns the item named name in t, or nil.
func (t *Table) Get(name string) Item {
	for _, it := range t.Items {
		if it.itemName() == name {
			return it
		}
	}
	return nil
}

// KV returns the key/value named key, or nil.
func (t *Table) KV(key string) *KV {
	kv, _ := t.Get(key).(*KV)
	return kv
}

// Sub returns the child table named name, creating it if create is set.
func (t *Table) Sub(name string, create bool) (*Table, error) {
	switch it := t.Get(name).(type) {
	case *Sub:
		return it.T, nil
	case nil:
		if !create {
			return nil, nil
		}
		s := &Sub{Name: name, T: &Table{}}
		t.Items = append(t.Items, s)
		return s.T, nil
	case *Arr:
		if len(it.Ts) == 0 {
			return nil, fmt.Errorf("%s is an empty array of tables", name)
		}
		return it.Ts[len(it.Ts)-1], nil // [[a]] then [a.b]: b belongs to the last a
	default:
		return nil, fmt.Errorf("%s is a key, not a table", name)
	}
}

// Set sets key to value (TOML text), replacing an existing value.
func (t *Table) Set(key, value string, comment ...string) {
	if kv := t.KV(key); kv != nil {
		kv.Value = value
		if len(comment) > 0 {
			kv.Comment = comment
		}
		return
	}
	t.Items = append(t.Items, &KV{Key: key, Value: value, Comment: comment})
}

// Append adds a new table to the array of tables name.
func (t *Table) Append(name string) (*Table, error) {
	nt := &Table{}
	switch it := t.Get(name).(type) {
	case nil:
		t.Items = append(t.Items, &Arr{Name: name, Ts: []*Table{nt}})
	case *Arr:
		it.Ts = append(it.Ts, nt)
	default:
		return nil, fmt.Errorf("%s is already a key or a table, not an array of tables", name)
	}
	return nt, nil
}

// Parse reads TOML text in the subset into a table.
func Parse(text string) (*Table, error) {
	root := &Table{}
	cur := root
	var pending, rawPending []string
	blank := false
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		n := i + 1
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			if len(pending) == 0 {
				blank = true
			} else {
				pending = append(pending, "")
				rawPending = append(rawPending, "")
			}
			continue
		case strings.HasPrefix(line, "#"):
			pending = append(pending, line)
			rawPending = append(rawPending, strings.TrimRight(lines[i], " \t"))
		case strings.HasPrefix(line, "["):
			arr := strings.HasPrefix(line, "[[")
			body, trailing, err := splitComment(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			var path string
			if arr {
				if !strings.HasSuffix(body, "]]") {
					return nil, fmt.Errorf("line %d: bad header %q", n, line)
				}
				path = body[2 : len(body)-2]
			} else {
				if !strings.HasSuffix(body, "]") {
					return nil, fmt.Errorf("line %d: bad header %q", n, line)
				}
				path = body[1 : len(body)-1]
			}
			parts, err := splitPath(path)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			t := root
			for _, p := range parts[:len(parts)-1] {
				if t, err = t.Sub(p, true); err != nil {
					return nil, fmt.Errorf("line %d: %v", n, err)
				}
			}
			last := parts[len(parts)-1]
			if arr {
				cur, err = t.Append(last)
			} else {
				cur, err = t.Sub(last, true)
			}
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			cur.Comment = append(cur.Comment, trimBlank(pending)...)
			if trailing != "" {
				cur.Trailing = trailing
			}
			pending, rawPending, blank = nil, nil, false
		default:
			eq := strings.Index(line, "=")
			if eq < 0 {
				return nil, fmt.Errorf("line %d: expected key = value: %q", n, line)
			}
			key, err := parseKey(strings.TrimSpace(line[:eq]))
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			rest := strings.TrimSpace(line[eq+1:])
			// An array can go on over several lines until its brackets close.
			for depth(rest) > 0 && i+1 < len(lines) {
				i++
				rest += "\n" + lines[i]
			}
			value, trailing, err := splitComment(rest)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			if strings.Contains(value, "\n") {
				value = joinArray(value)
			}
			if _, err := Decode(value); err != nil {
				return nil, fmt.Errorf("line %d: %s: %v", n, key, err)
			}
			if cur.KV(key) != nil {
				return nil, fmt.Errorf("line %d: %s is set twice", n, key)
			}
			if cur.Get(key) != nil {
				return nil, fmt.Errorf("line %d: %s is already a table", n, key)
			}
			cur.Items = append(cur.Items, &KV{Key: key, Value: value, Comment: trimBlank(pending), Trailing: trailing,
				Blank: blank || (len(pending) > 0 && pending[0] == "")})
			pending, rawPending, blank = nil, nil, false
		}
	}
	if len(rawPending) > 0 {
		cur.Tail = append(cur.Tail, rawPending...)
	}
	return root, nil
}

// trimBlank drops blank lines inside a comment block (a comment, a blank
// line, then a key: the comment still belongs to the key).
func trimBlank(c []string) []string {
	var out []string
	for _, l := range c {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Decode parses one TOML value.
func Decode(value string) (any, error) {
	var m map[string]any
	if _, err := toml.Decode("v = "+value, &m); err != nil {
		return nil, fmt.Errorf("not a TOML value: %s", strings.TrimPrefix(err.Error(), "toml: "))
	}
	return m["v"], nil
}

// Equal says whether two values (TOML text) mean the same.
func Equal(a, b string) bool {
	va, ea := Decode(a)
	vb, eb := Decode(b)
	return ea == nil && eb == nil && reflect.DeepEqual(va, vb)
}

// splitComment splits "value # comment" outside strings.
func splitComment(s string) (string, string, error) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			if strings.HasPrefix(s[i:], `"""`) || strings.HasPrefix(s[i:], "'''") {
				return "", "", fmt.Errorf("multi-line strings aren't supported in flk.toml fragments")
			}
			quote = c
		case c == '#':
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:]), nil
		}
	}
	if quote != 0 {
		return "", "", fmt.Errorf("unterminated string")
	}
	return strings.TrimSpace(s), "", nil
}

// depth is the bracket depth left open at the end of s (outside strings).
func depth(s string) int {
	d := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '[' || c == '{':
			d++
		case c == ']' || c == '}':
			d--
		}
	}
	return d
}

// joinArray puts a multi-line array on one line, dropping comments inside.
func joinArray(v string) string {
	var parts []string
	for _, l := range strings.Split(v, "\n") {
		body, _, _ := splitComment(strings.TrimSpace(l))
		if body != "" {
			parts = append(parts, body)
		}
	}
	s := strings.Join(parts, " ")
	s = strings.ReplaceAll(s, "[ ", "[")
	s = strings.ReplaceAll(s, ", ]", "]")
	s = strings.ReplaceAll(s, ",]", "]")
	return strings.ReplaceAll(s, " ]", "]")
}

func splitPath(p string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(p, ".") {
		k, err := parseKey(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

func parseKey(k string) (string, error) {
	if len(k) >= 2 && k[0] == '"' && k[len(k)-1] == '"' {
		return k[1 : len(k)-1], nil
	}
	if k == "" {
		return "", fmt.Errorf("empty key")
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			if c == '.' {
				return "", fmt.Errorf("dotted key %q: write it as a table", k)
			}
			return "", fmt.Errorf("bad key %q", k)
		}
	}
	return k, nil
}

// FormatKey quotes a key when it isn't bare.
func FormatKey(k string) string {
	if k == "" {
		return `""`
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return Quote(k)
		}
	}
	return k
}
