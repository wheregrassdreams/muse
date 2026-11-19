package nativeui

import (
	"path/filepath"
	"testing"
	"time"

	"muse/nativeui/naviconpicker"
	componenticons "muse/nativeui/ui/icons"
)

func TestNavTreeIconPickerFiltersAndPersistsVaultRelativeChoice(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool { return a.vault != nil && a.vaultSnapshot.Ready })
	item := filepath.Join(a.root, "Home.md")
	rightClickTreeRow(t, tt, "Home.md")
	if err := tt.ChooseMenuItem("更改图标…"); err != nil {
		t.Fatalf("open tree icon picker from context menu: %v", err)
	}
	var matches []naviconpicker.Option
	var loading bool
	for i := 0; i < 100; i++ {
		tt.Frame()
		snapshot := a.navIconPicker.Snapshot()
		matches, loading = snapshot.Results, snapshot.Loading
		if !loading {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if loading || len(matches) < 2000 || !tt.HasText("选择图标") {
		t.Fatalf("compact icon picker is not visible: %q", tt.Texts())
	}
	runtimeShot(t, tt, "nav-icon-picker")
	tt.Scroll(150, 500, 0, 360)
	tt.Frame()
	nearRect, nearOK := tt.Find("Align End Horizontal")
	pickerTitle, titleOK := tt.Find("选择图标")
	if !nearOK || !titleOK || nearRect.Y < pickerTitle.Y+60 || nearRect.Y > pickerTitle.Y+400 {
		t.Fatalf("icons disappeared after a short scroll: row=%+v title=%+v scroll=%+v texts=%q", nearRect, pickerTitle, a.navIconPicker.Snapshot().Scroll, tt.Texts())
	}
	runtimeShot(t, tt, "nav-icon-picker-scroll-short")
	tt.Scroll(150, 500, 0, 3000)
	tt.Frame()
	if a.navIconPicker.Snapshot().Scroll.Y < 3000 {
		t.Fatalf("icon list did not scroll to its middle: y=%v", a.navIconPicker.Snapshot().Scroll.Y)
	}
	middleRect, middleOK := tt.Find("Book Type")
	if !middleOK || !titleOK || middleRect.Y < pickerTitle.Y+60 || middleRect.Y > pickerTitle.Y+400 {
		t.Fatalf("middle virtual row is outside the visible picker viewport: row=%+v title=%+v scroll=%+v texts=%q", middleRect, pickerTitle, a.navIconPicker.Snapshot().Scroll, tt.Texts())
	}
	runtimeShot(t, tt, "nav-icon-picker-scroll-middle")
	tt.Scroll(150, 500, 0, 30000)
	tt.Frame()
	if !tt.HasText("Zoom In") {
		t.Fatalf("last virtual rows are missing at the end of the icon list: %q", tt.Texts())
	}
	runtimeShot(t, tt, "nav-icon-picker-scroll-end")
	if err := tt.Click("搜索图标"); err != nil {
		t.Fatalf("focus icon search: %v", err)
	}
	tt.Type("bookmark")
	for i := 0; i < 100; i++ {
		tt.Frame()
		snapshot := a.navIconPicker.Snapshot()
		matches, loading = snapshot.Results, snapshot.Loading
		if !loading && len(matches) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	containsBookmark := false
	for _, icon := range matches {
		containsBookmark = containsBookmark || icon.ID == "bookmark"
	}
	if loading || len(matches) == 0 || !containsBookmark || !tt.HasText("书签") || len(matches) >= 2356 {
		t.Fatalf("icon query did not filter picker options: %q", tt.Texts())
	}
	if err := tt.Click("书签"); err != nil {
		t.Fatalf("choose bookmark icon: %v", err)
	}
	bookmarked, _ := componenticons.Lookup("bookmark")
	if a.navIconPicker.IsOpen() || a.navIconPicker.IconID(item) != "bookmark" || a.navIconPicker.IconForPath(item, false) != bookmarked {
		t.Fatalf("tree icon was not applied: picker=%v ID=%q", a.navIconPicker.IsOpen(), a.navIconPicker.IconID(item))
	}
	if got := a.navIconPicker.IconID(item); got != "bookmark" {
		t.Fatalf("persisted icon key lookup = %q", got)
	}
	runtimeShot(t, tt, "nav-tree-custom-icon")
	rightClickTreeRow(t, tt, "Home.md")
	if err := tt.ChooseMenuItem("更改图标…"); err != nil {
		t.Fatalf("reopen tree icon picker: %v", err)
	}
	tt.Frame()
	if err := tt.Click("恢复默认图标"); err != nil {
		t.Fatalf("reset icon in picker: %v", err)
	}
	if a.navIconPicker.IsOpen() || a.navIconPicker.IconID(item) != "" || a.navIconPicker.IconForPath(item, false) != componenticons.File() {
		t.Fatalf("reset-to-default did not restore file icon: picker=%v ID=%q", a.navIconPicker.IsOpen(), a.navIconPicker.IconID(item))
	}
	if got := a.navIconPicker.IconID(item); got != "" {
		t.Fatalf("reset to default left a custom icon: %q", got)
	}
}
