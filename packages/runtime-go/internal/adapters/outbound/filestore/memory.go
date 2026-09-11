package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmemory "analytix.local/runtime-go/internal/domain/memory"
)

type PersistentMemoryStore struct {
	root       string
	recordsDir string
	mu         sync.Mutex
	seq        int
}

func NewPersistentMemoryStore(dataDir string) (*PersistentMemoryStore, error) {
	root := filepath.Join(dataDir, "memory")
	store := &PersistentMemoryStore{
		root:       root,
		recordsDir: filepath.Join(root, "records"),
	}
	if err := os.MkdirAll(store.recordsDir, 0o700); err != nil {
		return nil, err
	}
	if err := store.scrubLegacyTombstones(); err != nil {
		return nil, err
	}
	seq, err := maxPrefixedRecordSeq(store.recordsDir, "mem_go_")
	if err != nil {
		return nil, err
	}
	store.seq = seq
	return store, nil
}

func (s *PersistentMemoryStore) List(includeDeleted bool, workspace string) ([]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readAllNoLock()
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(records))
	for _, memory := range records {
		if !includeDeleted && stringField(memory, "deletedAt") != "" {
			continue
		}
		if workspace != "" && stringField(memory, "workspace") != workspace {
			continue
		}
		out = append(out, contracts.CloneMap(memory))
	}
	return out, nil
}

func (s *PersistentMemoryStore) Create(body map[string]any) (map[string]any, error) {
	if err := validateMemoryMutationKeysV1(body, map[string]struct{}{
		"content": {}, "scope": {}, "workspace": {}, "project": {}, "tags": {}, "confidence": {},
	}); err != nil {
		return nil, err
	}
	if err := validateMemoryMutationValuesV1(body, true); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(stringField(body, "content"))
	if err := domainmemory.ValidateManualContentV1(content); err != nil {
		return nil, err
	}
	scope, err := validatedMemoryScope(body["scope"], true)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq += 1
	id := fmt.Sprintf("mem_go_%d", s.seq)
	memory := map[string]any{
		"id":             id,
		"content":        content,
		"scope":          scope,
		"tags":           []any{},
		"confidence":     float64(1),
		"createdAt":      now,
		"updatedAt":      now,
		"provenance":     "manual-general",
		"captureMode":    "manual",
		"modelInjection": false,
	}
	for _, key := range []string{"workspace", "project"} {
		if value := strings.TrimSpace(stringField(body, key)); value != "" {
			memory[key] = value
		}
	}
	if tags, ok := body["tags"].([]any); ok {
		memory["tags"] = contracts.CloneValue(tags)
	}
	if confidence, ok := body["confidence"]; ok {
		memory["confidence"] = contracts.CloneValue(confidence)
	}
	if err := WriteJSONMapFile(s.recordPath(id), memory); err != nil {
		return nil, err
	}
	return contracts.CloneMap(memory), nil
}

func (s *PersistentMemoryStore) Patch(id string, body map[string]any) (map[string]any, bool, error) {
	if err := validateMemoryMutationKeysV1(body, map[string]struct{}{
		"content": {}, "tags": {}, "confidence": {}, "disabled": {},
	}); err != nil {
		return nil, false, err
	}
	if err := validateMemoryMutationValuesV1(body, false); err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !domainmemory.IsCanonicalRecordID(id) {
		return nil, false, nil
	}
	memory, err := ReadJSONMapFile(s.recordPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if stringField(memory, "id") != id {
		return nil, false, nil
	}
	if !isAdmissibleStoredMemoryRecordV1(memory) {
		return nil, false, domainmemory.ErrMutationNotAdmissibleV1
	}
	if stringField(memory, "deletedAt") != "" {
		return nil, false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if value, ok := body["content"]; ok {
		content, ok := value.(string)
		if !ok {
			return nil, false, errors.New("memory content is not admissible")
		}
		content = strings.TrimSpace(content)
		if err := domainmemory.ValidateManualContentV1(content); err != nil {
			return nil, false, err
		}
		memory["content"] = content
	}
	if tags, ok := body["tags"]; ok {
		memory["tags"] = contracts.CloneValue(tags)
	}
	if confidence, ok := body["confidence"]; ok {
		memory["confidence"] = contracts.CloneValue(confidence)
	}
	if disabled, ok := body["disabled"].(bool); ok {
		if disabled {
			memory["disabledAt"] = now
		} else {
			delete(memory, "disabledAt")
		}
	}
	memory["updatedAt"] = now
	memory["provenance"] = "manual-general"
	memory["captureMode"] = "manual"
	memory["modelInjection"] = false
	if err := WriteJSONMapFile(s.recordPath(id), memory); err != nil {
		return nil, false, err
	}
	return contracts.CloneMap(memory), true, nil
}

func (s *PersistentMemoryStore) Delete(id string) (map[string]any, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !domainmemory.IsCanonicalRecordID(id) {
		return nil, false, nil
	}
	memory, err := ReadJSONMapFile(s.recordPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if stringField(memory, "id") != id {
		return nil, false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tombstone := privacyPreservingMemoryTombstone(memory, id, now)
	if err := WriteJSONMapFile(s.recordPath(id), tombstone); err != nil {
		return nil, false, err
	}
	return contracts.CloneMap(tombstone), true, nil
}

func (s *PersistentMemoryStore) scrubLegacyTombstones() error {
	entries, err := os.ReadDir(s.recordsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if !domainmemory.IsCanonicalRecordID(fileID) {
			continue
		}
		path := filepath.Join(s.recordsDir, entry.Name())
		memory, err := ReadJSONMapFile(path)
		if err != nil || stringField(memory, "id") != fileID || stringField(memory, "deletedAt") == "" || isPrivacyPreservingMemoryTombstone(memory) {
			continue
		}
		tombstone := privacyPreservingMemoryTombstone(memory, fileID, stringField(memory, "deletedAt"))
		if err := WriteJSONMapFile(path, tombstone); err != nil {
			return err
		}
	}
	return nil
}

func privacyPreservingMemoryTombstone(memory map[string]any, fallbackID string, deletedAt string) map[string]any {
	id := stringField(memory, "id")
	if id == "" {
		id = fallbackID
	}
	if deletedAt == "" {
		deletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	createdAt := stringField(memory, "createdAt")
	if createdAt == "" {
		createdAt = deletedAt
	}
	updatedAt := stringField(memory, "updatedAt")
	if stringField(memory, "deletedAt") == "" || updatedAt == "" {
		updatedAt = deletedAt
	}
	return map[string]any{
		"id":        id,
		"createdAt": createdAt,
		"updatedAt": updatedAt,
		"deletedAt": deletedAt,
	}
}

func isPrivacyPreservingMemoryTombstone(memory map[string]any) bool {
	if len(memory) != 4 {
		return false
	}
	for _, key := range []string{"id", "createdAt", "updatedAt", "deletedAt"} {
		if stringField(memory, key) == "" {
			return false
		}
	}
	return true
}

func (s *PersistentMemoryStore) Diagnostics() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readAllNoLock()
	if err != nil {
		return map[string]any{
			"enabled":            true,
			"status":             "unavailable",
			"reasonCode":         "memory_store_read_failed",
			"rootDir":            filepath.ToSlash(s.root),
			"lastInjectedIds":    []any{},
			"admittedProvenance": "manual-general",
			"captureMode":        "manual",
			"modelInjection":     false,
		}
	}
	active := 0
	tombstone := 0
	for _, memory := range records {
		if stringField(memory, "deletedAt") != "" {
			tombstone += 1
		} else {
			active += 1
		}
	}
	return map[string]any{
		"enabled":            true,
		"status":             "ok",
		"rootDir":            filepath.ToSlash(s.root),
		"activeCount":        float64(active),
		"tombstoneCount":     float64(tombstone),
		"lastInjectedIds":    []any{},
		"admittedProvenance": "manual-general",
		"captureMode":        "manual",
		"modelInjection":     false,
	}
}

func validateMemoryMutationKeysV1(body map[string]any, allowed map[string]struct{}) error {
	if body == nil {
		return domainmemory.ErrMutationNotAdmissibleV1
	}
	for key := range body {
		if _, ok := allowed[key]; !ok {
			return domainmemory.ErrMutationNotAdmissibleV1
		}
	}
	return nil
}

func validateMemoryMutationValuesV1(body map[string]any, create bool) error {
	for _, key := range []string{"workspace", "project"} {
		if value, found := body[key]; found {
			text, ok := value.(string)
			if !ok || text != strings.TrimSpace(text) || text == "" || len(text) > 4096 {
				return domainmemory.ErrMutationNotAdmissibleV1
			}
		}
	}
	if value, found := body["tags"]; found {
		tags, ok := value.([]any)
		if !ok || len(tags) > 64 {
			return domainmemory.ErrMutationNotAdmissibleV1
		}
		for _, raw := range tags {
			tag, ok := raw.(string)
			if !ok || tag != strings.TrimSpace(tag) || tag == "" || len(tag) > 256 ||
				domainmemory.ValidateManualContentV1(tag) != nil {
				return domainmemory.ErrMutationNotAdmissibleV1
			}
		}
	}
	if value, found := body["confidence"]; found {
		confidence, ok := value.(float64)
		if !ok || confidence < 0 || confidence > 1 {
			return domainmemory.ErrMutationNotAdmissibleV1
		}
	}
	if value, found := body["disabled"]; found {
		if _, ok := value.(bool); !ok || create {
			return domainmemory.ErrMutationNotAdmissibleV1
		}
	}
	return nil
}

func (s *PersistentMemoryStore) readAllNoLock() ([]map[string]any, error) {
	entries, err := os.ReadDir(s.recordsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := []map[string]any{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if !domainmemory.IsCanonicalRecordID(fileID) {
			continue
		}
		path := filepath.Join(s.recordsDir, entry.Name())
		record, err := ReadJSONMapFile(path)
		if err != nil {
			return nil, fmt.Errorf("read memory record %s: %w", fileID, err)
		}
		if stringField(record, "id") != fileID {
			return nil, fmt.Errorf("memory record %s id does not match filename", fileID)
		}
		if !isAdmissibleStoredMemoryRecordV1(record) {
			continue
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		left := stringField(records[i], "updatedAt")
		right := stringField(records[j], "updatedAt")
		if left == right {
			return stringField(records[i], "id") < stringField(records[j], "id")
		}
		return left > right
	})
	return records, nil
}

func isAdmissibleStoredMemoryRecordV1(record map[string]any) bool {
	if isPrivacyPreservingMemoryTombstone(record) {
		return true
	}
	if stringField(record, "deletedAt") != "" {
		return false
	}
	allowed := map[string]struct{}{
		"id": {}, "content": {}, "scope": {}, "workspace": {}, "project": {}, "tags": {}, "confidence": {},
		"createdAt": {}, "updatedAt": {}, "disabledAt": {}, "deletedAt": {},
		"provenance": {}, "captureMode": {}, "modelInjection": {},
	}
	for key := range record {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	if stringField(record, "provenance") != "manual-general" || stringField(record, "captureMode") != "manual" {
		return false
	}
	modelInjection, ok := record["modelInjection"].(bool)
	if !ok || modelInjection {
		return false
	}
	if _, err := validatedMemoryScope(record["scope"], false); err != nil {
		return false
	}
	storedMetadata := map[string]any{}
	for _, key := range []string{"workspace", "project", "tags", "confidence"} {
		if value, found := record[key]; found {
			storedMetadata[key] = value
		}
	}
	if err := validateMemoryMutationValuesV1(storedMetadata, true); err != nil {
		return false
	}
	content, ok := record["content"].(string)
	return ok && domainmemory.ValidateManualContentV1(content) == nil
}

func (s *PersistentMemoryStore) recordPath(id string) string {
	if !domainmemory.IsCanonicalRecordID(id) {
		return ""
	}
	return filepath.Join(s.recordsDir, id+".json")
}

func validatedMemoryScope(raw any, allowDefault bool) (string, error) {
	if raw == nil {
		if allowDefault {
			return "workspace", nil
		}
		return "", fmt.Errorf("memory scope must be user, workspace, or project")
	}
	scope, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("memory scope must be user, workspace, or project")
	}
	scope = strings.TrimSpace(scope)
	switch scope {
	case "user", "workspace", "project":
		return scope, nil
	default:
		return "", fmt.Errorf("memory scope must be user, workspace, or project")
	}
}

func maxPrefixedRecordSeq(dir string, prefix string) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	maxSeq := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if prefix != domainmemory.RecordIDPrefix || !domainmemory.IsCanonicalRecordID(id) {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
		if err == nil && seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq, nil
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}
