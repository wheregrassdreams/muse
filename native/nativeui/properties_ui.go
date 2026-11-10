package nativeui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"muse/frontmatter"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const propertiesSidebarWidth float32 = 340

type propertyDocumentCache struct {
	source, path string
	doc          frontmatter.Document
}

type systemProperty struct{ key, value string }

type fieldDisplayPreference struct {
	Pinned bool `json:"pinned,omitempty"`
	Hidden bool `json:"hidden,omitempty"`
	Order  int  `json:"order,omitempty"`
}

type documentPropertyPrefs struct {
	Fields map[string]fieldDisplayPreference `json:"fields,omitempty"`
}

type propertyPrefsFile map[string]documentPropertyPrefs

func (a *App) propertiesEnabled() bool { return !a.imageDocument.open && codeLanguage(a.path) == "" }

// propertyDocument parses only once for each source/path pair. The frontmatter
// parser itself stops at the closing YAML delimiter and never scans the body.
func (a *App) propertyDocument() frontmatter.Document {
	if a.propertyCache.path != a.path {
		a.propertyPrefs = loadDocumentPropertyPrefs(a.path)
	}
	if a.propertyCache.source != a.source || a.propertyCache.path != a.path {
		a.propertyCache = propertyDocumentCache{source: a.source, path: a.path, doc: frontmatter.Parse(a.source)}
	}
	return a.propertyCache.doc
}

func (a *App) documentTitle(c *ui.Context) {
	if !a.propertiesEnabled() {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Children(func() {
		ui.Box(c).Height(40).Shrink(0)
		a.propertySummary(c)
		ui.Box(c).Height(14).Shrink(0)
		ui.Row(c).Height(56).Shrink(0).FillWidth().AlignItems(ui.Center).Children(func() {
			if a.propertyTitleEditing {
				input := ui.TextInputBase(c.Key("document-title-input"), &a.propertyTitleDraft).Grow(1).
					Height(42).Padding(0, 4).TextColor(t.Text).FocusRing(false).
					FontSize(25).FontWeight(650).Placeholder(filepath.Base(strings.TrimSuffix(a.path, filepath.Ext(a.path)))).Label("文档标题").
					AutoFocus().OnSubmit(func() { a.commitDocumentTitle() })
				if input.Focused() {
					a.propertyTitleFocused = true
				} else if a.propertyTitleFocused {
					a.commitDocumentTitle()
				}
				return
			}
			label := "未命名"
			if a.path != "" {
				label = strings.TrimSuffix(filepath.Base(a.path), filepath.Ext(a.path))
			}
			if cached := a.propertyDocument(); cached.Err == nil {
				for _, f := range cached.Fields {
					if f.Key == "title" && strings.TrimSpace(f.Value) != "" {
						label = f.Value
						break
					}
				}
			}
			b := ui.ButtonBase(c).Key("document-title").Grow(1).MinWidth(0).Height(42).
				Padding(0, 4).Justify(ui.Start).AlignItems(ui.Center).Radius(5).Label("编辑文档标题").
				OnClick(func() {
					a.propertyTitleDraft = label
					a.propertyTitleEditing, a.propertyTitleFocused = true, false
				})
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() {
				ui.Text(c, label).FontSize(25).FontWeight(650).TextColor(t.Text).SingleLine().Ellipsis("…")
				if b.Hovered() {
					ui.Icon(c, iconEdit).Size(14, 14).TextColor(t.TextMuted)
				}
			})
		})
	})
}

func (a *App) commitDocumentTitle() {
	title := strings.TrimSpace(a.propertyTitleDraft)
	var err error
	if title == "" {
		err = a.propertyDelete("title")
	} else {
		err = a.propertySet("title", title)
	}
	if err != nil {
		a.propertyError = err.Error()
		return
	}
	a.propertyError = ""
	a.propertyTitleEditing = false
	a.invalidate()
}

func (a *App) propertySummary(c *ui.Context) {
	d := a.propertyDocument()
	if d.Err != nil {
		return
	}
	fields := a.orderedProperties(d.Fields)
	shown := 0
	visible := make([]frontmatter.Field, 0, len(fields))
	for _, f := range fields {
		if f.Key != "title" && !a.propertyPrefs.Fields[f.Key].Hidden {
			visible = append(visible, f)
		}
	}
	ui.Row(c).Height(30).Shrink(0).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "笔记").FontSize(12).TextColor(c.Theme().TextMuted)
		for _, f := range fields {
			pref := a.propertyPrefs.Fields[f.Key]
			if pref.Hidden || f.Key == "title" || shown >= 4 {
				continue
			}
			ui.Text(c, "·").FontSize(12).TextColor(c.Theme().TextMuted)
			shown++
			a.propertySummaryItem(c, f)
		}
		if extra := len(visible) - shown; extra > 0 {
			a.propertyButton(c, fmt.Sprintf("+%d 项", extra), a.openProperties)
		}
	})
}

func (a *App) propertySummaryItem(c *ui.Context, f frontmatter.Field) {
	if !f.Editable {
		t := c.Theme()
		ui.Text(c, displayPropertyValue(f)).FontSize(11).TextColor(t.TextMuted).Padding(0, 7).SingleLine().Ellipsis("…")
		return
	}
	if a.propertyEditKey == f.Key && a.propertyEditLocation == "summary" {
		a.propertyInlineEditor(c, f, "summary")
		return
	}
	t := c.Theme()
	b := ui.ButtonBase(c).Height(28).MinWidth(0).MaxWidth(230).Padding(0, 1).Radius(5).Label("编辑属性 " + f.Key)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() { a.startPropertyEdit(f, "summary") })
	b.Children(func() {
		val := displayPropertyValue(f)
		switch {
		case f.Key == "tags":
			vals := strings.Split(val, ",")
			ui.Row(c).Height(28).Gap(4).AlignItems(ui.Center).Children(func() {
				for i, item := range vals {
					if i >= 3 {
						break
					}
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					bg, fg := propertyTagColors(item, t.Dark)
					ui.Text(c, item).MaxWidth(112).FontSize(12).TextColor(fg).Padding(4, 9).Radius(14).Background(bg).SingleLine().Ellipsis("…")
				}
				if len(vals) > 3 {
					ui.Text(c, "…").FontSize(12).TextColor(t.TextMuted)
				}
			})

		default:
			ui.Text(c, val).MaxWidth(160).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		}
	})
}

func displayPropertyValue(f frontmatter.Field) string {
	v := strings.TrimSpace(f.Value)
	if strings.EqualFold(f.Key, "status") {
		switch strings.ToLower(v) {
		case "draft":
			return "草稿"
		case "done":
			return "已完成"
		case "in_progress":
			return "进行中"
		}
	}
	if f.Kind == "date" || isNaturalDateProperty(f.Key) {
		return naturalPropertyDate(v)
	}
	return v
}

func isNaturalDateProperty(key string) bool {
	switch strings.ToLower(key) {
	case "date", "created", "updated", "modified":
		return true
	default:
		return false
	}
}

func (a *App) startPropertyEdit(f frontmatter.Field, location string) {
	a.propertyEditKey, a.propertyEditLocation, a.propertyEditDraft = f.Key, location, f.Value
}

func (a *App) propertyInlineEditor(c *ui.Context, f frontmatter.Field, location string) {
	if f.Kind == "bool" {
		ui.Row(c).Gap(4).Children(func() {
			a.propertyButton(c, "true", func() { a.propertySaveField(f.Key, "true") })
			a.propertyButton(c, "false", func() { a.propertySaveField(f.Key, "false") })
			a.propertyButton(c, "取消", func() { a.propertyEditKey, a.propertyEditLocation = "", "" })
		})
		return
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		input := ui.TextInput(c.Key("property-inline-"+location+"-"+f.Key), &a.propertyEditDraft).
			Width(180).Placeholder("list 可逗号分隔").Label(f.Key).AutoFocus().OnSubmit(func() { a.propertySaveField(f.Key, a.propertyEditDraft) })
		_ = input
		a.propertyButton(c, "保存", func() { a.propertySaveField(f.Key, a.propertyEditDraft) })
		a.propertyButton(c, "取消", func() { a.propertyEditKey, a.propertyEditLocation = "", "" })
	})
}

func naturalPropertyDate(value string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if d, err := time.Parse(layout, value); err == nil {
			return d.Local().Format("2006年1月2日")
		}
	}
	return value
}

func (a *App) openProperties() {
	a.propertiesOpen = true
	a.propertySourceMode = false
	a.propertyError = ""
	a.refreshSystemProperties()
	a.invalidate()
}

func (a *App) refreshSystemProperties() {
	chars, _, _ := countText(a.source)
	body := a.source
	if d := a.propertyDocument(); d.Err == nil && d.End > 0 && d.End <= len(body) {
		body = body[d.End:]
	}
	props := []systemProperty{
		{key: "文件名", value: filepath.Base(a.path)},
		{key: "路径", value: a.path},
		{key: "字数", value: fmt.Sprintf("%d 字 · %d 词", chars, len(strings.Fields(body)))},
	}
	if st, err := os.Stat(a.path); err == nil {
		props = append(props,
			systemProperty{key: "文件大小", value: formatPropertySize(st.Size())},
			systemProperty{key: "文件修改时间", value: st.ModTime().Format("2006-01-02 15:04")},
		)
	}
	a.propertySystem = props
}

func formatPropertySize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func (a *App) propertiesSidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Key("properties-sidebar").Width(propertiesSidebarWidth).Shrink(0).FillHeight().
		Border(1, t.Border).Background(sidebarColor(t)).Children(func() {
		ui.Row(c).Height(52).Shrink(0).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "属性").FontSize(15).FontWeight(600)
			ui.Spacer(c)
			if a.propertySourceMode {
				a.propertyButton(c, "返回属性", func() { a.propertySourceMode = false; a.invalidate() })
			} else {
				a.propertyButton(c, "YAML 源码", func() { a.propertySourceMode = true; a.propertySourceDraft = a.yamlPropertySource(); a.invalidate() })
			}
			a.propertyButton(c, "关闭", func() { a.propertiesOpen = false; a.invalidate() })
		})
		if a.propertySourceMode {
			a.propertySourceEditor(c)
			return
		}
		if a.propertyError != "" {
			ui.Text(c, a.propertyError).FillWidth().Padding(8, 12).TextColor(ui.Hex("#c34242")).FontSize(12)
		}
		showScroll := a.propertyIndicator.Visible(c.Now(), a.propertyScroll.X, a.propertyScroll.Y)
		if showScroll {
			c.After(a.propertyIndicator.Remaining(c.Now()))
		}
		ui.Scroll(c).Fill().TrackScroll(&a.propertyScroll).Scrollbars(showScroll).Children(func() {
			ui.Column(c).FillWidth().Padding(12).Gap(10).Children(func() {
				d := a.propertyDocument()
				if d.Err != nil {
					ui.Text(c, "YAML frontmatter 无法解析").FontWeight(600)
					ui.Text(c, d.Err.Error()).FillWidth().TextColor(t.TextMuted).FontSize(11)
					return
				}
				if len(d.Fields) == 0 {
					ui.Text(c, "还没有属性").TextColor(t.TextMuted)
				}
				for _, f := range a.orderedProperties(d.Fields) {
					a.propertyFieldRow(c, f)
				}
				a.propertyAddForm(c)
				ui.Box(c).FillWidth().Height(1).Background(t.Border)
				ui.Text(c, "系统信息").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
				for _, p := range a.propertySystem {
					ui.Column(c).FillWidth().Gap(3).Padding(5, 0).Children(func() {
						ui.Text(c, p.key+" · 只读").FontSize(10).TextColor(t.TextMuted)
						ui.Text(c, p.value).FillWidth().FontSize(12).TextColor(t.TextMuted)
					})
				}
			})
		})
	})
}

func (a *App) propertySourceEditor(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).Padding(12).Gap(8).Children(func() {
		ui.Text(c, "仅编辑 YAML frontmatter；正文会原样保留。").FillWidth().FontSize(11).TextColor(t.TextMuted)
		ui.TextArea(c.Key("properties-yaml-source"), &a.propertySourceDraft).Grow(1).MinHeight(120).FillWidth().Font("monospace").Label("YAML 源码")
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			a.propertyButton(c, "应用源码", func() {
				if err := a.applyPropertyYAMLSource(a.propertySourceDraft); err != nil {
					a.propertyError = err.Error()
					return
				}
				a.propertyError = ""
				a.refreshSystemProperties()
				a.invalidate()
			})
		})
	})
}

func (a *App) yamlPropertySource() string {
	d := a.propertyDocument()
	if d.End > 0 {
		return a.source[:d.End]
	}
	return "---\n---\n"
}

func (a *App) applyPropertyYAMLSource(draft string) error {
	end := a.propertyDocument().End
	if strings.HasPrefix(a.source, "---") && end == 0 {
		return fmt.Errorf("frontmatter 缺少结束分隔符；无法安全判断正文边界")
	}
	d := frontmatter.Parse(draft)
	if d.Err != nil || d.End == 0 {
		return fmt.Errorf("请输入完整有效的 YAML frontmatter")
	}
	if strings.TrimSpace(draft[d.End:]) != "" {
		return fmt.Errorf("源码框只接受 YAML frontmatter，不接受正文")
	}
	prefix := draft[:d.End]
	if !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if end == 0 && !strings.HasSuffix(prefix, "\n\n") {
		prefix = strings.TrimRight(prefix, "\r\n") + "\n\n"
	}
	a.applyPropertySource(prefix + a.source[end:])
	return nil
}

func (a *App) propertyFieldRow(c *ui.Context, f frontmatter.Field) {
	t := c.Theme()
	editing := a.propertyEditKey == f.Key && a.propertyEditLocation == "sidebar"
	pref := a.propertyPrefs.Fields[f.Key]
	ui.Column(c).FillWidth().Gap(3).Padding(5, 7).Radius(8).Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, f.Key).Grow(1).MinWidth(0).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			ui.Text(c, propertyTypeName(f.Kind)).FontSize(9).TextColor(t.TextMuted)
			if f.Editable {
				pinLabel, hideLabel := "置顶", "隐藏"
				if pref.Pinned {
					pinLabel = "取消置顶"
				}
				if pref.Hidden {
					hideLabel = "显示"
				}
				a.propertyButton(c, pinLabel, func() { a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Pinned = !p.Pinned }) })
				a.propertyButton(c, hideLabel, func() { a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Hidden = !p.Hidden }) })
				a.propertyButton(c, "上移", func() { a.moveProperty(f.Key, -1) })
				a.propertyButton(c, "下移", func() { a.moveProperty(f.Key, 1) })
			}
		})
		if editing {
			a.propertyInlineEditor(c, f, "sidebar")
			return
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			if f.Kind == "bool" && f.Editable {
				a.propertyButton(c, f.Value, func() {
					if f.Value == "true" {
						a.propertySet(f.Key, "false")
					} else {
						a.propertySet(f.Key, "true")
					}
				})
			} else if f.Editable {
				value := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(26).Padding(0, 3).Justify(ui.Start).Label("编辑属性 " + f.Key)
				value.OnClick(func() { a.startPropertyEdit(f, "sidebar") })
				value.Children(func() { a.propertyFieldValue(c, f, t) })
			} else {
				a.propertyFieldValue(c, f, t)
			}
			if f.Editable {
				a.propertyButton(c, "删除", func() {
					if err := a.propertyDelete(f.Key); err != nil {
						a.propertyError = err.Error()
					}
				})
			}
		})
	})
}

func (a *App) propertyFieldValue(c *ui.Context, f frontmatter.Field, t *ui.Theme) {
	val := displayPropertyValue(f)
	switch {
	case f.Key == "tags" || f.Key == "aliases" || f.Kind == "list":
		ui.Text(c, val).FillWidth().FontSize(11).Padding(2, 5).Radius(8).Background(t.Background).SingleLine().Ellipsis("…")
	case strings.EqualFold(f.Key, "status"):
		ui.Text(c, val).FontSize(11).TextColor(t.Accent).Padding(3, 8).Radius(9).Background(t.Accent.Alpha(0.12)).SingleLine().Ellipsis("…")
	default:
		ui.Text(c, val).Grow(1).MinWidth(0).FontSize(12).TextColor(t.Text).SingleLine().Ellipsis("…")
	}
}

func (a *App) propertyAddForm(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(6).Padding(8, 0).Children(func() {
		ui.Text(c, "添加属性").FontSize(12).FontWeight(600)
		ui.TextInput(c, &a.propertyAddKey).FillWidth().Placeholder("属性名").Label("属性名")
		ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			kind := a.propertyAddKind
			if kind == "" {
				kind = "text"
			}
			k := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(6).Background(t.Surface).Label("类型：" + propertyTypeName(kind))
			k.Menu(func(m *ui.Menu) {
				for _, typ := range []string{"text", "list", "bool", "number", "date"} {
					if m.Item(propertyTypeName(typ)).Chosen() {
						a.propertyAddKind = typ
					}
				}
			})
			k.Children(func() { ui.Text(c, "类型："+propertyTypeName(kind)).FontSize(11) })
			ui.Spacer(c)
		})
		kind := a.propertyAddKind
		if kind == "" {
			kind = "text"
		}
		if kind == "bool" {
			ui.Row(c).Gap(6).Children(func() {
				a.propertyButton(c, "添加 false", func() { a.propertyAdd("false") })
				a.propertyButton(c, "添加 true", func() { a.propertyAdd("true") })
			})
		} else {
			placeholder := "值"
			if kind == "list" {
				placeholder = "值，逗号分隔"
			}
			if kind == "date" {
				placeholder = "日期 YYYY-MM-DD"
			}
			ui.TextInput(c, &a.propertyAddValue).FillWidth().Placeholder(placeholder).Label("属性值").OnSubmit(func() { a.propertyAdd(a.propertyAddValue) })
			a.propertyButton(c, "添加", func() { a.propertyAdd(a.propertyAddValue) })
		}
	})
}

func propertyTypeName(kind string) string {
	switch kind {
	case "list":
		return "列表"
	case "bool":
		return "布尔"
	case "number":
		return "数字"
	case "date":
		return "日期"
	case "structure":
		return "复杂结构"
	default:
		return "文本"
	}
}

func (a *App) propertyAdd(value string) {
	key := strings.TrimSpace(a.propertyAddKey)
	kind := a.propertyAddKind
	if kind == "" {
		kind = "text"
	}
	if strings.EqualFold(key, "tags") || strings.EqualFold(key, "aliases") {
		kind = "list"
	}
	next, err := frontmatter.SetTyped(a.source, key, value, kind)
	if err != nil {
		a.propertyError = err.Error()
		a.invalidate()
		return
	}
	a.applyPropertySource(next)
	a.propertyAddKey, a.propertyAddValue, a.propertyError = "", "", ""
	a.refreshSystemProperties()
}

func (a *App) propertySet(key, value string) error {
	if key == "title" && strings.TrimSpace(value) == "" {
		return a.propertyDelete(key)
	}
	next, err := frontmatter.Set(a.source, key, value)
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	a.refreshSystemProperties()
	return nil
}

func (a *App) propertyDelete(key string) error {
	next, err := frontmatter.Delete(a.source, key)
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	a.refreshSystemProperties()
	return nil
}

func (a *App) applyPropertySource(next string) {
	if next != a.source {
		a.ed.ReplaceAll(next)
	}
	a.propertyCache = propertyDocumentCache{}
	a.invalidate()
}

func (a *App) propertySaveField(key, value string) {
	if err := a.propertySet(key, value); err != nil {
		a.propertyError = err.Error()
		return
	}
	a.propertyEditKey, a.propertyEditLocation, a.propertyError = "", "", ""
	a.invalidate()
}

func (a *App) propertyButton(c *ui.Context, label string, fn func()) {
	b := ui.ButtonBase(c).Height(25).Padding(0, 7).Radius(5).Label(label).OnClick(fn)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted) })
}

func (a *App) orderedProperties(fields []frontmatter.Field) []frontmatter.Field {
	out := append([]frontmatter.Field(nil), fields...)
	rank := func(key string) int {
		switch strings.ToLower(key) {
		case "tags":
			return 0
		case "status":
			return 1
		case "date":
			return 2
		case "aliases":
			return 3
		default:
			return 10
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := a.propertyPrefs.Fields[out[i].Key], a.propertyPrefs.Fields[out[j].Key]
		if pi.Pinned != pj.Pinned {
			return pi.Pinned
		}
		if pi.Order > 0 || pj.Order > 0 {
			oi, oj := pi.Order, pj.Order
			if oi == 0 {
				oi = int(^uint(0) >> 1)
			}
			if oj == 0 {
				oj = int(^uint(0) >> 1)
			}
			if oi != oj {
				return oi < oj
			}
		}
		if rank(out[i].Key) != rank(out[j].Key) {
			return rank(out[i].Key) < rank(out[j].Key)
		}
		return false
	})
	return out
}

func (a *App) moveProperty(key string, delta int) {
	d := a.propertyDocument()
	fields := a.orderedProperties(d.Fields)
	index := -1
	for i, f := range fields {
		if f.Key == key {
			index = i
			break
		}
	}
	to := index + delta
	if index < 0 || to < 0 || to >= len(fields) {
		return
	}
	if a.propertyPrefs.Fields[fields[index].Key].Pinned != a.propertyPrefs.Fields[fields[to].Key].Pinned {
		return
	}
	fields[index], fields[to] = fields[to], fields[index]
	if a.propertyPrefs.Fields == nil {
		a.propertyPrefs.Fields = map[string]fieldDisplayPreference{}
	}
	for i, f := range fields {
		p := a.propertyPrefs.Fields[f.Key]
		if p.Pinned == a.propertyPrefs.Fields[key].Pinned {
			p.Order = i + 1
		}
		a.propertyPrefs.Fields[f.Key] = p
	}
	if err := saveDocumentPropertyPrefs(a.path, a.propertyPrefs); err != nil {
		a.propertyError = err.Error()
	}
	a.invalidate()
}

func (a *App) updatePropertyPreference(key string, change func(*fieldDisplayPreference)) {
	if a.propertyPrefs.Fields == nil {
		a.propertyPrefs.Fields = map[string]fieldDisplayPreference{}
	}
	p := a.propertyPrefs.Fields[key]
	change(&p)
	a.propertyPrefs.Fields[key] = p
	if err := saveDocumentPropertyPrefs(a.path, a.propertyPrefs); err != nil {
		a.propertyError = err.Error()
	}
	a.invalidate()
}

func documentPropertyPrefsPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "document-properties.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "document-properties.json")
}

func loadDocumentPropertyPrefs(path string) documentPropertyPrefs {
	var all propertyPrefsFile
	b, err := os.ReadFile(documentPropertyPrefsPath())
	if err == nil {
		_ = json.Unmarshal(b, &all)
	}
	if all == nil {
		return documentPropertyPrefs{}
	}
	return all[path]
}

func saveDocumentPropertyPrefs(path string, prefs documentPropertyPrefs) error {
	filename := documentPropertyPrefsPath()
	var all propertyPrefsFile
	if b, err := os.ReadFile(filename); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	if all == nil {
		all = propertyPrefsFile{}
	}
	all[path] = prefs
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, b, 0o600)
}

// Stable tag colors keep the same label recognizable across notes and themes.
func propertyTagColors(tag string, dark bool) (ui.Color, ui.Color) {
	light := [][2]string{{"#F0E7B1", "#665713"}, {"#F7DCE8", "#862D54"}, {"#D8EEE7", "#285F52"}, {"#DEE8F6", "#315A89"}, {"#EBDFF2", "#68447C"}}
	night := [][2]string{{"#665D14", "#EEE19B"}, {"#540A2D", "#F0ABC8"}, {"#194C49", "#A4D9CF"}, {"#213C62", "#B5CDF1"}, {"#463255", "#DCC0EE"}}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tag))
	p := light[h.Sum32()%uint32(len(light))]
	if dark {
		p = night[h.Sum32()%uint32(len(night))]
	}
	return ui.Hex(p[0]), ui.Hex(p[1])
}
