package input

import "testing"

func TestSequenceCountsAndLeadingZero(t *testing.T) {
	var sequence Sequence
	if sequence.FeedDigit('0') {
		t.Fatal("leading zero was treated as a count")
	}
	for _, digit := range "120" {
		if !sequence.FeedDigit(digit) {
			t.Fatalf("digit %q was rejected", digit)
		}
	}
	if sequence.Count != 120 {
		t.Fatalf("count = %d, want 120", sequence.Count)
	}
	for _, digit := range "99999" {
		sequence.FeedDigit(digit)
	}
	if sequence.Count != maxCount {
		t.Fatalf("count was not capped: %d", sequence.Count)
	}
}

func TestSequenceConsumesMotionCountAndRetainsOperatorCount(t *testing.T) {
	sequence := Sequence{}
	sequence.BeginOperator("d", 3)
	for _, digit := range "2" {
		sequence.FeedDigit(digit)
	}
	motion, operator := sequence.ConsumeCounts()
	if motion != 2 || operator != 3 || sequence.Count != 0 || sequence.OperatorCount != 3 {
		t.Fatalf("counts = motion %d operator %d state %+v", motion, operator, sequence)
	}
	sequence.Reset()
	if sequence != (Sequence{}) {
		t.Fatalf("Reset retained pending state: %+v", sequence)
	}
}
