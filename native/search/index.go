package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
	"github.com/gofrs/flock"
	"muse/document/types"
)

const (
	fieldID         = "id"
	fieldPath       = "path"
	fieldPathEN     = "path_en"
	fieldPathEdge   = "path_edge"
	fieldPathHanNG  = "path_han_ng"
	fieldRevision   = "revision"
	fieldSource     = "source"
	fieldTitleEN    = "title_en"
	fieldTitleEdge  = "title_edge"
	fieldTitleHanNG = "title_han_ng"
	fieldBodyEN     = "body_en"
	fieldBodyHanNG  = "body_han_ng"
	fieldMetaEN     = "meta_en"
	fieldMetaHanNG  = "meta_han_ng"

	indexSubdir           = "tantivy"
	manifestName          = "manifest.json"
	dirtyName             = "dirty.marker"
	ngramMax              = 2
	literalBatchSize      = 64
	nativeDeleteBatchSize = 128
	nativeUpsertBatchSize = 1024
	// A small document count alone does not bound copies of large notes.
	// A single oversized note is allowed on its own.
	nativeBatchSourceBytes = 2 << 20
)

type manifest struct {
	Version int                       `json:"version"`
	Docs    map[string]manifestRecord `json:"docs"`
}

type manifestRecord struct {
	Path     string         `json:"path"`
	Title    string         `json:"title,omitempty"`
	Revision string         `json:"revision"`
	Metadata types.Metadata `json:"metadata,omitempty"`
}

// Index serializes mutation and protects the native context lifetime. Tantivy
// stores source text in its document store; this struct retains only metadata.
type indexState struct {
	mu       sync.RWMutex
	dir      string
	dataDir  string
	manifest manifest
	engine   *tantivy.TantivyContext
	fileLock *flock.Flock
	closed   bool
	poisoned bool

	// literalDocs holds lowercase searchable text for the current in-memory
	// snapshot. It avoids fetching and normalizing every stored source for each
	// literal query; original sources remain in Tantivy and are loaded only for
	// matching documents whose snippets are returned.
	literalDocs  map[string]literalDoc
	literalBytes int64
	literalReady bool
	// literalOrder is an optional path/ID-sorted manifest key projection used
	// to avoid sorting the full corpus for every literal query. Its separate
	// budget bounds the extra string headers; IDs share storage with manifest.
	literalOrder      []string
	literalOrderBytes int64
	literalOrderReady bool
}

func openIndex(dir string) (*Index, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("search: empty index directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("search: resolve index directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("search: create index directory: %w", err)
	}
	fileLock := flock.New(filepath.Join(abs, ".search.lock"))
	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("search: acquire index lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("search: index in use: %s", abs)
	}
	opened := false
	defer func() {
		if !opened {
			_ = fileLock.Unlock()
		}
	}()
	dataDir := filepath.Join(abs, indexSubdir)
	manifestPath := filepath.Join(abs, manifestName)
	dirtyPath := filepath.Join(abs, dirtyName)
	meta, manifestErr := readManifest(manifestPath)
	if manifestErr != nil && !recoverableManifestError(manifestErr) {
		return nil, manifestErr
	}
	_, dataErr := os.Stat(dataDir)
	dataExists := dataErr == nil
	if dataErr != nil && !errors.Is(dataErr, os.ErrNotExist) {
		return nil, fmt.Errorf("search: inspect Tantivy directory: %w", dataErr)
	}
	_, manifestStatErr := os.Stat(manifestPath)
	manifestMissing := errors.Is(manifestStatErr, os.ErrNotExist)
	if manifestStatErr != nil && !manifestMissing {
		return nil, fmt.Errorf("search: inspect manifest: %w", manifestStatErr)
	}
	_, dirtyErr := os.Stat(dirtyPath)
	dirty := dirtyErr == nil
	if dirtyErr != nil && !errors.Is(dirtyErr, os.ErrNotExist) {
		return nil, fmt.Errorf("search: inspect dirty marker: %w", dirtyErr)
	}
	recovering := dirty || manifestMissing || manifestErr != nil || !dataExists
	if recovering && !dirty {
		if err := writeDirtyMarker(dirtyPath); err != nil {
			return nil, fmt.Errorf("search: mark recovery in progress: %w", err)
		}
	}
	resetManifest := manifestErr != nil || !dataExists
	if dirty || manifestErr != nil || manifestMissing || !dataExists {
		reason := "manifest missing"
		if dirty {
			reason = "an interrupted index transaction was marked dirty"
		}
		if manifestErr != nil {
			reason = manifestErr.Error()
		} else if !dataExists {
			reason = "Tantivy data is missing"
		}
		if dataExists {
			quarantine, qErr := quarantineIndexDir(dataDir)
			if qErr != nil {
				return nil, fmt.Errorf("search: quarantine index after %s: %w", reason, qErr)
			}
			log.Printf("search: quarantined index at %s: %s", quarantine, reason)
			dataExists = false
		}
		resetManifest = true
		meta = manifest{Version: 1, Docs: make(map[string]manifestRecord)}
	}
	if manifestErr != nil {
		quarantinedManifest := manifestPath + ".corrupt-" + fmt.Sprint(time.Now().UnixNano())
		if err := os.Rename(manifestPath, quarantinedManifest); err != nil {
			return nil, fmt.Errorf("search: quarantine invalid manifest: %w", err)
		}
		log.Printf("search: quarantined invalid manifest at %s: %v", quarantinedManifest, manifestErr)
	}
	if !dataExists {
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return nil, fmt.Errorf("search: create Tantivy directory: %w", err)
		}
	}
	engine, err := openEngine(dataDir)
	if err != nil && isRecoverableOpenError(err) {
		openErr := err
		if !recovering {
			if markerErr := writeDirtyMarker(dirtyPath); markerErr != nil {
				return nil, fmt.Errorf("search: mark incompatible-index recovery: %w", markerErr)
			}
			recovering = true
		}
		quarantine := dataDir + ".corrupt-" + fmt.Sprint(time.Now().UnixNano())
		if renameErr := os.Rename(dataDir, quarantine); renameErr != nil {
			return nil, fmt.Errorf("search: Tantivy index is incompatible (%v), quarantine failed: %w", err, renameErr)
		}
		if mkdirErr := os.MkdirAll(dataDir, 0o700); mkdirErr != nil {
			return nil, fmt.Errorf("search: create replacement Tantivy directory: %w", mkdirErr)
		}
		engine, err = openEngine(dataDir)
		if err != nil {
			return nil, fmt.Errorf("search: open replacement Tantivy index: %w", err)
		}
		meta.Docs = make(map[string]manifestRecord)
		if saveErr := writeManifest(manifestPath, meta); saveErr != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("search: save rebuilt manifest: %w", saveErr)
		}
		log.Printf("search: quarantined incompatible index at %s: %v", quarantine, openErr)
	} else if err != nil {
		return nil, fmt.Errorf("search: open Tantivy index: %w", err)
	}
	if meta.Docs == nil {
		meta.Docs = make(map[string]manifestRecord)
	}
	if manifestMissing || manifestErr != nil || resetManifest {
		if err := writeManifest(manifestPath, meta); err != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("search: initialize recovered manifest: %w", err)
		}
	}
	if recovering {
		if err := clearDirtyMarker(dirtyPath, abs); err != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("search: finish index recovery: %w", err)
		}
	}
	s := &indexState{dir: abs, dataDir: dataDir, manifest: meta, engine: engine, fileLock: fileLock}
	opened = true
	return &Index{state: s}, nil
}

func recoverableManifestError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "decode manifest") || strings.Contains(message, "unsupported or invalid manifest version")
}

func quarantineIndexDir(path string) (string, error) {
	quarantine := path + ".corrupt-" + fmt.Sprint(time.Now().UnixNano())
	if err := os.Rename(path, quarantine); err != nil {
		return "", err
	}
	return quarantine, nil
}

func readManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifest{Version: 1, Docs: make(map[string]manifestRecord)}, nil
	}
	if err != nil {
		return manifest{}, fmt.Errorf("search: read manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("search: decode manifest: %w", err)
	}
	if m.Version != 1 || m.Docs == nil {
		return manifest{}, fmt.Errorf("search: unsupported or invalid manifest version %d", m.Version)
	}
	return m, nil
}

func writeManifest(path string, m manifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("write temporary manifest: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close temporary manifest: %w", closeErr)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	return nil
}

func isRecoverableOpenError(err error) bool {
	s := strings.ToLower(err.Error())
	for _, marker := range []string{"schema does not match", "version mismatch", "incompatible version", "corrupt index", "failed to deserialize", "invalid meta.json"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func openEngine(dir string) (*tantivy.TantivyContext, error) {
	builder, err := tantivy.NewSchemaBuilder()
	if err != nil {
		return nil, fmt.Errorf("create schema builder: %w", err)
	}
	add := func(name string, stored, isText, fast bool, option int, tokenizer string) error {
		return builder.AddTextField(name, stored, isText, fast, option, tokenizer)
	}
	for _, field := range []struct {
		name               string
		stored, text, fast bool
		option             int
		tokenizer          string
	}{
		{fieldID, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldPath, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldPathEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldPathEdge, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerEdgeNgram},
		{fieldPathHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldRevision, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldSource, true, false, false, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldTitleEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldTitleEdge, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerEdgeNgram},
		{fieldTitleHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldBodyEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldBodyHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldMetaEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldMetaHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
	} {
		if err := add(field.name, field.stored, field.text, field.fast, field.option, field.tokenizer); err != nil {
			return nil, fmt.Errorf("add schema field %q: %w", field.name, err)
		}
	}
	schema, err := builder.BuildSchema()
	if err != nil {
		return nil, fmt.Errorf("build schema: %w", err)
	}
	engine, err := tantivy.NewTantivyContextWithSchema(dir, schema)
	if err != nil {
		return nil, err
	}
	registrations := []struct {
		name string
		run  func() error
	}{
		{tantivy.TokenizerSimple, func() error { return engine.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 100, tantivy.English) }},
		{tantivy.TokenizerNgram, func() error { return engine.RegisterTextAnalyzerNgram(tantivy.TokenizerNgram, 1, ngramMax, false) }},
		{tantivy.TokenizerRaw, func() error { return engine.RegisterTextAnalyzerRaw(tantivy.TokenizerRaw) }},
		{tantivy.TokenizerEdgeNgram, func() error { return engine.RegisterTextAnalyzerEdgeNgram(tantivy.TokenizerEdgeNgram, 1, 32, 256) }},
	}
	for _, registration := range registrations {
		if err := registration.run(); err != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("register %s tokenizer: %w", registration.name, err)
		}
	}
	return engine, nil
}

func (i *Index) getState() (*indexState, error) {
	if i == nil || i.state == nil {
		return nil, errors.New("search: index is closed or uninitialized")
	}
	return i.state, nil
}

func (i *Index) reconcile(ctx context.Context, docs []Document) error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	// Build the bounded projection outside the exclusive Tantivy/index lock so
	// normalization of large snapshots does not block concurrent readers.
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return errors.New("search: index is closed")
	}
	previous := make(map[string]literalDoc, len(s.literalDocs))
	for id, projection := range s.literalDocs {
		previous[id] = projection
	}
	s.mu.RUnlock()
	nextLiteral, nextLiteralBytes, nextOrder, nextOrderReady, err := buildLiteralProjection(ctx, docs, previous)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("search: index is closed")
	}
	if s.poisoned {
		return errors.New("search: index transaction is incomplete; close and reopen to rebuild")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(docs))
	upserts := make([]Document, 0, len(docs))
	for _, d := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateDocument(d); err != nil {
			return err
		}
		if _, ok := seen[d.ID]; ok {
			return fmt.Errorf("search: duplicate document ID %q", d.ID)
		}
		seen[d.ID] = struct{}{}
		key := d.Revision
		if key == "" {
			key = sourceDigest(d.Source)
		}
		if old, ok := s.manifest.Docs[d.ID]; ok && old.Path == d.Path && old.Title == d.Title && old.Revision == key && reflect.DeepEqual(old.Metadata, d.Metadata) {
			continue
		}
		d.Revision = key
		upserts = append(upserts, d)
	}
	deletes := make([]string, 0)
	for id := range s.manifest.Docs {
		if _, ok := seen[id]; !ok {
			deletes = append(deletes, id)
		}
	}
	sort.Strings(deletes)
	if len(upserts) == 0 && len(deletes) == 0 {
		s.literalDocs = nextLiteral
		s.literalBytes = nextLiteralBytes
		s.literalReady = true
		s.literalOrder = nextOrder
		s.literalOrderBytes = literalOrderSize(nextOrder)
		s.literalOrderReady = nextOrderReady
		return nil
	}
	if err := s.applyLocked(ctx, upserts, deletes); err != nil {
		return err
	}
	s.literalDocs = nextLiteral
	s.literalBytes = nextLiteralBytes
	s.literalReady = true
	s.literalOrder = nextOrder
	s.literalOrderBytes = literalOrderSize(nextOrder)
	s.literalOrderReady = nextOrderReady
	return nil
}

// NeedsReconcile reports whether the durable Tantivy snapshot differs from
// docs without mutating the index or building its query-time projections.
// Callers can use this to move expensive full-corpus indexing into a short-lived
// worker process while keeping the long-lived UI process read-only.
func (i *Index) needsReconcile(docs []Document) (bool, error) {
	s, err := i.getState()
	if err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false, errors.New("search: index is closed")
	}
	if s.poisoned {
		return false, errors.New("search: index transaction is incomplete; close and reopen to rebuild")
	}
	if len(docs) != len(s.manifest.Docs) {
		return true, nil
	}
	seen := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		if err := validateDocument(d); err != nil {
			return false, err
		}
		if _, ok := seen[d.ID]; ok {
			return false, fmt.Errorf("search: duplicate document ID %q", d.ID)
		}
		seen[d.ID] = struct{}{}
		revision := d.Revision
		if revision == "" {
			revision = sourceDigest(d.Source)
		}
		old, ok := s.manifest.Docs[d.ID]
		if !ok || old.Path != d.Path || old.Title != d.Title || old.Revision != revision || !reflect.DeepEqual(old.Metadata, d.Metadata) {
			return true, nil
		}
	}
	return false, nil
}

func (i *Index) apply(ctx context.Context, upserts []Document, deletes []string) error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	prepared := append([]Document(nil), upserts...)
	s.mu.RLock()
	prepareProjection := s.literalReady
	availableProjectionBytes := int64(0)
	if prepareProjection {
		availableProjectionBytes = literalProjectionBudget - s.literalBytes
		reclaim := make(map[string]struct{}, len(deletes)+len(prepared))
		for _, id := range deletes {
			reclaim[id] = struct{}{}
		}
		for _, d := range prepared {
			reclaim[d.ID] = struct{}{}
		}
		for id := range reclaim {
			if old, ok := s.literalDocs[id]; ok {
				availableProjectionBytes += old.bytes(id)
			}
		}
		availableProjectionBytes = min(availableProjectionBytes, literalProjectionBudget)
	}
	s.mu.RUnlock()
	projections := make(map[string]literalDoc)
	if prepareProjection {
		for n := range prepared {
			if prepared[n].Revision == "" {
				prepared[n].Revision = sourceDigest(prepared[n].Source)
			}
		}
		projections = prepareLiteralProjections(prepared, availableProjectionBytes)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("search: index is closed")
	}
	if s.poisoned {
		return errors.New("search: index transaction is incomplete; close and reopen to rebuild")
	}
	if err := s.applyLocked(ctx, prepared, deletes); err != nil {
		return err
	}
	if s.literalReady {
		s.applyLiteralProjection(prepared, deletes, projections)
	}
	s.applyLiteralOrder(prepared, deletes)
	return nil
}

func (s *indexState) applyLocked(ctx context.Context, upserts []Document, deletes []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	upsertIDs := make(map[string]struct{}, len(upserts))
	for _, d := range upserts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateDocument(d); err != nil {
			return err
		}
		if _, ok := upsertIDs[d.ID]; ok {
			return fmt.Errorf("search: duplicate upsert ID %q", d.ID)
		}
		upsertIDs[d.ID] = struct{}{}
	}
	deleteSet := make(map[string]struct{}, len(deletes)+len(upserts))
	explicitDeletes := make(map[string]struct{}, len(deletes))
	for _, id := range deletes {
		if id == "" {
			return errors.New("search: empty delete ID")
		}
		deleteSet[id] = struct{}{}
		if _, replaced := upsertIDs[id]; !replaced {
			explicitDeletes[id] = struct{}{}
		}
	}
	for id := range upsertIDs {
		deleteSet[id] = struct{}{}
	}
	deleteIDs := make([]string, 0, len(explicitDeletes))
	for id := range explicitDeletes {
		deleteIDs = append(deleteIDs, id)
	}
	sort.Strings(deleteIDs)
	if len(upserts) == 0 && len(deleteIDs) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeDirtyMarker(filepath.Join(s.dir, dirtyName)); err != nil {
		return fmt.Errorf("search: mark index transaction dirty: %w", err)
	}
	s.poisoned = true
	// Tantivy commits each bounded batch independently. The marker makes a
	// partial operation recoverable; it is cleared only after the final manifest.
	for at := 0; at < len(deleteIDs); at += nativeDeleteBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(at+nativeDeleteBatchSize, len(deleteIDs))
		if _, err := s.engine.BatchAddAndDeleteDocumentsWithOpstamp(nil, fieldID, deleteIDs[at:end]); err != nil {
			return fmt.Errorf("search: apply Tantivy delete batch %d: %w", at/nativeDeleteBatchSize, err)
		}
	}
	for at := 0; at < len(upserts); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end, sourceBytes := at, 0
		for end < len(upserts) && end-at < nativeUpsertBatchSize {
			d := upserts[end]
			size := len(d.Source) + len(d.Title) + len(d.Path)
			for _, value := range d.Metadata.Aliases {
				size += len(value)
			}
			for _, value := range d.Metadata.Tags {
				size += len(value)
			}
			if end > at && sourceBytes+size > nativeBatchSourceBytes {
				break
			}
			sourceBytes += size
			end++
		}
		docs := make([]*tantivy.Document, 0, end-at)
		batchIDs := make([]string, 0, end-at)
		for _, input := range upserts[at:end] {
			d := input // shallow copy; Source and metadata strings remain caller-owned.
			if d.Revision == "" {
				d.Revision = sourceDigest(d.Source)
			}
			doc, err := makeNativeDocument(s.engine, d)
			if err != nil {
				for _, pending := range docs {
					pending.Free()
				}
				return err
			}
			docs = append(docs, doc)
			batchIDs = append(batchIDs, d.ID)
		}
		if _, err := s.engine.BatchAddAndDeleteDocumentsWithOpstamp(docs, fieldID, batchIDs); err != nil {
			return fmt.Errorf("search: apply Tantivy upsert batch %d: %w", at/nativeUpsertBatchSize, err)
		}
		at = end
	}
	if err := s.engine.ReloadReader(); err != nil {
		return fmt.Errorf("search: reload committed index: %w", err)
	}
	next := manifest{Version: 1, Docs: make(map[string]manifestRecord, len(s.manifest.Docs)+len(upserts))}
	for id, rec := range s.manifest.Docs {
		if _, removed := deleteSet[id]; !removed {
			next.Docs[id] = rec
		}
	}
	for _, d := range upserts {
		revision := d.Revision
		if revision == "" {
			revision = sourceDigest(d.Source)
		}
		next.Docs[d.ID] = manifestRecord{Path: d.Path, Title: d.Title, Revision: revision, Metadata: d.Metadata.Clone()}
	}
	if err := writeManifest(filepath.Join(s.dir, manifestName), next); err != nil {
		return fmt.Errorf("search: commit succeeded but manifest update failed: %w", err)
	}
	if err := clearDirtyMarker(filepath.Join(s.dir, dirtyName), s.dir); err != nil {
		return fmt.Errorf("search: commit succeeded but dirty marker could not be cleared: %w", err)
	}
	s.manifest = next
	s.poisoned = false
	return nil
}

func writeDirtyMarker(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString("transaction in progress\n")
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func clearDirtyMarker(path, dir string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	return errors.Join(syncErr, closeErr)
}

func makeNativeDocument(engine *tantivy.TantivyContext, d Document) (*tantivy.Document, error) {
	doc := tantivy.NewDocument()
	add := func(field, value string) error { return doc.AddField(value, engine, field) }
	for _, f := range []struct{ field, value string }{
		{fieldID, d.ID}, {fieldPath, d.Path}, {fieldPathEN, d.Path}, {fieldPathEdge, d.Path}, {fieldPathHanNG, hanOnly(d.Path)}, {fieldRevision, d.Revision}, {fieldSource, d.Source},
		{fieldTitleEN, d.Title}, {fieldTitleEdge, d.Title},
		{fieldBodyEN, d.Source},
		{fieldTitleHanNG, hanOnly(d.Title)}, {fieldBodyHanNG, hanOnly(d.Source)},
	} {
		if err := add(f.field, f.value); err != nil {
			doc.Free()
			return nil, fmt.Errorf("search: index %s for %q: %w", f.field, d.ID, err)
		}
	}
	metaValues := append(append([]string(nil), d.Metadata.Aliases...), d.Metadata.Tags...)
	metaValues = append(metaValues, string(d.Metadata.Type), d.Metadata.CreatedAt, d.Metadata.UpdatedAt)
	meta := strings.Join(metaValues, "\n")
	for _, f := range []struct{ field, value string }{{fieldMetaEN, meta}, {fieldMetaHanNG, hanOnly(meta)}} {
		if err := add(f.field, f.value); err != nil {
			doc.Free()
			return nil, fmt.Errorf("search: index %s metadata for %q: %w", f.field, d.ID, err)
		}
	}
	return doc, nil
}

func validateDocument(d Document) error {
	if d.ID == "" || d.Path == "" {
		return errors.New("search: document ID and path are required")
	}
	if !utf8.ValidString(d.ID) || !utf8.ValidString(d.Path) || !utf8.ValidString(d.Title) || !utf8.ValidString(d.Source) || !utf8.ValidString(d.Revision) {
		return fmt.Errorf("search: document %q contains invalid UTF-8", d.ID)
	}
	return nil
}

func sourceDigest(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func hanOnly(source string) string {
	var out strings.Builder
	out.Grow(len(source))
	hasHan, previousHan := false, false
	for _, r := range source {
		if unicode.Is(unicode.Han, r) {
			// Defer the separator until another Han rune arrives. This is
			// equivalent to collapsing intervening non-Han runs with Fields,
			// without allocating builder and Fields copies.
			if hasHan && !previousHan {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			hasHan, previousHan = true, true
		} else {
			previousHan = false
		}
	}
	return out.String()
}

func (i *Index) close() error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("search: index already closed")
	}
	s.closed = true
	s.literalDocs = nil
	s.literalBytes = 0
	s.literalOrder = nil
	s.literalOrderBytes = 0
	s.literalOrderReady = false
	engineErr := s.engine.Close()
	lockErr := s.fileLock.Unlock()
	return errors.Join(wrapCloseError("close Tantivy index", engineErr), wrapCloseError("release index lock", lockErr))
}

func wrapCloseError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("search: %s: %w", action, err)
}

// A conservative guard prevents invalid numeric scores from disrupting sort.
func safeScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}
