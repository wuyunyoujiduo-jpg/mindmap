package model

import (
	"hash/maphash"
	"time"
)

// When a topic last changed (Node.Changed) is kept by what it says, not
// by who changed it: a print of its title, note, task, tags and links is
// taken before a change and compared after it, once a step of the
// history, so that no edit has to say which topics it touched, and none
// can forget to.

var printSeed = maphash.MakeSeed()

// print is the topic's title, note, task, tags and links as one number.
// Where it stands, how it looks and whether it is folded are not of it.
func (n *Node) print() uint64 {
	var h maphash.Hash
	h.SetSeed(printSeed)
	word := func(s string) {
		h.WriteString(s)
		h.WriteByte(0)
	}
	day := func(t *time.Time) {
		if t == nil {
			h.WriteByte(1)
			return
		}
		var b [8]byte
		u := uint64(t.Unix())
		for i := range b {
			b[i] = byte(u >> (8 * i))
		}
		h.Write(b[:])
	}
	word(n.Title)
	word(n.Note)
	for _, t := range n.Tags {
		word(t)
	}
	h.WriteByte(2)
	for _, l := range n.Links {
		word(l)
	}
	if t := n.Task; t != nil {
		word(string(t.Status))
		h.WriteByte(byte(t.Priority))
		day(t.Start)
		day(t.Due)
		word(t.Remind)
		word(t.Time)
		word(t.EndTime)
		if r := t.Repeat; r != nil {
			word(r.Label())
		}
	}
	return h.Sum64()
}

// Prints are the prints of a file's topics, by their IDs.
type Prints map[string]uint64

// PrintsOf takes the prints of every topic of the file.
func PrintsOf(f *File) Prints {
	p := Prints{}
	if f != nil {
		f.Walk(func(n *Node) { p[n.ID] = n.print() })
	}
	return p
}

// Stamp says of every topic of the file that says otherwise than its
// print that it changed now, and of every topic the prints have not that
// says no time of its own (a topic made; one pasted or moved in keeps
// its own). The prints are then the file's. It returns the topics said so.
func (p Prints) Stamp(f *File, now time.Time) []*Node {
	now = now.Truncate(time.Second)
	var out []*Node
	f.Walk(func(n *Node) {
		was, ok := p[n.ID]
		is := n.print()
		if ok && was == is || !ok && !n.Changed.IsZero() {
			p[n.ID] = is
			return
		}
		p[n.ID] = is
		n.Changed = now
		out = append(out, n)
	})
	return out
}

// StampChanges says of the topics of after that say otherwise than in
// before, and of those made, that they changed now: for what changes a
// file whole, as an agent does.
func StampChanges(before, after *File, now time.Time) {
	PrintsOf(before).Stamp(after, now)
}

// ChangedAt is when the topic last changed: its own time, or, saying
// none, the file's.
func (f *File) ChangedAt(n *Node) time.Time {
	if !n.Changed.IsZero() {
		return n.Changed
	}
	return f.Dated
}
