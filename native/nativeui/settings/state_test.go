package settings

import "testing"

func TestControllerOwnsTransientPageAndScrollState(t *testing.T) {
	controller := newController()
	if controller.IsOpen() || controller.Page() != AppearancePage || controller.ScrollOffset() != 0 {
		t.Fatalf("new controller = open:%v page:%v scroll:%v", controller.IsOpen(), controller.Page(), controller.ScrollOffset())
	}

	controller.OpenAt(AssistantPage)
	controller.SetScrollOffset(84)
	if !controller.IsOpen() || controller.Page() != AssistantPage || controller.ScrollOffset() != 84 {
		t.Fatalf("opened controller = open:%v page:%v scroll:%v", controller.IsOpen(), controller.Page(), controller.ScrollOffset())
	}

	controller.Apply(Action{Kind: SelectPage, Page: EditorPage})
	if controller.Page() != EditorPage {
		t.Fatalf("page after typed action = %v, want %v", controller.Page(), EditorPage)
	}
	controller.Apply(Action{Kind: Close})
	if controller.IsOpen() {
		t.Fatal("close action left the settings panel open")
	}
}

func TestZeroControllerCanOpenAndApplyActions(t *testing.T) {
	var controller controller
	controller.OpenAt(EditorPage)
	controller.Apply(Action{Kind: Close})
	if controller.IsOpen() || controller.Page() != EditorPage {
		t.Fatalf("zero controller = open:%v page:%v", controller.IsOpen(), controller.Page())
	}
}
