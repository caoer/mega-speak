package main

import (
	"strings"
	"time"

	"github.com/caoer/mega-speak/internal/i18n"
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
		return kind{"exclamationmark.triangle.fill", "red", i18n.T("row.urgent")}
	case "cheerful", "excited":
		return kind{"checkmark.circle.fill", "green", i18n.T("row.good")}
	case "calm", "serious":
		return kind{"questionmark.circle.fill", "orange", i18n.T("row.waiting")}
	}
	return kind{"info.circle.fill", "gray", i18n.T("row.info")}
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
		a.text, a.sub = i18n.T("row.unknown"), w.status
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
		em.sub = i18n.T("row.dry_run")
	}
	l.cells[colEmotion] = em

	l.pane = w.pane
	name := firstNonEmpty(w.label, w.id, i18n.T("row.this_agent"))
	switch {
	case w.pane != "":
	case w.status == "":
		l.why = i18n.T("row.no_herdr")
	case w.status == "no agent recorded":
		l.why = i18n.T("row.no_agent")
	case strings.HasPrefix(w.status, "on "):
		l.why = i18n.F("row.remote", name, e.Host)
	default:
		l.why = i18n.F("row.gone", name)
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
		return i18n.T("row.just_now")
	case d < time.Hour:
		return i18n.F("row.min_ago", int(d/time.Minute))
	case !t.Before(midnight):
		return i18n.F("row.h_ago", int(d/time.Hour))
	case !t.Before(midnight.AddDate(0, 0, -1)):
		return i18n.T("row.yesterday")
	}
	return t.Format(i18n.T("row.date"))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
