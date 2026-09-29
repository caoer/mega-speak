package main

import (
	"fmt"
	"strings"
	"time"
)

// Columns of the panel, in order.
const (
	colKind = iota
	colTime
	colAgent
	colText
	colEmotion
	nCols
)

// cell is one table cell: a first line and an optional second line in the
// secondary colour.
type cell struct {
	text, sub, tip string
	dim            bool   // text in the secondary colour
	icon, tint     string // an SF Symbol before the text and its colour
	subTint        string // the second line's colour
}

// kind is the sort of news a line is, read from its emotion: the icon,
// colour and tooltip of its first column.
type kind struct{ icon, tint, name string }

func kindOf(emotion string) kind {
	switch emotion {
	case "urgent":
		return kind{"exclamationmark.triangle.fill", "red", "something broke: look now"}
	case "cheerful", "excited":
		return kind{"checkmark.circle.fill", "green", "done, and it went well"}
	case "calm", "serious":
		return kind{"questionmark.circle.fill", "orange", "waiting on your answer"}
	}
	return kind{"info.circle.fill", "gray", "for your information"}
}

// line is one row of the panel and the pane a click on it focuses.
type line struct {
	cells [nCols]cell
	pane  string
	why   string // why a click cannot focus, when pane is ""
}

func lineOf(e entry, w where, now time.Time) line {
	var l line
	k := kindOf(e.Emotion)
	l.cells[colKind] = cell{icon: k.icon, tint: k.tint, tip: k.name}
	ts := e.TS.Local()
	l.cells[colTime] = cell{text: ts.Format("15:04"), sub: age(ts, now), tip: ts.Format("Mon Jan 2 15:04:05")}

	a := cell{text: w.label}
	switch {
	case a.text != "":
		a.sub = join(w.id, w.status)
	case w.id != "":
		a.text, a.sub = w.id, w.status
	default:
		a.text, a.sub = "unknown caller", w.status
		if e.Host != "" && e.Host != "unknown" {
			a.text = e.Host
		}
	}
	if e.Session != "" {
		a.tip = e.Session
	}
	if e.Host != "" {
		a.tip = join(a.tip, e.Host)
	}
	switch {
	case w.pane != "":
		a.icon, a.tint, a.subTint = "arrow.right.circle.fill", "blue", "blue"
	case strings.HasPrefix(w.status, "on "):
		a.icon, a.tint, a.dim = "network", "gray", true
	case w.status != "":
		a.icon, a.tint, a.dim = "xmark.circle", "gray", true
	}
	l.cells[colAgent] = a

	l.cells[colText] = cell{text: strings.TrimSpace(e.Text), dim: e.DryRun}
	em := cell{text: e.Emotion, tip: e.Instruction}
	if em.text == "" {
		em.text = "neutral"
	}
	if e.DryRun {
		em.sub = "dry run"
	}
	l.cells[colEmotion] = em

	l.pane = w.pane
	name := firstNonEmpty(w.label, w.id, "This agent")
	switch {
	case w.pane != "":
	case w.status == "":
		l.why = "herdr is not answering"
	case w.status == "no agent recorded":
		l.why = "This line names no agent"
	case strings.HasPrefix(w.status, "on "):
		l.why = name + " runs on " + e.Host + ", outside this Mac's herdr"
	default:
		l.why = name + " is gone: no herdr pane has it"
	}
	return l
}

// age is how long ago t was, as a second line under the clock time.
func age(t, now time.Time) string {
	d := now.Sub(t)
	y, m, dd := now.Date()
	midnight := time.Date(y, m, dd, 0, 0, 0, 0, now.Location())
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case !t.Before(midnight):
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	case !t.Before(midnight.AddDate(0, 0, -1)):
		return "yesterday"
	}
	return t.Format("Jan 2")
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
