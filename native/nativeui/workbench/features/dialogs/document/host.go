package documentdialogs

import (
	"strings"

	"github.com/egoist/mygo/ui"
	documentapp "muse/application/document"
	"muse/locales"
)

// MigrationSource is the detached vault snapshot used to preview a metadata
// migration. The feature never receives a vault session or App pointer.
type MigrationSource struct {
	Ready      bool
	Token      string
	Root       string
	OpenPath   string
	Candidates []documentapp.CoreMetadataCandidate
}

// MigrationPorts expose only the application workflow and platform scheduling
// needed by the dialog. Preview and Apply decisions remain in application/document.
type MigrationPorts struct {
	Source     func() MigrationSource
	IsCurrent  func(MigrationSource) bool
	Preview    func(root, openPath string, candidates []documentapp.CoreMetadataCandidate) (documentapp.CoreMetadataPlan, error)
	Apply      func(plan documentapp.CoreMetadataPlan, openPath string) (documentapp.CoreMetadataApplyResult, error)
	Refresh    func(MigrationSource, []string)
	Go         func(func())
	OnMain     func(func())
	Notify     func(string)
	Applied    func()
	Invalidate func()
}

// MigrationHost owns migration dialog lifecycle and transient preview state.
// It delegates the data migration policy and mutation to application ports.
type MigrationHost struct {
	handle     *Handle
	controller *MigrationController
	ports      MigrationPorts
	plan       *documentapp.CoreMetadataPlan
	loading    bool
	running    bool
	source     *MigrationSource
}

func NewMigrationHost(handle *Handle, ports MigrationPorts) *MigrationHost {
	if handle == nil {
		handle = NewHandle()
	}
	host := &MigrationHost{handle: handle, ports: ports}
	host.controller = NewMigrationController(handle, MigrationActions{
		Apply: host.apply,
		Close: host.invalidate,
	})
	return host
}

func (h *MigrationHost) OpenPreview() {
	if h == nil || h.ports.Source == nil || h.ports.Preview == nil {
		return
	}
	source := h.ports.Source()
	if !source.Ready {
		if h.ports.Notify != nil {
			h.ports.Notify(locales.VaultOpenFirst())
		}
		return
	}
	h.plan = nil
	h.loading = true
	h.source = &source
	h.handle.OpenCoreMigration()
	h.invalidate()
	work := func() {
		plan, err := h.ports.Preview(source.Root, source.OpenPath, source.Candidates)
		h.onMain(func() {
			if h.ports.IsCurrent != nil && !h.ports.IsCurrent(source) {
				h.loading = false
				h.plan = nil
				h.source = nil
				h.source = nil
				h.handle.CloseCoreMigration()
				h.invalidate()
				return
			}
			h.loading = false
			if err != nil {
				h.plan = nil
				h.source = nil
				if h.ports.Notify != nil {
					h.ports.Notify(err.Error())
				}
			} else {
				h.plan = &plan
			}
			h.invalidate()
		})
	}
	h.goAsync(work)
}

func (h *MigrationHost) Render(c *ui.Context) {
	if h == nil {
		return
	}
	model := MigrationModel{Loading: h.loading, Running: h.running, HasPlan: h.plan != nil}
	if h.plan != nil {
		model.Summary = migrationPreviewSummary(h.plan)
		model.HasChanges = len(h.plan.Changes) > 0
	}
	h.controller.Render(c, model)
}

func (h *MigrationHost) HandleAction(action Action) {
	if h == nil {
		return
	}
	h.controller.HandleAction(action)
}

// Close releases the transient preview and dismisses the migration dialog.
func (h *MigrationHost) Close() {
	if h == nil {
		return
	}
	h.plan = nil
	h.source = nil
	h.loading = false
	h.running = false
	h.handle.CloseCoreMigration()
	h.invalidate()
}

func (h *MigrationHost) Snapshot() MigrationSnapshot {
	if h == nil {
		return MigrationSnapshot{}
	}
	return MigrationSnapshot{Open: h.handle != nil && h.handle.IsCoreMigrationOpen(), Loading: h.loading, Running: h.running, HasPlan: h.plan != nil}
}

type MigrationSnapshot struct {
	Open, Loading, Running, HasPlan bool
}

func (h *MigrationHost) apply() {
	if h.plan == nil || h.loading || h.running || h.ports.Source == nil || h.ports.Apply == nil {
		return
	}
	if h.source == nil {
		return
	}
	previewSource := *h.source
	source := h.ports.Source() // re-read open path before applying the preview.
	if h.ports.IsCurrent != nil && !h.ports.IsCurrent(previewSource) {
		h.plan = nil
		h.source = nil
		h.handle.CloseCoreMigration()
		h.invalidate()
		return
	}
	plan := *h.plan
	h.running = true
	h.handle.CloseCoreMigration()
	h.invalidate()
	work := func() {
		result, err := h.ports.Apply(plan, source.OpenPath)
		h.onMain(func() {
			h.running = false
			if h.ports.IsCurrent != nil && !h.ports.IsCurrent(previewSource) {
				h.plan = nil
				h.source = nil
				h.invalidate()
				return
			}
			if err != nil {
				if h.ports.Notify != nil {
					h.ports.Notify(err.Error())
				}
				h.invalidate()
				return
			}
			h.plan = nil
			h.source = nil
			if h.ports.Refresh != nil {
				h.ports.Refresh(source, result.AppliedPaths)
			}
			if h.ports.Notify != nil {
				h.ports.Notify(locales.CoreMigrationCompleted(len(result.AppliedPaths), result.Skipped))
			}
			if h.ports.Applied != nil {
				h.ports.Applied()
			}
			h.invalidate()
		})
	}
	h.goAsync(work)
}

func (h *MigrationHost) goAsync(fn func()) {
	if h.ports.Go != nil {
		h.ports.Go(fn)
	} else {
		go fn()
	}
}

func (h *MigrationHost) onMain(fn func()) {
	if h.ports.OnMain != nil {
		h.ports.OnMain(fn)
	} else {
		fn()
	}
}

func (h *MigrationHost) invalidate() {
	if h.ports.Invalidate != nil {
		h.ports.Invalidate()
	}
}

func migrationPreviewSummary(p *documentapp.CoreMetadataPlan) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(locales.CoreMigrationSummary(len(p.Changes)))
	b.WriteString("\n\n" + locales.CoreMigrationCreationBreakdown(p.CreatedFromBirth, p.UnknownCreation, p.UpdatedFromMod))
	if p.OpenDocument > 0 {
		b.WriteString("\n" + locales.CoreMigrationOpenSkipped())
	}
	if p.Invalid > 0 {
		b.WriteString("\n" + locales.CoreMigrationInvalidCount(p.Invalid))
	}
	if len(p.Samples) > 0 {
		b.WriteString("\n\n" + locales.CoreMigrationExamples(strings.Join(p.Samples, "、")))
	}
	return b.String()
}
