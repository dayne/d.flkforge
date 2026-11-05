package snippet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dayne/d.flkforge/internal/flktoml"
)

// ParseValue reads a typed-in value for a param (the command line, the
// wizard): strings as they are
// (or TOML-quoted), numbers and booleans by type, lists as TOML arrays or
// comma-separated, maps as TOML inline tables.
func ParseValue(p *Param, raw string) (any, error) {
	switch p.Type {
	case "int", "port":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q isn't a whole number", raw)
		}
		return n, nil
	case "bool":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%q isn't true or false", raw)
		}
		return b, nil
	case "duration":
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return n, nil
		}
		return raw, nil
	case "list":
		if strings.HasPrefix(strings.TrimSpace(raw), "[") {
			v, err := flktoml.Decode(raw)
			if err != nil {
				return nil, err
			}
			return v, nil
		}
		if raw == "" {
			return []any{}, nil
		}
		var out []any
		for _, s := range strings.Split(raw, ",") {
			out = append(out, strings.TrimSpace(s))
		}
		return out, nil
	case "map":
		v, err := flktoml.Decode(raw)
		if err != nil {
			return nil, fmt.Errorf("want a TOML inline table, such as '{ NAME = \"value\" }': %v", err)
		}
		return v, nil
	}
	// strings: a TOML-quoted one is unquoted, anything else is taken as is
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') {
		if v, err := flktoml.Decode(raw); err == nil {
			if s, ok := v.(string); ok {
				return s, nil
			}
		}
	}
	return raw, nil
}

// FormatValue shows a param value the way ParseValue reads it back.
func FormatValue(p *Param, v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []string:
		return formatList(x)
	case []any:
		l := make([]string, len(x))
		for i, e := range x {
			l[i] = fmt.Sprint(e)
		}
		return formatList(l)
	case map[string]string, map[string]any:
		return flktoml.Encode(x, false)
	}
	return fmt.Sprint(v)
}

// formatList: comma-separated when that reads back the same, else a TOML array.
func formatList(l []string) string {
	for _, s := range l {
		if strings.ContainsAny(s, ",\"'[") || strings.TrimSpace(s) != s || s == "" {
			return flktoml.Encode(l, false)
		}
	}
	return strings.Join(l, ", ")
}
