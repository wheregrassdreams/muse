package diagrams

import (
	"strconv"
	"strings"
	"time"
)

// GanttChart is a parsed timeline model independent of the drawing backend.
type GanttChart struct {
	Title    string
	Format   string
	Tasks    []*GanttTask
	Sections []string
	Rows     []GanttRow
	Start    time.Time
	End      time.Time
}

// GanttTask is one dated work item.
type GanttTask struct {
	Name      string
	ID        string
	Section   string
	StartSpec string
	Duration  time.Duration
	Start     time.Time
	Done      bool
	Active    bool
	Critical  bool
	Milestone bool
}

// GanttRow is either a section header or its task.
type GanttRow struct {
	Section string
	Task    *GanttTask
}

// GanttOptions injects the localized default section and the meaning of today.
type GanttOptions struct {
	DefaultSection string
	Today          time.Time
}

// ParseGantt parses Muse's supported Mermaid Gantt subset.
func ParseGantt(source string, opts GanttOptions) (*GanttChart, bool) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || !strings.EqualFold(strings.TrimSpace(lines[0]), "gantt") {
		return nil, false
	}
	chart := &GanttChart{Format: "2006-01-02"}
	section := opts.DefaultSection
	if section == "" {
		section = "Task"
	}
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "title "):
			chart.Title = strings.TrimSpace(line[len("title "):])
			continue
		case strings.HasPrefix(low, "dateformat "):
			chart.Format = ganttDateLayout(strings.TrimSpace(line[len("dateformat "):]))
			continue
		case strings.HasPrefix(low, "axisformat "), strings.HasPrefix(low, "excludes "),
			strings.HasPrefix(low, "todaymarker"), strings.HasPrefix(low, "tickinterval"),
			strings.HasPrefix(low, "weekday"), strings.HasPrefix(low, "inclusiveenddates"):
			continue
		case strings.HasPrefix(low, "section "):
			section = strings.TrimSpace(line[len("section "):])
			if !hasString(chart.Sections, section) {
				chart.Sections = append(chart.Sections, section)
			}
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Split(line[colon+1:], ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		if len(fields) < 2 {
			continue
		}
		task := &GanttTask{Name: name, Section: section}
		for _, f := range fields[:len(fields)-2] {
			switch strings.ToLower(f) {
			case "done":
				task.Done = true
			case "active":
				task.Active = true
			case "crit":
				task.Critical = true
			case "milestone":
				task.Milestone = true
			default:
				if task.ID == "" {
					task.ID = f
				}
			}
		}
		task.StartSpec = fields[len(fields)-2]
		task.Duration = ganttDuration(fields[len(fields)-1])
		chart.Tasks = append(chart.Tasks, task)
	}
	if len(chart.Tasks) == 0 {
		return nil, false
	}
	for pass := 0; pass < len(chart.Tasks)+1; pass++ {
		for _, task := range chart.Tasks {
			if !task.Start.IsZero() {
				continue
			}
			spec := strings.ToLower(task.StartSpec)
			if strings.HasPrefix(spec, "after ") {
				id := strings.TrimSpace(strings.TrimPrefix(spec, "after "))
				if dep := chart.taskByID(id); dep != nil && !dep.Start.IsZero() {
					task.Start = dep.Start.Add(dep.Duration)
				}
				continue
			}
			if strings.HasPrefix(spec, "until ") {
				continue
			}
			if strings.EqualFold(spec, "today") {
				today := opts.Today
				if today.IsZero() {
					today = time.Now()
				}
				task.Start = today.Truncate(24 * time.Hour)
				continue
			}
			if t, err := time.Parse(chart.Format, task.StartSpec); err == nil {
				task.Start = t
			}
		}
	}
	for _, task := range chart.Tasks {
		if task.Start.IsZero() {
			continue
		}
		if chart.Start.IsZero() || task.Start.Before(chart.Start) {
			chart.Start = task.Start
		}
		end := task.Start.Add(task.Duration)
		if end.After(chart.End) {
			chart.End = end
		}
	}
	if chart.Start.IsZero() || !chart.End.After(chart.Start) {
		return nil, false
	}
	for _, sec := range chart.Sections {
		chart.Rows = append(chart.Rows, GanttRow{Section: sec})
		for _, task := range chart.Tasks {
			if task.Section == sec {
				chart.Rows = append(chart.Rows, GanttRow{Task: task})
			}
		}
	}
	for _, task := range chart.Tasks {
		if !hasString(chart.Sections, task.Section) {
			chart.Sections = append(chart.Sections, task.Section)
			chart.Rows = append(chart.Rows, GanttRow{Section: task.Section}, GanttRow{Task: task})
		}
	}
	return chart, true
}

// Height returns the chart's fixed row layout height.
func (g *GanttChart) Height() float32 {
	h := float32(20)
	if g.Title != "" {
		h += 24
	}
	h += 26
	for range g.Rows {
		h += 26
	}
	return h + 20
}

// Width returns the minimum width for the labels and date axis.
func (g *GanttChart) Width() float32 { return 300 }

func (g *GanttChart) taskByID(id string) *GanttTask {
	id = strings.TrimSpace(id)
	for _, task := range g.Tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

func hasString(list []string, s string) bool {
	for _, value := range list {
		if value == s {
			return true
		}
	}
	return false
}

func ganttDateLayout(s string) string {
	switch strings.ToUpper(s) {
	case "YYYY-MM-DD", "YYYY/MM/DD":
		out := strings.ToUpper(s)
		out = strings.ReplaceAll(out, "YYYY", "2006")
		out = strings.ReplaceAll(out, "MM", "01")
		out = strings.ReplaceAll(out, "DD", "02")
		return out
	}
	if strings.Contains(s, "/") {
		return "2006/01/02"
	}
	return "2006-01-02"
}

func ganttDuration(s string) time.Duration {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	unit := s[len(s)-1]
	number := s
	mult := 24 * float64(time.Hour)
	switch unit {
	case 'd':
		number = s[:len(s)-1]
	case 'w':
		number = s[:len(s)-1]
		mult *= 7
	case 'h':
		number = s[:len(s)-1]
		mult = float64(time.Hour)
	case 'm':
		number = s[:len(s)-1]
		mult *= 30
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n * mult)
}
