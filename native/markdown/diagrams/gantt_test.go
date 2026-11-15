package diagrams

import (
	"testing"
	"time"
)

func TestParseGanttResolvesDependenciesAndRows(t *testing.T) {
	g, ok := ParseGantt(`gantt
title Plan
dateFormat YYYY-MM-DD
section Design
Requirements :a1, 2026-10-01, 3d
Review :after a1, 2d
section Build
Implement :done, 2026-10-06, 1w
`, GanttOptions{DefaultSection: "Tasks", Today: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)})
	if !ok || len(g.Tasks) != 3 || !g.End.After(g.Start) || len(g.Rows) != 5 {
		t.Fatalf("gantt = %+v", g)
	}
	if got := g.Tasks[1].Start.Format("2006-01-02"); got != "2026-10-04" {
		t.Fatalf("after start = %s", got)
	}
	if got := g.Rows[0].Section; got != "Design" {
		t.Fatalf("first row section = %q", got)
	}
}

func TestParseGanttUsesInjectedToday(t *testing.T) {
	today := time.Date(2026, 11, 15, 17, 30, 0, 0, time.FixedZone("local", 8*60*60))
	g, ok := ParseGantt("gantt\nTask :today, 2d", GanttOptions{Today: today})
	if !ok {
		t.Fatal("today task was not parsed")
	}
	if got, want := g.Tasks[0].Start.Format("2006-01-02"), "2026-11-15"; got != want {
		t.Fatalf("today = %s, want %s", got, want)
	}
}
