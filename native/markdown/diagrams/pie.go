package diagrams

import (
	"strconv"
	"strings"
)

// PieSlice is a labeled positive value in a Mermaid pie chart.
type PieSlice struct {
	Label string
	Value float64
}

// PieChart is the backend-neutral parsed model for a Mermaid pie diagram.
type PieChart struct {
	Title  string
	Slices []PieSlice
}

// ParsePie parses Mermaid pie-chart data without retaining UI state.
func ParsePie(source string) (*PieChart, bool) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 {
		return nil, false
	}
	head := strings.ToLower(strings.TrimSpace(lines[0]))
	if head != "pie" && !strings.HasPrefix(head, "pie ") {
		return nil, false
	}
	chart := &PieChart{}
	if strings.HasPrefix(head, "pie title ") {
		chart.Title = strings.TrimSpace(lines[0][len("pie title "):])
	}
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "title ") {
			chart.Title = strings.TrimSpace(line[len("title "):])
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "showdata") {
			continue
		}
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			continue
		}
		label := strings.Trim(strings.TrimSpace(line[:colon]), `"`)
		value, err := strconv.ParseFloat(strings.TrimSpace(line[colon+1:]), 64)
		if err != nil || value <= 0 || label == "" {
			continue
		}
		chart.Slices = append(chart.Slices, PieSlice{Label: label, Value: value})
	}
	if len(chart.Slices) == 0 {
		return nil, false
	}
	return chart, true
}

// PieWidth is the minimum comfortable width for the pie and legend layout.
const PieWidth = 540
