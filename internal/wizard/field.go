package wizard

import (
	"strings"
	"unicode/utf8"
)

// field is a one-line text editor: deterministic (no blinking cursor), so
// screens can be golden-tested.
type field struct {
	text []rune
	cur  int
	err  string
}

func newField(s string) *field {
	r := []rune(s)
	return &field{text: r, cur: len(r)}
}

func (f *field) String() string { return string(f.text) }

func (f *field) set(s string) {
	f.text = []rune(s)
	f.cur = len(f.text)
}

// key applies an editing key; it reports whether the text changed.
func (f *field) key(k string, text string) (changed, handled bool) {
	switch k {
	case "left":
		if f.cur > 0 {
			f.cur--
		}
		return false, true
	case "right":
		if f.cur < len(f.text) {
			f.cur++
		}
		return false, true
	case "home", "ctrl+a":
		f.cur = 0
		return false, true
	case "end", "ctrl+e":
		f.cur = len(f.text)
		return false, true
	case "backspace":
		if f.cur > 0 {
			f.text = append(f.text[:f.cur-1], f.text[f.cur:]...)
			f.cur--
			return true, true
		}
		return false, true
	case "delete":
		if f.cur < len(f.text) {
			f.text = append(f.text[:f.cur], f.text[f.cur+1:]...)
			return true, true
		}
		return false, true
	case "ctrl+u":
		changed := len(f.text) > 0
		f.text, f.cur = nil, 0
		return changed, true
	}
	if text != "" && utf8.ValidString(text) && !strings.ContainsAny(text, "\n\r\t") {
		ins := []rune(text)
		f.text = append(f.text[:f.cur], append(ins, f.text[f.cur:]...)...)
		f.cur += len(ins)
		return true, true
	}
	return false, false
}

// view shows the text with a cursor (a reversed cell) when focused.
func (f *field) view(focused bool, cursor func(string) string) string {
	if !focused {
		return string(f.text)
	}
	before := string(f.text[:f.cur])
	at := " "
	after := ""
	if f.cur < len(f.text) {
		at = string(f.text[f.cur])
		after = string(f.text[f.cur+1:])
	}
	return before + cursor(at) + after
}
