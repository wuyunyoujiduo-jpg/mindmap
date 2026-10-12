package model

import (
	"slices"
	"strconv"
	"strings"
)

// Measures: numbers a map's nodes carry, as hours, cost or story points,
// whose totals add up the tree, as MindMup's.

// Total is the node's measure with those of all below it.
func (n *Node) Total(name string) float64 {
	t := n.Values[name]
	for _, c := range n.Children {
		t += c.Total(name)
	}
	return t
}

// Measured reports whether the node or a node below it has a value of the
// measure.
func (n *Node) Measured(name string) bool {
	if _, ok := n.Values[name]; ok {
		return true
	}
	for _, c := range n.Children {
		if c.Measured(name) {
			return true
		}
	}
	return false
}

// SetValue sets the node's own value of a measure; ok false clears it.
func (n *Node) SetValue(name string, v float64, ok bool) {
	if !ok {
		delete(n.Values, name)
		if len(n.Values) == 0 {
			n.Values = nil
		}
		return
	}
	if n.Values == nil {
		n.Values = map[string]float64{}
	}
	n.Values[name] = v
}

// AddMeasure adds a measure by name, and reports whether it is new.
func (f *File) AddMeasure(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || slices.Contains(f.Measures, name) {
		return false
	}
	f.Measures = append(f.Measures, name)
	return true
}

// RenameMeasure renames a measure, in the nodes' values too.
func (f *File) RenameMeasure(from, to string) bool {
	to = strings.TrimSpace(to)
	i := slices.Index(f.Measures, from)
	if i < 0 || to == "" || to == from || slices.Contains(f.Measures, to) {
		return false
	}
	f.Measures[i] = to
	f.eachNode(func(n *Node) {
		if v, ok := n.Values[from]; ok {
			delete(n.Values, from)
			n.Values[to] = v
		}
	})
	return true
}

// RemoveMeasure removes a measure and the nodes' values of it.
func (f *File) RemoveMeasure(name string) {
	i := slices.Index(f.Measures, name)
	if i < 0 {
		return
	}
	f.Measures = slices.Delete(f.Measures, i, i+1)
	f.eachNode(func(n *Node) { n.SetValue(name, 0, false) })
}

func (f *File) eachNode(fn func(*Node)) {
	for _, d := range f.Sheets {
		d.Walk(func(n, _ *Node, _ int) bool {
			fn(n)
			return true
		})
		for _, fl := range d.Floating {
			var walk func(*Node)
			walk = func(n *Node) {
				fn(n)
				for _, c := range n.Children {
					walk(c)
				}
			}
			walk(fl)
		}
	}
}

// FormatValue writes a measure's value short: 12, 3.5, 1.25.
func FormatValue(v float64) string {
	s := strings.TrimRight(strings.TrimRight(formatFloat(v), "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
