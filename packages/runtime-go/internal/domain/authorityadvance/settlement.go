package authorityadvance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	monotonicAdvanceSettlementSignatureDomainV1 = []byte("analytix.monotonic-advance-settlement/signature/v1\x00")
	monotonicAdvanceSettlementDigestDomainV1    = []byte("analytix.monotonic-advance-settlement/digest/v1\x00")
)

type MonotonicAdvanceSettlementKindV1 string

const (
	MonotonicAdvanceSettlementCommittedV1  MonotonicAdvanceSettlementKindV1 = "committed"
	MonotonicAdvanceSettlementSupersededV1 MonotonicAdvanceSettlementKindV1 = "superseded"
)

type MonotonicAdvanceCommittedV1 struct {
	Receipt domainsecurity.MonotonicHeadAdvanceReceiptV1 `json:"receipt"`
}

type MonotonicAdvanceSupersededV1 struct {
	ObserveRequest  domainsecurity.MonotonicHeadObserveRequestV1 `json:"observeRequest"`
	Observation     domainsecurity.MonotonicHeadObservationV1    `json:"observation"`
	RangeReferences []MonotonicAdvanceRangeReferenceV1           `json:"rangeReferences"`
}

// MonotonicAdvanceSettlementV1 is one immutable tagged union. A committed
// branch carries the exact witness receipt. A superseded branch carries a
// fresh signed observation and references to a separately supplied, fully
// validated contiguous committed range. Neither branch can coexist.
type MonotonicAdvanceSettlementV1 struct {
	SchemaVersion      int                              `json:"schemaVersion"`
	Purpose            string                           `json:"purpose"`
	Kind               MonotonicAdvanceSettlementKindV1 `json:"kind"`
	InstallationID     string                           `json:"installationId"`
	EnrollmentID       string                           `json:"enrollmentId"`
	Namespace          string                           `json:"namespace"`
	Root               AdvanceRootV1                    `json:"root"`
	MutationID         string                           `json:"mutationId"`
	IntentDigest       string                           `json:"intentDigest"`
	RequestDigest      string                           `json:"requestDigest"`
	Committed          *MonotonicAdvanceCommittedV1     `json:"committed,omitempty"`
	Superseded         *MonotonicAdvanceSupersededV1    `json:"superseded,omitempty"`
	AuthorityAlgorithm string                           `json:"authorityAlgorithm"`
	AuthorityKeyID     string                           `json:"authorityKeyId"`
	AuthorityPublicKey string                           `json:"authorityPublicKey"`
	AuthoritySignature string                           `json:"authoritySignature"`
	RecordDigest       string                           `json:"recordDigest"`
}

func NewCommittedMonotonicAdvanceSettlementV1(
	intent MonotonicAdvanceIntentV1,
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	sign MonotonicAdvanceSignFuncV1,
) (MonotonicAdvanceSettlementV1, error) {
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceV1(intent.PreviousCheckpoint, intent.AdvanceRequest, receipt); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	settlement := newMonotonicAdvanceSettlementV1(intent, MonotonicAdvanceSettlementCommittedV1)
	settlement.Committed = &MonotonicAdvanceCommittedV1{Receipt: receipt}
	if err := signMonotonicAdvanceSettlementV1(&settlement, sign); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	if err := ValidateMonotonicAdvanceSettlementForIntentV1(settlement, intent, nil); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	return settlement, nil
}

func NewSupersededMonotonicAdvanceSettlementV1(
	intent MonotonicAdvanceIntentV1,
	observeRequest domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	committedRange []MonotonicAdvanceCommittedStepV1,
	sign MonotonicAdvanceSignFuncV1,
) (MonotonicAdvanceSettlementV1, error) {
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	references, err := NewMonotonicAdvanceRangeReferencesV1(
		intent.PreviousCheckpoint, committedRange, observation.Checkpoint,
	)
	if err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	settlement := newMonotonicAdvanceSettlementV1(intent, MonotonicAdvanceSettlementSupersededV1)
	settlement.Superseded = &MonotonicAdvanceSupersededV1{
		ObserveRequest: observeRequest, Observation: observation, RangeReferences: references,
	}
	if err := signMonotonicAdvanceSettlementV1(&settlement, sign); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	if err := ValidateMonotonicAdvanceSettlementForIntentV1(settlement, intent, committedRange); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	return settlement, nil
}

func ValidateMonotonicAdvanceSettlementV1(settlement MonotonicAdvanceSettlementV1) error {
	if err := validateMonotonicAdvanceSettlementPayloadV1(settlement); err != nil {
		return err
	}
	encoded, err := json.Marshal(settlement)
	if err != nil || len(encoded) > maxMonotonicAdvanceSettlementBytesV1 {
		return errors.New("monotonic advance settlement exceeds its canonical bound")
	}
	if !canonicalSHA256V1(settlement.RecordDigest) || settlement.RecordDigest != monotonicAdvanceSettlementDigestV1(settlement) {
		return errors.New("monotonic advance settlement digest is invalid")
	}
	publicKey, publicErr := decodeCanonicalPublicKeyV1(settlement.AuthorityPublicKey)
	signature, signatureErr := decodeCanonicalSignatureV1(settlement.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || settlement.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), MonotonicAdvanceSettlementSigningBytesV1(settlement), signature) {
		return errors.New("monotonic advance settlement signature is invalid")
	}
	return nil
}

// ValidateMonotonicAdvanceSettlementForIntentV1 is the authority-bearing
// validator. A superseded record is never accepted from references alone: the
// caller must provide the exact committed range those references name.
func ValidateMonotonicAdvanceSettlementForIntentV1(
	settlement MonotonicAdvanceSettlementV1,
	intent MonotonicAdvanceIntentV1,
	committedRange []MonotonicAdvanceCommittedStepV1,
) error {
	if err := ValidateMonotonicAdvanceSettlementV1(settlement); err != nil {
		return err
	}
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return err
	}
	if settlement.InstallationID != intent.InstallationID || settlement.EnrollmentID != intent.EnrollmentID ||
		settlement.Namespace != intent.Namespace || settlement.Root != intent.Root ||
		settlement.MutationID != intent.MutationID || settlement.IntentDigest != intent.RecordDigest ||
		settlement.RequestDigest != intent.RequestDigest || settlement.AuthorityAlgorithm != intent.AuthorityAlgorithm ||
		settlement.AuthorityKeyID != intent.AuthorityKeyID || settlement.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("monotonic advance settlement intent binding mismatch")
	}
	switch settlement.Kind {
	case MonotonicAdvanceSettlementCommittedV1:
		if len(committedRange) != 0 {
			return errors.New("committed settlement must not carry a supersession range")
		}
		return validateCommittedSettlementForIntentV1(settlement, intent)
	case MonotonicAdvanceSettlementSupersededV1:
		return ValidateMonotonicAdvanceSupersededSettlementV1(settlement, intent, committedRange)
	default:
		return errors.New("monotonic advance settlement kind is unknown")
	}
}

// ValidateCommittedMonotonicAdvanceExactReplayV1 binds both exact replay
// request bytes and exact receipt bytes to the stored intent/settlement pair.
func ValidateCommittedMonotonicAdvanceExactReplayV1(
	intent MonotonicAdvanceIntentV1,
	settlement MonotonicAdvanceSettlementV1,
	replayRequest domainsecurity.MonotonicHeadAdvanceRequestV1,
	replayReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
) error {
	if err := ValidateMonotonicAdvanceSettlementForIntentV1(settlement, intent, nil); err != nil {
		return err
	}
	if settlement.Kind != MonotonicAdvanceSettlementCommittedV1 {
		return errors.New("monotonic advance replay settlement is not committed")
	}
	return domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(
		intent.AdvanceRequest, settlement.Committed.Receipt, replayRequest, replayReceipt,
	)
}

func ParseMonotonicAdvanceSettlementV1(body []byte) (MonotonicAdvanceSettlementV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxMonotonicAdvanceSettlementBytesV1, MaxDepth: 16,
		MaxTokens: 2_000_000, MaxStringBytes: 64 << 10,
	}); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var settlement MonotonicAdvanceSettlementV1
	if err := decoder.Decode(&settlement); err != nil {
		return MonotonicAdvanceSettlementV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MonotonicAdvanceSettlementV1{}, errors.New("monotonic advance settlement contains trailing JSON")
	}
	canonical, err := json.Marshal(settlement)
	if err != nil || !bytes.Equal(body, canonical) {
		return MonotonicAdvanceSettlementV1{}, errors.New("monotonic advance settlement is not canonically encoded")
	}
	return settlement, ValidateMonotonicAdvanceSettlementV1(settlement)
}

func MonotonicAdvanceSettlementV1Bytes(settlement MonotonicAdvanceSettlementV1) ([]byte, error) {
	if err := ValidateMonotonicAdvanceSettlementV1(settlement); err != nil {
		return nil, err
	}
	return json.Marshal(settlement)
}

func MonotonicAdvanceSettlementSigningBytesV1(settlement MonotonicAdvanceSettlementV1) []byte {
	settlement.AuthoritySignature = ""
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), monotonicAdvanceSettlementSignatureDomainV1...)
	return append(out, digest[:]...)
}

func newMonotonicAdvanceSettlementV1(
	intent MonotonicAdvanceIntentV1,
	kind MonotonicAdvanceSettlementKindV1,
) MonotonicAdvanceSettlementV1 {
	return MonotonicAdvanceSettlementV1{
		SchemaVersion:      MonotonicAdvanceSettlementSchemaVersionV1,
		Purpose:            MonotonicAdvanceSettlementPurposeV1,
		Kind:               kind,
		InstallationID:     intent.InstallationID,
		EnrollmentID:       intent.EnrollmentID,
		Namespace:          intent.Namespace,
		Root:               intent.Root,
		MutationID:         intent.MutationID,
		IntentDigest:       intent.RecordDigest,
		RequestDigest:      intent.RequestDigest,
		AuthorityAlgorithm: intent.AuthorityAlgorithm,
		AuthorityKeyID:     intent.AuthorityKeyID,
		AuthorityPublicKey: intent.AuthorityPublicKey,
	}
}

func signMonotonicAdvanceSettlementV1(
	settlement *MonotonicAdvanceSettlementV1,
	sign MonotonicAdvanceSignFuncV1,
) error {
	if settlement == nil || sign == nil {
		return errors.New("monotonic advance settlement signing authority is invalid")
	}
	if err := validateMonotonicAdvanceSettlementPayloadV1(*settlement); err != nil {
		return err
	}
	signature, err := sign(MonotonicAdvanceSettlementSigningBytesV1(*settlement))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("monotonic advance settlement signing failed")
	}
	settlement.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	settlement.RecordDigest = monotonicAdvanceSettlementDigestV1(*settlement)
	return ValidateMonotonicAdvanceSettlementV1(*settlement)
}

func validateMonotonicAdvanceSettlementPayloadV1(settlement MonotonicAdvanceSettlementV1) error {
	if settlement.SchemaVersion != MonotonicAdvanceSettlementSchemaVersionV1 ||
		settlement.Purpose != MonotonicAdvanceSettlementPurposeV1 ||
		!canonicalSHA256V1(settlement.InstallationID) || !canonicalSHA256V1(settlement.EnrollmentID) ||
		!canonicalSHA256V1(settlement.MutationID) || !canonicalSHA256V1(settlement.IntentDigest) ||
		!canonicalSHA256V1(settlement.RequestDigest) || settlement.AuthorityAlgorithm != MonotonicAdvanceAlgorithmV1 ||
		!canonicalSHA256V1(settlement.AuthorityKeyID) {
		return errors.New("monotonic advance settlement is incomplete")
	}
	namespace, err := NamespaceForAdvanceRootV1(settlement.Root)
	if err != nil || namespace != settlement.Namespace {
		return errors.New("monotonic advance settlement root namespace mismatch")
	}
	if settlement.Committed != nil == (settlement.Superseded != nil) {
		return errors.New("monotonic advance settlement must select exactly one branch")
	}
	switch settlement.Kind {
	case MonotonicAdvanceSettlementCommittedV1:
		if settlement.Committed == nil || settlement.Superseded != nil ||
			domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(settlement.Committed.Receipt) != nil ||
			settlement.Committed.Receipt.MutationID != settlement.MutationID ||
			settlement.Committed.Receipt.RequestDigest != settlement.RequestDigest {
			return errors.New("committed monotonic advance settlement is invalid")
		}
	case MonotonicAdvanceSettlementSupersededV1:
		if settlement.Superseded == nil || settlement.Committed != nil ||
			domainsecurity.ValidateMonotonicHeadObserveRequestV1(settlement.Superseded.ObserveRequest) != nil ||
			domainsecurity.ValidateMonotonicHeadObservationV1(settlement.Superseded.Observation) != nil ||
			settlement.Superseded.ObserveRequest.InstallationID != settlement.InstallationID ||
			settlement.Superseded.ObserveRequest.EnrollmentID != settlement.EnrollmentID ||
			settlement.Superseded.ObserveRequest.Namespace != settlement.Namespace ||
			settlement.Superseded.Observation.Checkpoint.InstallationID != settlement.InstallationID ||
			settlement.Superseded.Observation.Checkpoint.EnrollmentID != settlement.EnrollmentID ||
			settlement.Superseded.Observation.Checkpoint.Namespace != settlement.Namespace ||
			settlement.Superseded.Observation.RequestDigest != settlement.Superseded.ObserveRequest.RequestDigest ||
			settlement.Superseded.Observation.ChallengeNonce != settlement.Superseded.ObserveRequest.ChallengeNonce ||
			len(settlement.Superseded.RangeReferences) == 0 ||
			len(settlement.Superseded.RangeReferences) > maxMonotonicAdvanceRangeStepsV1 {
			return errors.New("superseded monotonic advance settlement is invalid")
		}
		seen := make(map[string]struct{}, len(settlement.Superseded.RangeReferences))
		for _, reference := range settlement.Superseded.RangeReferences {
			if err := ValidateMonotonicAdvanceRangeReferenceV1(reference); err != nil {
				return err
			}
			referenceNamespace, err := NamespaceForAdvanceRootV1(reference.Root)
			if err != nil || referenceNamespace != settlement.Namespace {
				return errors.New("superseded monotonic advance range reference namespace mismatch")
			}
			if _, exists := seen[reference.SettlementDigest]; exists {
				return errors.New("superseded monotonic advance range reference is duplicated")
			}
			seen[reference.SettlementDigest] = struct{}{}
		}
	default:
		return errors.New("monotonic advance settlement kind is unknown")
	}
	return nil
}

func validateCommittedSettlementForIntentV1(
	settlement MonotonicAdvanceSettlementV1,
	intent MonotonicAdvanceIntentV1,
) error {
	if settlement.Kind != MonotonicAdvanceSettlementCommittedV1 || settlement.Committed == nil || settlement.Superseded != nil {
		return errors.New("monotonic advance settlement is not committed")
	}
	return domainsecurity.ValidateMonotonicHeadAdvanceV1(
		intent.PreviousCheckpoint, intent.AdvanceRequest, settlement.Committed.Receipt,
	)
}

func monotonicAdvanceSettlementDigestV1(settlement MonotonicAdvanceSettlementV1) string {
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	payload := append([]byte(nil), monotonicAdvanceSettlementDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}
