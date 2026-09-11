package pluginmaterialization

import (
	"errors"
	"time"
)

type JournalPhaseV1 string

const (
	JournalPreparedV1            JournalPhaseV1 = "prepared"
	JournalStagedV1              JournalPhaseV1 = "staged"
	JournalAuthorizedV1          JournalPhaseV1 = "authorized"
	JournalPriorQuarantinedV1    JournalPhaseV1 = "prior_quarantined"
	JournalGenerationCommittedV1 JournalPhaseV1 = "generation_committed"
	JournalIndexCommittedV1      JournalPhaseV1 = "index_committed"
	JournalCompletedV1           JournalPhaseV1 = "completed"
)

type JournalV1 struct {
	SchemaVersion       int            `json:"schemaVersion"`
	Purpose             string         `json:"purpose"`
	RecordDigest        string         `json:"recordDigest"`
	TransactionID       string         `json:"transactionId"`
	Sequence            uint64         `json:"sequence"`
	Phase               JournalPhaseV1 `json:"phase"`
	IntentID            string         `json:"intentId"`
	GenerationID        string         `json:"generationId"`
	StagingRelativePath string         `json:"stagingRelativePath"`
	ActiveRelativePath  string         `json:"activeRelativePath"`
	RecordedAt          string         `json:"recordedAt"`
}

func NewJournalV1(intent IntentV1, generationID, stagingRelativePath, activeRelativePath string, sequence uint64, phase JournalPhaseV1, recordedAt time.Time) (JournalV1, error) {
	record := JournalV1{
		SchemaVersion: SchemaVersionV1, Purpose: JournalPurposeV1,
		TransactionID: intent.IntentID, Sequence: sequence, Phase: phase, IntentID: intent.IntentID,
		GenerationID: generationID, StagingRelativePath: stagingRelativePath, ActiveRelativePath: activeRelativePath,
		RecordedAt: recordedAt.UTC().Format(time.RFC3339Nano),
	}
	record.RecordDigest = deriveJournalDigest(record)
	if err := ValidateJournalV1(record); err != nil {
		return JournalV1{}, err
	}
	return record, nil
}

func ValidateJournalV1(record JournalV1) error {
	if record.SchemaVersion != SchemaVersionV1 || record.Purpose != JournalPurposeV1 ||
		!canonicalDigest(record.RecordDigest) || record.RecordDigest != deriveJournalDigest(record) ||
		!canonicalDigest(record.TransactionID) || record.TransactionID != record.IntentID ||
		record.Sequence == 0 || !validJournalPhase(record.Sequence, record.Phase) ||
		!canonicalDigest(record.IntentID) || !canonicalDigest(record.GenerationID) ||
		!canonicalRelativePath(record.StagingRelativePath) || !canonicalRelativePath(record.ActiveRelativePath) ||
		!canonicalTime(record.RecordedAt) {
		return errors.New("bundled plugin materialization journal record is invalid")
	}
	return nil
}

func JournalV1Bytes(record JournalV1) ([]byte, error) {
	return canonicalBytes(record, func() error { return ValidateJournalV1(record) })
}

func ParseJournalV1(body []byte) (JournalV1, error) {
	var record JournalV1
	err := strictParse(body, &record, func() error { return ValidateJournalV1(record) })
	return record, err
}

func validJournalPhase(sequence uint64, phase JournalPhaseV1) bool {
	want := map[JournalPhaseV1]uint64{
		JournalPreparedV1: 1, JournalStagedV1: 2, JournalAuthorizedV1: 3,
		JournalPriorQuarantinedV1: 4, JournalGenerationCommittedV1: 5,
		JournalIndexCommittedV1: 6, JournalCompletedV1: 7,
	}
	return want[phase] == sequence
}

func deriveJournalDigest(record JournalV1) string {
	record.RecordDigest = ""
	return digestWithDomain("analytix.bundled-plugin-materialization-journal/digest/v1", record)
}
