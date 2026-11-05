package flktoml

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Conflict is a key that two fragments set to different values.
type Conflict struct {
	Path     string
	Old, New string
}

func (c *Conflict) Error() string {
	return fmt.Sprintf("%s is set twice, to %s and to %s", c.Path, c.Old, c.New)
}

// Merge merges src into dst (SPEC 6.2): tables merge, arrays of tables
// append, and a key set twice must have the same value unless rules names
// a merge rule for its path ("max": keep the larger number or duration).
func Merge(dst, src *Table, rules map[string]string) error {
	return merge(dst, src, "", rules)
}

func merge(dst, src *Table, prefix string, rules map[string]string) error {
	if len(dst.Comment) == 0 {
		dst.Comment = src.Comment
	}
	if dst.Trailing == "" {
		dst.Trailing = src.Trailing
	}
	dst.Tail = append(dst.Tail, src.Tail...)
	for _, it := range src.Items {
		path := prefix + it.itemName()
		existing := dst.Get(it.itemName())
		switch s := it.(type) {
		case *KV:
			switch e := existing.(type) {
			case nil:
				dst.Items = append(dst.Items, s)
			case *KV:
				if Equal(e.Value, s.Value) {
					continue
				}
				switch rules[path] {
				case "max":
					bigger, err := larger(e.Value, s.Value)
					if err != nil {
						return fmt.Errorf("%s: %v", path, err)
					}
					if bigger == s.Value {
						*e = *s
					}
				default:
					return &Conflict{Path: path, Old: e.Value, New: s.Value}
				}
			default:
				return fmt.Errorf("%s is a table in one fragment and a key in another", path)
			}
		case *Sub:
			switch e := existing.(type) {
			case nil:
				dst.Items = append(dst.Items, s)
			case *Sub:
				if err := merge(e.T, s.T, path+".", rules); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%s is a table in one fragment and not in another", path)
			}
		case *Arr:
			switch e := existing.(type) {
			case nil:
				dst.Items = append(dst.Items, s)
			case *Arr:
				e.Ts = append(e.Ts, s.Ts...)
			default:
				return fmt.Errorf("%s is an array of tables in one fragment and not in another", path)
			}
		}
	}
	return nil
}

// larger returns whichever of two values (numbers, or durations such as
// "30s" and "2m") is larger.
func larger(a, b string) (string, error) {
	na, err := seconds(a)
	if err != nil {
		return "", err
	}
	nb, err := seconds(b)
	if err != nil {
		return "", err
	}
	if nb > na {
		return b, nil
	}
	return a, nil
}

func seconds(v string) (float64, error) {
	x, err := Decode(v)
	if err != nil {
		return 0, err
	}
	switch n := x.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, nil
		}
		d, err := time.ParseDuration(strings.TrimSpace(n))
		if err != nil {
			return 0, fmt.Errorf("%s isn't a number or a duration", v)
		}
		return d.Seconds(), nil
	}
	return 0, fmt.Errorf("%s isn't a number or a duration", v)
}
