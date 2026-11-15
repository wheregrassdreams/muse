// Package notedate parses and presents dates stored in note metadata.
// Stored values stay portable; presentation is controlled by FormatOptions so
// the UI can later supply locale-specific styles without changing the schema.
package notedate

import (
	"strings"
	"time"

	"github.com/araddon/dateparse"
)

type Precision uint8

const (
	Date Precision = iota
	DateTime
)

type FormatOptions struct {
	// Locale is reserved for locale-aware format selection by the UI layer.
	Locale         string
	Location       *time.Location
	DateLayout     string
	DateTimeLayout string
}

func Parse(value string) (time.Time, Precision, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, Date, &dateValueError{}
	}
	layout, err := dateparse.ParseFormat(value)
	if err != nil {
		return time.Time{}, Date, err
	}
	parsed, err := dateparse.ParseStrict(value)
	if err != nil {
		return time.Time{}, Date, err
	}
	// dateparse's strict parser supplies timezone-aware values where present.
	// For a timestamp with no zone, preserve the written local wall clock.
	if !layoutHasZone(layout, value) {
		parsed, err = time.ParseInLocation(layout, value, time.Local)
		if err != nil {
			return time.Time{}, Date, err
		}
	}
	if layoutHasClock(layout) {
		return parsed, DateTime, nil
	}
	return parsed, Date, nil
}

// Format returns the original value unchanged when it is empty or invalid.
// Date-only values retain their written calendar day across time zones.
func Format(value string, options FormatOptions) string {
	parsed, precision, err := Parse(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	location := options.Location
	if location == nil {
		location = time.Local
	}
	if precision == Date {
		if options.DateLayout == "" {
			options.DateLayout = "2006-01-02"
		}
		year, month, day := parsed.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, location).Format(options.DateLayout)
	}
	if options.DateTimeLayout == "" {
		options.DateTimeLayout = "2006-01-02 15:04"
	}
	return parsed.In(location).Format(options.DateTimeLayout)
}

func layoutHasClock(layout string) bool {
	for _, token := range []string{"15", "3:04", "04:05", "PM", "pm"} {
		if strings.Contains(layout, token) {
			return true
		}
	}
	return false
}

func layoutHasZone(layout, value string) bool {
	for _, token := range []string{"-0700", "-07:00", "Z07", "MST"} {
		if strings.Contains(layout, token) {
			return true
		}
	}
	upper := strings.ToUpper(strings.TrimSpace(value))
	for _, suffix := range []string{"Z", " UTC", " GMT"} {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

type dateValueError struct{}

func (*dateValueError) Error() string { return "empty date value" }
