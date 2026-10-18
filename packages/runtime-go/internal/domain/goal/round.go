package goal

import (
	"context"
	"encoding/json"
	"math"
	"strings"
)

const DefaultMaxRoundsV1 = 256

type RefV1 struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

type RoundSourceV1 struct {
	Kind     string `json:"kind"`
	GoalID   string `json:"goalId"`
	Revision int    `json:"revision"`
	Round    int    `json:"round"`
}

type ActivationV1 struct {
	RefV1
	Activation string `json:"activation"`
}

// RoundAdmissionV1 is a host-only in-process witness. Public input has no
// carrier for it; lifecycle ownership remains the existing Goal driver.
type RoundAdmissionV1 interface {
	Source() RoundSourceV1
	Prompt() string
	ThreadID() string
	Validate(string, string) error
	WithAdmission(func() (bool, error)) error
	BindExecutionCancel(context.CancelFunc) error
}

func IntegerV1(value any, minimum int) (int, bool) {
	var number int64
	switch v := value.(type) {
	case int:
		number = int64(v)
	case int64:
		number = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v > 9007199254740991 {
			return 0, false
		}
		number = int64(v)
	case json.Number:
		var err error
		number, err = v.Int64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if number < int64(minimum) || number > 9007199254740991 || int64(int(number)) != number {
		return 0, false
	}
	return int(number), true
}

func RefFromRecordV1(record map[string]any) (RefV1, bool) {
	id, ok := record["id"].(string)
	revision, valid := IntegerV1(record["revision"], 1)
	return RefV1{ID: id, Revision: revision}, ok && strings.TrimSpace(id) == id && id != "" && valid
}

// CountedRecordV1 never adopts a legacy record by supplying missing facts.
func CountedRecordV1(record map[string]any) (RefV1, int, int, bool) {
	ref, known := RefFromRecordV1(record)
	maximum, validMax := IntegerV1(record["maxGoalRounds"], 1)
	started, validStarted := IntegerV1(record["roundsStarted"], 0)
	return ref, maximum, started, known && validMax && validStarted && started <= maximum
}

func RoundSourceRecordV1(source RoundSourceV1) map[string]any {
	return map[string]any{"kind": "goal", "goalId": source.GoalID, "revision": source.Revision, "round": source.Round}
}

func ParseRoundSourceV1(value any) (RoundSourceV1, bool) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 4 || record["kind"] != "goal" {
		return RoundSourceV1{}, false
	}
	id, idOK := record["goalId"].(string)
	revision, revOK := IntegerV1(record["revision"], 1)
	round, roundOK := IntegerV1(record["round"], 1)
	return RoundSourceV1{Kind: "goal", GoalID: id, Revision: revision, Round: round}, idOK && id != "" && id == strings.TrimSpace(id) && revOK && roundOK
}
