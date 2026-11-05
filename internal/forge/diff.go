package forge

import (
	"fmt"
	"strings"
)

// Diff is a unified diff of a and b (as lines), with ctx lines of context
// around each change, and at most limit lines of output (0: no limit).
func Diff(aName, bName, a, b string, ctx, limit int) []string {
	al := strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	bl := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	n, m := len(al), len(bl)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		kind   byte // ' ', '-', '+'
		text   string
		ai, bi int
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && al[i] == bl[j]:
			ops = append(ops, op{' ', al[i], i, j})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', al[i], i, j})
			i++
		default:
			ops = append(ops, op{'+', bl[j], i, j})
			j++
		}
	}
	var out []string
	for k := 0; k < len(ops); {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start := max(0, k-ctx)
		end := k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run-end > 2*ctx || run == len(ops) {
				end = min(len(ops), end+ctx)
				break
			}
			end = run
		}
		if len(out) == 0 {
			out = append(out, "--- "+aName, "+++ "+bName)
		}
		as, bs := ops[start].ai, ops[start].bi
		ac, bc := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				ac++
			}
			if o.kind != '-' {
				bc++
			}
		}
		out = append(out, fmt.Sprintf("@@ -%d,%d +%d,%d @@", as+1, ac, bs+1, bc))
		for _, o := range ops[start:end] {
			out = append(out, string(o.kind)+o.text)
		}
		k = end
	}
	if limit > 0 && len(out) > limit {
		out = append(out[:limit], fmt.Sprintf("... (%d more lines)", len(out)-limit))
	}
	return out
}
