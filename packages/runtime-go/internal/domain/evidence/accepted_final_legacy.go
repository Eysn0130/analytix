package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const LegacyAcceptedFinalRecordVersion = 1

// LegacyAcceptedFinalRecord is integrity-only historical data from the
// pre-authority format. It is display-only and can never support a new claim,
// provider history, tool execution, report, or publication decision.
type LegacyAcceptedFinalRecord struct {
	SchemaVersion      int                `json:"schemaVersion"`
	EnvelopeDigest     string             `json:"envelopeDigest"`
	ContextDigest      string             `json:"contextDigest"`
	ContextEpoch       uint64             `json:"contextEpoch"`
	DatasetSnapshotID  string             `json:"datasetSnapshotId"`
	Variant            FinalAnswerVariant `json:"variant"`
	TerminalReason     string             `json:"terminalReason"`
	RenderedTextSHA256 string             `json:"renderedTextSha256"`
	AcceptedAt         string             `json:"acceptedAt"`
	RecordDigest       string             `json:"recordDigest"`
}

func ParseLegacyAcceptedFinalRecord(value any) (LegacyAcceptedFinalRecord, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return LegacyAcceptedFinalRecord{}, err
	}
	var record LegacyAcceptedFinalRecord
	if err := decodeStrictJSON(body, &record, "legacy accepted final record"); err != nil {
		return LegacyAcceptedFinalRecord{}, err
	}
	if err := ValidateLegacyAcceptedFinalRecord(record); err != nil {
		return LegacyAcceptedFinalRecord{}, err
	}
	return record, nil
}

func ValidateLegacyAcceptedFinalRecord(record LegacyAcceptedFinalRecord) error {
	if record.SchemaVersion != LegacyAcceptedFinalRecordVersion || !validSHA256(record.EnvelopeDigest) ||
		!validSHA256(record.ContextDigest) || record.ContextEpoch == 0 || strings.TrimSpace(record.DatasetSnapshotID) == "" ||
		!validFinalAnswerVariant(record.Variant) || !validFinalTerminalReason(record.TerminalReason) ||
		!validSHA256(record.RenderedTextSHA256) || !validSHA256(record.RecordDigest) {
		return errors.New("legacy accepted final record is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, record.AcceptedAt); err != nil {
		return errors.New("legacy accepted final acceptedAt is invalid")
	}
	if legacyAcceptedFinalRecordDigest(record) != record.RecordDigest {
		return errors.New("legacy accepted final record integrity is invalid")
	}
	return nil
}

func legacyAcceptedFinalRecordDigest(record LegacyAcceptedFinalRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func LegacyAcceptedFinalRecordMap(record LegacyAcceptedFinalRecord) map[string]any {
	body, _ := json.Marshal(record)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}
