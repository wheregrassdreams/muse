package settings

import "testing"

func TestHandleOwnsTransientPageAndScrollState(t *testing.T) {
	handle := NewHandle()
	if handle.IsOpen() || handle.Page() != AppearancePage || handle.ScrollOffset() != 0 {
		t.Fatalf("new handle = open:%v page:%v scroll:%v", handle.IsOpen(), handle.Page(), handle.ScrollOffset())
	}

	handle.OpenAt(AssistantPage)
	handle.SetScrollOffset(84)
	if !handle.IsOpen() || handle.Page() != AssistantPage || handle.ScrollOffset() != 84 {
		t.Fatalf("opened handle = open:%v page:%v scroll:%v", handle.IsOpen(), handle.Page(), handle.ScrollOffset())
	}

	handle.Apply(Action{Kind: SelectPage, Page: EditorPage})
	if handle.Page() != EditorPage {
		t.Fatalf("page after typed action = %v, want %v", handle.Page(), EditorPage)
	}
	handle.Apply(Action{Kind: Close})
	if handle.IsOpen() {
		t.Fatal("close action left the settings panel open")
	}
}

func TestZeroHandleCanOpenAndApplyActions(t *testing.T) {
	var handle Handle
	handle.OpenAt(EditorPage)
	handle.Apply(Action{Kind: Close})
	if handle.IsOpen() || handle.Page() != EditorPage {
		t.Fatalf("zero handle = open:%v page:%v", handle.IsOpen(), handle.Page())
	}
}
