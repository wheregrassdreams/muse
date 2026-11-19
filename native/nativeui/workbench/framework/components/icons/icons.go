// Package icons adapts the shared, UI-independent Lucide catalog to MyGo SVGs.
package icons

import (
	"sync"

	"github.com/egoist/mygo/ui"
	"muse/ui/iconlibrary"
)

var parsed sync.Map // map[string]*ui.SVG; catalog assets are immutable.

// Lookup adapts one catalog entry to a MyGo SVG. The parsed SVG is shared and
// immutable; callers apply their own size and color when rendering it.
func Lookup(id string) (*ui.SVG, bool) {
	if cached, ok := parsed.Load(id); ok {
		return cached.(*ui.SVG), true
	}
	asset, ok := iconlibrary.Lookup(id)
	if !ok {
		return nil, false
	}
	icon := ui.MustParseSVG([]byte(asset.SVG))
	actual, _ := parsed.LoadOrStore(id, icon)
	return actual.(*ui.SVG), true
}

func must(id string) *ui.SVG {
	icon, ok := Lookup(id)
	if !ok {
		panic("nativeui/icons: missing catalog icon " + id)
	}
	return icon
}

func New() *ui.SVG            { return must("square-pen") }
func OpenFile() *ui.SVG       { return must("file-up") }
func FolderOpen() *ui.SVG     { return must("folder-open") }
func Files() *ui.SVG          { return must("files") }
func Outline() *ui.SVG        { return must("list") }
func Search() *ui.SVG         { return must("search") }
func AI() *ui.SVG             { return must("sparkles") }
func Sun() *ui.SVG            { return must("sun") }
func Moon() *ui.SVG           { return must("moon") }
func Settings() *ui.SVG       { return must("settings") }
func File() *ui.SVG           { return must("file") }
func Image() *ui.SVG          { return must("image") }
func Folder() *ui.SVG         { return must("folder") }
func FilePlus() *ui.SVG       { return must("file-plus") }
func FolderPlus() *ui.SVG     { return must("folder-plus") }
func Edit() *ui.SVG           { return must("pencil") }
func Eye() *ui.SVG            { return must("eye") }
func Save() *ui.SVG           { return must("save") }
func Close() *ui.SVG          { return must("x") }
func WindowMinimize() *ui.SVG { return must("minimize") }
func WindowMaximize() *ui.SVG { return must("maximize") }
func WindowRestore() *ui.SVG  { return must("copy") }
func Chevron() *ui.SVG        { return must("chevron-right") }
func ChevronLeft() *ui.SVG    { return must("chevron-left") }
func Info() *ui.SVG           { return must("info") }
func More() *ui.SVG           { return must("ellipsis") }
func ChevronDown() *ui.SVG    { return must("chevron-down") }
func PanelLeftClose() *ui.SVG { return must("panel-left-close") }
func PanelLeftOpen() *ui.SVG  { return must("panel-left-open") }
func ArrowUp() *ui.SVG        { return must("arrow-up") }
func Stop() *ui.SVG           { return must("square") }
func Trash() *ui.SVG          { return must("trash") }
func TrashOpen() *ui.SVG      { return must("trash-2") }
