// Package formats converts maps to and from other formats: OPML outlines,
// iCalendar events and PDF documents of pictures.
package formats

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// OPML: an outline of "outline" elements, with "text" and, as many
// outliners write, the note in "_note" and a task done in "_complete".
// What OPML has no name for is in attributes of this app's, which other
// outliners keep or pass over: a topic's tags and links, its task, that it
// is the topic of a summary or floats, a canvas's name. Without them a map
// came back with its floating topics, tags and links gone, and of its
// tasks only those done, as done.

type opmlDoc struct {
	XMLName xml.Name       `xml:"opml"`
	Version string         `xml:"version,attr"`
	Title   string         `xml:"head>title"`
	Body    []*opmlOutline `xml:"body>outline"`
}

type opmlOutline struct {
	Text     string         `xml:"text,attr"`
	Note     string         `xml:"_note,attr,omitempty"`
	Complete string         `xml:"_complete,attr,omitempty"`
	Tags     string         `xml:"_tags,attr,omitempty"`     // with commas between
	Links    string         `xml:"_links,attr,omitempty"`    // a line each
	Status   string         `xml:"_status,attr,omitempty"`   // idea, todo, doing or done: a task
	Priority string         `xml:"_priority,attr,omitempty"` // low, medium or high
	Start    string         `xml:"_start,attr,omitempty"`    // days, as 2006-01-02
	Due      string         `xml:"_due,attr,omitempty"`
	Time     string         `xml:"_time,attr,omitempty"` // of its day, as 15:04
	EndTime  string         `xml:"_endTime,attr,omitempty"`
	Remind   string         `xml:"_remind,attr,omitempty"`
	Repeat   string         `xml:"_repeat,attr,omitempty"` // how it comes again, as the map's file has it
	Times    string         `xml:"_times,attr,omitempty"`
	DoneAt   string         `xml:"_doneAt,attr,omitempty"`
	Summary  string         `xml:"_summary,attr,omitempty"`  // "true": the topic of the summary of the outline it is in
	Floating string         `xml:"_floating,attr,omitempty"` // "true": a floating topic of the canvas it is in, at _x, _y
	X        string         `xml:"_x,attr,omitempty"`
	Y        string         `xml:"_y,attr,omitempty"`
	Sheet    string         `xml:"_sheet,attr,omitempty"` // of a top-level outline: the name of its canvas
	Children []*opmlOutline `xml:"outline"`
}

const opmlDay = "2006-01-02"

var opmlPriorities = map[model.Priority]string{model.PriorityLow: "low", model.PriorityMedium: "medium", model.PriorityHigh: "high"}

// OPML writes a file's canvases as an OPML outline, a top-level outline
// each, its floating topics the last outlines in it.
func OPML(f *model.File, title string) ([]byte, error) {
	doc := opmlDoc{Version: "2.0", Title: title}
	var conv func(n *model.Node) *opmlOutline
	conv = func(n *model.Node) *opmlOutline {
		o := &opmlOutline{Text: n.Title, Note: n.Note, Tags: strings.Join(n.Tags, ","), Links: strings.Join(n.Links, "\n")}
		if t := n.Task; t != nil {
			o.Status, o.Priority = string(t.Status), opmlPriorities[t.Priority]
			o.Time, o.EndTime, o.Remind = t.Time, t.EndTime, t.Remind
			if t.Status == model.StatusDone {
				o.Complete = "true"
			}
			if t.Start != nil {
				o.Start = t.Start.Format(opmlDay)
			}
			if t.Due != nil {
				o.Due = t.Due.Format(opmlDay)
			}
			if t.DoneAt != nil {
				o.DoneAt = t.DoneAt.Format(time.RFC3339)
			}
			if t.Times > 0 {
				o.Times = strconv.Itoa(t.Times)
			}
			if t.Repeat != nil {
				if b, err := json.Marshal(t.Repeat); err == nil {
					o.Repeat = string(b)
				}
			}
		}
		for _, c := range n.Children {
			o.Children = append(o.Children, conv(c))
		}
		if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 { // after what it sums up
			s := conv(t)
			s.Text, s.Summary = model.SummaryPrefix+s.Text, "true"
			o.Children = append(o.Children, s)
		}
		return o
	}
	for _, d := range f.Sheets {
		top := conv(d.Root)
		top.Sheet = d.Title
		for _, fl := range d.Floating {
			o := conv(fl)
			o.Floating = "true"
			if fl.Pos != nil {
				o.X = strconv.FormatFloat(float64(fl.Pos.X), 'f', -1, 32)
				o.Y = strconv.FormatFloat(float64(fl.Pos.Y), 'f', -1, 32)
			}
			top.Children = append(top.Children, o)
		}
		doc.Body = append(doc.Body, top)
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// ParseOPML reads an OPML outline: one top-level outline becomes the root,
// several go under a root named title; those this app wrote, each named
// for its canvas, are a canvas each.
func ParseOPML(data []byte, title string) (*model.File, error) {
	var doc opmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("无法读取 OPML：%w", err)
	}
	if len(doc.Body) == 0 {
		return nil, errors.New("OPML 里没有内容")
	}
	day := func(s string) *time.Time {
		if d, err := time.ParseInLocation(opmlDay, s, time.Local); err == nil {
			return &d
		}
		return nil
	}
	// conv makes a node of an outline; the floating topics in it, which
	// are the canvas's and not the node's, are added to floating.
	var conv func(o *opmlOutline, floating *[]*model.Node) *model.Node
	conv = func(o *opmlOutline, floating *[]*model.Node) *model.Node {
		n := model.NewNode(strings.TrimSpace(o.Text))
		n.Note = strings.TrimSpace(o.Note)
		if o.Tags != "" {
			n.Tags = strings.Split(o.Tags, ",")
		}
		if o.Links != "" {
			n.Links = strings.Split(o.Links, "\n")
		}
		switch s := model.Status(o.Status); {
		case slices.Contains(model.Statuses, s):
			n.Task = &model.Task{Status: s}
		case o.Complete == "true":
			n.Task = &model.Task{Status: model.StatusDone}
		}
		if t := n.Task; t != nil {
			for p, name := range opmlPriorities {
				if name == o.Priority {
					t.Priority = p
				}
			}
			t.Start, t.Due = day(o.Start), day(o.Due)
			t.Time, t.EndTime, t.Remind = o.Time, o.EndTime, o.Remind
			t.Times, _ = strconv.Atoi(o.Times)
			if at, err := time.Parse(time.RFC3339, o.DoneAt); err == nil {
				t.DoneAt = &at
			}
			var r model.Repeat
			if o.Repeat != "" && json.Unmarshal([]byte(o.Repeat), &r) == nil && r.Unit != "" {
				t.Repeat = &r
			}
		}
		for _, c := range o.Children {
			switch {
			case c.Floating == "true":
				fl := conv(c, floating)
				x, errX := strconv.ParseFloat(c.X, 32)
				y, errY := strconv.ParseFloat(c.Y, 32)
				if errX == nil && errY == nil {
					fl.Pos = &model.Point{X: float32(x), Y: float32(y)}
				}
				*floating = append(*floating, fl)
			case c.Summary == "true" && n.Summary == nil && len(n.Children) > 0:
				s := conv(c, floating)
				s.Title = strings.TrimPrefix(s.Title, model.SummaryPrefix)
				n.Summary = &model.Mark{Topic: s}
			default:
				n.Children = append(n.Children, conv(c, floating))
			}
		}
		return n
	}
	if doc.Title != "" {
		title = doc.Title
	}
	f := model.NewFile(title)
	sheets := len(doc.Body) > 1
	for _, o := range doc.Body {
		sheets = sheets && o.Sheet != ""
	}
	switch {
	case sheets: // as this app wrote them
		f.Sheets = nil
		for _, o := range doc.Body {
			d := &model.Document{ID: model.NewID(), Title: o.Sheet}
			d.Root = conv(o, &d.Floating)
			f.Sheets = append(f.Sheets, d)
		}
	case len(doc.Body) == 1:
		d := f.Sheets[0]
		d.Root = conv(doc.Body[0], &d.Floating)
		if doc.Body[0].Sheet != "" {
			d.Title = doc.Body[0].Sheet
		}
	default:
		d := f.Sheets[0]
		for _, o := range doc.Body {
			d.Root.Children = append(d.Root.Children, conv(o, &d.Floating))
		}
	}
	return f, nil
}

// ParseMarkdown reads a Markdown outline: a first heading of the top level
// becomes the root, under which the rest goes nested by headings and list
// indentation; without one, a root named title holds it all. A document,
// with prose or tables under its headings, is read as one (parseDocument).
// A map written out as Markdown comes back as it was (model.Markdown): its
// notes, tags, tasks, links, summaries and floating topics, and its
// canvases each a canvas.
func ParseMarkdown(text, title string) *model.File {
	if isDocument(text) {
		return parseDocument(text, title)
	}
	f := model.NewFile(title)
	for i, part := range canvases(text) {
		d := f.Sheets[0]
		if i > 0 {
			d = f.AddSheet(-1)
			d.Root.Title = title
		}
		nodes, floating := model.ParseCanvas(part)
		if len(nodes) == 1 {
			d.Root = nodes[0]
		} else {
			d.Root.Children = nodes
		}
		d.Floating = floating
	}
	return f
}

// canvases are the canvases of a map written out as Markdown: the parts
// between its rules ("---"), when each begins with a heading of the top
// level, as MarkdownFile writes them; else the text is one.
func canvases(text string) []string {
	var parts []string
	var cur []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimRight(line, " ") == "---" {
			parts, cur = append(parts, strings.Join(cur, "\n")), nil
			continue
		}
		cur = append(cur, line)
	}
	parts = append(parts, strings.Join(cur, "\n"))
	if len(parts) < 2 {
		return []string{text}
	}
	for _, p := range parts {
		if !strings.HasPrefix(strings.TrimLeft(p, " \n"), "# ") {
			return []string{text}
		}
	}
	return parts
}

// ICS writes the tasks with dates of a file as events of an iCalendar:
// from the start day to the due day, or on the one given; of the whole
// day, or, for a task with a time of its day, from that time to its end
// (an hour, when it has none), in the calendar's own time. A task that
// repeats is an event that does, where a rule of iCalendar's says how
// (rrule). Done tasks are marked ✓.
func ICS(f *model.File, calendar string, now time.Time) []byte {
	var b strings.Builder
	line := func(s string) { b.WriteString(fold(s) + "\r\n") }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//MyMind//CN")
	line("CALSCALE:GREGORIAN")
	line("X-WR-CALNAME:" + escapeICS(calendar))
	stamp := now.UTC().Format("20060102T150405Z")
	for _, tn := range f.Tasks() {
		t := tn.Node.Task
		start, end := t.Start, t.Due
		if start == nil {
			start = end
		}
		if end == nil {
			end = start
		}
		if start == nil {
			continue
		}
		title := tn.Node.Title
		if t.Status == model.StatusDone {
			title = "✓ " + title
		}
		desc := strings.Join(tn.Parents, " › ")
		if tn.Node.Note != "" {
			desc += "\n\n" + tn.Node.Note
		}
		line("BEGIN:VEVENT")
		line("UID:" + tn.Node.ID + "@mindmap")
		line("DTSTAMP:" + stamp)
		h, m, timed := clock(t.Time)
		if timed {
			at := func(day time.Time, h, m int) time.Time {
				return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.Local)
			}
			from := at(*start, h, m)
			to := at(*end, h, m).Add(time.Hour)
			if eh, em, ok := clock(t.EndTime); ok && at(*end, eh, em).After(from) {
				to = at(*end, eh, em)
			}
			line("DTSTART:" + from.Format("20060102T150405"))
			line("DTEND:" + to.Format("20060102T150405"))
		} else {
			line("DTSTART;VALUE=DATE:" + start.Format("20060102"))
			line("DTEND;VALUE=DATE:" + end.AddDate(0, 0, 1).Format("20060102"))
		}
		if rule := rrule(t, *start, timed); rule != "" {
			line("RRULE:" + rule)
		}
		line("SUMMARY:" + escapeICS(title))
		if desc != "" {
			line("DESCRIPTION:" + escapeICS(desc))
		}
		line("STATUS:" + map[bool]string{true: "CONFIRMED", false: "TENTATIVE"}[t.Status == model.StatusDone || t.Status == model.StatusDoing])
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return []byte(b.String())
}

// clock reads a time of the day written as 15:04.
func clock(s string) (h, m int, ok bool) {
	t, err := time.Parse("15:04", s)
	return t.Hour(), t.Minute(), err == nil
}

// rrule is how a task that repeats comes again, as a rule of iCalendar's
// for an event that begins on its day; "" for a task that does not, and
// for what such a rule cannot say, which is left out rather than said
// wrongly: a task that comes again counted from the day it was done, one
// of every day worked by the holidays' notices, and one of several days of
// the month of which one is the 29th or the 30th (which here is the
// month's last day where the month is shorter).
func rrule(t *model.Task, on time.Time, timed bool) string {
	r := t.Repeat
	if r == nil || r.FromDone || t.Status == model.StatusDone {
		return ""
	}
	weekdays := []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}
	weekday := func(d int) string {
		if d < 1 || d > 7 {
			return ""
		}
		return weekdays[d-1]
	}
	var parts []string
	switch r.Unit {
	case model.RepeatDay:
		parts = append(parts, "FREQ=DAILY")
	case model.RepeatWeek:
		parts = append(parts, "FREQ=WEEKLY")
	case model.RepeatMonth:
		parts = append(parts, "FREQ=MONTHLY")
	case model.RepeatYear:
		parts = append(parts, "FREQ=YEARLY")
	default:
		return ""
	}
	if r.Every > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(r.Every))
	}
	switch {
	case r.Unit == model.RepeatWeek && len(r.Days) > 0:
		var days []string
		for _, d := range r.Days {
			if weekday(d) == "" {
				return ""
			}
			days = append(days, weekday(d))
		}
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	case r.Unit == model.RepeatMonth && r.Nth != 0:
		wd := (int(on.Weekday())+6)%7 + 1
		if len(r.Days) > 0 {
			wd = r.Days[0]
		}
		if weekday(wd) == "" || r.Nth > 5 {
			return ""
		}
		parts = append(parts, "BYDAY="+strconv.Itoa(max(r.Nth, -1))+weekday(wd))
	case r.Unit == model.RepeatMonth:
		days := r.MonthDays
		if len(days) == 0 {
			days = []int{on.Day()}
		}
		var list []string
		last := false // the last of the days listed that the month has
		for _, d := range days {
			switch {
			case d < 0 || d >= 31: // the month's last day
				list = append(list, "-1")
			case d >= 29 && len(days) > 1:
				return ""
			case d >= 29: // that day, or the last the month has
				for i := 28; i <= d; i++ {
					list = append(list, strconv.Itoa(i))
				}
				last = true
			case d > 0:
				list = append(list, strconv.Itoa(d))
			}
		}
		if len(list) == 0 {
			return ""
		}
		parts = append(parts, "BYMONTHDAY="+strings.Join(list, ","))
		if last {
			parts = append(parts, "BYSETPOS=-1")
		}
	}
	switch {
	case r.Until != nil && timed:
		parts = append(parts, "UNTIL="+r.Until.Format("20060102")+"T235959")
	case r.Until != nil:
		parts = append(parts, "UNTIL="+r.Until.Format("20060102"))
	case r.Count > 0: // in all: those left
		parts = append(parts, "COUNT="+strconv.Itoa(max(r.Count-t.Times, 1)))
	}
	return strings.Join(parts, ";")
}

func escapeICS(s string) string {

	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(s)
}

// fold breaks a line of iCalendar at 75 bytes, between characters.
func fold(s string) string {
	if len(s) <= 75 {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		size := len(string(r))
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}

// PDF writes pictures as the pages of a PDF, a page each, at its size in
// points of the width given (the height following the picture).
func PDF(w io.Writer, pages []image.Image, pageWidth float64) error {
	var buf bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// 1: catalog, 2: pages; then per page: page, content, image.
	n := len(pages)
	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i*3)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	for i, img := range pages {
		bounds := img.Bounds()
		pw := pageWidth
		ph := pageWidth * float64(bounds.Dy()) / float64(bounds.Dx())
		var jp bytes.Buffer
		if err := jpeg.Encode(&jp, img, &jpeg.Options{Quality: 92}); err != nil {
			return err
		}
		page, content, im := 3+i*3, 4+i*3, 5+i*3
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /XObject << /Im%d %d 0 R >> >> /Contents %d 0 R >>",
			pw, ph, i, im, content))
		_ = page
		draw := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Im%d Do Q", pw, ph, i)
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(draw), draw))
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			len(offsets), bounds.Dx(), bounds.Dy(), jp.Len())
		buf.Write(jp.Bytes())
		buf.WriteString("\nendstream\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(buf.Bytes())
	return err
}
