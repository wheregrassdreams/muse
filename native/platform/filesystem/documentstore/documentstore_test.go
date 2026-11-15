package documentstore

import "muse/document"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRevisionConflictAndExternalDeletion(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "config", "recovery"))
	path := filepath.Join(root, "notes", "新建.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	createdRevision, err := store.Save(path, "你好，世界\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if createdRevision != document.RevisionOf("你好，世界\n") {
		t.Fatalf("revision = %q", createdRevision)
	}
	got, err := store.Read(path)
	if err != nil || got.Source != "你好，世界\n" || got.Revision != createdRevision {
		t.Fatalf("Read() = %#v, %v", got, err)
	}

	if err := os.WriteFile(path, []byte("外部修改"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(path, "不应覆盖", createdRevision); !errors.Is(err, document.ErrConflict) {
		t.Fatalf("Save after external edit error = %v, want ErrConflict", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(path, "不应复活", createdRevision); !errors.Is(err, document.ErrConflict) {
		t.Fatalf("Save after external deletion error = %v, want ErrConflict", err)
	}
	if _, err := store.Save(path, "重新创建", ""); err != nil {
		t.Fatalf("Save new path: %v", err)
	}
	if _, err := store.Save(path, "拒绝意外覆盖", ""); !errors.Is(err, document.ErrConflict) {
		t.Fatalf("Save existing path with empty expected revision error = %v", err)
	}
}

func TestSaveHistoryRestoreOnceAndPermissions(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "recovery"))
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("原始内容"), 0o640); err != nil {
		t.Fatal(err)
	}
	first := document.RevisionOf("原始内容")
	second, err := store.Save(path, "更新后的内容", first)
	if err != nil {
		t.Fatal(err)
	}
	if same, err := store.Save(path, "更新后的内容", second); err != nil || same != second {
		t.Fatalf("no-op Save() = %q, %v", same, err)
	}
	itemsBeforeNoop, err := store.ListHistory(path)
	if err != nil || len(itemsBeforeNoop) != 1 {
		t.Fatalf("no-op Save created history: %#v, %v", itemsBeforeNoop, err)
	}
	mode, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0o640 {
		t.Fatalf("permissions = %o, want 640", mode.Mode().Perm())
	}

	items, err := store.ListHistory(path)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListHistory() = %#v, %v", items, err)
	}
	restored, err := store.ReadHistory(path, items[0].ID)
	if err != nil || restored.Source != "原始内容" || restored.Revision != first {
		t.Fatalf("ReadHistory() = %#v, %v", restored, err)
	}

	// Restore uses the same compare-and-save API as a regular edit; the
	// version being replaced is captured before the restore takes effect.
	restoreRevision, err := store.Save(path, restored.Source, second)
	if err != nil || restoreRevision != first {
		t.Fatalf("restore Save() = %q, %v", restoreRevision, err)
	}
	items, err = store.ListHistory(path)
	if err != nil || len(items) != 2 {
		t.Fatalf("history after restore = %#v, %v", items, err)
	}
	var foundUpdated bool
	for _, item := range items {
		doc, readErr := store.ReadHistory(path, item.ID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if doc.Source == "更新后的内容" {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Fatal("restore did not preserve the text it replaced")
	}
	if err := store.DeleteHistory(path, items[0].ID); err != nil {
		t.Fatalf("DeleteHistory(): %v", err)
	}
}

func TestDraftLifecycleIncludesUntitledKey(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "recovery"))
	const key = "untitled:session-中文-1"
	if err := store.WriteDraft(key, "", "草稿内容\n"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(store.dir, "drafts"))
	if err != nil || len(entries) != 1 || entries[0].Name() != revisionOf(key)+".json" {
		t.Fatalf("draft is not stored by hashed key: %#v, %v", entries, err)
	}
	got, err := store.ReadDraft(key)
	if err != nil || got.Key != key || got.Path != "" || got.Text != "草稿内容\n" || got.UpdatedAt.IsZero() {
		t.Fatalf("ReadDraft() = %#v, %v", got, err)
	}
	drafts, err := store.ListDrafts()
	if err != nil || len(drafts) != 1 || drafts[0].Text != got.Text {
		t.Fatalf("ListDrafts() = %#v, %v", drafts, err)
	}
	if err := store.DeleteDraft(key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadDraft(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadDraft after delete error = %v, want not-exist", err)
	}
}

func TestConflictDoesNotClearDraft(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "recovery"))
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteDraft("note-1", path, "unsaved draft"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(path, "stale write", document.RevisionOf("old disk")); !errors.Is(err, document.ErrConflict) {
		t.Fatalf("Save() error = %v, want conflict", err)
	}
	draft, err := store.ReadDraft("note-1")
	if err != nil || draft.Text != "unsaved draft" {
		t.Fatalf("draft after failed Save() = %#v, %v", draft, err)
	}
}

func TestHistoryRetentionLimit(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "recovery"))
	path := filepath.Join(root, "note.md")
	revision, err := store.Save(path, "version 0", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= historyLimit+1; i++ {
		text := fmt.Sprintf("version %d", i)
		revision, err = store.Save(path, text, revision)
		if err != nil {
			t.Fatalf("save version %d: %v", i, err)
		}
	}
	items, err := store.ListHistory(path)
	if err != nil || len(items) != historyLimit {
		t.Fatalf("history count = %d, %v; want %d", len(items), err, historyLimit)
	}
}
