package flktoml

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// HouseOrder is the order of top-level keys and tables in a flk.toml
// (docs/flk-primer.md section 5); anything else goes after, in the order
// it was added.
var HouseOrder = []string{
	"app", "primary_region", "kill_signal", "kill_timeout",
	"build", "env", "experimental", "restart", "flk", "checks", "vm",
	"mounts", "http_service", "services",
}

// Format writes a table as TOML in the house style: two-space indents
// under each header, a blank line before each table and before each
// commented key, trailing comments lined up, and top-level items in
// order (HouseOrder for a flk.toml; nil keeps the insertion order).
func Format(root *Table, order []string) string {
	f := &formatter{}
	for _, c := range root.Comment {
		f.line(0, c)
	}
	if len(root.Comment) > 0 {
		f.blank()
	}
	items := sorted(root.Items, order)
	f.kvs(items, 0)
	for _, it := range items {
		switch s := it.(type) {
		case *Sub:
			f.sub(FormatKey(s.Name), s.T, 0)
		case *Arr:
			f.arr(FormatKey(s.Name), s.Ts, 0, false)
		}
	}
	f.tail(root.Tail, 0)
	return strings.TrimRight(f.b.String(), "\n") + "\n"
}

type formatter struct {
	b       strings.Builder
	n       int  // lines written
	isBlank bool // the last line written was blank
}

func (f *formatter) line(indent int, s string) {
	f.b.WriteString(strings.Repeat(" ", indent))
	f.b.WriteString(s)
	f.b.WriteByte('\n')
	f.n++
	f.isBlank = false
}

func (f *formatter) blank() {
	if f.n > 0 && !f.isBlank {
		f.b.WriteByte('\n')
		f.isBlank = true
	}
}

// hasOwn says whether a table needs its own header: it has keys or
// comments, or no child tables either (an empty table).
func hasOwn(t *Table) bool {
	if len(t.Comment) > 0 || t.Trailing != "" || len(t.Tail) > 0 {
		return true
	}
	for _, it := range t.Items {
		if _, ok := it.(*KV); ok {
			return true
		}
	}
	return len(t.Items) == 0
}

func (f *formatter) sub(path string, t *Table, indent int) {
	keyIndent := indent
	if hasOwn(t) {
		f.blank()
		f.header(indent, "["+path+"]", t)
		keyIndent = indent + 2
		f.kvs(t.Items, keyIndent)
		f.tail(t.Tail, keyIndent)
	}
	f.children(path, t, indent, keyIndent)
}

// arr writes an array of tables. nested: right under its parent's keys
// ([[services.ports]] inside a [[services]]), with no blank line.
func (f *formatter) arr(path string, ts []*Table, indent int, nested bool) {
	for _, t := range ts {
		if !nested {
			f.blank()
		}
		f.header(indent, "[["+path+"]]", t)
		f.kvs(t.Items, indent+2)
		f.tail(t.Tail, indent+2)
		f.children(path, t, indent+2, indent+2)
	}
}

// children writes a table's child tables: sub-tables with their headers
// at subIndent, arrays of tables nested under the keys at keyIndent.
func (f *formatter) children(path string, t *Table, subIndent, keyIndent int) {
	for _, it := range t.Items {
		switch s := it.(type) {
		case *Sub:
			f.sub(path+"."+FormatKey(s.Name), s.T, subIndent)
		case *Arr:
			f.arr(path+"."+FormatKey(s.Name), s.Ts, keyIndent, keyIndent > subIndent || keyIndent == subIndent && subIndent > 0)
		}
	}
}

func (f *formatter) header(indent int, h string, t *Table) {
	for _, c := range t.Comment {
		f.line(indent, c)
	}
	if t.Trailing != "" {
		h += "   " + t.Trailing
	}
	f.line(indent, h)
}

// tail writes a table's closing comments as they were written, after a
// blank line (they're usually about what follows, or commented-out keys).
func (f *formatter) tail(lines []string, _ int) {
	if len(lines) == 0 {
		return
	}
	f.blank()
	for _, c := range lines {
		if c == "" {
			f.blank()
			continue
		}
		f.line(0, c)
	}
}

// kvs writes a table's keys. Trailing comments line up within a run of
// keys that has no comment lines between them.
func (f *formatter) kvs(items []Item, indent int) {
	var kvs []*KV
	for _, it := range items {
		if kv, ok := it.(*KV); ok {
			kvs = append(kvs, kv)
		}
	}
	for i := 0; i < len(kvs); {
		j := i + 1
		for j < len(kvs) && len(kvs[j].Comment) == 0 && !kvs[j].Blank {
			j++
		}
		width := 0
		for _, kv := range kvs[i:j] {
			if kv.Trailing != "" && !strings.Contains(kv.Value, "\n") {
				width = max(width, len(FormatKey(kv.Key))+3+len(kv.Value))
			}
		}
		for k, kv := range kvs[i:j] {
			n := i + k
			newGroup := n > 0 && len(kv.Comment) > 0 && kv.Origin != kvs[n-1].Origin
			if n > 0 && (kv.Blank || newGroup) {
				f.blank()
			}
			for _, c := range kv.Comment {
				f.line(indent, c)
			}
			s := FormatKey(kv.Key) + " = " + kv.Value
			if kv.Trailing != "" {
				pad := width - len(s)
				if pad < 0 || strings.Contains(kv.Value, "\n") {
					pad = 0
				}
				s += strings.Repeat(" ", pad+2) + kv.Trailing
			}
			if strings.Contains(s, "\n") {
				// a multi-line string: only its first line is indented
				f.b.WriteString(strings.Repeat(" ", indent) + s + "\n")
				f.n++
				f.isBlank = false
				continue
			}
			f.line(indent, s)
		}
		i = j
	}
}

// sorted puts items in order (by name), then the rest as they came.
func sorted(items []Item, order []string) []Item {
	if order == nil {
		return items
	}
	rank := map[string]int{}
	for i, n := range order {
		rank[n] = i
	}
	out := append([]Item(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := rank[out[i].itemName()]
		rj, okj := rank[out[j].itemName()]
		switch {
		case oki && okj:
			return ri < rj
		case oki:
			return true
		default:
			return false
		}
	})
	return out
}

// Quote writes s as a TOML basic string.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Encode writes a Go value as TOML value text. Strings with newlines
// become multi-line literal strings when multiline is set and they can;
// everything else fits on one line.
func Encode(v any, multiline bool) string {
	switch x := v.(type) {
	case nil:
		return `""`
	case string:
		if multiline && strings.Contains(x, "\n") && !strings.Contains(x, "'''") && !strings.Contains(x, "\r") {
			return "'''\n" + x + "'''"
		}
		return Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []string:
		parts := make([]string, len(x))
		for i, s := range x {
			parts[i] = Quote(s)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = Encode(e, false)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]string:
		m := map[string]any{}
		for k, v := range x {
			m[k] = v
		}
		return Encode(m, false)
	case map[string]any:
		if len(x) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = FormatKey(k) + " = " + Encode(x[k], false)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		return Quote(fmt.Sprint(x))
	}
}
