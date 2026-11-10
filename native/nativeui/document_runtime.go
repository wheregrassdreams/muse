package nativeui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/notestore"
	"muse/scrollui"
)

const autosaveDelay = 750 * time.Millisecond

type saveResult struct {
	path, source, revision string
	err                    error
	generation             uint64
}
type documentRuntime struct {
	recoveryScroll, historyScroll       ui.ScrollState
	recoveryIndicator, historyIndicator scrollui.Indicator
	saveMu                              sync.Mutex
	token                               atomic.Uint64
	store                               *notestore.Store
	generation                          uint64
	revision                            string
	due                                 time.Time
	saving                              bool
	results                             chan saveResult
	draftKey                            string
	conflict                            string
	conflictOpen                        bool
	recoveryOpen                        bool
	drafts                              []notestore.Draft
	historyOpen                         bool
	historyPath                         string
	history                             []notestore.HistoryEntry
}

func (a *App) enableDocumentServices() {
	if a.documents != nil {
		return
	}
	a.documents = &documentRuntime{store: notestore.New(filepath.Join(filepath.Dir(nativePreferencesPath()), "recovery")), results: make(chan saveResult, 1), draftKey: fmt.Sprintf("untitled-%d", time.Now().UnixNano())}
	if a.path != "" {
		a.documents.revision = notestore.Revision(a.diskSource())
	}
}
func (a *App) documentLoaded(raw string) {
	if r := a.documents; r != nil {
		if a.path == "" {
			r.draftKey = fmt.Sprintf("untitled-%d", time.Now().UnixNano())
		}
		r.generation = r.token.Add(1)
		r.saveMu.Lock()
		r.saveMu.Unlock()
		r.conflictOpen = false
		r.revision = notestore.Revision(raw)
		r.due = time.Time{}
		r.conflict = ""
	}
}
func (a *App) documentChanged() {
	if r := a.documents; r != nil {
		r.due = time.Now().Add(autosaveDelay)
	}
	if v := a.vault; v != nil && a.workspaceSearch {
		v.queryToken++
		if v.queryCancel != nil {
			v.queryCancel()
			v.queryCancel = nil
		}
		if v.query != "" {
			v.due = time.Now().Add(120 * time.Millisecond)
		}
	}
}
func (a *App) documentTick(c *ui.Context) {
	r := a.documents
	if r == nil {
		return
	}
	c.After(100 * time.Millisecond)
	select {
	case done := <-r.results:
		r.saving = false
		if done.path == a.path && done.generation == r.generation {
			if done.err != nil {
				if errors.Is(done.err, notestore.ErrConflict) {
					r.conflict = done.path
					r.conflictOpen = true
					a.notice("磁盘文件已变化，自动保存已暂停")
				} else {
					a.notice("自动保存失败：" + done.err.Error())
				}
			} else if done.path != "" && done.revision != "" {
				r.revision = done.revision
				if done.source == a.diskSource() {
					a.dirty = false
					a.updateTitle()
				}
			}
		}
	default:
	}
	if r.saving || r.due.IsZero() || time.Now().Before(r.due) || !a.dirty {
		return
	}
	r.due = time.Time{}
	path, source, revision := a.path, a.diskSource(), r.revision
	key := path
	if key == "" {
		key = r.draftKey
	}
	generation := r.generation
	blocked := r.conflict != ""
	r.saving = true
	go func() {
		err := r.store.WriteDraft(key, path, source)
		var next string
		if err == nil && path != "" && !blocked {
			r.saveMu.Lock()
			if r.token.Load() == generation {
				next, err = r.store.Save(path, source, revision)
				if err == nil {
					_ = r.store.DeleteDraft(key)
				}
			}
			r.saveMu.Unlock()
		}
		r.results <- saveResult{path: path, source: source, revision: next, err: err, generation: generation}
		a.invalidate()
	}()
}
func (a *App) writeDocument(path, source, revision string) error {
	if a.documents == nil {
		return writeFileAtomic(path, []byte(source))
	}
	if err := a.documents.store.WriteDraft(path, path, source); err != nil {
		return err
	}
	a.documents.saveMu.Lock()
	defer a.documents.saveMu.Unlock()
	_, err := a.documents.store.Save(path, source, revision)
	if err == nil {
		_ = a.documents.store.DeleteDraft(path)
	}
	return err
}
func (a *App) openRecovery() {
	if a.documents == nil {
		a.enableDocumentServices()
	}
	drafts, err := a.documents.store.ListDrafts()
	if err != nil {
		a.notice("读取恢复草稿失败：" + err.Error())
		return
	}
	a.documents.drafts = drafts
	a.documents.recoveryOpen = true
}
func (a *App) documentDialogs(c *ui.Context) {
	r := a.documents
	if r == nil {
		return
	}
	ui.Modal(c, &r.conflictOpen, func() {
		ui.Column(c).Width(440).Padding(20).Gap(12).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, "文件在外部发生了变化").FontWeight(700)
			ui.Text(c, "你的编辑已保留为恢复草稿。选择重新加载磁盘内容，或将当前编辑保留为未命名副本。").Wrap()
			if ui.Button(c, "保留为未命名副本").Clicked() {
				if !a.preserveCurrentDraft() {
					return
				}
				src := a.diskSource()
				a.path = ""
				a.loadDoc(src)
				a.dirty = true
				a.documentChanged()
				r.conflict = ""
				r.conflictOpen = false
				a.updateTitle()
			}
			if ui.Button(c, "重新加载磁盘内容").Clicked() {
				data, err := os.ReadFile(a.path)
				if err != nil {
					a.notice(err.Error())
				} else if !isTextData(data) {
					a.notice("外部文件不是文本，无法加载")
				} else {
					if !a.preserveCurrentDraft() {
						return
					}
					a.loadDoc(string(data))
					a.dirty = false
					r.conflict = ""
					r.conflictOpen = false
					a.updateTitle()
				}
			}
		})
	})
	ui.Modal(c, &r.recoveryOpen, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, "恢复草稿").FontWeight(700)
			ui.Text(c, "恢复会打开为未命名副本，不覆盖原文件。").Wrap()
			if len(r.drafts) == 0 {
				ui.Text(c, "暂无恢复草稿")
			}
			quietScroll(c, &r.recoveryScroll, &r.recoveryIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, draft := range r.drafts {
						d := draft
						if ui.Button(c, d.Key).Clicked() {
							if !a.preserveCurrentDraft() {
								return
							}
							a.path = ""
							a.loadDoc(d.Text)
							a.dirty = true
							a.documentChanged()
							r.recoveryOpen = false
							a.updateTitle()
						}
					}
				})
			})
			if ui.Button(c, "关闭").Clicked() {
				r.recoveryOpen = false
			}
		})
	})
}

// Saving to a different destination follows the user's Save As confirmation,
// but still compares its current revision before replacement.
func (a *App) destinationRevision(path, original, revision string) (string, error) {
	if a.documents == nil || path == original {
		return revision, nil
	}
	d, err := a.documents.store.Read(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	return d.Revision, err
}
func (a *App) preserveCurrentDraft() bool {
	if !a.dirty || a.documents == nil {
		return true
	}
	key := a.path
	if key == "" {
		key = a.documents.draftKey
	}
	if err := a.documents.store.WriteDraft(key, a.path, a.diskSource()); err != nil {
		a.notice("保留草稿失败：" + err.Error())
		return false
	}
	return true
}

func (a *App) openHistory() {
	if a.path == "" {
		a.notice("当前文档尚未保存")
		return
	}
	if a.documents == nil {
		a.enableDocumentServices()
	}
	entries, err := a.documents.store.ListHistory(a.path)
	if err != nil {
		a.notice("读取历史失败：" + err.Error())
		return
	}
	a.documents.historyPath = a.path
	a.documents.history = entries
	a.documents.historyOpen = true
}
func (a *App) historyDialog(c *ui.Context) {
	r := a.documents
	if r == nil {
		return
	}
	ui.Modal(c, &r.historyOpen, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, "版本历史").FontWeight(700)
			ui.Text(c, "选择版本打开为未命名副本，原文件不变。").Wrap()
			if len(r.history) == 0 {
				ui.Text(c, "暂无历史版本")
			}
			quietScroll(c, &r.historyScroll, &r.historyIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, entry := range r.history {
						h := entry
						if ui.Button(c, h.CreatedAt.Local().Format("2006-01-02 15:04:05")+" · "+h.Revision[:8]).Clicked() {
							doc, err := r.store.ReadHistory(r.historyPath, h.ID)
							if err != nil {
								a.notice(err.Error())
								return
							}
							if !a.preserveCurrentDraft() {
								return
							}
							a.path = ""
							a.loadDoc(doc.Text)
							a.dirty = true
							a.documentChanged()
							r.historyOpen = false
							a.updateTitle()
						}
					}
				})
			})
			if ui.Button(c, "关闭").Clicked() {
				r.historyOpen = false
			}
		})
	})
}
