package vaultsearch

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
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
	"github.com/gofrs/flock"
)

const (
	fieldID         = "id"
	fieldPath       = "path"
	fieldPathEN     = "path_en"
	fieldPathZH     = "path_zh"
	fieldPathEdge   = "path_edge"
	fieldPathHanNG  = "path_han_ng"
	fieldRevision   = "revision"
	fieldSource     = "source"
	fieldTitleEN    = "title_en"
	fieldTitleZH    = "title_zh"
	fieldTitleEdge  = "title_edge"
	fieldTitleHanNG = "title_han_ng"
	fieldBodyEN     = "body_en"
	fieldBodyZH     = "body_zh"
	fieldBodyHanNG  = "body_han_ng"
	fieldMetaEN     = "meta_en"
	fieldMetaZH     = "meta_zh"

	indexSubdir      = "tantivy"
	manifestName     = "manifest.json"
	dirtyName        = "dirty.marker"
	ngramMax         = 2
	literalBatchSize = 64
	nativeBatchSize  = 128
	// A small document count alone does not bound copies of large notes.
	// A single oversized note is allowed on its own.
	nativeBatchSourceBytes = 2 << 20
)

type manifest struct {
	Version int                       `json:"version"`
	Docs    map[string]manifestRecord `json:"docs"`
}

type manifestRecord struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
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
}

func openIndex(dir string) (*Index, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("vaultsearch: empty index directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("vaultsearch: resolve index directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("vaultsearch: create index directory: %w", err)
	}
	fileLock := flock.New(filepath.Join(abs, ".vaultsearch.lock"))
	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("vaultsearch: acquire index lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("vaultsearch: index in use: %s", abs)
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
		return nil, fmt.Errorf("vaultsearch: inspect Tantivy directory: %w", dataErr)
	}
	_, manifestStatErr := os.Stat(manifestPath)
	manifestMissing := errors.Is(manifestStatErr, os.ErrNotExist)
	if manifestStatErr != nil && !manifestMissing {
		return nil, fmt.Errorf("vaultsearch: inspect manifest: %w", manifestStatErr)
	}
	_, dirtyErr := os.Stat(dirtyPath)
	dirty := dirtyErr == nil
	if dirtyErr != nil && !errors.Is(dirtyErr, os.ErrNotExist) {
		return nil, fmt.Errorf("vaultsearch: inspect dirty marker: %w", dirtyErr)
	}
	recovering := dirty || manifestMissing || manifestErr != nil || !dataExists
	if recovering && !dirty {
		if err := writeDirtyMarker(dirtyPath); err != nil {
			return nil, fmt.Errorf("vaultsearch: mark recovery in progress: %w", err)
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
				return nil, fmt.Errorf("vaultsearch: quarantine index after %s: %w", reason, qErr)
			}
			log.Printf("vaultsearch: quarantined index at %s: %s", quarantine, reason)
			dataExists = false
		}
		resetManifest = true
		meta = manifest{Version: 1, Docs: make(map[string]manifestRecord)}
	}
	if manifestErr != nil {
		quarantinedManifest := manifestPath + ".corrupt-" + fmt.Sprint(time.Now().UnixNano())
		if err := os.Rename(manifestPath, quarantinedManifest); err != nil {
			return nil, fmt.Errorf("vaultsearch: quarantine invalid manifest: %w", err)
		}
		log.Printf("vaultsearch: quarantined invalid manifest at %s: %v", quarantinedManifest, manifestErr)
	}
	if !dataExists {
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return nil, fmt.Errorf("vaultsearch: create Tantivy directory: %w", err)
		}
	}
	engine, err := openEngine(dataDir)
	if err != nil && isRecoverableOpenError(err) {
		openErr := err
		if !recovering {
			if markerErr := writeDirtyMarker(dirtyPath); markerErr != nil {
				return nil, fmt.Errorf("vaultsearch: mark incompatible-index recovery: %w", markerErr)
			}
			recovering = true
		}
		quarantine := dataDir + ".corrupt-" + fmt.Sprint(time.Now().UnixNano())
		if renameErr := os.Rename(dataDir, quarantine); renameErr != nil {
			return nil, fmt.Errorf("vaultsearch: Tantivy index is incompatible (%v), quarantine failed: %w", err, renameErr)
		}
		if mkdirErr := os.MkdirAll(dataDir, 0o700); mkdirErr != nil {
			return nil, fmt.Errorf("vaultsearch: create replacement Tantivy directory: %w", mkdirErr)
		}
		engine, err = openEngine(dataDir)
		if err != nil {
			return nil, fmt.Errorf("vaultsearch: open replacement Tantivy index: %w", err)
		}
		meta.Docs = make(map[string]manifestRecord)
		if saveErr := writeManifest(manifestPath, meta); saveErr != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("vaultsearch: save rebuilt manifest: %w", saveErr)
		}
		log.Printf("vaultsearch: quarantined incompatible index at %s: %v", quarantine, openErr)
	} else if err != nil {
		return nil, fmt.Errorf("vaultsearch: open Tantivy index: %w", err)
	}
	if meta.Docs == nil {
		meta.Docs = make(map[string]manifestRecord)
	}
	if manifestMissing || manifestErr != nil || resetManifest {
		if err := writeManifest(manifestPath, meta); err != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("vaultsearch: initialize recovered manifest: %w", err)
		}
	}
	if recovering {
		if err := clearDirtyMarker(dirtyPath, abs); err != nil {
			_ = engine.Close()
			return nil, fmt.Errorf("vaultsearch: finish index recovery: %w", err)
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
		return manifest{}, fmt.Errorf("vaultsearch: read manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("vaultsearch: decode manifest: %w", err)
	}
	if m.Version != 1 || m.Docs == nil {
		return manifest{}, fmt.Errorf("vaultsearch: unsupported or invalid manifest version %d", m.Version)
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
		{fieldPathZH, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
		{fieldPathEdge, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerEdgeNgram},
		{fieldPathHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldRevision, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldSource, true, false, false, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
		{fieldTitleEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldTitleZH, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
		{fieldTitleEdge, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerEdgeNgram},
		{fieldTitleHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldBodyEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldBodyZH, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
		{fieldBodyHanNG, false, true, false, tantivy.IndexRecordOptionWithFreqs, tantivy.TokenizerNgram},
		{fieldMetaEN, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
		{fieldMetaZH, false, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
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
		{tantivy.TokenizerJieba, func() error { return engine.RegisterTextAnalyzerJieba(tantivy.TokenizerJieba, 100) }},
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
		return nil, errors.New("vaultsearch: index is closed or uninitialized")
	}
	return i.state, nil
}

func (i *Index) reconcile(ctx context.Context, docs []Document) error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("vaultsearch: index is closed")
	}
	if s.poisoned {
		return errors.New("vaultsearch: index transaction is incomplete; close and reopen to rebuild")
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
			return fmt.Errorf("vaultsearch: duplicate document ID %q", d.ID)
		}
		seen[d.ID] = struct{}{}
		key := d.Revision
		if key == "" {
			key = sourceDigest(d.Source)
		}
		if old, ok := s.manifest.Docs[d.ID]; ok && old.Path == d.Path && old.Revision == key {
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
		return nil
	}
	return s.applyLocked(ctx, upserts, deletes)
}

func (i *Index) apply(ctx context.Context, upserts []Document, deletes []string) error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("vaultsearch: index is closed")
	}
	if s.poisoned {
		return errors.New("vaultsearch: index transaction is incomplete; close and reopen to rebuild")
	}
	return s.applyLocked(ctx, upserts, deletes)
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
			return fmt.Errorf("vaultsearch: duplicate upsert ID %q", d.ID)
		}
		upsertIDs[d.ID] = struct{}{}
	}
	deleteSet := make(map[string]struct{}, len(deletes)+len(upserts))
	explicitDeletes := make(map[string]struct{}, len(deletes))
	for _, id := range deletes {
		if id == "" {
			return errors.New("vaultsearch: empty delete ID")
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
		return fmt.Errorf("vaultsearch: mark index transaction dirty: %w", err)
	}
	s.poisoned = true
	// Tantivy commits each bounded batch independently. The marker makes a
	// partial operation recoverable; it is cleared only after the final manifest.
	for at := 0; at < len(deleteIDs); at += nativeBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(at+nativeBatchSize, len(deleteIDs))
		if _, err := s.engine.BatchAddAndDeleteDocumentsWithOpstamp(nil, fieldID, deleteIDs[at:end]); err != nil {
			return fmt.Errorf("vaultsearch: apply Tantivy delete batch %d: %w", at/nativeBatchSize, err)
		}
	}
	for at := 0; at < len(upserts); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end, sourceBytes := at, 0
		for end < len(upserts) && end-at < nativeBatchSize {
			d := upserts[end]
			size := len(d.Source) + len(d.Title) + len(d.Path)
			for _, value := range d.Aliases {
				size += len(value)
			}
			for _, value := range d.Tags {
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
			return fmt.Errorf("vaultsearch: apply Tantivy upsert batch %d: %w", at/nativeBatchSize, err)
		}
		at = end
	}
	if err := s.engine.ReloadReader(); err != nil {
		return fmt.Errorf("vaultsearch: reload committed index: %w", err)
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
		next.Docs[d.ID] = manifestRecord{Path: d.Path, Revision: revision}
	}
	if err := writeManifest(filepath.Join(s.dir, manifestName), next); err != nil {
		return fmt.Errorf("vaultsearch: commit succeeded but manifest update failed: %w", err)
	}
	if err := clearDirtyMarker(filepath.Join(s.dir, dirtyName), s.dir); err != nil {
		return fmt.Errorf("vaultsearch: commit succeeded but dirty marker could not be cleared: %w", err)
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
		{fieldID, d.ID}, {fieldPath, d.Path}, {fieldPathEN, d.Path}, {fieldPathZH, d.Path}, {fieldPathEdge, d.Path}, {fieldPathHanNG, hanOnly(d.Path)}, {fieldRevision, d.Revision}, {fieldSource, d.Source},
		{fieldTitleEN, d.Title}, {fieldTitleZH, d.Title}, {fieldTitleEdge, d.Title},
		{fieldBodyEN, d.Source}, {fieldBodyZH, d.Source},
		{fieldTitleHanNG, hanOnly(d.Title)}, {fieldBodyHanNG, hanOnly(d.Source)},
	} {
		if err := add(f.field, f.value); err != nil {
			doc.Free()
			return nil, fmt.Errorf("vaultsearch: index %s for %q: %w", f.field, d.ID, err)
		}
	}
	meta := strings.Join(append(append([]string(nil), d.Aliases...), d.Tags...), "\n")
	for _, field := range []string{fieldMetaEN, fieldMetaZH} {
		if err := add(field, meta); err != nil {
			doc.Free()
			return nil, fmt.Errorf("vaultsearch: index metadata for %q: %w", d.ID, err)
		}
	}
	return doc, nil
}

func validateDocument(d Document) error {
	if d.ID == "" || d.Path == "" {
		return errors.New("vaultsearch: document ID and path are required")
	}
	if !utf8.ValidString(d.ID) || !utf8.ValidString(d.Path) || !utf8.ValidString(d.Title) || !utf8.ValidString(d.Source) || !utf8.ValidString(d.Revision) {
		return fmt.Errorf("vaultsearch: document %q contains invalid UTF-8", d.ID)
	}
	return nil
}

func sourceDigest(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func hanOnly(source string) string {
	var out strings.Builder
	for _, r := range source {
		if unicode.Is(unicode.Han, r) {
			out.WriteRune(r)
		} else if out.Len() > 0 {
			out.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func (i *Index) close() error {
	s, err := i.getState()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("vaultsearch: index already closed")
	}
	s.closed = true
	engineErr := s.engine.Close()
	lockErr := s.fileLock.Unlock()
	return errors.Join(wrapCloseError("close Tantivy index", engineErr), wrapCloseError("release index lock", lockErr))
}

func wrapCloseError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("vaultsearch: %s: %w", action, err)
}

// A conservative guard prevents invalid numeric scores from disrupting sort.
func safeScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}
