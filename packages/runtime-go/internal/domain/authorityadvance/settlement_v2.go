package authorityadvance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	monotonicAdvanceSettlementSignatureDomainV2 = []byte("analytix.monotonic-advance-settlement/signature/v2\x00")
	monotonicAdvanceSettlementDigestDomainV2    = []byte("analytix.monotonic-advance-settlement/digest/v2\x00")
)

type MonotonicAdvanceSettlementKindV2 string

const (
	MonotonicAdvanceSettlementCommittedV2  MonotonicAdvanceSettlementKindV2 = "committed"
	MonotonicAdvanceSettlementSupersededV2 MonotonicAdvanceSettlementKindV2 = "superseded"
)

type MonotonicAdvanceCommittedV2 struct {
	Receipt domainsecurity.MonotonicHeadAdvanceReceiptV1 `json:"receipt"`
}

type MonotonicAdvanceSupersededV2 struct {
	ObserveRequest  domainsecurity.MonotonicHeadObserveRequestV1 `json:"observeRequest"`
	Observation     domainsecurity.MonotonicHeadObservationV1    `json:"observation"`
	RangeReferences []MonotonicAdvanceRangeReferenceV2           `json:"rangeReferences"`
}

type MonotonicAdvanceSettlementV2 struct {
	SchemaVersion      int                              `json:"schemaVersion"`
	Purpose            string                           `json:"purpose"`
	Kind               MonotonicAdvanceSettlementKindV2 `json:"kind"`
	InstallationID     string                           `json:"installationId"`
	EnrollmentID       string                           `json:"enrollmentId"`
	Namespace          string                           `json:"namespace"`
	Root               AdvanceRootV2                    `json:"root"`
	MutationID         string                           `json:"mutationId"`
	IntentDigest       string                           `json:"intentDigest"`
	RequestDigest      string                           `json:"requestDigest"`
	Committed          *MonotonicAdvanceCommittedV2     `json:"committed,omitempty"`
	Superseded         *MonotonicAdvanceSupersededV2    `json:"superseded,omitempty"`
	AuthorityAlgorithm string                           `json:"authorityAlgorithm"`
	AuthorityKeyID     string                           `json:"authorityKeyId"`
	AuthorityPublicKey string                           `json:"authorityPublicKey"`
	AuthoritySignature string                           `json:"authoritySignature"`
	RecordDigest       string                           `json:"recordDigest"`
}

func NewCommittedMonotonicAdvanceSettlementV2(
	intent MonotonicAdvanceIntentV2,
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	sign MonotonicAdvanceSignFuncV2,
) (MonotonicAdvanceSettlementV2, error) {
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	settlement := newMonotonicAdvanceSettlementV2(intent, MonotonicAdvanceSettlementCommittedV2)
	settlement.Committed = &MonotonicAdvanceCommittedV2{Receipt: receipt}
	if validateCommittedSettlementForIntentV2(settlement, intent) != nil {
		return MonotonicAdvanceSettlementV2{}, errors.New("committed monotonic advance V2 receipt is invalid")
	}
	if err := signMonotonicAdvanceSettlementV2(&settlement, sign); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	if err := ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	return settlement, nil
}

func NewSupersededMonotonicAdvanceSettlementV2(
	intent MonotonicAdvanceIntentV2,
	observeRequest domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	committedRange []MonotonicAdvanceCommittedStepV2,
	sign MonotonicAdvanceSignFuncV2,
) (MonotonicAdvanceSettlementV2, error) {
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	references, err := NewMonotonicAdvanceRangeReferencesV2(
		intent.PreviousCheckpoint, committedRange, observation.Checkpoint,
	)
	if err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	settlement := newMonotonicAdvanceSettlementV2(intent, MonotonicAdvanceSettlementSupersededV2)
	settlement.Superseded = &MonotonicAdvanceSupersededV2{
		ObserveRequest: observeRequest, Observation: observation, RangeReferences: references,
	}
	if err := signMonotonicAdvanceSettlementV2(&settlement, sign); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	if err := ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, committedRange); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	return settlement, nil
}

func ValidateMonotonicAdvanceSettlementV2(settlement MonotonicAdvanceSettlementV2) error {
	if err := validateMonotonicAdvanceSettlementPayloadV2(settlement); err != nil {
		return err
	}
	encoded, err := json.Marshal(settlement)
	if err != nil || len(encoded) > MaxMonotonicAdvanceJournalRecordBytesV2 {
		return errors.New("monotonic advance V2 settlement exceeds its canonical bound")
	}
	if !canonicalSHA256V1(settlement.RecordDigest) || settlement.RecordDigest != monotonicAdvanceSettlementDigestV2(settlement) {
		return errors.New("monotonic advance V2 settlement digest is invalid")
	}
	publicKey, publicErr := decodeCanonicalPublicKeyV1(settlement.AuthorityPublicKey)
	signature, signatureErr := decodeCanonicalSignatureV1(settlement.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || settlement.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), MonotonicAdvanceSettlementSigningBytesV2(settlement), signature) {
		return errors.New("monotonic advance V2 settlement signature is invalid")
	}
	return nil
}

func ValidateMonotonicAdvanceSettlementForIntentV2(
	settlement MonotonicAdvanceSettlementV2,
	intent MonotonicAdvanceIntentV2,
	committedRange []MonotonicAdvanceCommittedStepV2,
) error {
	if err := ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent); err != nil {
		return err
	}
	switch settlement.Kind {
	case MonotonicAdvanceSettlementCommittedV2:
		if len(committedRange) != 0 {
			return errors.New("committed V2 settlement must not carry a supersession range")
		}
		return validateCommittedSettlementForIntentV2(settlement, intent)
	case MonotonicAdvanceSettlementSupersededV2:
		return ValidateMonotonicAdvanceSupersededSettlementV2(settlement, intent, committedRange)
	default:
		return errors.New("monotonic advance V2 settlement kind is unknown")
	}
}

// ValidateMonotonicAdvanceSettlementIntentHeaderV2 authenticates the complete
// immutable record and its exact intent/authority header without treating a
// superseded range reference as proof. Callers may use it before resolving the
// separately stored committed range.
func ValidateMonotonicAdvanceSettlementIntentHeaderV2(
	settlement MonotonicAdvanceSettlementV2,
	intent MonotonicAdvanceIntentV2,
) error {
	if err := ValidateMonotonicAdvanceSettlementV2(settlement); err != nil {
		return err
	}
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return err
	}
	if settlement.InstallationID != intent.InstallationID || settlement.EnrollmentID != intent.EnrollmentID ||
		settlement.Namespace != intent.Namespace || settlement.Root != intent.Root ||
		settlement.MutationID != intent.MutationID || settlement.IntentDigest != intent.RecordDigest ||
		settlement.RequestDigest != intent.RequestDigest || settlement.AuthorityAlgorithm != intent.AuthorityAlgorithm ||
		settlement.AuthorityKeyID != intent.AuthorityKeyID || settlement.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("monotonic advance V2 settlement intent binding mismatch")
	}
	return nil
}

func ValidateCommittedMonotonicAdvanceExactReplayV2(
	intent MonotonicAdvanceIntentV2,
	settlement MonotonicAdvanceSettlementV2,
	replayRequest domainsecurity.MonotonicHeadAdvanceRequestV1,
	replayReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
) error {
	if err := ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil); err != nil {
		return err
	}
	if settlement.Kind != MonotonicAdvanceSettlementCommittedV2 {
		return errors.New("monotonic advance V2 replay settlement is not committed")
	}
	return domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(
		intent.AdvanceRequest, settlement.Committed.Receipt, replayRequest, replayReceipt,
	)
}

func ParseMonotonicAdvanceSettlementV2(body []byte) (MonotonicAdvanceSettlementV2, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxMonotonicAdvanceJournalRecordBytesV2, MaxDepth: 16,
		MaxTokens: 2_000_000, MaxStringBytes: 64 << 10,
	}); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var settlement MonotonicAdvanceSettlementV2
	if err := decoder.Decode(&settlement); err != nil {
		return MonotonicAdvanceSettlementV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MonotonicAdvanceSettlementV2{}, errors.New("monotonic advance V2 settlement contains trailing JSON")
	}
	canonical, err := json.Marshal(settlement)
	if err != nil || !bytes.Equal(body, canonical) {
		return MonotonicAdvanceSettlementV2{}, errors.New("monotonic advance V2 settlement is not canonically encoded")
	}
	return settlement, ValidateMonotonicAdvanceSettlementV2(settlement)
}

func MonotonicAdvanceSettlementV2Bytes(settlement MonotonicAdvanceSettlementV2) ([]byte, error) {
	if err := ValidateMonotonicAdvanceSettlementV2(settlement); err != nil {
		return nil, err
	}
	return json.Marshal(settlement)
}

func MonotonicAdvanceSettlementSigningBytesV2(settlement MonotonicAdvanceSettlementV2) []byte {
	settlement.AuthoritySignature = ""
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), monotonicAdvanceSettlementSignatureDomainV2...)
	return append(out, digest[:]...)
}

func newMonotonicAdvanceSettlementV2(
	intent MonotonicAdvanceIntentV2,
	kind MonotonicAdvanceSettlementKindV2,
) MonotonicAdvanceSettlementV2 {
	return MonotonicAdvanceSettlementV2{
		SchemaVersion:      MonotonicAdvanceSettlementSchemaVersionV2,
		Purpose:            MonotonicAdvanceSettlementPurposeV2,
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

func signMonotonicAdvanceSettlementV2(
	settlement *MonotonicAdvanceSettlementV2,
	sign MonotonicAdvanceSignFuncV2,
) error {
	if settlement == nil || sign == nil {
		return errors.New("monotonic advance V2 settlement signing authority is invalid")
	}
	if err := validateMonotonicAdvanceSettlementPayloadV2(*settlement); err != nil {
		return err
	}
	signature, err := sign(MonotonicAdvanceSettlementSigningBytesV2(*settlement))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("monotonic advance V2 settlement signing failed")
	}
	settlement.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	settlement.RecordDigest = monotonicAdvanceSettlementDigestV2(*settlement)
	return ValidateMonotonicAdvanceSettlementV2(*settlement)
}

func validateMonotonicAdvanceSettlementPayloadV2(settlement MonotonicAdvanceSettlementV2) error {
	if settlement.SchemaVersion != MonotonicAdvanceSettlementSchemaVersionV2 ||
		settlement.Purpose != MonotonicAdvanceSettlementPurposeV2 ||
		!canonicalSHA256V1(settlement.InstallationID) || !canonicalSHA256V1(settlement.EnrollmentID) ||
		!canonicalSHA256V1(settlement.MutationID) || !canonicalSHA256V1(settlement.IntentDigest) ||
		!canonicalSHA256V1(settlement.RequestDigest) || settlement.AuthorityAlgorithm != MonotonicAdvanceAlgorithmV2 ||
		!canonicalSHA256V1(settlement.AuthorityKeyID) {
		return errors.New("monotonic advance V2 settlement is incomplete")
	}
	namespace, err := NamespaceForAdvanceRootV2(settlement.Root)
	if err != nil || namespace != settlement.Namespace {
		return errors.New("monotonic advance V2 settlement root namespace mismatch")
	}
	if settlement.Committed != nil == (settlement.Superseded != nil) {
		return errors.New("monotonic advance V2 settlement must select exactly one branch")
	}
	switch settlement.Kind {
	case MonotonicAdvanceSettlementCommittedV2:
		if settlement.Committed == nil || settlement.Superseded != nil ||
			domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(settlement.Committed.Receipt) != nil ||
			settlement.Committed.Receipt.MutationID != settlement.MutationID ||
			settlement.Committed.Receipt.RequestDigest != settlement.RequestDigest {
			return errors.New("committed monotonic advance V2 settlement is invalid")
		}
	case MonotonicAdvanceSettlementSupersededV2:
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
			len(settlement.Superseded.RangeReferences) > MaxMonotonicAdvanceRangeStepsV2 {
			return errors.New("superseded monotonic advance V2 settlement is invalid")
		}
		seen := make(map[string]struct{}, len(settlement.Superseded.RangeReferences))
		for _, reference := range settlement.Superseded.RangeReferences {
			if err := ValidateMonotonicAdvanceRangeReferenceV2(reference); err != nil {
				return err
			}
			referenceNamespace, err := NamespaceForAdvanceRootV2(reference.Root)
			if err != nil || referenceNamespace != settlement.Namespace {
				return errors.New("superseded monotonic advance V2 reference namespace mismatch")
			}
			if _, exists := seen[reference.SettlementDigest]; exists {
				return errors.New("superseded monotonic advance V2 reference is duplicated")
			}
			seen[reference.SettlementDigest] = struct{}{}
		}
	default:
		return errors.New("monotonic advance V2 settlement kind is unknown")
	}
	return nil
}

func validateCommittedSettlementForIntentV2(
	settlement MonotonicAdvanceSettlementV2,
	intent MonotonicAdvanceIntentV2,
) error {
	if settlement.Kind != MonotonicAdvanceSettlementCommittedV2 || settlement.Committed == nil ||
		settlement.Superseded != nil {
		return errors.New("monotonic advance V2 settlement is not committed")
	}
	receipt := settlement.Committed.Receipt
	switch intent.Root {
	case AdvanceRootThreadRiskV2:
		if intent.Transition.ThreadRiskGenesis != nil {
			return domainsecurity.ValidateThreadRiskAuthorityFirstWitnessAdvanceV1(
				intent.Transition.ThreadRiskGenesis.FirstIndex,
				intent.PreviousCheckpoint,
				intent.AdvanceRequest,
				receipt,
			)
		}
		return domainsecurity.ValidateThreadRiskAuthorityWitnessAdvanceV1(
			intent.Transition.ThreadRisk.PreviousIndex,
			intent.Transition.ThreadRisk.NextIndex,
			intent.PreviousCheckpoint,
			intent.AdvanceRequest,
			receipt,
		)
	case AdvanceRootEvidenceGenesisV2:
		return domainevidence.ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(
			intent.Transition.EvidenceGenesis.FirstBundle,
			intent.PreviousCheckpoint,
			intent.AdvanceRequest,
			receipt,
		)
	case AdvanceRootDatasetSnapshotV2, AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2:
		return domainevidence.ValidateEvidenceAuthorityBundleWitnessAdvanceV1(
			intent.Transition.EvidenceBundle.PreviousBundle,
			intent.Transition.EvidenceBundle.NextBundle,
			intent.PreviousCheckpoint,
			intent.AdvanceRequest,
			receipt,
		)
	default:
		return errors.New("committed monotonic advance V2 root is unknown")
	}
}

func monotonicAdvanceSettlementDigestV2(settlement MonotonicAdvanceSettlementV2) string {
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	payload := append([]byte(nil), monotonicAdvanceSettlementDigestDomainV2...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}
