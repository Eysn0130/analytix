package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	TerminalPublicationIntentVersion      = 1
	AcceptedFinalDispositionRecordVersion = 1
	AcceptedFinalDispositionRecordV2      = 2
	AcceptedFinalDispositionPurpose       = "analytix.case-final-disposition/v1"
	AcceptedFinalDispositionPurposeV2     = "analytix.case-final-disposition/v2"
)

var acceptedFinalDispositionSignatureDomain = []byte("analytix.final-answer-disposition/v1\x00")
var acceptedFinalDispositionSignatureDomainV2 = []byte("analytix.final-answer-disposition/v2\x00")

type TerminalPublicationIntent struct {
	SchemaVersion         int            `json:"schemaVersion"`
	CreatedAt             string         `json:"createdAt"`
	Model                 string         `json:"model,omitempty"`
	Usage                 map[string]any `json:"usage"`
	CacheDiagnostics      map[string]any `json:"cacheDiagnostics"`
	UsageSource           string         `json:"usageSource,omitempty"`
	ChildRunID            string         `json:"childRunId,omitempty"`
	TerminalStatus        string         `json:"terminalStatus"`
	TerminalCode          string         `json:"terminalCode,omitempty"`
	TerminalMessage       string         `json:"terminalMessage,omitempty"`
	TerminalSeverity      string         `json:"terminalSeverity,omitempty"`
	Discard               bool           `json:"discard"`
	Cancelled             bool           `json:"cancelled"`
	CancelledPendingGates int            `json:"cancelledPendingGates"`
}

type TerminalPublicationIntentInput struct {
	CreatedAt             string
	Model                 string
	Usage                 map[string]any
	CacheDiagnostics      map[string]any
	UsageSource           string
	ChildRunID            string
	TerminalStatus        string
	TerminalCode          string
	TerminalMessage       string
	TerminalSeverity      string
	Discard               bool
	Cancelled             bool
	CancelledPendingGates int
}

type AcceptedFinalDispositionState string
type AcceptedFinalDispositionDecisionBasis string

const (
	AcceptedFinalCommitted              AcceptedFinalDispositionState = "committed"
	AcceptedFinalExplicitlyNotCommitted AcceptedFinalDispositionState = "explicitly_not_committed"

	AcceptedFinalDecisionSamePublicWinner      AcceptedFinalDispositionDecisionBasis = "same_public_winner"
	AcceptedFinalDecisionDifferentPublicWinner AcceptedFinalDispositionDecisionBasis = "different_public_winner"
)

type AcceptedFinalDispositionRecord struct {
	SchemaVersion       int                                   `json:"schemaVersion"`
	AuthorityPurpose    string                                `json:"authorityPurpose"`
	AuthorityAlgorithm  string                                `json:"authorityAlgorithm"`
	AuthorityKeyID      string                                `json:"authorityKeyId"`
	AuthorityPublicKey  string                                `json:"authorityPublicKey"`
	AcceptedFinalDigest string                                `json:"acceptedFinalDigest"`
	PrivateRecordDigest string                                `json:"privateRecordDigest"`
	ThreadID            string                                `json:"threadId"`
	TurnID              string                                `json:"turnId"`
	State               AcceptedFinalDispositionState         `json:"state"`
	EventManifestDigest string                                `json:"eventManifestDigest"`
	DecisionBasis       AcceptedFinalDispositionDecisionBasis `json:"decisionBasis,omitempty"`
	TurnCASDigest       string                                `json:"turnCasDigest,omitempty"`
	WinnerDigest        string                                `json:"winnerDigest,omitempty"`
	DecidedAt           string                                `json:"decidedAt"`
	AuthoritySignature  string                                `json:"authoritySignature"`
	RecordDigest        string                                `json:"recordDigest"`
}

type AcceptedFinalDispositionInput struct {
	AcceptedFinal       AcceptedFinalRecord
	State               AcceptedFinalDispositionState
	EventManifestDigest string
	DecidedAt           time.Time
	AuthorityKeyID      string
	AuthorityPublicKey  []byte
}

func NewTerminalPublicationIntent(input TerminalPublicationIntentInput, terminalReason string) (TerminalPublicationIntent, error) {
	createdAt := strings.TrimSpace(input.CreatedAt)
	if createdAt == "" {
		return TerminalPublicationIntent{}, errors.New("terminal publication createdAt is required")
	}
	usage, err := canonicalPublicationMap(input.Usage)
	if err != nil {
		return TerminalPublicationIntent{}, errors.New("terminal publication usage is not canonical JSON")
	}
	cacheDiagnostics, err := canonicalPublicationMap(input.CacheDiagnostics)
	if err != nil {
		return TerminalPublicationIntent{}, errors.New("terminal publication cache diagnostics are not canonical JSON")
	}
	intent := TerminalPublicationIntent{
		SchemaVersion: TerminalPublicationIntentVersion, CreatedAt: createdAt, Model: strings.TrimSpace(input.Model),
		Usage: usage, CacheDiagnostics: cacheDiagnostics,
		UsageSource: strings.TrimSpace(input.UsageSource), ChildRunID: strings.TrimSpace(input.ChildRunID),
		TerminalStatus: strings.TrimSpace(input.TerminalStatus), TerminalCode: strings.TrimSpace(input.TerminalCode),
		TerminalMessage: strings.TrimSpace(input.TerminalMessage), TerminalSeverity: strings.TrimSpace(input.TerminalSeverity),
		Discard: input.Discard, Cancelled: input.Cancelled, CancelledPendingGates: input.CancelledPendingGates,
	}
	if err := ValidateTerminalPublicationIntent(intent, terminalReason); err != nil {
		return TerminalPublicationIntent{}, err
	}
	return intent, nil
}

func ValidateTerminalPublicationIntent(intent TerminalPublicationIntent, terminalReason string) error {
	expectedStatus, ok := FinalAnswerTerminalStatus(strings.TrimSpace(terminalReason))
	if intent.SchemaVersion != TerminalPublicationIntentVersion || !ok || strings.TrimSpace(intent.TerminalStatus) != expectedStatus ||
		intent.CancelledPendingGates < 0 || intent.Usage == nil || intent.CacheDiagnostics == nil {
		return errors.New("terminal publication intent is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, intent.CreatedAt); err != nil {
		return errors.New("terminal publication createdAt is invalid")
	}
	if domainevent.ValidatePublicRecord(intent.Usage) != nil || domainevent.ValidatePublicRecord(intent.CacheDiagnostics) != nil {
		return errors.New("terminal publication intent contains restricted content")
	}
	if body, err := json.Marshal(intent); err != nil || len(body) == 0 {
		return errors.New("terminal publication intent is not canonical JSON")
	}
	return nil
}

func NewAcceptedFinalDispositionRecord(input AcceptedFinalDispositionInput, sign AcceptedFinalSignFunc) (AcceptedFinalDispositionRecord, error) {
	return newAcceptedFinalDispositionRecord(input, nil, "", sign)
}

func NewAcceptedFinalDispositionRecordV2(
	input AcceptedFinalDispositionInput,
	observation AcceptedFinalCASObservationV1,
	basis AcceptedFinalDispositionDecisionBasis,
	sign AcceptedFinalSignFunc,
) (AcceptedFinalDispositionRecord, error) {
	return newAcceptedFinalDispositionRecord(input, &observation, basis, sign)
}

func newAcceptedFinalDispositionRecord(
	input AcceptedFinalDispositionInput,
	observation *AcceptedFinalCASObservationV1,
	basis AcceptedFinalDispositionDecisionBasis,
	sign AcceptedFinalSignFunc,
) (AcceptedFinalDispositionRecord, error) {
	if sign == nil || ValidateAcceptedFinalRecord(input.AcceptedFinal) != nil || !validSHA256(input.EventManifestDigest) {
		return AcceptedFinalDispositionRecord{}, errors.New("accepted final disposition input is invalid")
	}
	if input.State != AcceptedFinalCommitted && input.State != AcceptedFinalExplicitlyNotCommitted {
		return AcceptedFinalDispositionRecord{}, errors.New("accepted final disposition state is invalid")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != strings.TrimSpace(input.AuthorityKeyID) {
		return AcceptedFinalDispositionRecord{}, errors.New("accepted final disposition authority key is invalid")
	}
	decidedAt := input.DecidedAt.UTC()
	if decidedAt.IsZero() {
		decidedAt = time.Now().UTC()
	}
	record := AcceptedFinalDispositionRecord{
		SchemaVersion: AcceptedFinalDispositionRecordVersion, AuthorityPurpose: AcceptedFinalDispositionPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), AcceptedFinalDigest: input.AcceptedFinal.RecordDigest,
		PrivateRecordDigest: input.AcceptedFinal.PrivateRecordDigest, ThreadID: input.AcceptedFinal.ThreadID,
		TurnID: input.AcceptedFinal.TurnID, State: input.State, EventManifestDigest: input.EventManifestDigest,
		DecidedAt: decidedAt.Format(time.RFC3339Nano),
	}
	if observation != nil {
		if err := validateDispositionCASBasis(input, *observation, basis); err != nil {
			return AcceptedFinalDispositionRecord{}, err
		}
		record.SchemaVersion = AcceptedFinalDispositionRecordV2
		record.AuthorityPurpose = AcceptedFinalDispositionPurposeV2
		record.DecisionBasis = basis
		record.TurnCASDigest = observation.TurnProjectionSHA256
		if observation.HasWinner {
			record.WinnerDigest = observation.Winner.RecordDigest
		}
	}
	signature, err := sign(AcceptedFinalDispositionSigningBytes(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return AcceptedFinalDispositionRecord{}, errors.New("accepted final disposition signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = acceptedFinalDispositionRecordDigest(record)
	if err := ValidateAcceptedFinalDispositionRecord(record); err != nil {
		return AcceptedFinalDispositionRecord{}, err
	}
	return record, nil
}

func validateDispositionCASBasis(input AcceptedFinalDispositionInput, observation AcceptedFinalCASObservationV1, basis AcceptedFinalDispositionDecisionBasis) error {
	if ValidateAcceptedFinalCASObservationV1(observation) != nil || observation.ThreadID != input.AcceptedFinal.ThreadID ||
		observation.TurnID != input.AcceptedFinal.TurnID {
		return errors.New("accepted final disposition CAS observation is invalid")
	}
	sameWinner := observation.HasWinner && observation.Winner.RecordDigest == input.AcceptedFinal.RecordDigest
	switch basis {
	case AcceptedFinalDecisionSamePublicWinner:
		if input.State != AcceptedFinalCommitted || !sameWinner {
			return errors.New("accepted final committed disposition lacks its exact CAS winner")
		}
	case AcceptedFinalDecisionDifferentPublicWinner:
		if input.State != AcceptedFinalExplicitlyNotCommitted || !observation.HasWinner || sameWinner {
			return errors.New("accepted final losing disposition lacks the different CAS winner")
		}
	default:
		return errors.New("accepted final disposition decision basis is invalid")
	}
	return nil
}

func ValidateAcceptedFinalDispositionRecord(record AcceptedFinalDispositionRecord) error {
	versionValid := record.SchemaVersion == AcceptedFinalDispositionRecordVersion && record.AuthorityPurpose == AcceptedFinalDispositionPurpose ||
		record.SchemaVersion == AcceptedFinalDispositionRecordV2 && record.AuthorityPurpose == AcceptedFinalDispositionPurposeV2
	if !versionValid ||
		record.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm || !validSHA256(record.AuthorityKeyID) ||
		!validSHA256(record.AcceptedFinalDigest) || !validSHA256(record.PrivateRecordDigest) ||
		strings.TrimSpace(record.ThreadID) == "" || strings.TrimSpace(record.TurnID) == "" ||
		(record.State != AcceptedFinalCommitted && record.State != AcceptedFinalExplicitlyNotCommitted) ||
		!validSHA256(record.EventManifestDigest) || !validSHA256(record.RecordDigest) {
		return errors.New("accepted final disposition record is incomplete")
	}
	if record.SchemaVersion == AcceptedFinalDispositionRecordVersion {
		if record.DecisionBasis != "" || record.TurnCASDigest != "" || record.WinnerDigest != "" {
			return errors.New("legacy accepted final disposition contains V2 CAS fields")
		}
	} else {
		if !validSHA256(record.TurnCASDigest) || !validSHA256(record.WinnerDigest) {
			return errors.New("accepted final disposition V2 CAS binding is incomplete")
		}
		switch record.DecisionBasis {
		case AcceptedFinalDecisionSamePublicWinner:
			if record.State != AcceptedFinalCommitted || record.WinnerDigest != record.AcceptedFinalDigest {
				return errors.New("accepted final disposition V2 committed basis is inconsistent")
			}
		case AcceptedFinalDecisionDifferentPublicWinner:
			if record.State != AcceptedFinalExplicitlyNotCommitted || record.WinnerDigest == record.AcceptedFinalDigest {
				return errors.New("accepted final disposition V2 losing basis is inconsistent")
			}
		default:
			return errors.New("accepted final disposition V2 basis is unknown")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, record.DecidedAt); err != nil {
		return errors.New("accepted final disposition decidedAt is invalid")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != record.AuthorityKeyID {
		return errors.New("accepted final disposition public key is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), AcceptedFinalDispositionSigningBytes(record), signature) {
		return errors.New("accepted final disposition signature is invalid")
	}
	if acceptedFinalDispositionRecordDigest(record) != record.RecordDigest {
		return errors.New("accepted final disposition integrity is invalid")
	}
	return nil
}

func ParseAcceptedFinalDispositionRecord(raw []byte) (AcceptedFinalDispositionRecord, error) {
	var record AcceptedFinalDispositionRecord
	if err := decodeStrictJSON(raw, &record, "accepted final disposition record"); err != nil {
		return AcceptedFinalDispositionRecord{}, err
	}
	if err := ValidateAcceptedFinalDispositionRecord(record); err != nil {
		return AcceptedFinalDispositionRecord{}, err
	}
	return record, nil
}

func AcceptedFinalDispositionSigningBytes(record AcceptedFinalDispositionRecord) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	domain := acceptedFinalDispositionSignatureDomain
	if record.SchemaVersion == AcceptedFinalDispositionRecordV2 {
		domain = acceptedFinalDispositionSignatureDomainV2
	}
	out := make([]byte, 0, len(domain)+len(digest))
	out = append(out, domain...)
	out = append(out, digest[:]...)
	return out
}

func AcceptedFinalDispositionAuthorityMaterial(record AcceptedFinalDispositionRecord) (string, []byte, []byte, error) {
	if err := ValidateAcceptedFinalDispositionRecord(record); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	return record.AuthorityKeyID, publicKey, signature, nil
}

func AcceptedFinalDispositionRecordBytes(record AcceptedFinalDispositionRecord) ([]byte, error) {
	if err := ValidateAcceptedFinalDispositionRecord(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func acceptedFinalDispositionRecordDigest(record AcceptedFinalDispositionRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func canonicalPublicationMap(value map[string]any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var canonical map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return canonical, nil
}

func AcceptedFinalDispositionRecordMap(record AcceptedFinalDispositionRecord) map[string]any {
	body, _ := json.Marshal(record)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}
