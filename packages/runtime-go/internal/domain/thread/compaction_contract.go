package thread

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

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
