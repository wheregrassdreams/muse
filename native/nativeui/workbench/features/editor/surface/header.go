package noteeditor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	icons "muse/nativeui/workbench/framework/components/icons"
	"muse/nativeui/workbench/framework/shell/windowcontrols"
)

type HeaderActionKind uint8

const (
	HeaderBack HeaderActionKind = iota + 1
	HeaderForward
	HeaderOpenImagePreview
	HeaderSave
	HeaderOpenProperties
	HeaderRevealFile
	HeaderExportMarkdown
	HeaderExportHTML
	HeaderExportPDF
	HeaderOpenHistory
	HeaderOpenRecovery
	HeaderOpenBacklinks
	HeaderCopyPath
	HeaderSetWorkspace
)

type HeaderAction struct {
	Kind HeaderActionKind
}

type HeaderModel struct {
	Flat          bool
	PanelWidth    float32
	HeaderHeight  float32
	Path          string
	HasPrevious   bool
	HasNext       bool
	ImageDocument bool
	Mode          string
	Status        string
	Dirty         bool
	Properties    bool
	CanExport     bool
	InfoItems     []string
	HasFile       bool
}

// RenderHeader is the editor feature's document header. It emits typed user
// intents; the application router performs navigation, save and export.
func RenderHeader(c *ui.Context, model HeaderModel, dispatch func(HeaderAction)) {
	if c == nil {
		return
	}
	height := model.HeaderHeight
	if height <= 0 {
		height = max(float32(44), c.TitleBar().Height)
	}
	bar := c.TitleBar()
	right := float32(18) + bar.Right
	if windowcontrols.Visible() {
		right += windowcontrols.RightPadding()
	}
	row := ui.Row(c).Height(height).Shrink(0).Padding(0, right, 0, 6).Gap(2).DragWindow().AlignItems(ui.Center)
	if model.Flat {
		row.Absolute().Top(0).Left(0).Right(0).ZIndex(2)
	}
	row.Children(func() {
		ui.Box(c).Width(max(float32(30), bar.Left+38-model.PanelWidth)).Shrink(0)
		tint := c.Theme().TextMuted
		if c.Theme().Dark {
			tint = ui.RGBA(255, 255, 255, 0.92)
		}
		headerIcon(c, icons.ChevronLeft(), locales.ToolbarBack(), model.HasPrevious, tint, dispatch, HeaderBack)
		headerIcon(c, icons.Chevron(), locales.ToolbarForward(), model.HasNext, tint, dispatch, HeaderForward)
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).Padding(8, 0).Children(func() {
			vw, _ := c.Size()
			avail := vw - 56 - model.PanelWidth - 220
			ui.Text(c, tailPath(c, model.Path, 12.5, avail)).FontSize(12.5).TextColor(c.Theme().TextMuted).SingleLine()
		})
		if model.ImageDocument {
			headerIcon(c, icons.Image(), locales.ToolbarGlobalPreview(), true, tint, dispatch, HeaderOpenImagePreview)
		}
		if model.Mode != "" {
			ui.Text(c, model.Mode).FontSize(11).TextColor(c.Theme().TextMuted).Label(locales.DocumentMode())
		}
		if model.Status != "" {
			ui.Text(c, model.Status).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…")
		}
		if model.Dirty && !model.ImageDocument {
			headerIcon(c, icons.Save(), locales.MenuSave(), true, tint, dispatch, HeaderSave)
		}
		if model.Properties {
			headerIcon(c, icons.Info(), locales.DocumentInfo(), true, tint, dispatch, HeaderOpenProperties)
		} else {
			info := headerButton(c, icons.Info(), locales.DocumentInfo(), false, tint, nil)
			info.Menu(func(menu *ui.Menu) {
				for _, item := range model.InfoItems {
					menu.Item(item)
				}
				menu.Separator()
				if menu.Item(locales.FilesInFinder()).Chosen() && model.HasFile {
					dispatchHeader(dispatch, HeaderRevealFile)
				}
			})
		}
		more := headerButton(c, icons.More(), locales.DocumentMore(), false, tint, nil)
		more.Menu(func(menu *ui.Menu) {
			if model.CanExport {
				menu.Submenu(locales.DocumentExport(), func(export *ui.Menu) {
					if export.Item(locales.DocumentExportMarkdown()).Chosen() {
						dispatchHeader(dispatch, HeaderExportMarkdown)
					}
					if export.Item(locales.DocumentExportHTML()).Chosen() {
						dispatchHeader(dispatch, HeaderExportHTML)
					}
					if export.Item(locales.DocumentExportPDF()).Chosen() {
						dispatchHeader(dispatch, HeaderExportPDF)
					}
				})
				menu.Separator()
			}
			if menu.Item(locales.DocumentHistoryMenu()).Chosen() {
				dispatchHeader(dispatch, HeaderOpenHistory)
			}
			if menu.Item(locales.DocumentRecoveryMenu()).Chosen() {
				dispatchHeader(dispatch, HeaderOpenRecovery)
			}
			if menu.Item(locales.DocumentLinksMenu()).Chosen() {
				dispatchHeader(dispatch, HeaderOpenBacklinks)
			}
			if menu.Item(locales.MenuSave()).Chosen() {
				dispatchHeader(dispatch, HeaderSave)
			}
			if menu.Item(locales.DocumentCopyPathMenu()).Chosen() && model.HasFile {
				dispatchHeader(dispatch, HeaderCopyPath)
			}
			if menu.Item(locales.FilesSetWorkspace()).Chosen() && model.HasFile {
				dispatchHeader(dispatch, HeaderSetWorkspace)
			}
		})
	})
}

func headerIcon(c *ui.Context, icon *ui.SVG, label string, enabled bool, tint ui.Color, dispatch func(HeaderAction), action HeaderActionKind) {
	fn := func() { dispatchHeader(dispatch, action) }
	if !enabled {
		tint = tint.Alpha(0.35)
	}
	headerButton(c, icon, label, enabled, tint, fn)
}

func headerButton(c *ui.Context, icon *ui.SVG, label string, enabled bool, tint ui.Color, fn func()) ui.Element {
	b := ui.ButtonBase(c).Width(26).Height(26).Radius(6).Padding(0).
		Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	if b.Hovered() && enabled {
		b.Background(c.Theme().SurfaceHover)
	}
	if enabled && fn != nil {
		b.OnClick(fn)
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(tint) })
	return b
}

func dispatchHeader(dispatch func(HeaderAction), kind HeaderActionKind) {
	if dispatch != nil {
		dispatch(HeaderAction{Kind: kind})
	}
}

func tailPath(c *ui.Context, path string, size, width float32) string {
	measure := func(value string) float32 { w, _ := c.MeasureText(0, ui.Span{Text: value, Size: size}); return w }
	if width <= 0 || measure(path) <= width {
		return path
	}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if value := "…/" + strings.Join(parts[i:], "/"); measure(value) <= width {
			return value
		}
	}
	last := []rune(parts[len(parts)-1])
	for i := 1; i < len(last); i++ {
		if value := "…" + string(last[i:]); measure(value) <= width {
			return value
		}
	}
	return "…"
}
