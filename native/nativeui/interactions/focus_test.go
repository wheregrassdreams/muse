package interactions

import (
	"errors"
	"reflect"
	"testing"
)

func TestFocusRouterMovesWithoutWrappingAndCyclesWithWrapping(t *testing.T) {
	r, err := NewFocusRouter([]PanelID{PanelIDFiles, PanelIDOutline, PanelIDEditor}, PanelIDFiles)
	if err != nil {
		t.Fatal(err)
	}
	if got, moved := r.Move(-1); got != PanelIDFiles || moved {
		t.Fatalf("move before first = %q, %v", got, moved)
	}
	if got, moved := r.Move(1); got != PanelIDOutline || !moved {
		t.Fatalf("move forward = %q, %v", got, moved)
	}
	if focused := r.Focus(PanelIDEditor); !focused || r.Current() != PanelIDEditor {
		t.Fatalf("focus known panel = %v, current=%q", focused, r.Current())
	}
	if got, moved := r.Move(1); got != PanelIDEditor || moved {
		t.Fatalf("move past last = %q, %v", got, moved)
	}
	if got, moved := r.Cycle(false); got != PanelIDFiles || !moved {
		t.Fatalf("cycle forward = %q, %v", got, moved)
	}
	if got, moved := r.Cycle(true); got != PanelIDEditor || !moved {
		t.Fatalf("cycle backward = %q, %v", got, moved)
	}
}

func TestFocusRouterValidatesAndCopiesItsOrder(t *testing.T) {
	if _, err := NewFocusRouter(nil, ""); !errors.Is(err, ErrEmptyPanelOrder) {
		t.Fatalf("empty-order error = %v", err)
	}
	if _, err := NewFocusRouter([]PanelID{"files", ""}, "files"); !errors.Is(err, ErrEmptyPanelID) {
		t.Fatalf("empty-ID error = %v", err)
	}
	if _, err := NewFocusRouter([]PanelID{"files", "files"}, "files"); !errors.Is(err, ErrDuplicatePanel) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := NewFocusRouter([]PanelID{"files"}, "editor"); !errors.Is(err, ErrUnknownPanel) {
		t.Fatalf("unknown-initial error = %v", err)
	}
	order := []PanelID{PanelIDFiles, PanelIDEditor}
	r, err := NewFocusRouter(order, "")
	if err != nil {
		t.Fatal(err)
	}
	order[0] = PanelIDAI
	copy := r.Order()
	if !reflect.DeepEqual(copy, []PanelID{PanelIDFiles, PanelIDEditor}) {
		t.Fatalf("router order aliased caller data: %v", copy)
	}
	copy[0] = PanelIDAI
	if r.Current() != PanelIDFiles || !reflect.DeepEqual(r.Order(), []PanelID{PanelIDFiles, PanelIDEditor}) {
		t.Fatal("returned order mutated router state")
	}
	if r.Focus(PanelIDSearch) || r.Current() != PanelIDFiles {
		t.Fatal("unknown panel changed focus")
	}
}
