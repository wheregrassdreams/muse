// Package documentdialogs renders the document recovery, history, and
// external-change dialogs and emits typed user actions.
package documentdialogs

import (
	"github.com/egoist/mygo/ui"
	"muse/document"
	"muse/locales"
	"muse/nativeui/components"
	"muse/ui/scrollbar"
)

type State struct {
	ConflictOpen bool
	RecoveryOpen bool
	Drafts       []document.Draft
	HistoryOpen  bool
	HistoryPath  string
	History      []document.HistoryEntry

	recoveryScroll, historyScroll       ui.ScrollState
	recoveryIndicator, historyIndicator scrollbar.Indicator
}

func New() *State { return &State{} }

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
)

type Action struct {
	Kind ActionKind
	Key  string
	ID   string
}

func (s *State) AnyOpen() bool {
	return s != nil && (s.ConflictOpen || s.RecoveryOpen || s.HistoryOpen)
}

func (s *State) OpenRecovery(drafts []document.Draft) {
	if s == nil {
		return
	}
	s.Drafts = append(s.Drafts[:0], drafts...)
	s.RecoveryOpen = true
}

func (s *State) OpenHistory(path string, entries []document.HistoryEntry) {
	if s == nil {
		return
	}
	s.HistoryPath = path
	s.History = append(s.History[:0], entries...)
	s.HistoryOpen = true
}

func (s *State) CloseAll() {
	if s == nil {
		return
	}
	s.ConflictOpen = false
	s.RecoveryOpen = false
	s.HistoryOpen = false
	s.Drafts = nil
	s.History = nil
	s.HistoryPath = ""
}

func Render(c *ui.Context, state *State, dispatch func(Action)) {
	if state == nil {
		return
	}
	renderConflict(c, state, dispatch)
	renderRecovery(c, state, dispatch)
	renderHistory(c, state, dispatch)
}

func renderConflict(c *ui.Context, state *State, dispatch func(Action)) {
	if !state.ConflictOpen {
		return
	}
	open := state.ConflictOpen
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

func renderRecovery(c *ui.Context, state *State, dispatch func(Action)) {
	if !state.RecoveryOpen {
		return
	}
	open := state.RecoveryOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.DocumentRecoveryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentRecoveryDetail()).Wrap()
			if len(state.Drafts) == 0 {
				ui.Text(c, locales.DocumentRecoveryEmpty())
			}
			components.QuietScroll(c, &state.recoveryScroll, &state.recoveryIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, draft := range state.Drafts {
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

func renderHistory(c *ui.Context, state *State, dispatch func(Action)) {
	if !state.HistoryOpen {
		return
	}
	open := state.HistoryOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.DocumentHistoryTitle()).FontWeight(700)
			ui.Text(c, locales.DocumentHistoryDetail()).Wrap()
			if len(state.History) == 0 {
				ui.Text(c, locales.DocumentHistoryEmpty())
			}
			components.QuietScroll(c, &state.historyScroll, &state.historyIndicator).Height(300).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, entry := range state.History {
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
