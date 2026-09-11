package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	// V5 is the only current-write/live accepted-final contract. V3 boundary
	// and V4 witnessed-fact records remain strict audit inputs only.
	AcceptedFinalRecordVersion                = 5
	PrivateAcceptedFinalRecordVersion         = 5
	BoundaryAcceptedFinalRecordVersion        = 3
	WitnessedFactAcceptedFinalRecordVersion   = 4
	WitnessedFactPrivateFinalRecordVersion    = 4
	PreviousAcceptedFinalRecordVersion        = 2
	EvidenceRegistryHeadVersion               = 1
	AcceptedFinalAuthorityPurpose             = "analytix.case-final/v1"
	AcceptedFinalAuthorityAlgorithm           = "Ed25519"
	HistoricalFinalAnswerRendererVersion      = "analytix.host-final-renderer/v1"
	FinalAnswerRendererVersion                = "analytix.host-final-renderer/v2"
	FinalEvidenceGateVersion                  = "analytix.final-evidence-gate/v4"
	HistoricalBoundaryFinalGateVersion        = "analytix.final-evidence-gate/v2"
	WitnessedFinalEvidenceGateVersion         = "analytix.final-evidence-gate/v3"
	LegacyFinalEvidenceGateVersion            = "analytix.final-evidence-gate/v1"
	ClaimVerifierPolicyVersion                = "analytix.claim-verifier-policy/v1"
	CaseEvidenceAuthorityUnavailableBlockerV1 = "case_public_authority_unavailable"
)

var acceptedFinalSignatureDomain = []byte("analytix.final-answer-authority/v1\x00")

func validFinalAnswerRendererVersion(value string) bool {
	return value == HistoricalFinalAnswerRendererVersion || value == FinalAnswerRendererVersion
}

// EvidenceRegistryHead is the immutable registry state against which the
// final evidence gate ran. Sequence zero is the valid empty-registry state.
type EvidenceRegistryHead struct {
	SchemaVersion     int    `json:"schemaVersion"`
	ContextDigest     string `json:"contextDigest"`
	DatasetSnapshotID string `json:"datasetSnapshotId"`
	Sequence          uint64 `json:"sequence"`
	StateDigest       string `json:"stateDigest"`
}

// AcceptedFinalRecord is the public, PII-free proof that a deterministic host
// rendering passed the final evidence gate. The self-contained public-key
// check proves mathematical integrity; production trust additionally requires
// the key to match the private installation authority during runtime preflight.
type AcceptedFinalRecord struct {
	SchemaVersion                  int                            `json:"schemaVersion"`
	AuthorityPurpose               string                         `json:"authorityPurpose"`
	AuthorityAlgorithm             string                         `json:"authorityAlgorithm"`
	AuthorityKeyID                 string                         `json:"authorityKeyId"`
	AuthorityPublicKey             string                         `json:"authorityPublicKey"`
	ThreadID                       string                         `json:"threadId"`
	TurnID                         string                         `json:"turnId"`
	EnvelopeDigest                 string                         `json:"envelopeDigest"`
	ContextDigest                  string                         `json:"contextDigest"`
	ContextEpoch                   uint64                         `json:"contextEpoch"`
	DatasetSnapshotID              string                         `json:"datasetSnapshotId"`
	Variant                        FinalAnswerVariant             `json:"variant"`
	TerminalReason                 string                         `json:"terminalReason"`
	RenderedTextSHA256             string                         `json:"renderedTextSha256"`
	RegistrySequence               uint64                         `json:"registrySequence"`
	RegistryStateDigest            string                         `json:"registryStateDigest"`
	RendererVersion                string                         `json:"rendererVersion"`
	FinalGateVersion               string                         `json:"finalGateVersion"`
	VerifierVersion                string                         `json:"verifierVersion"`
	PublicView                     *AcceptedFinalPublicViewCoreV2 `json:"publicView,omitempty"`
	PublicViewDigest               string                         `json:"publicViewDigest,omitempty"`
	PublicationSnapshotProofDigest string                         `json:"publicationSnapshotProofDigest,omitempty"`
	FactFinalWitnessAdmission      *FactFinalWitnessAdmissionV1   `json:"factFinalWitnessAdmission,omitempty"`
	PrivateRecordDigest            string                         `json:"privateRecordDigest"`
	AcceptedAt                     string                         `json:"acceptedAt"`
	AuthoritySignature             string                         `json:"authoritySignature"`
	RecordDigest                   string                         `json:"recordDigest"`
}

type AcceptedFinalRecordInput struct {
	Context                   domainsecurity.TurnSecurityContext
	Envelope                  FinalAnswerEnvelope
	RenderedText              string
	RegistryHead              EvidenceRegistryHead
	PublicationSnapshotProof  *PublicationSnapshotProof
	FactFinalWitnessAdmission *FactFinalWitnessAdmissionV1
	FactFinalWitnessAuthority *FactFinalWitnessAdmissionInputV1
	PrivateRecordDigest       string
	AcceptedAt                time.Time
	AuthorityKeyID            string
	AuthorityPublicKey        []byte
}

// PrivateAcceptedFinalRecord keeps the complete host-authoritative final on
// the protected evidence side. Public history contains only AcceptedFinal.
type PrivateAcceptedFinalRecord struct {
	SchemaVersion            int                                `json:"schemaVersion"`
	SecurityContext          domainsecurity.TurnSecurityContext `json:"securityContext"`
	Envelope                 FinalAnswerEnvelope                `json:"envelope"`
	RenderedText             string                             `json:"renderedText"`
	RegistryHead             EvidenceRegistryHead               `json:"registryHead"`
	PublicationIntent        TerminalPublicationIntent          `json:"publicationIntent"`
	PublicationSnapshotProof *PublicationSnapshotProof          `json:"publicationSnapshotProof,omitempty"`
	AcceptedFinal            AcceptedFinalRecord                `json:"acceptedFinal"`
	PrivateRecordDigest      string                             `json:"privateRecordDigest"`
	StoreDigest              string                             `json:"storeDigest"`
}

type AcceptedFinalSignFunc func([]byte) ([]byte, error)

func NewEvidenceRegistryHead(registry EvidenceReceiptRegistry) (EvidenceRegistryHead, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceRegistryHead{}, err
	}
	head := EvidenceRegistryHead{
		SchemaVersion: EvidenceRegistryHeadVersion, ContextDigest: registry.ContextDigest,
		DatasetSnapshotID: registry.DatasetSnapshotID, Sequence: registry.Sequence, StateDigest: registry.StateDigest,
	}
	return head, ValidateEvidenceRegistryHead(head)
}

func ValidateEvidenceRegistryHead(head EvidenceRegistryHead) error {
	if head.SchemaVersion != EvidenceRegistryHeadVersion || !validSHA256(head.ContextDigest) ||
		strings.TrimSpace(head.DatasetSnapshotID) == "" || !validSHA256(head.StateDigest) {
		return errors.New("evidence registry head is invalid")
	}
	return nil
}

func PrivateAcceptedFinalDigest(context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, renderedText string, intent TerminalPublicationIntent, proofs ...*PublicationSnapshotProof) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(context) != nil ||
		(FinalAnswerRequiresPublicationSnapshotProof(envelope) && domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil) {
		return "", errors.New("private accepted final requires current V2 case publication authority")
	}
	var proof *PublicationSnapshotProof
	if len(proofs) > 0 {
		proof = proofs[0]
	}
	if FinalAnswerRequiresPublicationSnapshotProof(envelope) {
		return "", errors.New("fact-bearing private accepted final requires witnessed evidence authority")
	}
	return privateAcceptedFinalDigestForVersion(PrivateAcceptedFinalRecordVersion, context, envelope, renderedText, intent, proof, nil)
}

func PrivateAcceptedFinalDigestWithFactWitnessV1(
	context domainsecurity.TurnSecurityContext,
	envelope FinalAnswerEnvelope,
	renderedText string,
	intent TerminalPublicationIntent,
	proof *PublicationSnapshotProof,
	admission *FactFinalWitnessAdmissionV1,
) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!FinalAnswerRequiresPublicationSnapshotProof(envelope) || admission == nil {
		return "", errors.New("fact-bearing private accepted final requires witnessed evidence authority")
	}
	return privateAcceptedFinalDigestForVersion(
		PrivateAcceptedFinalRecordVersion, context, envelope, renderedText, intent, proof, admission,
	)
}

func privateAcceptedFinalDigestForVersion(version int, context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, renderedText string, intent TerminalPublicationIntent, proof *PublicationSnapshotProof, admission *FactFinalWitnessAdmissionV1) (string, error) {
	return privateAcceptedFinalDigestForVersionAndRenderer(
		version, FinalAnswerRendererVersion, context, envelope, renderedText, intent, proof, admission,
	)
}

func privateAcceptedFinalDigestForVersionAndRenderer(version int, rendererVersion string, context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, renderedText string, intent TerminalPublicationIntent, proof *PublicationSnapshotProof, admission *FactFinalWitnessAdmissionV1) (string, error) {
	if err := domainsecurity.ValidateTurnSecurityContext(context); err != nil || ValidateFinalAnswerEnvelope(envelope) != nil ||
		ValidateTerminalPublicationIntent(intent, envelope.TerminalReason) != nil {
		return "", errors.New("private accepted final input is invalid")
	}
	if version == WitnessedFactPrivateFinalRecordVersion && rendererVersion != HistoricalFinalAnswerRendererVersion {
		return "", errors.New("historical private accepted final renderer is invalid")
	}
	expectedText, err := RenderFinalAnswerAtVersion(envelope, rendererVersion)
	if err != nil || renderedText != expectedText || strings.TrimSpace(renderedText) == "" ||
		envelope.ContextDigest != context.ContextDigest || envelope.ContextEpoch != context.ContextEpoch ||
		envelope.DatasetSnapshotID != context.DatasetSnapshotID {
		return "", errors.New("private accepted final body is inconsistent")
	}
	if FinalAnswerRequiresPublicationSnapshotProof(envelope) {
		if ValidatePublicationSnapshotProofValue(proof, context, envelope, nil) != nil {
			return "", errors.New("private accepted final lacks publication snapshot proof")
		}
		if version == WitnessedFactPrivateFinalRecordVersion || version == PrivateAcceptedFinalRecordVersion {
			if admission == nil || ValidateFactFinalWitnessAdmissionV1(*admission) != nil ||
				(version == WitnessedFactPrivateFinalRecordVersion &&
					admission.SchemaVersion != FactFinalWitnessAdmissionSchemaVersionV1) ||
				admission.ContextDigest != context.ContextDigest || admission.DatasetSnapshotID != context.DatasetSnapshotID ||
				admission.SourceManifestHash != context.SourceManifestHash || admission.EnvelopeDigest != envelope.EnvelopeDigest ||
				admission.RenderedTextSHA256 != domainsecurity.SHA256Hex([]byte(renderedText)) ||
				admission.PublicationSnapshotProofDigest != proof.ProofDigest ||
				admission.EvidenceReceiptCount != uint64(len(envelope.EvidenceReceiptIDs)) ||
				admission.EvidenceReceiptIDsDigest != factFinalWitnessReceiptIDsDigestV1(envelope.EvidenceReceiptIDs) {
				return "", errors.New("private accepted final lacks witnessed evidence authority")
			}
		} else if admission != nil {
			return "", errors.New("historical fact final cannot carry witnessed evidence authority")
		}
	} else if proof != nil || admission != nil {
		return "", errors.New("boundary accepted final must not contain publication snapshot proof")
	}
	body := struct {
		SchemaVersion             int                                `json:"schemaVersion"`
		SecurityContext           domainsecurity.TurnSecurityContext `json:"securityContext"`
		Envelope                  FinalAnswerEnvelope                `json:"envelope"`
		RenderedText              string                             `json:"renderedText"`
		Intent                    TerminalPublicationIntent          `json:"publicationIntent"`
		PublicationSnapshotProof  *PublicationSnapshotProof          `json:"publicationSnapshotProof,omitempty"`
		FactFinalWitnessAdmission *FactFinalWitnessAdmissionV1       `json:"factFinalWitnessAdmission,omitempty"`
	}{version, context, envelope, renderedText, intent, clonePublicationSnapshotProof(proof), cloneFactFinalWitnessAdmissionV1(admission)}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded), nil
}

func NewAcceptedFinalRecord(input AcceptedFinalRecordInput, sign AcceptedFinalSignFunc) (AcceptedFinalRecord, error) {
	factBearing := FinalAnswerRequiresPublicationSnapshotProof(input.Envelope)
	if sign == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(input.Context) != nil ||
		(factBearing && domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil) ||
		ValidateFinalAnswerEnvelope(input.Envelope) != nil || ValidateEvidenceRegistryHead(input.RegistryHead) != nil {
		return AcceptedFinalRecord{}, errors.New("accepted final input is invalid")
	}
	if (factBearing && (input.FactFinalWitnessAdmission == nil || input.FactFinalWitnessAuthority == nil)) ||
		(!factBearing && (input.FactFinalWitnessAdmission != nil || input.FactFinalWitnessAuthority != nil)) {
		return AcceptedFinalRecord{}, errors.New("accepted final witness admission is invalid")
	}
	acceptedAt := input.AcceptedAt.UTC()
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
	if input.Envelope.ContextDigest != input.Context.ContextDigest || input.Envelope.ContextEpoch != input.Context.ContextEpoch ||
		input.Envelope.DatasetSnapshotID != input.Context.DatasetSnapshotID || input.RegistryHead.ContextDigest != input.Context.ContextDigest ||
		input.RegistryHead.DatasetSnapshotID != input.Context.DatasetSnapshotID || !validSHA256(input.PrivateRecordDigest) {
		return AcceptedFinalRecord{}, errors.New("accepted final authority binding is invalid")
	}
	proof := clonePublicationSnapshotProof(input.PublicationSnapshotProof)
	if err := ValidatePublicationSnapshotProofValue(proof, input.Context, input.Envelope, &input.RegistryHead); err != nil {
		return AcceptedFinalRecord{}, err
	}
	if proof != nil {
		checkedAt, err := time.Parse(time.RFC3339Nano, proof.CheckedAt)
		if err != nil || acceptedAt.Before(checkedAt) {
			return AcceptedFinalRecord{}, errors.New("accepted final predates its publication snapshot proof")
		}
	}
	admission := cloneFactFinalWitnessAdmissionV1(input.FactFinalWitnessAdmission)
	if admission != nil {
		if ValidateFactFinalWitnessAdmissionExactV1(*admission, *input.FactFinalWitnessAuthority) != nil ||
			input.FactFinalWitnessAuthority.AuthorityKeyID != strings.TrimSpace(input.AuthorityKeyID) ||
			!bytes.Equal(input.FactFinalWitnessAuthority.AuthorityPublicKey, input.AuthorityPublicKey) {
			return AcceptedFinalRecord{}, errors.New("accepted final witness admission lacks exact authority")
		}
		admittedAt, err := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
		if err != nil || acceptedAt.Before(admittedAt) {
			return AcceptedFinalRecord{}, errors.New("accepted final predates its witnessed evidence admission")
		}
	}
	expectedText, err := RenderFinalAnswer(input.Envelope)
	if err != nil || expectedText != input.RenderedText || strings.TrimSpace(input.RenderedText) == "" {
		return AcceptedFinalRecord{}, errors.New("accepted final text is not the deterministic host rendering")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != strings.TrimSpace(input.AuthorityKeyID) {
		return AcceptedFinalRecord{}, errors.New("accepted final authority key is invalid")
	}
	record := AcceptedFinalRecord{
		SchemaVersion: AcceptedFinalRecordVersion, AuthorityPurpose: AcceptedFinalAuthorityPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: input.Context.ThreadID,
		TurnID: input.Context.TurnID, EnvelopeDigest: input.Envelope.EnvelopeDigest, ContextDigest: input.Context.ContextDigest,
		ContextEpoch: input.Context.ContextEpoch, DatasetSnapshotID: input.Context.DatasetSnapshotID, Variant: input.Envelope.Variant,
		TerminalReason: strings.TrimSpace(input.Envelope.TerminalReason), RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(input.RenderedText)),
		RegistrySequence: input.RegistryHead.Sequence, RegistryStateDigest: input.RegistryHead.StateDigest,
		RendererVersion: FinalAnswerRendererVersion, FinalGateVersion: FinalEvidenceGateVersion,
		VerifierVersion: ClaimVerifierPolicyVersion, PrivateRecordDigest: input.PrivateRecordDigest,
		AcceptedAt: acceptedAt.Format(time.RFC3339Nano), FactFinalWitnessAdmission: admission,
	}
	if proof != nil {
		record.PublicationSnapshotProofDigest = proof.ProofDigest
	}
	publicView := buildAcceptedFinalPublicViewCoreV2(input.Envelope, record)
	record.PublicView = &publicView
	record.PublicViewDigest = acceptedFinalPublicViewCoreV2Digest(publicView)
	if factBearing && ValidateFactFinalWitnessAdmissionForRecordV1(admission, record) != nil {
		return AcceptedFinalRecord{}, errors.New("accepted final witness admission does not match final authority")
	}
	signature, err := sign(AcceptedFinalSigningBytes(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return AcceptedFinalRecord{}, errors.New("accepted final authority signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = acceptedFinalRecordDigest(record)
	if err := ValidateAcceptedFinalRecord(record); err != nil {
		return AcceptedFinalRecord{}, err
	}
	return record, nil
}

func NewPrivateAcceptedFinalRecord(context domainsecurity.TurnSecurityContext, envelope FinalAnswerEnvelope, renderedText string, head EvidenceRegistryHead, intent TerminalPublicationIntent, acceptedFinal AcceptedFinalRecord, proofs ...*PublicationSnapshotProof) (PrivateAcceptedFinalRecord, error) {
	factBearing := FinalAnswerRequiresPublicationSnapshotProof(envelope)
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(context) != nil ||
		(factBearing && domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil) {
		return PrivateAcceptedFinalRecord{}, errors.New("private accepted final requires current V2 case publication authority")
	}
	var proof *PublicationSnapshotProof
	if len(proofs) > 0 {
		proof = proofs[0]
	}
	expectedVersion := AcceptedFinalRecordVersion
	privateDigest, err := privateAcceptedFinalDigestForVersionAndRenderer(
		expectedVersion, acceptedFinal.RendererVersion, context, envelope, renderedText, intent, proof, acceptedFinal.FactFinalWitnessAdmission,
	)
	if err != nil || ValidateAcceptedFinalRecord(acceptedFinal) != nil || ValidateEvidenceRegistryHead(head) != nil ||
		acceptedFinal.SchemaVersion != expectedVersion || privateDigest != acceptedFinal.PrivateRecordDigest {
		return PrivateAcceptedFinalRecord{}, errors.New("private accepted final input is invalid")
	}
	record := PrivateAcceptedFinalRecord{
		SchemaVersion: expectedVersion, SecurityContext: context, Envelope: envelope,
		RenderedText: renderedText, RegistryHead: head, PublicationIntent: intent, PublicationSnapshotProof: clonePublicationSnapshotProof(proof),
		AcceptedFinal: acceptedFinal, PrivateRecordDigest: privateDigest,
	}
	record.StoreDigest = privateAcceptedFinalStoreDigest(record)
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return PrivateAcceptedFinalRecord{}, err
	}
	return record, nil
}

func ParseAcceptedFinalRecord(value any) (AcceptedFinalRecord, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalRecord{}, err
	}
	var record AcceptedFinalRecord
	if err := decodeStrictJSON(body, &record, "accepted final record"); err != nil {
		return AcceptedFinalRecord{}, err
	}
	if err := ValidateAcceptedFinalRecord(record); err != nil {
		return AcceptedFinalRecord{}, err
	}
	return record, nil
}

func ParsePrivateAcceptedFinalRecord(raw []byte) (PrivateAcceptedFinalRecord, error) {
	var record PrivateAcceptedFinalRecord
	if err := decodeStrictJSON(raw, &record, "private accepted final record"); err != nil {
		return PrivateAcceptedFinalRecord{}, err
	}
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return PrivateAcceptedFinalRecord{}, err
	}
	return record, nil
}

func ValidateAcceptedFinalRecord(record AcceptedFinalRecord) error {
	previous := record.SchemaVersion == PreviousAcceptedFinalRecordVersion
	boundaryV3 := record.SchemaVersion == BoundaryAcceptedFinalRecordVersion
	witnessedFactV4 := record.SchemaVersion == WitnessedFactAcceptedFinalRecordVersion
	liveV5 := record.SchemaVersion == AcceptedFinalRecordVersion
	if (!previous && !boundaryV3 && !witnessedFactV4 && !liveV5) || record.AuthorityPurpose != AcceptedFinalAuthorityPurpose ||
		record.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm || !validSHA256(record.AuthorityKeyID) ||
		strings.TrimSpace(record.ThreadID) == "" || strings.TrimSpace(record.TurnID) == "" || !validSHA256(record.EnvelopeDigest) ||
		!validSHA256(record.ContextDigest) || record.ContextEpoch == 0 || strings.TrimSpace(record.DatasetSnapshotID) == "" ||
		!validFinalAnswerVariant(record.Variant) || !validFinalTerminalReason(record.TerminalReason) ||
		!validSHA256(record.RenderedTextSHA256) || !validSHA256(record.RegistryStateDigest) || !validFinalAnswerRendererVersion(record.RendererVersion) ||
		record.VerifierVersion != ClaimVerifierPolicyVersion || !validSHA256(record.PrivateRecordDigest) || !validSHA256(record.RecordDigest) {
		return errors.New("accepted final record is incomplete")
	}
	requiresProof := finalAnswerVariantRequiresPublicationSnapshotProof(record.Variant)
	switch {
	case previous:
		if record.FinalGateVersion != LegacyFinalEvidenceGateVersion || requiresProof || record.PublicationSnapshotProofDigest != "" ||
			record.FactFinalWitnessAdmission != nil || record.PublicView != nil || record.PublicViewDigest != "" {
			return errors.New("legacy fact-bearing accepted final is not publishable")
		}
	case boundaryV3:
		if record.FinalGateVersion != HistoricalBoundaryFinalGateVersion || record.FactFinalWitnessAdmission != nil ||
			record.PublicView != nil || record.PublicViewDigest != "" ||
			(requiresProof && !validSHA256(record.PublicationSnapshotProofDigest)) || (!requiresProof && record.PublicationSnapshotProofDigest != "") {
			return errors.New("accepted final publication snapshot proof is invalid")
		}
	case witnessedFactV4:
		if record.FinalGateVersion != WitnessedFinalEvidenceGateVersion ||
			record.RendererVersion != HistoricalFinalAnswerRendererVersion || !requiresProof ||
			!validSHA256(record.PublicationSnapshotProofDigest) || record.PublicView != nil || record.PublicViewDigest != "" ||
			record.FactFinalWitnessAdmission == nil ||
			record.FactFinalWitnessAdmission.SchemaVersion != FactFinalWitnessAdmissionSchemaVersionV1 ||
			ValidateFactFinalWitnessAdmissionForRecordV1(record.FactFinalWitnessAdmission, record) != nil {
			return errors.New("accepted final witnessed evidence authority is invalid")
		}
	case liveV5:
		if record.FinalGateVersion != FinalEvidenceGateVersion ||
			(requiresProof && (!validSHA256(record.PublicationSnapshotProofDigest) ||
				ValidateFactFinalWitnessAdmissionForRecordV1(record.FactFinalWitnessAdmission, record) != nil)) ||
			(!requiresProof && (record.PublicationSnapshotProofDigest != "" || record.FactFinalWitnessAdmission != nil)) {
			return errors.New("accepted final V5 public or evidence authority is invalid")
		}
		if err := ValidateAcceptedFinalPublicViewCoreV2(record); err != nil {
			return errors.Join(errors.New("accepted final V5 public or evidence authority is invalid"), err)
		}
	}
	acceptedAt, err := time.Parse(time.RFC3339Nano, record.AcceptedAt)
	if err != nil {
		return errors.New("accepted final record acceptedAt is invalid")
	}
	if witnessedFactV4 || liveV5 {
		if acceptedAt.Location() != time.UTC || acceptedAt.UTC().Format(time.RFC3339Nano) != record.AcceptedAt {
			return errors.New("accepted final timestamp authority is invalid")
		}
	}
	if witnessedFactV4 || (liveV5 && record.FactFinalWitnessAdmission != nil) {
		admittedAt, admissionErr := time.Parse(time.RFC3339Nano, record.FactFinalWitnessAdmission.AdmittedAt)
		if admissionErr != nil || acceptedAt.Before(admittedAt) {
			return errors.New("accepted final witnessed timestamp authority is invalid")
		}
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != record.AuthorityKeyID {
		return errors.New("accepted final record public key is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(publicKey), AcceptedFinalSigningBytes(record), signature) {
		return errors.New("accepted final record signature is invalid")
	}
	if acceptedFinalRecordDigest(record) != record.RecordDigest {
		return errors.New("accepted final record integrity is invalid")
	}
	return nil
}

func ValidatePrivateAcceptedFinalRecord(record PrivateAcceptedFinalRecord) error {
	previous := record.SchemaVersion == PreviousAcceptedFinalRecordVersion
	boundaryV3 := record.SchemaVersion == BoundaryAcceptedFinalRecordVersion
	witnessedFactV4 := record.SchemaVersion == WitnessedFactPrivateFinalRecordVersion
	liveV5 := record.SchemaVersion == PrivateAcceptedFinalRecordVersion
	if (!previous && !boundaryV3 && !witnessedFactV4 && !liveV5) || !validSHA256(record.PrivateRecordDigest) ||
		!validSHA256(record.StoreDigest) || domainsecurity.ValidateTurnSecurityContext(record.SecurityContext) != nil ||
		ValidateFinalAnswerEnvelope(record.Envelope) != nil || ValidateEvidenceRegistryHead(record.RegistryHead) != nil ||
		ValidateTerminalPublicationIntent(record.PublicationIntent, record.Envelope.TerminalReason) != nil ||
		ValidateAcceptedFinalRecord(record.AcceptedFinal) != nil {
		return errors.New("private accepted final record is incomplete")
	}
	if record.AcceptedFinal.SchemaVersion != record.SchemaVersion ||
		ValidatePublicationSnapshotProofValue(record.PublicationSnapshotProof, record.SecurityContext, record.Envelope, &record.RegistryHead) != nil {
		return errors.New("private accepted final publication snapshot proof is invalid")
	}
	requiresProof := FinalAnswerRequiresPublicationSnapshotProof(record.Envelope)
	if (witnessedFactV4 && !requiresProof) || (previous && requiresProof) ||
		(boundaryV3 && record.AcceptedFinal.FactFinalWitnessAdmission != nil) {
		return errors.New("private accepted final version does not match its fact authority")
	}
	privateDigest, err := privateAcceptedFinalDigestForVersionAndRenderer(
		record.SchemaVersion, record.AcceptedFinal.RendererVersion, record.SecurityContext, record.Envelope, record.RenderedText, record.PublicationIntent,
		record.PublicationSnapshotProof, record.AcceptedFinal.FactFinalWitnessAdmission,
	)
	acceptedAt, acceptedAtErr := time.Parse(time.RFC3339Nano, record.AcceptedFinal.AcceptedAt)
	proofCheckedAt := acceptedAt
	if record.PublicationSnapshotProof != nil {
		proofCheckedAt, _ = time.Parse(time.RFC3339Nano, record.PublicationSnapshotProof.CheckedAt)
	}
	admissionAt := acceptedAt
	if record.AcceptedFinal.FactFinalWitnessAdmission != nil {
		admissionAt, _ = time.Parse(time.RFC3339Nano, record.AcceptedFinal.FactFinalWitnessAdmission.AdmittedAt)
	}
	if err != nil || privateDigest != record.PrivateRecordDigest || privateDigest != record.AcceptedFinal.PrivateRecordDigest ||
		acceptedAtErr != nil || acceptedAt.Before(proofCheckedAt) ||
		acceptedAt.Before(admissionAt) ||
		record.RegistryHead.ContextDigest != record.SecurityContext.ContextDigest ||
		record.RegistryHead.DatasetSnapshotID != record.SecurityContext.DatasetSnapshotID ||
		record.RegistryHead.Sequence != record.AcceptedFinal.RegistrySequence ||
		record.RegistryHead.StateDigest != record.AcceptedFinal.RegistryStateDigest ||
		record.AcceptedFinal.ThreadID != record.SecurityContext.ThreadID || record.AcceptedFinal.TurnID != record.SecurityContext.TurnID ||
		record.AcceptedFinal.ContextDigest != record.SecurityContext.ContextDigest || record.AcceptedFinal.ContextEpoch != record.SecurityContext.ContextEpoch ||
		record.AcceptedFinal.DatasetSnapshotID != record.SecurityContext.DatasetSnapshotID ||
		record.AcceptedFinal.EnvelopeDigest != record.Envelope.EnvelopeDigest || record.AcceptedFinal.Variant != record.Envelope.Variant ||
		record.AcceptedFinal.TerminalReason != record.Envelope.TerminalReason ||
		publicationSnapshotProofDigestValue(record.PublicationSnapshotProof) != record.AcceptedFinal.PublicationSnapshotProofDigest ||
		record.AcceptedFinal.RenderedTextSHA256 != domainsecurity.SHA256Hex([]byte(record.RenderedText)) ||
		privateAcceptedFinalStoreDigest(record) != record.StoreDigest {
		return errors.New("private accepted final record integrity is invalid")
	}
	if liveV5 {
		expectedView := buildAcceptedFinalPublicViewCoreV2(record.Envelope, record.AcceptedFinal)
		if record.AcceptedFinal.PublicView == nil || !reflect.DeepEqual(*record.AcceptedFinal.PublicView, expectedView) ||
			record.AcceptedFinal.PublicViewDigest != acceptedFinalPublicViewCoreV2Digest(expectedView) {
			return errors.New("private accepted final public view is detached from its envelope")
		}
	}
	return nil
}

// ValidatePrivateAcceptedFinalPublicationAuthority separates strict audit
// parsing from current publication authority. Historical records may remain
// structurally readable, but a V1 security context can never seed public
// projection, startup repair, event replay, report publication, or a new
// trusted-history index.
func ValidatePrivateAcceptedFinalPublicationAuthority(record PrivateAcceptedFinalRecord) error {
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return err
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(record.SecurityContext) != nil {
		return errors.New("private accepted final lacks current V2 case publication authority")
	}
	if FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) &&
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil {
		return errors.New("fact-bearing private accepted final lacks current V2 case fact authority")
	}
	if FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) {
		return errors.New("fact-bearing private accepted final requires fresh evidence witness replay authority")
	}
	if record.SchemaVersion != PrivateAcceptedFinalRecordVersion || record.AcceptedFinal.SchemaVersion != AcceptedFinalRecordVersion {
		return errors.New("historical accepted final is audit-only")
	}
	return nil
}

// ValidatePrivateAcceptedFinalAuditAuthority keeps structurally valid V2
// records available for forensic inspection without granting publication,
// projection, report, or recovery authority to historical fact finals.
func ValidatePrivateAcceptedFinalAuditAuthority(record PrivateAcceptedFinalRecord) error {
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return err
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(record.SecurityContext) != nil {
		return errors.New("private accepted final lacks auditable V2 case authority")
	}
	return nil
}

type FactFinalWitnessReplayVerifierV1 func(PrivateAcceptedFinalRecord) error

// ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1 is the only
// live path for a fact-bearing V5 final. The callback must re-observe the
// witness and prove the signed admission remains on the fresh authority chain;
// validating the embedded historical observation alone is insufficient.
func ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(record PrivateAcceptedFinalRecord, verify FactFinalWitnessReplayVerifierV1) error {
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return err
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(record.SecurityContext) != nil {
		return errors.New("private accepted final lacks current V2 case publication authority")
	}
	if record.SchemaVersion != PrivateAcceptedFinalRecordVersion ||
		record.AcceptedFinal.SchemaVersion != AcceptedFinalRecordVersion {
		return errors.New("historical accepted final is audit-only")
	}
	if !FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) {
		return nil
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil || verify == nil {
		return errors.New("fact-bearing private accepted final lacks witnessed publication authority")
	}
	if err := verify(record); err != nil {
		return errors.Join(errors.New("fact-bearing private accepted final witness replay failed"), err)
	}
	return nil
}

// ValidateAcceptedFinalForCurrentWriteV1 rejects historical V3 fact records
// at the mutable terminal boundary. It deliberately does not replace the
// fresh-witness replay required when a V4 fact record is loaded again.
func ValidateAcceptedFinalForCurrentWriteV1(record AcceptedFinalRecord) error {
	if err := ValidateAcceptedFinalRecord(record); err != nil {
		return err
	}
	if record.SchemaVersion != AcceptedFinalRecordVersion || record.PublicView == nil || !validSHA256(record.PublicViewDigest) {
		return errors.New("accepted final current-write version is invalid")
	}
	if finalAnswerVariantRequiresPublicationSnapshotProof(record.Variant) {
		if record.FactFinalWitnessAdmission == nil {
			return errors.New("fact-bearing accepted final lacks witnessed current-write authority")
		}
		return nil
	}
	if record.FactFinalWitnessAdmission != nil {
		return errors.New("boundary accepted final version is invalid")
	}
	return nil
}

func AcceptedFinalSigningBytes(record AcceptedFinalRecord) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	out := make([]byte, 0, len(acceptedFinalSignatureDomain)+len(digest))
	out = append(out, acceptedFinalSignatureDomain...)
	out = append(out, digest[:]...)
	return out
}

func AcceptedFinalAuthorityMaterial(record AcceptedFinalRecord) (string, []byte, []byte, error) {
	if err := ValidateAcceptedFinalRecord(record); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	return record.AuthorityKeyID, publicKey, signature, nil
}

func AcceptedFinalRecordMap(record AcceptedFinalRecord) map[string]any {
	body, _ := json.Marshal(record)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}

func PrivateAcceptedFinalRecordBytes(record PrivateAcceptedFinalRecord) ([]byte, error) {
	if err := ValidatePrivateAcceptedFinalRecord(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func acceptedFinalRecordDigest(record AcceptedFinalRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func privateAcceptedFinalStoreDigest(record PrivateAcceptedFinalRecord) string {
	record.StoreDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func authorityKeyID(publicKey []byte) string {
	return domainsecurity.SHA256Hex(publicKey)
}

func decodeStrictJSON(raw []byte, destination any, name string) error {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxDepth: 256, MaxTokens: 1_000_000, MaxStringBytes: 16 * 1024 * 1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New(name + " contains trailing JSON")
	}
	return nil
}
