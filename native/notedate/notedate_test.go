package notedate

import (
	"testing"
	"time"
)

func TestFormatCommonDateForms(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	cases := []struct {
		value, want string
	}{
		{"2026-11-13", "2026年11月13日"},
		{"2026-11-13T08:54:30.456789123Z", "2026年11月13日 16:54"},
		{"2026-11-13 08:54:30 +0800", "2026年11月13日 08:54"},
		{"Nov 13, 2026 8:54:30 AM UTC", "2026年11月13日 16:54"},
	}
	for _, tc := range cases {
		if got := Format(tc.value, FormatOptions{Location: location}); got != tc.want {
			t.Errorf("Format(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatPreservesInvalidAndRejectsAmbiguousDateParsing(t *testing.T) {
	if got := Format("not a date", FormatOptions{}); got != "not a date" {
		t.Fatalf("invalid input changed: %q", got)
	}
	if _, _, err := Parse("03/04/2026"); err == nil {
		t.Fatal("ambiguous numeric date should not be guessed")
	}
}

func TestFormatNaiveDateTimeAsLocalWallClock(t *testing.T) {
	if got := Format("2026-11-13 08:54", FormatOptions{}); got != "2026年11月13日 08:54" {
		t.Fatalf("naive local timestamp display = %q", got)
	}
}
