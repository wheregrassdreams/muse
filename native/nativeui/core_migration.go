package nativeui

import (
	"muse/locales"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	applicationvault "muse/application/vault"
	"muse/document"
	"muse/document/frontmatter"
	vaultindex "muse/vault/index"
)

type coreMigrationItem struct {
	path, source string
	expected     document.Revision
}

func (a *App) vaultSearchSnapshots(session *applicationvault.Session) []vaultindex.SearchSnapshot {
	if session == nil {
		return nil
	}
	root := session.Snapshot().Root
	paths := session.Paths()
	docs := make([]vaultindex.SearchSnapshot, 0, len(paths))
	for _, path := range paths {
		if filepath.Ext(path) != ".md" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		doc, ok := session.Document(filepath.ToSlash(rel))
		if !ok {
			continue
		}
		docs = append(docs, vaultindex.SearchSnapshot{Path: filepath.ToSlash(rel), Title: doc.Title, Source: doc.Source, Revision: doc.Revision, Metadata: doc.Metadata()})
	}
	return docs
}

type coreMigrationPreview struct {
	items                             []coreMigrationItem
	createdFromBirth, updatedFromMod  int
	unknownCreation, invalid, openDoc int
	samples                           []string
}

func previewCoreMetadataMigration(root, openPath string, docs []vaultindex.SearchSnapshot) coreMigrationPreview {
	var preview coreMigrationPreview
	for _, doc := range docs {
		full := filepath.Join(root, filepath.FromSlash(doc.Path))
		if filepath.Clean(full) == filepath.Clean(openPath) {
			preview.openDoc++
			continue
		}
		parsed := frontmatter.Parse(doc.Source)
		if parsed.Err != nil {
			preview.invalid++
			continue
		}
		present := make(map[string]bool, len(parsed.Fields))
		for _, field := range parsed.Fields {
			present[field.Key] = true
		}
		updated, err := frontmatter.EnsureCore(doc.Source)
		if err != nil {
			preview.invalid++
			continue
		}
		info, statErr := os.Stat(full)
		if statErr != nil {
			preview.invalid++
			continue
		}
		if !present["created_at"] {
			if created, ok := trustedFileBirthTime(info); ok {
				updated, err = frontmatter.SetTyped(updated, "created_at", created.UTC().Format(time.RFC3339Nano), "text")
				if err == nil {
					preview.createdFromBirth++
				}
			} else {
				preview.unknownCreation++
			}
		}
		if err == nil && !present["updated_at"] {
			updated, err = frontmatter.SetTyped(updated, "updated_at", info.ModTime().UTC().Format(time.RFC3339Nano), "text")
			if err == nil {
				preview.updatedFromMod++
			}
		}
		if err != nil {
			preview.invalid++
			continue
		}
		if updated == doc.Source {
			continue
		}
		preview.items = append(preview.items, coreMigrationItem{path: full, expected: document.RevisionOf(doc.Source), source: updated})
		if len(preview.samples) < 5 {
			preview.samples = append(preview.samples, doc.Path)
		}
	}
	return preview
}

func trustedFileBirthTime(info os.FileInfo) (time.Time, bool) {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	field := v.FieldByName("Birthtimespec")
	if !field.IsValid() {
		field = v.FieldByName("Birthtime")
	}
	if !field.IsValid() || field.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	sec, nsec := field.FieldByName("Sec"), field.FieldByName("Nsec")
	if !sec.IsValid() || !nsec.IsValid() || sec.Kind() < reflect.Int || sec.Kind() > reflect.Int64 || nsec.Kind() < reflect.Int || nsec.Kind() > reflect.Int64 {
		return time.Time{}, false
	}
	return time.Unix(sec.Int(), nsec.Int()).UTC(), true
}

func (a *App) openCoreMigrationPreview() {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		a.notice(locales.VaultOpenFirst())
		return
	}
	v, root, path := a.vault, a.vaultSnapshot.Root, a.path
	a.coreMigration = nil
	a.coreMigrationOpen = true
	a.coreMigrationLoading = true
	a.invalidate()
	go func() {
		preview := previewCoreMetadataMigration(root, path, a.vaultSearchSnapshots(v))
		a.onMain(func() {
			if a.vault != v {
				return
			}
			a.coreMigration = &preview
			a.coreMigrationLoading = false
			a.invalidate()
		})
	}()
}

func (a *App) applyCoreMetadataMigration() {
	if a.coreMigration == nil || a.coreMigrationLoading || a.coreMigrationRunning || a.vault == nil || a.documentSession == nil {
		return
	}
	plan := *a.coreMigration
	v, session, openPath := a.vault, a.documentSession, filepath.Clean(a.path)
	a.coreMigrationRunning = true
	a.coreMigrationOpen = false
	go func() {
		applied, skipped := 0, 0
		for _, item := range plan.items {
			if filepath.Clean(item.path) == openPath {
				skipped++
				continue
			}
			_, err := session.SaveNow(item.path, item.path, item.source, item.expected)
			if err != nil {
				skipped++
				continue
			}
			_ = v.Refresh(item.path)
			applied++
		}
		a.onMain(func() {
			a.coreMigrationRunning = false
			a.coreMigration = nil
			message := locales.CoreMigrationCompleted(applied, skipped)
			a.notice(message)
			if a.ed != nil {
				a.ed.SetOptions(a.editorOptions())
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
				if a.coreMigration == nil || len(a.coreMigration.items) == 0 {
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

func migrationPreviewSummary(p *coreMigrationPreview) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(locales.CoreMigrationSummary(len(p.items)))
	b.WriteString("\n\n" + locales.CoreMigrationCreationBreakdown(p.createdFromBirth, p.unknownCreation, p.updatedFromMod))
	if p.openDoc > 0 {
		b.WriteString("\n" + locales.CoreMigrationOpenSkipped())
	}
	if p.invalid > 0 {
		b.WriteString("\n" + locales.CoreMigrationInvalidCount(p.invalid))
	}
	if len(p.samples) > 0 {
		b.WriteString("\n\n" + locales.CoreMigrationExamples(strings.Join(p.samples, "、")))
	}
	return b.String()
}
