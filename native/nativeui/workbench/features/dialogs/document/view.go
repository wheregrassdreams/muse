// Package documentdialogs renders the document recovery, history, and
// external-change dialogs and emits typed user actions.
package documentdialogs

import (
	"github.com/egoist/mygo/ui"
	"muse/document"
	"muse/locales"
	scrollui "muse/nativeui/workbench/framework/components/scroll"
	"muse/ui/scrollbar"
)

type state struct {
	conflictOpen  bool
	recoveryOpen  bool
	drafts        []document.Draft
	historyOpen   bool
	historyPath   string
	history       []document.HistoryEntry
	migrationOpen bool

	recoveryScroll, historyScroll       ui.ScrollState
	recoveryIndicator, historyIndicator scrollbar.Indicator
}

// Snapshot exposes only the current document path needed by restore routing.
// Modal and list state remain private to this component.
type Snapshot struct {
	HistoryPath string
}

// Handle is the only stateful capability exposed to the composition root.
// Dialog lifecycle, lists, and toolkit scroll state remain private here.
type Handle struct{ state *state }

func NewHandle() *Handle { return &Handle{state: &state{}} }

func (h *Handle) ensureState() *state {
	if h == nil {
		return nil
	}
	if h.state == nil {
		h.state = &state{}
	}
	return h.state
}

type ActionKind uint8

const (
	TryMerge ActionKind = iota + 1
	KeepCopy
	ReloadDisk
	RestoreDraft
	RestoreHistory
	CloseConflict
	CloseRecovery
	CloseHistory
	ApplyCoreMigration
	CloseCoreMigration
	OpenConflict
	OpenCoreMigration
)

type Action struct {
	Kind ActionKind
	Key  string
	ID   string
}

// ExportFormat is a reusable user-facing document export choice.
type ExportFormat uint8

const (
	ExportMarkdown ExportFormat = iota + 1
	ExportHTML
	ExportPDF
)

// AddExportMenu contributes the common export submenu and emits the selected
// format. File dialogs, conversion and writing remain with the caller.
func AddExportMenu(menu *ui.Menu, dispatch func(ExportFormat)) {
	menu.Submenu(locales.DocumentExport(), func(export *ui.Menu) {
		if export.Item(locales.DocumentExportMarkdown()).Chosen() && dispatch != nil {
			dispatch(ExportMarkdown)
		}
		if export.Item(locales.DocumentExportHTML()).Chosen() && dispatch != nil {
			dispatch(ExportHTML)
		}
		if export.Item(locales.DocumentExportPDF()).Chosen() && dispatch != nil {
			dispatch(ExportPDF)
		}
	})
}

func (h *Handle) Snapshot() Snapshot {
	s := h.ensureState()
	if s == nil {
		return Snapshot{}
	}
	return Snapshot{
		HistoryPath: s.historyPath,
	}
}

func (h *Handle) ModalOpen() bool {
	s := h.ensureState()
	return s != nil && (s.conflictOpen || s.recoveryOpen || s.historyOpen || s.migrationOpen)
}

func (h *Handle) IsConflictOpen() bool {
	s := h.ensureState()
	return s != nil && s.conflictOpen
}

func (h *Handle) IsCoreMigrationOpen() bool {
	s := h.ensureState()
	return s != nil && s.migrationOpen
}

func (h *Handle) HistoryPath() string { return h.Snapshot().HistoryPath }

// HistoryEntries returns a detached copy for callers that need to map a
// selected history row to its restore workflow.
func (h *Handle) HistoryEntries() []document.HistoryEntry {
	s := h.ensureState()
	if s == nil {
		return nil
	}
	return append([]document.HistoryEntry(nil), s.history...)
}

func (h *Handle) OpenCoreMigration()  { h.Apply(Action{Kind: OpenCoreMigration}) }
func (h *Handle) CloseCoreMigration() { h.Apply(Action{Kind: CloseCoreMigration}) }

func (h *Handle) Apply(action Action) {
	s := h.ensureState()
	if s == nil {
		return
	}
	switch action.Kind {
	case OpenConflict:
		s.conflictOpen = true
	case CloseConflict:
		s.conflictOpen = false
	case OpenCoreMigration:
		s.migrationOpen = true
	case CloseRecovery, RestoreDraft:
		s.recoveryOpen = false
	case CloseHistory, RestoreHistory:
		s.historyOpen = false
	case CloseCoreMigration:
		s.migrationOpen = false
	}
}

func (h *Handle) OpenRecovery(drafts []document.Draft) {
	s := h.ensureState()
	if s == nil {
		return
	}
	s.drafts = append(s.drafts[:0], drafts...)
	s.recoveryOpen = true
}

func (h *Handle) OpenHistory(path string, entries []document.HistoryEntry) {
	s := h.ensureState()
	if s == nil {
		return
	}
	s.historyPath = path
	s.history = append(s.history[:0], entries...)
	s.historyOpen = true
}

// Close dismisses all dialogs and releases their document lists.
func (h *Handle) Close() {
	s := h.ensureState()
	if s == nil {
		return
	}
	s.conflictOpen = false
	s.recoveryOpen = false
	s.historyOpen = false
	s.migrationOpen = false
	s.drafts = nil
	s.history = nil
	s.historyPath = ""
}

func Render(c *ui.Context, handle *Handle, dispatch func(Action)) {
	state := handle.ensureState()
	if state == nil {
		return
	}
	renderConflict(c, state, dispatch)
	renderRecovery(c, state, dispatch)
	renderHistory(c, state, dispatch)
}

// MigrationModel contains display-ready progress and preview text. The
// application owns plan construction and mutation; this package owns only
// modal presentation and returns typed user intent.
type MigrationModel struct {
	Loading, Running, HasPlan, HasChanges bool
	Summary                               string
}

func RenderCoreMigration(c *ui.Context, handle *Handle, model MigrationModel, dispatch func(Action)) {
	state := handle.ensureState()
	if state == nil || !state.migrationOpen {
		return
	}
	open := state.migrationOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).MaxHeight(640).Padding(20).Gap(12).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.CoreMigrationPreview()).FontWeight(700)
			switch {
			case model.Loading:
				ui.Text(c, locales.CoreMigrationIndexing()).Wrap()
			case model.Running:
				ui.Text(c, locales.CoreMigrationApplying()).Wrap()
			case !model.HasPlan:
				ui.Text(c, locales.CoreMigrationNoPreview()).Wrap()
			default:
				ui.Text(c, model.Summary).Wrap()
			}
			ui.Row(c).Gap(8).Children(func() {
				if !model.Loading && !model.Running && model.HasPlan && model.HasChanges {
					if ui.Button(c, locales.CoreMigrationApply()).Clicked() {
						dispatch(Action{Kind: ApplyCoreMigration})
					}
				} else if !model.Loading && !model.Running && !model.HasChanges {
					ui.Text(c, locales.CoreMigrationNoChanges()).TextColor(c.Theme().TextMuted)
				}
				if ui.Button(c, locales.CommonClose()).Clicked() {
					dispatch(Action{Kind: CloseCoreMigration})
				}
			})
		})
	})
	if !open {
		dispatch(Action{Kind: CloseCoreMigration})
	}
}

func renderConflict(c *ui.Context, state *state, dispatch func(Action)) {
	if !state.conflictOpen {
		return
	}
	open := state.conflictOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Padding(20).Gap(12).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.DocumentExternalChanged()).FontWeight(700)
			ui.Text(c, locales.DocumentExternalChangedDetail()).Wrap()
			if ui.Button(c, locales.CommonTryMerge()).Clicked() {
				dispatch(Action{Kind: TryMerge})
			}
			if ui.Button(c, locales.CommonKeepCopy()).Clicked() {
				dispatch(Action{Kind: KeepCopy})
			}
			if ui.Button(c, locales.CommonReloadDisk()).Clicked() {
				dispatch(Action{Kind: ReloadDisk})
			}
		})
	})
	if !open {
		dispatch(Action{Kind: CloseConflict})
	}
}

func renderRecovery(c *ui.Context, state *state, dispatch func(Action)) {
	if !state.recoveryOpen {
		return
	}
	open := state.recoveryOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.DocumentRecoveryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentRecoveryDetail()).Wrap()
			if len(state.drafts) == 0 {
				ui.Text(c, locales.DocumentRecoveryEmpty())
			}
				scrollui.QuietScroll(c, &state.recoveryScroll, &state.recoveryIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, draft := range state.drafts {
						key := draft.Key
						if ui.Button(c, key).Clicked() {
							dispatch(Action{Kind: RestoreDraft, Key: key})
						}
					}
				})
			})
			if ui.Button(c, locales.CommonClose()).Clicked() {
				dispatch(Action{Kind: CloseRecovery})
			}
		})
	})
	if !open {
		dispatch(Action{Kind: CloseRecovery})
	}
}

func renderHistory(c *ui.Context, state *state, dispatch func(Action)) {
	if !state.historyOpen {
		return
	}
	open := state.historyOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.DocumentHistoryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentHistoryDetail()).Wrap()
			if len(state.history) == 0 {
				ui.Text(c, locales.DocumentHistoryEmpty())
			}
				scrollui.QuietScroll(c, &state.historyScroll, &state.historyIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, entry := range state.history {
						id := entry.ID
						label := locales.DocumentHistoryVersionLabel(entry.CreatedAt.Local(), shortRevision(string(entry.Revision)))
						if ui.Button(c, label).Clicked() {
							dispatch(Action{Kind: RestoreHistory, ID: id})
						}
					}
				})
			})
			if ui.Button(c, locales.CommonClose()).Clicked() {
				dispatch(Action{Kind: CloseHistory})
			}
		})
	})
	if !open {
		dispatch(Action{Kind: CloseHistory})
	}
}

func shortRevision(revision string) string {
	if len(revision) > 8 {
		return revision[:8]
	}
	return revision
}
