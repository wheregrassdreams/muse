package vaulthome

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestNewAndOpenActionsRenderAndDispatch(t *testing.T) {
	model := DefaultViewModel(nil, nil)
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if !tester.HasText(locales.DocumentNew()) || !tester.HasText(locales.FilesOpenFileMenu()) {
		t.Fatalf("landing actions missing: %q", tester.Texts())
	}
	if err := tester.Click(locales.DocumentNew()); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(locales.FilesOpenFileMenu()); err != nil {
		t.Fatal(err)
	}
	if want := []Action{NewDocument, OpenFile}; !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
}
