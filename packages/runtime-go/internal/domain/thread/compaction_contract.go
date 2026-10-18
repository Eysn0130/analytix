package thread

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// CompactionSummaryFactsV1 describes one complete, projected ordinary summary.
// The hashes bind content and selection, never evidence or execution authority.
type CompactionSummaryFactsV1 struct {
	CompactionID      string   `json:"compactionId"`
	Summary           string   `json:"summary"`
	SummaryDigest     string   `json:"summaryDigest"`
	SourceDigest      string   `json:"sourceDigest"`
	CheckpointDigest  string   `json:"checkpointDigest"`
	SourceItemIDs     []string `json:"sourceItemIds"`
	ShadowedItemCount int      `json:"shadowedItemCount"`
	ReplacedTokens    int      `json:"replacedTokens"`
	FinishedAt        string   `json:"finishedAt"`
}

func SealCompactionSummaryFactsV1(facts CompactionSummaryFactsV1) (CompactionSummaryFactsV1, error) {
	summaryHash := sha256.Sum256([]byte(facts.Summary))
	facts.SummaryDigest = hex.EncodeToString(summaryHash[:])
	facts.CheckpointDigest = ""
	body, err := json.Marshal(facts)
	if err != nil {
		return CompactionSummaryFactsV1{}, err
	}
	hash := sha256.Sum256(body)
	facts.CheckpointDigest = hex.EncodeToString(hash[:])
	if !validCompactionSummaryFactsShapeV1(facts) {
		return CompactionSummaryFactsV1{}, errors.New("ordinary compaction summary facts are invalid")
	}
	return facts, nil
}

func ValidateCompactionSummaryFactsV1(facts CompactionSummaryFactsV1) error {
	sealed, err := SealCompactionSummaryFactsV1(facts)
	if err != nil || sealed.SummaryDigest != facts.SummaryDigest || sealed.CheckpointDigest != facts.CheckpointDigest {
		return errors.New("ordinary compaction summary facts are invalid")
	}
	return nil
}

func validCompactionSummaryFactsShapeV1(facts CompactionSummaryFactsV1) bool {
	if !IsCanonicalRecordID(facts.CompactionID) || !utf8.ValidString(facts.Summary) || strings.TrimSpace(facts.Summary) == "" || len(facts.Summary) > 16*1024 ||
		len(facts.SourceItemIDs) == 0 || facts.ShadowedItemCount < len(facts.SourceItemIDs) || facts.ShadowedItemCount <= 0 || facts.ReplacedTokens <= 0 ||
		len(facts.SourceDigest) != 64 || len(facts.SummaryDigest) != 64 || len(facts.CheckpointDigest) != 64 {
		return false
	}
	if _, err := hex.DecodeString(facts.SourceDigest); err != nil || facts.SourceDigest != strings.ToLower(facts.SourceDigest) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, facts.FinishedAt); err != nil {
		return false
	}
	seen := map[string]bool{}
	for _, id := range facts.SourceItemIDs {
		if !IsCanonicalRecordID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

// CompactionHistoryDerivationV1 binds an explicitly exported summary to its
// derived data owner. It stores only provenance commitments, never a parent
// security context, execution grant, epoch witness or publication authority.
const CompactionHistoryDerivationFieldV1 = "compactionHistoryDerivation"

type CompactionHistoryDerivationV1 struct {
	Version                string `json:"version"`
	Derivation             string `json:"derivation"`
	SourceThreadID         string `json:"sourceThreadId"`
	SourceTurnID           string `json:"sourceTurnId"`
	SourceItemID           string `json:"sourceItemId"`
	SourceScopeDigest      string `json:"sourceScopeDigest"`
	SourceItemDigest       string `json:"sourceItemDigest"`
	SourceSummaryDigest    string `json:"sourceSummaryDigest"`
	SourceCheckpointDigest string `json:"sourceCheckpointDigest"`
	ThreadID               string `json:"threadId"`
	TurnID                 string `json:"turnId"`
	ItemID                 string `json:"itemId"`
	ScopeDigest            string `json:"scopeDigest"`
	ItemDigest             string `json:"itemDigest"`
	Digest                 string `json:"digest"`
}

func CompactionHistoryItemDigestV1(item map[string]any) string {
	body, err := json.Marshal(item)
	if err != nil || item == nil {
		return ""
	}
	hash := sha256.Sum256(append([]byte("analytix.compaction-history-data.v1\x00"), body...))
	return hex.EncodeToString(hash[:])
}

func SealCompactionHistoryDerivationV1(value CompactionHistoryDerivationV1) (CompactionHistoryDerivationV1, error) {
	if value.Version != "" && value.Version != "compaction-history-derivation.v1" ||
		!IsCanonicalRecordID(value.SourceThreadID) || !IsCanonicalRecordID(value.ThreadID) ||
		!IsCanonicalRecordID(value.SourceTurnID) || value.SourceTurnID != value.TurnID ||
		!IsCanonicalRecordID(value.SourceItemID) || value.SourceItemID != value.ItemID ||
		!isContinuationSHA256V1(value.SourceScopeDigest) || !isContinuationSHA256V1(value.ScopeDigest) ||
		!isContinuationSHA256V1(value.SourceItemDigest) || !isContinuationSHA256V1(value.ItemDigest) ||
		!isContinuationSHA256V1(value.SourceSummaryDigest) || !isContinuationSHA256V1(value.SourceCheckpointDigest) {
		return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation is invalid")
	}
	switch value.Derivation {
	case "fork", "resume":
		if value.SourceThreadID == value.ThreadID {
			return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation must change its data owner")
		}
	case "fork-prefix":
		if value.SourceThreadID != value.ThreadID || value.SourceScopeDigest != value.ScopeDigest || value.SourceItemDigest != value.ItemDigest {
			return CompactionHistoryDerivationV1{}, errors.New("compaction prefix must preserve exact source data")
		}
	default:
		return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation is invalid")
	}
	value.Version, value.Digest = "compaction-history-derivation.v1", ""
	body, err := json.Marshal(value)
	if err != nil {
		return CompactionHistoryDerivationV1{}, err
	}
	hash := sha256.Sum256(body)
	value.Digest = hex.EncodeToString(hash[:])
	return value, nil
}

func CompactionHistoryDerivationMapV1(value CompactionHistoryDerivationV1) map[string]any {
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func ParseCompactionHistoryDerivationV1(raw any) (CompactionHistoryDerivationV1, error) {
	body, err := json.Marshal(raw)
	if err != nil || len(body) > 8192 {
		return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation is invalid")
	}
	var value CompactionHistoryDerivationV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return CompactionHistoryDerivationV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation has trailing data")
	}
	sealed, err := SealCompactionHistoryDerivationV1(value)
	if err != nil || value.Version != "compaction-history-derivation.v1" || !isContinuationSHA256V1(value.Digest) || sealed.Digest != value.Digest {
		return CompactionHistoryDerivationV1{}, errors.New("compaction history derivation commitment is invalid")
	}
	return value, nil
}

const (
	TaskContinuationSchemaVersionV1 = 1
	TaskContinuationEvidenceStateV1 = "unverified_for_case_facts"
)

type TaskContinuationGoalV1 struct {
	GoalID      string `json:"goalId"`
	Objective   string `json:"objective"`
	Status      string `json:"status"`
	StateDigest string `json:"stateDigest"`
}

type TaskContinuationTodoV1 struct {
	TodoID           string `json:"todoId"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	StatusReasonCode string `json:"statusReasonCode,omitempty"`
	StateDigest      string `json:"stateDigest"`
}

type TaskContinuationEvidenceReferenceV1 struct {
	ReferenceDigest string `json:"referenceDigest"`
	SupportStatus   string `json:"supportStatus"`
}

type TaskContinuationSnapshotV1 struct {
	UserHistory                    *ContinuationUserHistoryV1            `json:"userHistory,omitempty"`
	SchemaVersion                  int                                   `json:"schemaVersion"`
	Goal                           *TaskContinuationGoalV1               `json:"goal,omitempty"`
	Todos                          []TaskContinuationTodoV1              `json:"todos"`
	LatestUserConstraints          []string                              `json:"latestUserConstraints"`
	EvidenceReferences             []TaskContinuationEvidenceReferenceV1 `json:"evidenceReferences"`
	EvidenceAuthority              string                                `json:"evidenceAuthority"`
	PreviousContinuationDigest     string                                `json:"previousContinuationDigest,omitempty"`
	PreviousCompactionSourceDigest string                                `json:"previousCompactionSourceDigest,omitempty"`
	StateDigest                    string                                `json:"stateDigest,omitempty"`
}

func SealTaskContinuationSnapshotV1(snapshot TaskContinuationSnapshotV1) (TaskContinuationSnapshotV1, error) {
	if snapshot.SchemaVersion != 0 && snapshot.SchemaVersion != TaskContinuationSchemaVersionV1 {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation schema version is invalid")
	}
	if snapshot.EvidenceAuthority != "" && snapshot.EvidenceAuthority != TaskContinuationEvidenceStateV1 {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation evidence authority is invalid")
	}
	snapshot.SchemaVersion = TaskContinuationSchemaVersionV1
	snapshot.EvidenceAuthority = TaskContinuationEvidenceStateV1
	snapshot.StateDigest = ""
	if err := validateTaskContinuationSnapshotFieldsV1(snapshot); err != nil {
		return TaskContinuationSnapshotV1{}, err
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return TaskContinuationSnapshotV1{}, err
	}
	sum := sha256.Sum256(body)
	snapshot.StateDigest = hex.EncodeToString(sum[:])
	return snapshot, nil
}

func ParseTaskContinuationSnapshotV1(value any) (TaskContinuationSnapshotV1, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) == 0 {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation snapshot is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var snapshot TaskContinuationSnapshotV1
	if err := decoder.Decode(&snapshot); err != nil {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation snapshot is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation snapshot has trailing content")
	}
	stateDigest := strings.TrimSpace(snapshot.StateDigest)
	if !isContinuationSHA256V1(stateDigest) {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation snapshot digest is invalid")
	}
	sealed, err := SealTaskContinuationSnapshotV1(snapshot)
	if err != nil || sealed.StateDigest != stateDigest {
		return TaskContinuationSnapshotV1{}, errors.New("task continuation snapshot digest does not match its state")
	}
	return snapshot, nil
}

func TaskContinuationSnapshotMapV1(snapshot TaskContinuationSnapshotV1) map[string]any {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func validateTaskContinuationSnapshotFieldsV1(snapshot TaskContinuationSnapshotV1) error {
	if err := snapshot.UserHistory.Validate(); err != nil {
		return err
	}
	if snapshot.SchemaVersion != TaskContinuationSchemaVersionV1 || snapshot.EvidenceAuthority != TaskContinuationEvidenceStateV1 ||
		len(snapshot.Todos) > 200 || len(snapshot.LatestUserConstraints) > 4 || len(snapshot.EvidenceReferences) > 32 ||
		!optionalContinuationDigestV1(snapshot.PreviousContinuationDigest) || !optionalContinuationDigestV1(snapshot.PreviousCompactionSourceDigest) {
		return errors.New("task continuation snapshot contract is invalid")
	}
	if snapshot.Goal != nil {
		if strings.TrimSpace(snapshot.Goal.GoalID) == "" || strings.TrimSpace(snapshot.Goal.Objective) == "" ||
			!validContinuationGoalStatusV1(snapshot.Goal.Status) || !isContinuationSHA256V1(snapshot.Goal.StateDigest) {
			return errors.New("task continuation goal is invalid")
		}
	}
	seenTodos := map[string]bool{}
	for _, todo := range snapshot.Todos {
		id := strings.TrimSpace(todo.TodoID)
		if id == "" || seenTodos[id] || strings.TrimSpace(todo.Content) == "" || !validContinuationTodoStatusV1(todo.Status) ||
			!validContinuationTodoReasonV1(todo.Status, todo.StatusReasonCode) || !isContinuationSHA256V1(todo.StateDigest) {
			return errors.New("task continuation todo is invalid")
		}
		seenTodos[id] = true
	}
	for _, constraint := range snapshot.LatestUserConstraints {
		if strings.TrimSpace(constraint) == "" {
			return errors.New("task continuation user constraint is invalid")
		}
	}
	seenEvidence := map[string]bool{}
	for _, reference := range snapshot.EvidenceReferences {
		digest := strings.TrimSpace(reference.ReferenceDigest)
		if !isContinuationSHA256V1(digest) || seenEvidence[digest] || reference.SupportStatus != TaskContinuationEvidenceStateV1 {
			return errors.New("task continuation evidence reference is invalid")
		}
		seenEvidence[digest] = true
	}
	return nil
}

func validContinuationGoalStatusV1(status string) bool {
	switch strings.TrimSpace(status) {
	case "active", "paused", "blocked", "usageLimited", "budgetLimited":
		return true
	default:
		return false
	}
}

func validContinuationTodoStatusV1(status string) bool {
	switch strings.TrimSpace(status) {
	case "pending", "in_progress", "failed", "canceled":
		return true
	default:
		return false
	}
}

func validContinuationTodoReasonV1(status, reason string) bool {
	reason = strings.TrimSpace(reason)
	if status == "failed" || status == "canceled" {
		return reason != ""
	}
	return reason == ""
}

func optionalContinuationDigestV1(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || isContinuationSHA256V1(value)
}

func isContinuationSHA256V1(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
