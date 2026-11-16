package skills

import "strings"

// A Line is a line of a difference: ' ' in both, '-' only in the first,
// '+' only in the second.
type Line struct {
	Kind byte
	Text string
}

// Diff is the lines of a and of b, those in both once, the others marked;
// past a few thousand lines, each whole.
func Diff(a, b string) []Line {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	if len(x)*len(y) > 4_000_000 {
		var out []Line
		for _, l := range x {
			out = append(out, Line{'-', l})
		}
		for _, l := range y {
			out = append(out, Line{'+', l})
		}
		return out
	}
	// The longest common lines, from the end.
	n, m := len(x), len(y)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []Line
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case x[i] == y[j]:
			out = append(out, Line{' ', x[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, Line{'-', x[i]})
			i++
		default:
			out = append(out, Line{'+', y[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, Line{'-', x[i]})
	}
	for ; j < m; j++ {
		out = append(out, Line{'+', y[j]})
	}
	return out
}

// Changed is the lines that differ, with ctx lines around each run of
// them; a run left out is a Line of Kind '~'.
func Changed(lines []Line, ctx int) []Line {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.Kind != ' ' {
			for k := max(0, i-ctx); k <= min(len(lines)-1, i+ctx); k++ {
				keep[k] = true
			}
		}
	}
	var out []Line
	gap := false
	for i, l := range lines {
		if keep[i] {
			out = append(out, l)
			gap = false
		} else if !gap {
			out = append(out, Line{'~', ""})
			gap = true
		}
	}
	return out
}
