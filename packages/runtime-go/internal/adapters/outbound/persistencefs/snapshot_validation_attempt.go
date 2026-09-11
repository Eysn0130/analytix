package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
)

const (
	startupSemanticValidatorSchemaVersionV1   = 1
	maxStartupSemanticValidationMemoEntriesV1 = 100_000
	startupEventSequenceModeNoneV1            = "none"
	startupEventSequenceModeStrictV1          = "strict_physical_v1"
)

// StartupSemanticValidationAttemptV1 owns an in-memory, process-local memo for
// one startup attempt. It never persists a validation result and cannot skip a
// byte read: snapshotScanner consults it only after recomputing the complete
// file SHA-256 under the current identity and fixed-point checks.
type StartupSemanticValidationAttemptV1 struct {
	memo *startupSemanticValidationMemoV1
}

type startupSemanticValidationContextKeyV1 struct{}

type startupSemanticValidationProfileV1 struct {
	ValidatorSchemaVersion int
	RootBindingDigest      string
	Extension              string
	TargetLabel            string
	RelativePath           string
	ThreadTree             bool
	StrictJSONLFraming     bool
	EventSequenceMode      string
}

type startupSemanticValidationMemoKeyV1 [sha256.Size]byte
type startupSemanticValidationCandidateKeyV1 [sha256.Size]byte

type startupSemanticValidationMemoEntryV1 struct {
	RecordCount int
}

type startupSemanticValidationMemoV1 struct {
	mu         sync.Mutex
	entries    map[startupSemanticValidationMemoKeyV1]startupSemanticValidationMemoEntryV1
	candidates map[startupSemanticValidationCandidateKeyV1]struct{}
	limit      int
	hits       int
	misses     int
	decodes    int
}

func NewStartupSemanticValidationAttemptV1() *StartupSemanticValidationAttemptV1 {
	return &StartupSemanticValidationAttemptV1{memo: &startupSemanticValidationMemoV1{
		entries:    map[startupSemanticValidationMemoKeyV1]startupSemanticValidationMemoEntryV1{},
		candidates: map[startupSemanticValidationCandidateKeyV1]struct{}{},
		limit:      maxStartupSemanticValidationMemoEntriesV1,
	}}
}

func (attempt *StartupSemanticValidationAttemptV1) WithContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if attempt == nil || attempt.memo == nil {
		return ctx
	}
	return context.WithValue(ctx, startupSemanticValidationContextKeyV1{}, attempt.memo)
}

func startupSemanticValidationMemoFromContextV1(ctx context.Context) *startupSemanticValidationMemoV1 {
	if ctx == nil {
		return nil
	}
	memo, _ := ctx.Value(startupSemanticValidationContextKeyV1{}).(*startupSemanticValidationMemoV1)
	return memo
}

func (memo *startupSemanticValidationMemoV1) reuse(
	path string,
	profile startupSemanticValidationProfileV1,
	hashed FileRecord,
) (FileRecord, bool) {
	key, valid := startupSemanticValidationMemoKey(profile, path, hashed)
	if memo == nil || !valid {
		return FileRecord{}, false
	}
	memo.mu.Lock()
	entry, ok := memo.entries[key]
	if ok {
		memo.hits++
	} else {
		memo.misses++
	}
	memo.mu.Unlock()
	if !ok || entry.RecordCount < 0 {
		return FileRecord{}, false
	}
	hashed.RecordCount = entry.RecordCount
	return hashed, true
}

func (memo *startupSemanticValidationMemoV1) remember(
	path string,
	profile startupSemanticValidationProfileV1,
	validated FileRecord,
) {
	key, valid := startupSemanticValidationMemoKey(profile, path, validated)
	if memo == nil || !valid || validated.RecordCount < 0 {
		return
	}
	candidateKey, candidateValid := startupSemanticValidationCandidateKey(profile, path)
	memo.mu.Lock()
	if _, exists := memo.entries[key]; exists || memo.limit > 0 && len(memo.entries) < memo.limit {
		memo.entries[key] = startupSemanticValidationMemoEntryV1{RecordCount: validated.RecordCount}
		if candidateValid {
			memo.candidates[candidateKey] = struct{}{}
		}
	}
	memo.mu.Unlock()
}

func (memo *startupSemanticValidationMemoV1) hasCandidate(
	path string,
	profile startupSemanticValidationProfileV1,
) bool {
	key, valid := startupSemanticValidationCandidateKey(profile, path)
	if memo == nil || !valid {
		return false
	}
	memo.mu.Lock()
	_, ok := memo.candidates[key]
	memo.mu.Unlock()
	return ok
}

func (memo *startupSemanticValidationMemoV1) noteSemanticDecode() {
	if memo == nil {
		return
	}
	memo.mu.Lock()
	memo.decodes++
	memo.mu.Unlock()
}

func startupSemanticValidationMemoKey(
	profile startupSemanticValidationProfileV1,
	path string,
	record FileRecord,
) (startupSemanticValidationMemoKeyV1, bool) {
	if path == "" || record.Path != path || profile.Extension == "" ||
		profile.ValidatorSchemaVersion != startupSemanticValidatorSchemaVersionV1 ||
		len(profile.RootBindingDigest) != 64 || !isLowerHex(profile.RootBindingDigest) ||
		record.Size < 0 || len(record.SHA256) != 64 || !isLowerHex(record.SHA256) {
		return startupSemanticValidationMemoKeyV1{}, false
	}
	body, err := json.Marshal(struct {
		SchemaVersion int                                `json:"schemaVersion"`
		Path          string                             `json:"path"`
		Profile       startupSemanticValidationProfileV1 `json:"profile"`
		SHA256        string                             `json:"sha256"`
		Size          int64                              `json:"size"`
	}{1, path, profile, record.SHA256, record.Size})
	if err != nil {
		return startupSemanticValidationMemoKeyV1{}, false
	}
	return sha256.Sum256(body), true
}

func startupSemanticValidationCandidateKey(
	profile startupSemanticValidationProfileV1,
	path string,
) (startupSemanticValidationCandidateKeyV1, bool) {
	if path == "" || profile.Extension == "" ||
		profile.ValidatorSchemaVersion != startupSemanticValidatorSchemaVersionV1 ||
		len(profile.RootBindingDigest) != 64 || !isLowerHex(profile.RootBindingDigest) {
		return startupSemanticValidationCandidateKeyV1{}, false
	}
	body, err := json.Marshal(struct {
		SchemaVersion int                                `json:"schemaVersion"`
		Path          string                             `json:"path"`
		Profile       startupSemanticValidationProfileV1 `json:"profile"`
	}{1, path, profile})
	if err != nil {
		return startupSemanticValidationCandidateKeyV1{}, false
	}
	return sha256.Sum256(body), true
}

type startupSemanticValidationMemoStatsV1 struct {
	Entries int
	Hits    int
	Misses  int
	Decodes int
}

func (attempt *StartupSemanticValidationAttemptV1) statsV1() startupSemanticValidationMemoStatsV1 {
	if attempt == nil || attempt.memo == nil {
		return startupSemanticValidationMemoStatsV1{}
	}
	attempt.memo.mu.Lock()
	defer attempt.memo.mu.Unlock()
	return startupSemanticValidationMemoStatsV1{
		Entries: len(attempt.memo.entries), Hits: attempt.memo.hits,
		Misses: attempt.memo.misses, Decodes: attempt.memo.decodes,
	}
}
