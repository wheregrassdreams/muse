package nativeui

import (
	"errors"
	"fmt"
	"muse/locales"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/frontmatter"
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
	baseSource                          string
	due                                 time.Time
	updatedAtDue                        time.Time
	applyingMetadata                    bool
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
		a.documents.baseSource = a.diskSource()
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
		r.baseSource = raw
		r.due = time.Time{}
		r.updatedAtDue = time.Time{}
		r.conflict = ""
	}
}
func (a *App) documentChanged() {
	if r := a.documents; r != nil {
		now := time.Now()
		r.due = now.Add(autosaveDelay)
		if !r.applyingMetadata {
			r.updatedAtDue = now.Add(time.Second)
		}
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
					a.notice(locales.DocumentExternalChangedAutosavePaused())
				} else {
					a.notice(locales.CommonAutosaveError(done.err.Error()))
				}
			} else if done.path != "" && done.revision != "" {
				r.revision = done.revision
				r.baseSource = done.source
				if done.source == a.diskSource() {
					a.dirty = false
					a.updateTitle()
				}
			}
		}
	default:
	}
	if !r.updatedAtDue.IsZero() && !time.Now().Before(r.updatedAtDue) {
		r.updatedAtDue = time.Time{}
		a.updateManagedTimestamp()
	}
	if r.saving || r.due.IsZero() || time.Now().Before(r.due) || !a.dirty {
		return
	}
	if !r.updatedAtDue.IsZero() {
		r.updatedAtDue = time.Time{}
		a.updateManagedTimestamp()
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

func (a *App) flushManagedTimestamp() {
	if a.documents == nil || a.documents.updatedAtDue.IsZero() {
		return
	}
	a.documents.updatedAtDue = time.Time{}
	a.updateManagedTimestamp()
}

func (a *App) updateManagedTimestamp() {
	if a.ed == nil || a.documents == nil || codeLanguage(a.path) != "" {
		return
	}
	source := a.ed.Source()
	next, err := frontmatter.EnsureCore(source)
	if err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
		return
	}
	next, err = frontmatter.SetTyped(next, "updated_at", time.Now().UTC().Format(time.RFC3339Nano), "text")
	if err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
		return
	}
	a.documents.applyingMetadata = true
	a.ed.ReplaceSystemAll(next)
	a.documents.applyingMetadata = false
}

func ensureCreatedAt(source string, fallback time.Time) (string, string, error) {
	core, err := frontmatter.EnsureCore(source)
	if err != nil {
		return source, "", err
	}
	for _, field := range frontmatter.Parse(core).Fields {
		if field.Key == "created_at" && strings.TrimSpace(field.Value) != "" {
			return core, field.Value, nil
		}
	}
	stamp := fallback.UTC().Format(time.RFC3339Nano)
	updated, err := frontmatter.SetTyped(core, "created_at", stamp, "text")
	return updated, stamp, err
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
		a.notice(locales.CommonRecoveryReadError(err.Error()))
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
			ui.Text(c, locales.DocumentExternalChanged()).FontWeight(700)
			ui.Text(c, locales.DocumentExternalChangedDetail()).Wrap()
			if ui.Button(c, locales.CommonTryMerge()).Clicked() {
				a.tryMergeExternalChanges()
			}
			if ui.Button(c, locales.CommonKeepCopy()).Clicked() {
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
			if ui.Button(c, locales.CommonReloadDisk()).Clicked() {
				data, err := os.ReadFile(a.path)
				if err != nil {
					a.notice(err.Error())
				} else if !isTextData(data) {
					a.notice(locales.DocumentExternalNotText())
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
			ui.Text(c, locales.DocumentRecoveryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentRecoveryDetail()).Wrap()
			if len(r.drafts) == 0 {
				ui.Text(c, locales.DocumentRecoveryEmpty())
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
			if ui.Button(c, locales.CommonClose()).Clicked() {
				r.recoveryOpen = false
			}
		})
	})
}

func (a *App) tryMergeExternalChanges() bool {
	r := a.documents
	if r == nil || a.path == "" {
		return false
	}
	external, err := os.ReadFile(a.path)
	if err != nil || !isTextData(external) {
		a.notice(locales.DocumentExternalReadFailed())
		return false
	}
	externalSource := string(external)
	merged, ok := mergeDisjointDocumentEdits(r.baseSource, a.diskSource(), externalSource)
	if !ok {
		a.notice(locales.DocumentExternalMergeConflict())
		return false
	}
	r.baseSource = externalSource
	r.revision = notestore.Revision(externalSource)
	a.crlf = strings.Contains(externalSource, "\r\n")
	a.ed.ReplaceAll(strings.ReplaceAll(merged, "\r\n", "\n"))
	a.dirty = a.diskSource() != externalSource
	r.conflict = ""
	r.conflictOpen = false
	if !a.dirty {
		r.due = time.Time{}
		r.updatedAtDue = time.Time{}
		a.updateManagedTimestamp()
		a.dirty = a.diskSource() != externalSource
		if !a.dirty {
			a.notice(locales.DocumentExternalLoaded())
			return true
		}
	}
	a.flushManagedTimestamp()
	if err := a.writeDocument(a.path, a.diskSource(), r.revision); err != nil {
		r.conflict, r.conflictOpen = a.path, true
		a.notice(locales.CommonMergedSaveError(err.Error()))
		return false
	}
	r.revision = notestore.Revision(a.diskSource())
	r.baseSource = a.diskSource()
	r.due = time.Time{}
	r.updatedAtDue = time.Time{}
	r.conflict = ""
	r.conflictOpen = false
	a.dirty = false
	_ = r.store.DeleteDraft(a.path)
	a.updateTitle()
	a.notice(locales.DocumentExternalMerged())
	return true
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
	a.flushManagedTimestamp()
	key := a.path
	if key == "" {
		key = a.documents.draftKey
	}
	if err := a.documents.store.WriteDraft(key, a.path, a.diskSource()); err != nil {
		a.notice(locales.CommonDraftPreserveError(err.Error()))
		return false
	}
	return true
}

func (a *App) openHistory() {
	if a.path == "" {
		a.notice(locales.DocumentUnsaved())
		return
	}
	if a.documents == nil {
		a.enableDocumentServices()
	}
	entries, err := a.documents.store.ListHistory(a.path)
	if err != nil {
		a.notice(locales.CommonHistoryReadError(err.Error()))
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
			ui.Text(c, locales.DocumentHistoryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentHistoryDetail()).Wrap()
			if len(r.history) == 0 {
				ui.Text(c, locales.DocumentHistoryEmpty())
			}
			quietScroll(c, &r.historyScroll, &r.historyIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, entry := range r.history {
						h := entry
						if ui.Button(c, locales.DocumentHistoryVersionLabel(h.CreatedAt.Local(), h.Revision[:8])).Clicked() {
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
			if ui.Button(c, locales.CommonClose()).Clicked() {
				r.historyOpen = false
			}
		})
	})
}
