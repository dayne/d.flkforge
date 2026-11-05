package snippet

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Param types (SPEC 4).
var ParamTypes = []string{"string", "int", "bool", "enum", "list", "map", "port", "size", "duration", "regex", "secret"}

// Param is one typed parameter.
type Param struct {
	Type     string   `toml:"type"`
	Prompt   string   `toml:"prompt"`
	Hint     string   `toml:"hint"`
	Default  any      `toml:"default"`
	Required bool     `toml:"required"`
	Values   []string `toml:"values"`   // enum
	Validate string   `toml:"validate"` // a regex for strings
	Min      *int64   `toml:"min"`      // int
	Max      *int64   `toml:"max"`      // int
}

var (
	sizeRE   = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?\s*(b|k|kb|m|mb|g|gb|t|tb)?$`)
	secretRE = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

func (p *Param) check() error {
	if p == nil {
		return fmt.Errorf("empty")
	}
	if !slices.Contains(ParamTypes, p.Type) {
		return fmt.Errorf("type %q isn't one of %s", p.Type, strings.Join(ParamTypes, ", "))
	}
	if p.Type == "enum" && len(p.Values) == 0 {
		return fmt.Errorf("an enum needs values")
	}
	if p.Validate != "" {
		if _, err := regexp.Compile(p.Validate); err != nil {
			return fmt.Errorf("validate: %v", err)
		}
	}
	if p.Default != nil {
		if _, err := p.Coerce(p.Default); err != nil {
			return fmt.Errorf("default: %v", err)
		}
	}
	if p.Type == "secret" && p.Default != nil {
		return fmt.Errorf("a secret's default would be a secret's name, which is fine, but never a value: use required instead")
	}
	return nil
}

// Coerce checks a value (as decoded from TOML) against the param's type
// and returns it in the type templates see: string, int64, bool,
// []string or map[string]string.
func (p *Param) Coerce(v any) (any, error) {
	switch p.Type {
	case "string", "regex", "secret", "size", "enum":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("want a string, got %s", show(v))
		}
		switch p.Type {
		case "regex":
			if _, err := regexp.Compile(s); err != nil {
				return nil, fmt.Errorf("not a regex: %v", err)
			}
		case "secret":
			if !secretRE.MatchString(s) {
				return nil, fmt.Errorf("%q isn't a secret name (A-Z, 0-9 and _): the recipe records names, never values", s)
			}
		case "size":
			if !sizeRE.MatchString(s) {
				return nil, fmt.Errorf("%q isn't a size such as \"256mb\" or \"6gb\"", s)
			}
		case "enum":
			if !slices.Contains(p.Values, s) {
				return nil, fmt.Errorf("%q isn't one of %s", s, strings.Join(p.Values, ", "))
			}
		}
		if p.Validate != "" && !regexp.MustCompile(p.Validate).MatchString(s) {
			return nil, fmt.Errorf("%q doesn't match %s", s, p.Validate)
		}
		return s, nil
	case "int", "port":
		n, ok := v.(int64)
		if !ok {
			if i, isInt := v.(int); isInt {
				n, ok = int64(i), true
			}
		}
		if !ok {
			return nil, fmt.Errorf("want a whole number, got %s", show(v))
		}
		if p.Type == "port" && (n < 1 || n > 65535) {
			return nil, fmt.Errorf("port %d isn't between 1 and 65535", n)
		}
		if p.Min != nil && n < *p.Min {
			return nil, fmt.Errorf("%d is below the minimum, %d", n, *p.Min)
		}
		if p.Max != nil && n > *p.Max {
			return nil, fmt.Errorf("%d is above the maximum, %d", n, *p.Max)
		}
		return n, nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("want true or false, got %s", show(v))
		}
		return b, nil
	case "duration":
		switch d := v.(type) {
		case int64:
			return fmt.Sprintf("%ds", d), nil
		case string:
			if _, err := time.ParseDuration(d); err != nil {
				return nil, fmt.Errorf("%q isn't a duration such as \"30s\" or \"5m\"", d)
			}
			return d, nil
		}
		return nil, fmt.Errorf("want a duration such as \"30s\", got %s", show(v))
	case "list":
		switch l := v.(type) {
		case []string:
			return l, nil
		case []any:
			out := make([]string, len(l))
			for i, e := range l {
				s, ok := e.(string)
				if !ok {
					return nil, fmt.Errorf("want a list of strings, got %s in it", show(e))
				}
				out[i] = s
			}
			return out, nil
		}
		return nil, fmt.Errorf("want a list of strings, got %s", show(v))
	case "map":
		switch m := v.(type) {
		case map[string]string:
			return maps.Clone(m), nil
		case map[string]any:
			out := map[string]string{}
			for k, e := range m {
				s, ok := e.(string)
				if !ok {
					return nil, fmt.Errorf("%s: want a string, got %s (flk's [env] values are strings)", k, show(e))
				}
				out[k] = s
			}
			return out, nil
		}
		return nil, fmt.Errorf("want a table of strings, got %s", show(v))
	}
	return nil, fmt.Errorf("unknown type %s", p.Type)
}

func show(v any) string {
	switch v.(type) {
	case nil:
		return "nothing"
	case string:
		return fmt.Sprintf("the string %q", v)
	}
	return fmt.Sprintf("%v (%T)", v, v)
}

// Resolve checks the values a recipe gives against the params, and fills
// in defaults. Every param is in the result; one with no value and no
// default is nil.
func (s *Snippet) Resolve(values map[string]any) (map[string]any, error) {
	var errs []string
	out := map[string]any{}
	for _, k := range sortedKeys(values) {
		if _, ok := s.Params[k]; !ok {
			errs = append(errs, fmt.Sprintf("%s: no such param (%s has: %s)", k, s.ID, strings.Join(s.ParamOrder, ", ")))
		}
	}
	for _, name := range s.ParamOrder {
		p := s.Params[name]
		v, ok := values[name]
		if !ok {
			v = p.Default
		}
		if v == nil {
			if p.Required {
				errs = append(errs, fmt.Sprintf("%s is required: %s", name, p.Prompt))
			}
			out[name] = nil
			continue
		}
		c, err := p.Coerce(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		out[name] = c
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %s", s.ID, strings.Join(errs, "; "))
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
