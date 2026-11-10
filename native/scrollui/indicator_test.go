package scrollui

import (
	"testing"
	"time"
)

func TestIndicatorOnlyTracksActualMovement(t *testing.T) {
	var i Indicator
	n := time.Now()
	if i.Visible(n, 0, 10) {
		t.Fatal("initial offset showed indicator")
	}
	if i.Visible(n.Add(time.Second), 0, 10) {
		t.Fatal("stationary hover showed indicator")
	}
	if !i.Visible(n.Add(time.Second), 0, 20) {
		t.Fatal("movement did not show indicator")
	}
	if !i.Visible(n.Add(time.Second+600*time.Millisecond), 0, 20) {
		t.Fatal("hid too early")
	}
	if i.Visible(n.Add(time.Second+VisibleFor), 0, 20) {
		t.Fatal("stationary indicator did not expire")
	}
	if !i.Visible(n.Add(2*time.Second), 4, 20) {
		t.Fatal("horizontal movement did not show indicator")
	}
}
