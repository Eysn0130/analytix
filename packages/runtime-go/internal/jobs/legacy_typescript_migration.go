package jobs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	legacyTypeScriptParentGoalIDV1       = "legacy-unbound"
	legacyTypeScriptSourceRefV1          = "analytix-legacy-typescript-child-run/v1/sha256/"
	legacyTypeScriptLineageWitnessDomain = "analytix.legacy-typescript-lineage-witness/v1"
	legacyTypeScriptMaxIDBytesV1         = 256
)

// LegacyTypeScriptLineageV1 is the only retired-record material that may be
// retained. The semantic-startup owner must prove these parent references
// against the same staged durable-thread fixed point before projection. Child
// references are intentionally absent because the retired producer could name
// a child thread that was never durably created.
type LegacyTypeScriptLineageV1 struct {
	SourceName       string
	SourceSHA256     string
	ParentThreadID   string
	ParentTurnID     string
	ParentToolCallID string
}

type LegacyTypeScriptLineageVerifierV1 interface {
	VerifyLegacyTypeScriptLineageV1(LegacyTypeScriptLineageV1) error
}

// LegacyTypeScriptLineageObservationEntryV1 contains only the exact source
// binding and parent references needed for the host to verify lineage before
// semantic redaction. Prompt, transcript, output, usage, and model fields are
// never returned by this observer.
type LegacyTypeScriptLineageObservationEntryV1 struct {
	SourceName   string
	SourceSHA256 string
	Lineage      LegacyTypeScriptLineageV1
}

// LegacyTypeScriptLineageObservationV1 is an ephemeral planning value. The
// runtime converts it to a hash-only witness before any durable parent tool
// payload is removed; it is never persisted or exposed as public authority.
type LegacyTypeScriptLineageObservationV1 struct {
	ChildRunRoot            string
	InventoryManifestSHA256 string
	Entries                 []LegacyTypeScriptLineageObservationEntryV1
}

// FrozenLegacyTypeScriptLineageWitnessV1 is the only witness type accepted by
// semantic-startup child-run migration. Its fields are intentionally private:
// callers can obtain one only by asking this package to observe the exact
// source inventory and verify every lineage against a host authority before
// redaction. It retains hashes, never raw parent identifiers or tool payloads.
type FrozenLegacyTypeScriptLineageWitnessV1 struct {
	childRunRoot     string
	inventorySHA256  string
	legacyEntryCount int
	verified         map[string]struct{}
}

type LegacyTypeScriptLineageVerifyFuncV1 func(LegacyTypeScriptLineageV1) error

func (verify LegacyTypeScriptLineageVerifyFuncV1) VerifyLegacyTypeScriptLineageV1(lineage LegacyTypeScriptLineageV1) error {
	if verify == nil {
		return errors.New("legacy TypeScript child-run lineage verifier is unavailable")
	}
	return verify(lineage)
}

// BuildLegacyTypeScriptLineageObservationV1 observes the retired child-run
// sources through the identity- and SHA-bound inventory. It deliberately does
// not trust their parent references: the runtime must still prove each exact
// tuple against the staged durable thread store before creating a witness.
func BuildLegacyTypeScriptLineageObservationV1(root string) (LegacyTypeScriptLineageObservationV1, error) {
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		return LegacyTypeScriptLineageObservationV1{}, err
	}
	observation := LegacyTypeScriptLineageObservationV1{
		ChildRunRoot:            inventory.Root,
		InventoryManifestSHA256: inventory.ManifestSHA256,
		Entries:                 []LegacyTypeScriptLineageObservationEntryV1{},
	}
	for _, entry := range inventory.Entries {
		if entry.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 {
			continue
		}
		raw, err := readChildRunInventoryEntryV1(inventory.Root, entry)
		if err != nil {
			return LegacyTypeScriptLineageObservationV1{}, err
		}
		legacy, err := decodeLegacyTypeScriptLineageSourceV1(raw, entry)
		if err != nil {
			return LegacyTypeScriptLineageObservationV1{}, err
		}
		lineage := LegacyTypeScriptLineageV1{
			SourceName: entry.Name, SourceSHA256: entry.SHA256,
			ParentThreadID: legacy.ParentThreadID, ParentTurnID: legacy.ParentTurnID,
			ParentToolCallID: legacy.ParentToolCallID,
		}
		observation.Entries = append(observation.Entries, LegacyTypeScriptLineageObservationEntryV1{
			SourceName: entry.Name, SourceSHA256: entry.SHA256, Lineage: lineage,
		})
	}
	if err := ValidateChildRunInventoryV1(inventory.Root, inventory); err != nil {
		return LegacyTypeScriptLineageObservationV1{}, err
	}
	return observation, nil
}

// FreezeLegacyTypeScriptLineageWitnessV1 proves every exact source-bound
// parent tuple before redaction and returns a concrete, non-forgeable-by-data
// witness for the later semantic projection step. A missing parent, storage
// failure, ambiguous call, or any other verification error fails closed; this
// migration never infers an orphan from an incomplete public view.
func FreezeLegacyTypeScriptLineageWitnessV1(
	root string,
	hostVerifier LegacyTypeScriptLineageVerifierV1,
) (*FrozenLegacyTypeScriptLineageWitnessV1, error) {
	if hostVerifier == nil {
		return nil, errors.New("legacy TypeScript child-run host lineage verifier is unavailable")
	}
	observation, err := BuildLegacyTypeScriptLineageObservationV1(root)
	if err != nil || strings.TrimSpace(observation.ChildRunRoot) == "" ||
		strings.TrimSpace(observation.InventoryManifestSHA256) == "" {
		return nil, errors.New("legacy TypeScript child-run lineage observation failed")
	}
	witness := &FrozenLegacyTypeScriptLineageWitnessV1{
		childRunRoot:     strings.TrimSpace(observation.ChildRunRoot),
		inventorySHA256:  strings.TrimSpace(observation.InventoryManifestSHA256),
		legacyEntryCount: len(observation.Entries),
		verified:         make(map[string]struct{}, len(observation.Entries)),
	}
	for _, entry := range observation.Entries {
		if entry.SourceName != entry.Lineage.SourceName || entry.SourceSHA256 != entry.Lineage.SourceSHA256 {
			return nil, errors.New("legacy TypeScript child-run lineage observation source binding is invalid")
		}
		key, err := legacyTypeScriptLineageWitnessKeyV1(entry.Lineage)
		if err != nil {
			return nil, err
		}
		if _, duplicate := witness.verified[key]; duplicate {
			return nil, errors.New("legacy TypeScript child-run lineage observation is duplicated")
		}
		if err := hostVerifier.VerifyLegacyTypeScriptLineageV1(entry.Lineage); err != nil {
			return nil, errors.New("legacy TypeScript child-run lineage could not be proven before redaction")
		}
		witness.verified[key] = struct{}{}
	}
	return witness, nil
}

func (witness *FrozenLegacyTypeScriptLineageWitnessV1) verifySourceInventoryV1(
	inventory ChildRunInventoryV1,
) error {
	if witness == nil || inventory.SchemaVersion != ChildRunInventorySchemaVersionV1 ||
		strings.TrimSpace(inventory.Root) != witness.childRunRoot ||
		strings.TrimSpace(inventory.ManifestSHA256) != witness.inventorySHA256 {
		return errors.New("legacy TypeScript child-run inventory does not match the pre-redaction witness")
	}
	legacyCount := 0
	for _, entry := range inventory.Entries {
		if entry.Kind == ChildRunInventoryLegacyTypeScriptRecordV1 {
			legacyCount++
		}
	}
	if legacyCount != witness.legacyEntryCount || len(witness.verified) != witness.legacyEntryCount {
		return errors.New("legacy TypeScript child-run inventory count does not match the pre-redaction witness")
	}
	return nil
}

func (witness *FrozenLegacyTypeScriptLineageWitnessV1) VerifyLegacyTypeScriptLineageV1(
	lineage LegacyTypeScriptLineageV1,
) error {
	if witness == nil {
		return errors.New("legacy TypeScript child-run lineage witness is unavailable")
	}
	key, err := legacyTypeScriptLineageWitnessKeyV1(lineage)
	if err != nil {
		return err
	}
	if _, ok := witness.verified[key]; !ok {
		return errors.New("legacy TypeScript child-run lineage is absent from the pre-redaction witness")
	}
	return nil
}

func legacyTypeScriptLineageWitnessKeyV1(lineage LegacyTypeScriptLineageV1) (string, error) {
	sourceDigest := strings.TrimSpace(lineage.SourceSHA256)
	decodedDigest, err := hex.DecodeString(sourceDigest)
	if err != nil || len(decodedDigest) != sha256.Size || hex.EncodeToString(decodedDigest) != sourceDigest {
		return "", errors.New("legacy TypeScript child-run lineage source digest is invalid")
	}
	fields := []string{
		strings.TrimSpace(lineage.SourceName), sourceDigest,
		strings.TrimSpace(lineage.ParentThreadID), strings.TrimSpace(lineage.ParentTurnID),
		strings.TrimSpace(lineage.ParentToolCallID),
	}
	for _, field := range fields {
		if field == "" {
			return "", errors.New("legacy TypeScript child-run lineage witness field is missing")
		}
	}
	digest := sha256.Sum256([]byte(legacyTypeScriptLineageWitnessDomain + "\x00" + strings.Join(fields, "\x00")))
	return hex.EncodeToString(digest[:]), nil
}

func decodeLegacyTypeScriptLineageSourceV1(
	raw []byte,
	entry ChildRunInventoryEntryV1,
) (legacyTypeScriptChildRunRecordV1, error) {
	legacyID, exactLegacyName := exactLegacyTypeScriptChildRunNameV1(entry.Name)
	if entry.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 || !exactLegacyName ||
		legacyID != entry.JobID || !isLowerSHA256V1(entry.SHA256) || legacyProjectionSHA256HexV1(raw) != entry.SHA256 {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run observation binding is invalid")
	}
	mode := os.FileMode(entry.Mode)
	permissions := mode.Perm()
	if permissions&0o400 == 0 || permissions&0o022 != 0 || permissions&0o111 != 0 ||
		mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run source permissions are unsafe")
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: childRunInventoryRecordMaxBytesV1, MaxDepth: 256,
		MaxTokens: 200_000, MaxStringBytes: 8 * 1024 * 1024, MaxNumberBytes: 256, MaxAbsExponent: 10_000,
	}); err != nil {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var legacy legacyTypeScriptChildRunRecordV1
	if err := decoder.Decode(&legacy); err != nil {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run shape is invalid")
	}
	if legacy.ID != entry.JobID || entry.Name != legacy.ID+".json" || legacy.ChildThreadID != legacy.ID {
		return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run observation identity is invalid")
	}
	for _, value := range []string{legacy.ParentThreadID, legacy.ParentTurnID, legacy.ParentToolCallID} {
		if !validLegacyTypeScriptReferenceV1(value) {
			return legacyTypeScriptChildRunRecordV1{}, errors.New("legacy TypeScript child-run observation reference is invalid")
		}
	}
	return legacy, nil
}

// legacyTypeScriptChildRunRecordV1 is the complete retired TypeScript
// ChildRunRecord wire shape. Model/provider text, summaries, prompt,
// transcript, usage, workspace, and old evidence flags are accepted only as
// bounded migration input and are deliberately not copied into current durable
// state. Keeping the closed top-level shape lets unknown historical producers
// remain fail-closed without treating arbitrary JSON as a legacy record.
type legacyTypeScriptChildRunRecordV1 struct {
	ID                    string          `json:"id"`
	ParentThreadID        string          `json:"parentThreadId"`
	ParentTurnID          string          `json:"parentTurnId"`
	ParentToolCallID      string          `json:"parentToolCallId,omitempty"`
	ChildThreadID         string          `json:"childThreadId,omitempty"`
	ChildTurnID           string          `json:"childTurnId,omitempty"`
	Label                 string          `json:"label,omitempty"`
	Prompt                string          `json:"prompt"`
	Workspace             string          `json:"workspace,omitempty"`
	Model                 string          `json:"model,omitempty"`
	ProviderID            string          `json:"providerId,omitempty"`
	EndpointFormat        string          `json:"endpointFormat,omitempty"`
	Variant               string          `json:"variant,omitempty"`
	ModelSource           string          `json:"modelSource,omitempty"`
	ModelExecution        json.RawMessage `json:"modelExecution,omitempty"`
	Effort                string          `json:"effort,omitempty"`
	MaxModelSteps         *int            `json:"maxModelSteps,omitempty"`
	Profile               string          `json:"profile,omitempty"`
	ToolPolicy            string          `json:"toolPolicy,omitempty"`
	ToolScope             []string        `json:"toolScope,omitempty"`
	PromptPreambleHash    string          `json:"promptPreambleHash,omitempty"`
	ApprovalPolicy        string          `json:"approvalPolicy,omitempty"`
	SandboxMode           string          `json:"sandboxMode,omitempty"`
	Status                string          `json:"status"`
	Summary               string          `json:"summary,omitempty"`
	Error                 string          `json:"error,omitempty"`
	Usage                 json.RawMessage `json:"usage"`
	PrefixReused          *bool           `json:"prefixReused,omitempty"`
	InheritedHistoryItems *int            `json:"inheritedHistoryItems,omitempty"`
	ToolInvocations       *int            `json:"toolInvocations,omitempty"`
	TranscriptItems       json.RawMessage `json:"transcriptItems,omitempty"`
	EvidenceLedgered      *bool           `json:"evidenceLedgered,omitempty"`
	EvidenceLedgerError   string          `json:"evidenceLedgerError,omitempty"`
	CacheDiagnostics      json.RawMessage `json:"cacheDiagnostics,omitempty"`
	DurationMs            *int            `json:"durationMs,omitempty"`
	QueuedMs              *int            `json:"queuedMs,omitempty"`
	CreatedAt             string          `json:"createdAt"`
	StartedAt             string          `json:"startedAt,omitempty"`
	UpdatedAt             string          `json:"updatedAt"`
}

type legacyTypeScriptProjectionReadbackV1 struct {
	SourceName string
	TargetName string
	Bytes      []byte
}

func projectLegacyTypeScriptChildRunV1(
	raw []byte,
	entry ChildRunInventoryEntryV1,
	newJobID string,
	lineageVerifier LegacyTypeScriptLineageVerifierV1,
) (Record, error) {
	legacyID, exactLegacyName := exactLegacyTypeScriptChildRunNameV1(entry.Name)
	if entry.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 || !exactLegacyName ||
		legacyID != entry.JobID || !validPersistedJobID(newJobID) || !isLowerSHA256V1(entry.SHA256) {
		return Record{}, errors.New("legacy TypeScript child-run migration binding is invalid")
	}
	mode := os.FileMode(entry.Mode)
	permissions := mode.Perm()
	if permissions&0o400 == 0 || permissions&0o022 != 0 || permissions&0o111 != 0 ||
		mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return Record{}, errors.New("legacy TypeScript child-run source permissions are unsafe")
	}
	if legacyProjectionSHA256HexV1(raw) != entry.SHA256 {
		return Record{}, errors.New("legacy TypeScript child-run source digest is invalid")
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       childRunInventoryRecordMaxBytesV1,
		MaxDepth:       256,
		MaxTokens:      200_000,
		MaxStringBytes: 8 * 1024 * 1024,
		MaxNumberBytes: 256,
		MaxAbsExponent: 10_000,
	}); err != nil {
		return Record{}, errors.New("legacy TypeScript child-run JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var legacy legacyTypeScriptChildRunRecordV1
	if err := decoder.Decode(&legacy); err != nil {
		return Record{}, errors.New("legacy TypeScript child-run shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Record{}, errors.New("legacy TypeScript child-run shape is invalid")
	}
	if legacy.ID != entry.JobID || entry.Name != legacy.ID+".json" {
		return Record{}, errors.New("legacy TypeScript child-run identity is invalid")
	}
	if legacy.ChildThreadID != legacy.ID {
		return Record{}, errors.New("legacy TypeScript child-run producer identity is invalid")
	}
	for _, value := range []string{
		legacy.ID,
		legacy.ParentThreadID,
		legacy.ParentTurnID,
		legacy.ParentToolCallID,
		legacy.ChildThreadID,
		legacy.ChildTurnID,
	} {
		if value != "" && !validLegacyTypeScriptReferenceV1(value) {
			return Record{}, errors.New("legacy TypeScript child-run reference is invalid")
		}
	}
	if strings.TrimSpace(legacy.ParentThreadID) == "" || strings.TrimSpace(legacy.ParentTurnID) == "" ||
		strings.TrimSpace(legacy.ParentToolCallID) == "" || strings.TrimSpace(legacy.Prompt) == "" {
		return Record{}, errors.New("legacy TypeScript child-run required fields are missing")
	}
	if legacy.Status != string(domainjob.StatusCompleted) && legacy.Status != string(domainjob.StatusFailed) &&
		legacy.Status != string(domainjob.StatusAborted) {
		return Record{}, errors.New("legacy TypeScript child-run is not terminal")
	}
	if err := validateLegacyTypeScriptDiscardedContainerV1(legacy.Usage, '{', true); err != nil ||
		validateLegacyTypeScriptDiscardedContainerV1(legacy.ModelExecution, '{', false) != nil ||
		validateLegacyTypeScriptDiscardedContainerV1(legacy.CacheDiagnostics, '{', false) != nil ||
		validateLegacyTypeScriptDiscardedContainerV1(legacy.TranscriptItems, '[', false) != nil {
		return Record{}, errors.New("legacy TypeScript child-run private container is invalid")
	}
	for _, value := range []*int{
		legacy.MaxModelSteps,
		legacy.InheritedHistoryItems,
		legacy.ToolInvocations,
		legacy.DurationMs,
		legacy.QueuedMs,
	} {
		if value != nil && *value < 0 {
			return Record{}, errors.New("legacy TypeScript child-run counter is invalid")
		}
	}
	createdAt, err := normalizeLegacyTypeScriptTimeV1(legacy.CreatedAt)
	if err != nil {
		return Record{}, err
	}
	updatedAt, err := normalizeLegacyTypeScriptTimeV1(legacy.UpdatedAt)
	if err != nil {
		return Record{}, errors.New("legacy TypeScript child-run update time is invalid")
	}
	createdTime, _ := time.Parse(time.RFC3339Nano, createdAt)
	producerMillis, parseProducerTimeErr := strconv.ParseInt(legacy.ID[len("child_"):len("child_")+8], 36, 64)
	if parseProducerTimeErr != nil || producerMillis != createdTime.UnixMilli() {
		return Record{}, errors.New("legacy TypeScript child-run producer time binding is invalid")
	}
	updatedTime, _ := time.Parse(time.RFC3339Nano, updatedAt)
	if updatedTime.Before(createdTime) {
		return Record{}, errors.New("legacy TypeScript child-run update time is invalid")
	}
	if strings.TrimSpace(legacy.StartedAt) == "" || legacy.QueuedMs == nil || legacy.DurationMs == nil || legacy.ToolInvocations == nil {
		return Record{}, errors.New("legacy TypeScript child-run terminal timing is incomplete")
	}
	startedAt, err := normalizeLegacyTypeScriptTimeV1(legacy.StartedAt)
	if err != nil {
		return Record{}, errors.New("legacy TypeScript child-run start time is invalid")
	}
	startedTime, _ := time.Parse(time.RFC3339Nano, startedAt)
	if startedTime.Before(createdTime) || startedTime.After(updatedTime) ||
		int64(*legacy.QueuedMs) != startedTime.Sub(createdTime).Milliseconds() ||
		int64(*legacy.DurationMs) != updatedTime.Sub(startedTime).Milliseconds() {
		return Record{}, errors.New("legacy TypeScript child-run start time is invalid")
	}
	if lineageVerifier == nil {
		return Record{}, errors.New("legacy TypeScript child-run parent lineage verifier is unavailable")
	}
	lineageErr := lineageVerifier.VerifyLegacyTypeScriptLineageV1(LegacyTypeScriptLineageV1{
		SourceName: entry.Name, SourceSHA256: entry.SHA256,
		ParentThreadID: legacy.ParentThreadID, ParentTurnID: legacy.ParentTurnID, ParentToolCallID: legacy.ParentToolCallID,
	})
	if lineageErr != nil {
		return Record{}, errors.New("legacy TypeScript child-run parent lineage is not present in the staged durable thread")
	}
	queuedMs := 0
	if legacy.QueuedMs != nil {
		queuedMs = *legacy.QueuedMs
	}
	record := Record{
		ID:               newJobID,
		ParentGoalID:     legacyTypeScriptParentGoalIDV1,
		ParentThreadID:   legacy.ParentThreadID,
		ParentTurnID:     legacy.ParentTurnID,
		ParentToolCallID: legacy.ParentToolCallID,
		Kind:             "child-run",
		Label:            "legacy child run",
		Status:           legacy.Status,
		LineageKey:       legacyTypeScriptParentGoalIDV1 + "/" + legacy.ParentThreadID,
		SourceRef:        legacyTypeScriptSourceRefV1 + entry.SHA256,
		QueuedAt:         createdAt,
		QueuedMs:         queuedMs,
		StartedAt:        startedAt,
		FinishedAt:       updatedAt,
		UpdatedAt:        updatedAt,
		FailureCode:      domainjob.NormalizeFailureCode("", legacy.Status, false),
		// The retired counter is not backed by current host receipts and cannot
		// be surfaced as verified execution. The source digest preserves audit
		// provenance without promoting that self-reported count.
		ToolInvocations: 0,
	}
	return domainjob.NormalizePersistedRecordV1(record), nil
}

func validLegacyTypeScriptReferenceV1(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > legacyTypeScriptMaxIDBytesV1 ||
		!utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validateLegacyTypeScriptDiscardedContainerV1(raw json.RawMessage, opening byte, required bool) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		if required {
			return errors.New("required private container is missing")
		}
		return nil
	}
	closing := byte('}')
	if opening == '[' {
		closing = ']'
	}
	if trimmed[0] != opening || trimmed[len(trimmed)-1] != closing {
		return errors.New("private container has the wrong shape")
	}
	return nil
}

func normalizeLegacyTypeScriptTimeV1(value string) (string, error) {
	const javaScriptISOStringLayoutV1 = "2006-01-02T15:04:05.000Z"
	parsed, err := time.Parse(javaScriptISOStringLayoutV1, value)
	if err != nil || parsed.IsZero() || parsed.UTC().Format(javaScriptISOStringLayoutV1) != value {
		return "", errors.New("legacy TypeScript child-run time is invalid")
	}
	return value, nil
}

func validateLegacyTypeScriptProjectionReadbackV1(
	root string,
	expected []legacyTypeScriptProjectionReadbackV1,
	requireSourceAbsent bool,
) error {
	if len(expected) == 0 {
		return nil
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		return errors.New("retired TypeScript child-run projection inventory failed")
	}
	byName := make(map[string]ChildRunInventoryEntryV1, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		byName[entry.Name] = entry
	}
	seenTargets := map[string]bool{}
	for _, projection := range expected {
		if projection.SourceName == "" || projection.TargetName == "" || projection.Bytes == nil ||
			seenTargets[projection.TargetName] {
			return errors.New("retired TypeScript child-run projection plan is invalid")
		}
		seenTargets[projection.TargetName] = true
		target, ok := byName[projection.TargetName]
		if !ok || target.Kind != ChildRunInventoryRecordV1 || target.Name != target.JobID+".json" ||
			os.FileMode(target.Mode).Perm() != 0o600 || target.SizeBytes != int64(len(projection.Bytes)) {
			return errors.New("retired TypeScript child-run projection readback is invalid")
		}
		digest := legacyProjectionSHA256HexV1(projection.Bytes)
		if target.SHA256 != digest {
			return errors.New("retired TypeScript child-run projection digest is invalid")
		}
		readback, err := readChildRunInventoryEntryV1(root, target)
		if err != nil || !bytes.Equal(readback, projection.Bytes) {
			return errors.New("retired TypeScript child-run projection bytes did not read back")
		}
		source, sourcePresent := byName[projection.SourceName]
		if requireSourceAbsent {
			if sourcePresent {
				return errors.New("retired TypeScript child-run source remains after projection")
			}
		} else if !sourcePresent || source.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 {
			return errors.New("retired TypeScript child-run source identity changed")
		}
	}
	return nil
}

func legacyProjectionSHA256HexV1(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func legacyTypeScriptTombstoneCandidateV1(record Record) bool {
	return strings.TrimSpace(record.ParentGoalID) == legacyTypeScriptParentGoalIDV1 ||
		strings.HasPrefix(strings.TrimSpace(record.SourceRef), legacyTypeScriptSourceRefV1)
}

// validateLegacyTypeScriptTombstoneV1 keeps the retired projection inert after
// it reaches the live manager. Only the parent provenance, terminal timing,
// deterministic sequence, counter, and non-authoritative source digest may
// survive. Any attempt to add child identity, output, receipts, execution
// binding, continuation, delivery, or mutable job state is rejected.
func validateLegacyTypeScriptTombstoneV1(record Record) error {
	if !legacyTypeScriptTombstoneCandidateV1(record) {
		return nil
	}
	expectedLabel := "legacy child run"
	if !validPersistedJobID(record.ID) || record.ChildSeq < 0 ||
		strings.TrimSpace(record.ParentGoalID) != legacyTypeScriptParentGoalIDV1 ||
		!validLegacyTypeScriptReferenceV1(record.ParentThreadID) ||
		!validLegacyTypeScriptReferenceV1(record.ParentTurnID) ||
		!validLegacyTypeScriptReferenceV1(record.ParentToolCallID) ||
		strings.TrimSpace(record.Kind) != "child-run" || strings.TrimSpace(record.Label) != expectedLabel ||
		strings.TrimSpace(record.LineageKey) != legacyTypeScriptParentGoalIDV1+"/"+strings.TrimSpace(record.ParentThreadID) ||
		!domainjob.TerminalStatusV1(record.Status) || record.ToolInvocations != 0 {
		return errors.New("legacy TypeScript child-run tombstone identity is invalid")
	}
	sourcePrefix := legacyTypeScriptSourceRefV1
	digest := strings.TrimPrefix(strings.TrimSpace(record.SourceRef), sourcePrefix)
	if !isLowerSHA256V1(digest) || record.SourceRef != sourcePrefix+digest {
		return errors.New("legacy TypeScript child-run tombstone source is invalid")
	}
	queuedAt, queuedErr := normalizeLegacyTypeScriptTimeV1(record.QueuedAt)
	startedAt, startedErr := normalizeLegacyTypeScriptTimeV1(record.StartedAt)
	finishedAt, finishedErr := normalizeLegacyTypeScriptTimeV1(record.FinishedAt)
	updatedAt, updatedErr := normalizeLegacyTypeScriptTimeV1(record.UpdatedAt)
	queuedTime, _ := time.Parse(time.RFC3339Nano, queuedAt)
	startedTime, _ := time.Parse(time.RFC3339Nano, startedAt)
	finishedTime, _ := time.Parse(time.RFC3339Nano, finishedAt)
	if queuedErr != nil || startedErr != nil || finishedErr != nil || updatedErr != nil || finishedAt != updatedAt ||
		startedTime.Before(queuedTime) || finishedTime.Before(startedTime) ||
		int64(record.QueuedMs) != startedTime.Sub(queuedTime).Milliseconds() {
		return errors.New("legacy TypeScript child-run tombstone timing is invalid")
	}
	expected := Record{
		ID: record.ID, ChildSeq: record.ChildSeq, ParentGoalID: legacyTypeScriptParentGoalIDV1,
		ParentThreadID: record.ParentThreadID, ParentTurnID: record.ParentTurnID, ParentToolCallID: record.ParentToolCallID,
		Kind: "child-run", Label: expectedLabel, Status: record.Status,
		LineageKey: legacyTypeScriptParentGoalIDV1 + "/" + record.ParentThreadID, SourceRef: sourcePrefix + digest,
		QueuedAt: queuedAt, QueuedMs: record.QueuedMs, StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: updatedAt,
		FailureCode: domainjob.NormalizeFailureCode("", record.Status, false), ToolInvocations: record.ToolInvocations,
	}
	expected = domainjob.NormalizePersistedRecordV1(expected)
	expected.PauseState = childRunPauseState(expected)
	expected.SteerState = childRunState(expected)
	if !reflect.DeepEqual(record, expected) {
		return errors.New("legacy TypeScript child-run tombstone carries mutable or authoritative state")
	}
	return nil
}
