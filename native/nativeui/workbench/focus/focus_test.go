package focus

import (
	"errors"
	"reflect"
	"testing"

	commands "muse/nativeui/workbench/commands"
)

func TestFocusRouterMovesWithoutWrappingAndCyclesWithWrapping(t *testing.T) {
	r, err := NewFocusRouter([]commands.PanelID{commands.PanelIDFiles, commands.PanelIDOutline, commands.PanelIDEditor}, commands.PanelIDFiles)
	if err != nil {
		t.Fatal(err)
	}
	if got, moved := r.Move(-1); got != commands.PanelIDFiles || moved {
		t.Fatalf("move before first = %q, %v", got, moved)
	}
	if got, moved := r.Move(1); got != commands.PanelIDOutline || !moved {
		t.Fatalf("move forward = %q, %v", got, moved)
	}
	if focused := r.Focus(commands.PanelIDEditor); !focused || r.Current() != commands.PanelIDEditor {
		t.Fatalf("focus known panel = %v, current=%q", focused, r.Current())
	}
	if got, moved := r.Move(1); got != commands.PanelIDEditor || moved {
		t.Fatalf("move past last = %q, %v", got, moved)
	}
	if got, moved := r.Cycle(false); got != commands.PanelIDFiles || !moved {
		t.Fatalf("cycle forward = %q, %v", got, moved)
	}
	if got, moved := r.Cycle(true); got != commands.PanelIDEditor || !moved {
		t.Fatalf("cycle backward = %q, %v", got, moved)
	}
}

func TestFocusRouterValidatesAndCopiesItsOrder(t *testing.T) {
	if _, err := NewFocusRouter(nil, ""); !errors.Is(err, ErrEmptyPanelOrder) {
		t.Fatalf("empty-order error = %v", err)
	}
	if _, err := NewFocusRouter([]commands.PanelID{"files", ""}, "files"); !errors.Is(err, ErrEmptyPanelID) {
		t.Fatalf("empty-ID error = %v", err)
	}
	if _, err := NewFocusRouter([]commands.PanelID{"files", "files"}, "files"); !errors.Is(err, ErrDuplicatePanel) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := NewFocusRouter([]commands.PanelID{"files"}, "editor"); !errors.Is(err, ErrUnknownPanel) {
		t.Fatalf("unknown-initial error = %v", err)
	}
	order := []commands.PanelID{commands.PanelIDFiles, commands.PanelIDEditor}
	r, err := NewFocusRouter(order, "")
	if err != nil {
		t.Fatal(err)
	}
	order[0] = commands.PanelIDAI
	copy := r.Order()
	if !reflect.DeepEqual(copy, []commands.PanelID{commands.PanelIDFiles, commands.PanelIDEditor}) {
		t.Fatalf("router order aliased caller data: %v", copy)
	}
	copy[0] = commands.PanelIDAI
	if r.Current() != commands.PanelIDFiles || !reflect.DeepEqual(r.Order(), []commands.PanelID{commands.PanelIDFiles, commands.PanelIDEditor}) {
		t.Fatal("returned order mutated router state")
	}
	if r.Focus(commands.PanelIDSearch) || r.Current() != commands.PanelIDFiles {
		t.Fatal("unknown panel changed focus")
	}
}
