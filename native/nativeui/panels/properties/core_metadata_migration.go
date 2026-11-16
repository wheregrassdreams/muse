package nativeui

import (
	"muse/locales"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	documentapp "muse/application/document"
	applicationvault "muse/application/vault"
)

func (a *App) vaultCoreMetadataCandidates(session *applicationvault.Session) []documentapp.CoreMetadataCandidate {
	if session == nil {
		return nil
	}
	root := session.Snapshot().Root
	paths := session.Paths()
	docs := make([]documentapp.CoreMetadataCandidate, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		doc, ok := session.Document(filepath.ToSlash(rel))
		if !ok {
			continue
		}
		docs = append(docs, documentapp.CoreMetadataCandidate{Path: path, Source: doc.Source, Revision: doc.Revision})
	}
	return docs
}

func (a *App) openCoreMigrationPreview() {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		a.notice(locales.VaultOpenFirst())
		return
	}
	v, root, path := a.vault, a.vaultSnapshot.Root, a.documentPath()
	a.coreMigration = nil
	a.coreMigrationOpen = true
	a.coreMigrationLoading = true
	a.invalidate()
	go func() {
		migration := documentapp.NewCoreMetadataMigration(a.metadataReader, a.documentRepository, a.documentRecovery, a.documentEvents)
		preview, err := migration.Preview(root, path, a.vaultCoreMetadataCandidates(v))
		a.onMain(func() {
			if a.vault != v {
				return
			}
			if err != nil {
				a.coreMigration = nil
				a.notice(err.Error())
			} else {
				a.coreMigration = &preview
			}
			a.coreMigrationLoading = false
			a.invalidate()
		})
	}()
}

func (a *App) applyCoreMetadataMigration() {
	if a.coreMigration == nil || a.coreMigrationLoading || a.coreMigrationRunning || a.vault == nil || a.documentRepository == nil {
		return
	}
	plan := *a.coreMigration
	v, openPath := a.vault, filepath.Clean(a.documentPath())
	a.coreMigrationRunning = true
	a.coreMigrationOpen = false
	go func() {
		migration := documentapp.NewCoreMetadataMigration(a.metadataReader, a.documentRepository, a.documentRecovery, a.documentEvents)
		result, err := migration.Apply(plan, openPath)
		if err != nil {
			a.onMain(func() {
				a.coreMigrationRunning = false
				a.notice(err.Error())
				a.invalidate()
			})
			return
		}
		for _, path := range result.AppliedPaths {
			_ = v.Refresh(path)
		}
		applied, skipped := len(result.AppliedPaths), result.Skipped
		a.onMain(func() {
			a.coreMigrationRunning = false
			a.coreMigration = nil
			message := locales.CoreMigrationCompleted(applied, skipped)
			a.notice(message)
			if a.activeNote != nil {
				a.configureActiveNote(0)
			}
			a.invalidate()
		})
	}()
}

func (a *App) coreMetadataMigrationDialog(c *ui.Context) {
	ui.Modal(c, &a.coreMigrationOpen, func() {
		ui.Column(c).Width(560).MaxHeight(640).Padding(20).Gap(12).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.CoreMigrationPreview()).FontWeight(700)
			if a.coreMigrationLoading {
				ui.Text(c, locales.CoreMigrationIndexing()).Wrap()
				return
			}
			if a.coreMigrationRunning {
				ui.Text(c, locales.CoreMigrationApplying()).Wrap()
				return
			}
			if a.coreMigration == nil {
				ui.Text(c, locales.CoreMigrationNoPreview()).Wrap()
			} else {
				ui.Text(c, migrationPreviewSummary(a.coreMigration)).Wrap()
			}
			ui.Row(c).Gap(8).Children(func() {
				if a.coreMigration == nil || len(a.coreMigration.Changes) == 0 {
					ui.Text(c, locales.CoreMigrationNoChanges()).TextColor(c.Theme().TextMuted)
				} else if ui.Button(c, locales.CoreMigrationApply()).Clicked() {
					a.applyCoreMetadataMigration()
				}
				if ui.Button(c, locales.CommonClose()).Clicked() {
					a.coreMigrationOpen = false
				}
			})
		})
	})
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
