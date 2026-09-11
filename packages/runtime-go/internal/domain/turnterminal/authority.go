package turnterminal

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	TurnTerminalIntentV1SchemaVersion      = "turn-terminal-intent.v1"
	TurnTerminalDispositionV1SchemaVersion = "turn-terminal-disposition.v1"
	TurnTerminalIntentV1Purpose            = "analytix.turn-terminal-intent/v1"
	TurnTerminalDispositionV1Purpose       = "analytix.turn-terminal-disposition/v1"
	TurnTerminalAuthorityAlgorithm         = "Ed25519"
)

var (
	turnTerminalIntentIDDomain             = []byte("analytix/turn-terminal-intent-id/v1\x00")
	turnTerminalIntentSignatureDomain      = []byte("analytix/turn-terminal-intent-signature/v1\x00")
	turnTerminalMaterialDomain             = []byte("analytix/turn-terminal-publication-material/v1\x00")
	turnTerminalDispositionIDDomain        = []byte("analytix/turn-terminal-disposition-id/v1\x00")
	turnTerminalDispositionSignatureDomain = []byte("analytix/turn-terminal-disposition-signature/v1\x00")
)

// TurnTerminalIntentV1 is the private, content-addressed authority prepared
// after the Final Evidence Gate and before provider closure or public turn
// mutation. It deliberately contains no rendered text, claim payload, raw
// evidence, provider body, reasoning, or complete PII.
type TurnTerminalIntentV1 struct {
	SchemaVersion             string                                            `json:"schemaVersion"`
	Purpose                   string                                            `json:"purpose"`
	IntentID                  string                                            `json:"intentId"`
	SecurityContext           domainsecurity.TurnSecurityContext                `json:"securityContext"`
	TerminalReasonCode        domaincachetelemetry.ProviderTurnTerminalReasonV1 `json:"terminalReasonCode"`
	AcceptedFinalDigest       string                                            `json:"acceptedFinalDigest"`
	PrivateFinalStoreDigest   string                                            `json:"privateFinalStoreDigest"`
	EventManifestDigest       string                                            `json:"eventManifestDigest"`
	PublicationMaterialDigest string                                            `json:"publicationMaterialDigest"`
	PreparedAt                string                                            `json:"preparedAt"`
	AuthorityAlgorithm        string                                            `json:"authorityAlgorithm"`
	AuthorityKeyID            string                                            `json:"authorityKeyId"`
	AuthorityPublicKey        string                                            `json:"authorityPublicKey"`
	AuthoritySignature        string                                            `json:"authoritySignature"`
}

type TurnTerminalIntentInputV1 struct {
	PrivateFinal        domainevidence.PrivateAcceptedFinalRecord
	EventManifestDigest string
	AuthorityKeyID      string
	AuthorityPublicKey  []byte
}

// TurnTerminalDispositionV1 seals the exact provider closure and the existing
// signed accepted-final CAS disposition. It does not duplicate the latter's
// state/basis vocabulary, so there is one source of truth for the public
// winner decision.
type TurnTerminalDispositionV1 struct {
	SchemaVersion                  string                                            `json:"schemaVersion"`
	Purpose                        string                                            `json:"purpose"`
	DispositionID                  string                                            `json:"dispositionId"`
	IntentID                       string                                            `json:"intentId"`
	ContextDigest                  string                                            `json:"contextDigest"`
	TerminalReasonCode             domaincachetelemetry.ProviderTurnTerminalReasonV1 `json:"terminalReasonCode"`
	PublicationMaterialDigest      string                                            `json:"publicationMaterialDigest"`
	AcceptedFinalDigest            string                                            `json:"acceptedFinalDigest"`
	EventManifestDigest            string                                            `json:"eventManifestDigest"`
	ProviderTurnBindingHMAC        string                                            `json:"providerTurnBindingHmac"`
	ProviderClosureID              string                                            `json:"providerClosureId"`
	ProviderClosureDigest          string                                            `json:"providerClosureDigest"`
	ProviderAggregateDigest        string                                            `json:"providerAggregateDigest"`
	AcceptedFinalDispositionDigest string                                            `json:"acceptedFinalDispositionDigest"`
	ProviderClosedAt               string                                            `json:"providerClosedAt"`
	DisposedAt                     string                                            `json:"disposedAt"`
	AuthorityAlgorithm             string                                            `json:"authorityAlgorithm"`
	AuthorityKeyID                 string                                            `json:"authorityKeyId"`
	AuthorityPublicKey             string                                            `json:"authorityPublicKey"`
	AuthoritySignature             string                                            `json:"authoritySignature"`
}

type TurnTerminalDispositionInputV1 struct {
	Intent                   TurnTerminalIntentV1
	ProviderClosure          domaincachetelemetry.ProviderTurnClosureV1
	AcceptedFinalDisposition domainevidence.AcceptedFinalDispositionRecord
	AuthorityKeyID           string
	AuthorityPublicKey       []byte
}

type SignFunc func([]byte) ([]byte, error)

func NewTurnTerminalIntentV1(input TurnTerminalIntentInputV1, sign SignFunc) (TurnTerminalIntentV1, error) {
	return newTurnTerminalIntentV1(input, domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority, sign)
}

func NewTurnTerminalIntentWithFactWitnessV1(
	input TurnTerminalIntentInputV1,
	verify domainevidence.FactFinalWitnessReplayVerifierV1,
	sign SignFunc,
) (TurnTerminalIntentV1, error) {
	return newTurnTerminalIntentV1(input, func(record domainevidence.PrivateAcceptedFinalRecord) error {
		return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(record, verify)
	}, sign)
}

func newTurnTerminalIntentV1(
	input TurnTerminalIntentInputV1,
	validatePrivate func(domainevidence.PrivateAcceptedFinalRecord) error,
	sign SignFunc,
) (TurnTerminalIntentV1, error) {
	privateFinal := input.PrivateFinal
	if validatePrivate == nil {
		return TurnTerminalIntentV1{}, errors.New("turn terminal intent private-final validator is unavailable")
	}
	if err := validatePrivate(privateFinal); err != nil {
		return TurnTerminalIntentV1{}, fmt.Errorf("turn terminal intent private final is invalid: %w", err)
	}
	reason := domaincachetelemetry.ProviderTurnTerminalReasonV1(privateFinal.Envelope.TerminalReason)
	if err := domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(reason); err != nil {
		return TurnTerminalIntentV1{}, err
	}
	preparedAt, err := canonicalTime(privateFinal.AcceptedFinal.AcceptedAt)
	if err != nil {
		return TurnTerminalIntentV1{}, errors.New("turn terminal intent accepted time is invalid")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	keyID := strings.TrimSpace(input.AuthorityKeyID)
	if !validAuthorityInput(keyID, publicKey, sign) || !acceptedFinalUsesAuthority(privateFinal.AcceptedFinal, keyID, publicKey) {
		return TurnTerminalIntentV1{}, errors.New("turn terminal intent authority does not match accepted final")
	}
	intent := TurnTerminalIntentV1{
		SchemaVersion: TurnTerminalIntentV1SchemaVersion, Purpose: TurnTerminalIntentV1Purpose,
		SecurityContext: privateFinal.SecurityContext, TerminalReasonCode: reason,
		AcceptedFinalDigest: privateFinal.AcceptedFinal.RecordDigest, PrivateFinalStoreDigest: privateFinal.StoreDigest,
		EventManifestDigest: strings.TrimSpace(input.EventManifestDigest), PreparedAt: preparedAt,
		AuthorityAlgorithm: TurnTerminalAuthorityAlgorithm, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	intent.PublicationMaterialDigest = publicationMaterialDigestV1(intent)
	intent.IntentID = turnTerminalIntentIDV1(intent)
	signature, err := sign(TurnTerminalIntentV1SigningBytes(intent))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return TurnTerminalIntentV1{}, errors.New("turn terminal intent signing failed")
	}
	intent.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := validateTurnTerminalIntentForPrivateFinalV1(intent, privateFinal, validatePrivate); err != nil {
		return TurnTerminalIntentV1{}, err
	}
	return intent, nil
}

func NewTurnTerminalDispositionV1(input TurnTerminalDispositionInputV1, sign SignFunc) (TurnTerminalDispositionV1, error) {
	if err := ValidateTurnTerminalIntentV1(input.Intent); err != nil {
		return TurnTerminalDispositionV1{}, fmt.Errorf("turn terminal disposition intent is invalid: %w", err)
	}
	if err := domaincachetelemetry.ValidateProviderTurnClosureV1(input.ProviderClosure); err != nil {
		return TurnTerminalDispositionV1{}, fmt.Errorf("turn terminal disposition provider closure is invalid: %w", err)
	}
	if err := domainevidence.ValidateAcceptedFinalDispositionRecord(input.AcceptedFinalDisposition); err != nil {
		return TurnTerminalDispositionV1{}, fmt.Errorf("turn terminal accepted-final disposition is invalid: %w", err)
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	keyID := strings.TrimSpace(input.AuthorityKeyID)
	if !validAuthorityInput(keyID, publicKey, sign) || !intentUsesAuthority(input.Intent, keyID, publicKey) ||
		!providerClosureUsesAuthority(input.ProviderClosure, keyID, publicKey) ||
		!acceptedFinalDispositionUsesAuthority(input.AcceptedFinalDisposition, keyID, publicKey) {
		return TurnTerminalDispositionV1{}, errors.New("turn terminal disposition authority is inconsistent")
	}
	closureBody, err := domaincachetelemetry.ProviderTurnClosureV1Bytes(input.ProviderClosure)
	if err != nil {
		return TurnTerminalDispositionV1{}, err
	}
	disposition := TurnTerminalDispositionV1{
		SchemaVersion: TurnTerminalDispositionV1SchemaVersion, Purpose: TurnTerminalDispositionV1Purpose,
		IntentID: input.Intent.IntentID, ContextDigest: input.Intent.SecurityContext.ContextDigest,
		TerminalReasonCode: input.Intent.TerminalReasonCode, PublicationMaterialDigest: input.Intent.PublicationMaterialDigest,
		AcceptedFinalDigest: input.Intent.AcceptedFinalDigest, EventManifestDigest: input.Intent.EventManifestDigest,
		ProviderTurnBindingHMAC: input.ProviderClosure.TurnBindingHMAC, ProviderClosureID: input.ProviderClosure.ClosureID,
		ProviderClosureDigest: domainsecurity.SHA256Hex(closureBody), ProviderAggregateDigest: input.ProviderClosure.AggregateDigest,
		AcceptedFinalDispositionDigest: input.AcceptedFinalDisposition.RecordDigest,
		ProviderClosedAt:               input.ProviderClosure.ClosedAt, DisposedAt: input.AcceptedFinalDisposition.DecidedAt,
		AuthorityAlgorithm: TurnTerminalAuthorityAlgorithm, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = turnTerminalDispositionIDV1(disposition)
	signature, err := sign(TurnTerminalDispositionV1SigningBytes(disposition))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return TurnTerminalDispositionV1{}, errors.New("turn terminal disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateTurnTerminalDispositionForAuthoritiesV1(disposition, input.Intent, input.ProviderClosure, input.AcceptedFinalDisposition); err != nil {
		return TurnTerminalDispositionV1{}, err
	}
	return disposition, nil
}

func ValidateTurnTerminalIntentV1(intent TurnTerminalIntentV1) error {
	if intent.SchemaVersion != TurnTerminalIntentV1SchemaVersion || intent.Purpose != TurnTerminalIntentV1Purpose ||
		!domainsecurity.IsSHA256Hex(intent.IntentID) || domainsecurity.ValidateTurnSecurityContextForCasePublication(intent.SecurityContext) != nil ||
		domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(intent.TerminalReasonCode) != nil ||
		!domainsecurity.IsSHA256Hex(intent.AcceptedFinalDigest) || !domainsecurity.IsSHA256Hex(intent.PrivateFinalStoreDigest) ||
		!domainsecurity.IsSHA256Hex(intent.EventManifestDigest) || !domainsecurity.IsSHA256Hex(intent.PublicationMaterialDigest) ||
		intent.AuthorityAlgorithm != TurnTerminalAuthorityAlgorithm || !domainsecurity.IsSHA256Hex(intent.AuthorityKeyID) {
		return errors.New("turn terminal intent identity is invalid")
	}
	if _, err := canonicalTime(intent.PreparedAt); err != nil {
		return errors.New("turn terminal intent prepared time is invalid")
	}
	if intent.PublicationMaterialDigest != publicationMaterialDigestV1(intent) || intent.IntentID != turnTerminalIntentIDV1(intent) {
		return errors.New("turn terminal intent integrity is invalid")
	}
	return validateSignature(intent.AuthorityKeyID, intent.AuthorityPublicKey, intent.AuthoritySignature,
		TurnTerminalIntentV1SigningBytes(intent), "turn terminal intent")
}

func ValidateTurnTerminalIntentForPrivateFinalV1(intent TurnTerminalIntentV1, privateFinal domainevidence.PrivateAcceptedFinalRecord) error {
	return validateTurnTerminalIntentForPrivateFinalV1(
		intent, privateFinal, domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority,
	)
}

func ValidateTurnTerminalIntentForPrivateFinalWithFactWitnessV1(
	intent TurnTerminalIntentV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	verify domainevidence.FactFinalWitnessReplayVerifierV1,
) error {
	return validateTurnTerminalIntentForPrivateFinalV1(
		intent,
		privateFinal,
		func(record domainevidence.PrivateAcceptedFinalRecord) error {
			return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(record, verify)
		},
	)
}

// ValidateTurnTerminalIntentForPrivateFinalAuditV1 verifies the immutable
// binding and installation signature material of an audit-only private final
// without granting recovery, publication, repair, or projection authority.
func ValidateTurnTerminalIntentForPrivateFinalAuditV1(intent TurnTerminalIntentV1, privateFinal domainevidence.PrivateAcceptedFinalRecord) error {
	return validateTurnTerminalIntentForPrivateFinalV1(
		intent, privateFinal, domainevidence.ValidatePrivateAcceptedFinalAuditAuthority,
	)
}

func validateTurnTerminalIntentForPrivateFinalV1(
	intent TurnTerminalIntentV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	validatePrivate func(domainevidence.PrivateAcceptedFinalRecord) error,
) error {
	if err := ValidateTurnTerminalIntentV1(intent); err != nil {
		return err
	}
	if validatePrivate == nil {
		return errors.New("turn terminal private-final validator is unavailable")
	}
	if err := validatePrivate(privateFinal); err != nil {
		return err
	}
	acceptedAt, err := canonicalTime(privateFinal.AcceptedFinal.AcceptedAt)
	if err != nil || intent.SecurityContext != privateFinal.SecurityContext ||
		string(intent.TerminalReasonCode) != privateFinal.Envelope.TerminalReason ||
		intent.AcceptedFinalDigest != privateFinal.AcceptedFinal.RecordDigest ||
		intent.PrivateFinalStoreDigest != privateFinal.StoreDigest || intent.PreparedAt != acceptedAt {
		return errors.New("turn terminal intent does not bind the exact private accepted final")
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(intent.AuthorityPublicKey)
	if !acceptedFinalUsesAuthority(privateFinal.AcceptedFinal, intent.AuthorityKeyID, publicKey) {
		return errors.New("turn terminal intent and accepted final authority differ")
	}
	return nil
}

func ValidateTurnTerminalDispositionV1(disposition TurnTerminalDispositionV1) error {
	if disposition.SchemaVersion != TurnTerminalDispositionV1SchemaVersion || disposition.Purpose != TurnTerminalDispositionV1Purpose ||
		!domainsecurity.IsSHA256Hex(disposition.DispositionID) || !domainsecurity.IsSHA256Hex(disposition.IntentID) ||
		!domainsecurity.IsSHA256Hex(disposition.ContextDigest) ||
		domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(disposition.TerminalReasonCode) != nil ||
		!domainsecurity.IsSHA256Hex(disposition.PublicationMaterialDigest) || !domainsecurity.IsSHA256Hex(disposition.AcceptedFinalDigest) ||
		!domainsecurity.IsSHA256Hex(disposition.EventManifestDigest) || !domainsecurity.IsSHA256Hex(disposition.ProviderTurnBindingHMAC) ||
		!domainsecurity.IsSHA256Hex(disposition.ProviderClosureID) || !domainsecurity.IsSHA256Hex(disposition.ProviderClosureDigest) ||
		!domainsecurity.IsSHA256Hex(disposition.ProviderAggregateDigest) ||
		!domainsecurity.IsSHA256Hex(disposition.AcceptedFinalDispositionDigest) ||
		disposition.AuthorityAlgorithm != TurnTerminalAuthorityAlgorithm || !domainsecurity.IsSHA256Hex(disposition.AuthorityKeyID) {
		return errors.New("turn terminal disposition identity is invalid")
	}
	closedAt, closedErr := parseCanonicalTime(disposition.ProviderClosedAt)
	disposedAt, disposedErr := parseCanonicalTime(disposition.DisposedAt)
	if closedErr != nil || disposedErr != nil || disposedAt.Before(closedAt) {
		return errors.New("turn terminal disposition times are invalid")
	}
	if disposition.DispositionID != turnTerminalDispositionIDV1(disposition) {
		return errors.New("turn terminal disposition integrity is invalid")
	}
	return validateSignature(disposition.AuthorityKeyID, disposition.AuthorityPublicKey, disposition.AuthoritySignature,
		TurnTerminalDispositionV1SigningBytes(disposition), "turn terminal disposition")
}

func ValidateTurnTerminalDispositionForIntentV1(disposition TurnTerminalDispositionV1, intent TurnTerminalIntentV1) error {
	if err := ValidateTurnTerminalDispositionV1(disposition); err != nil {
		return err
	}
	if err := ValidateTurnTerminalIntentV1(intent); err != nil {
		return err
	}
	if disposition.IntentID != intent.IntentID || disposition.ContextDigest != intent.SecurityContext.ContextDigest ||
		disposition.TerminalReasonCode != intent.TerminalReasonCode || disposition.PublicationMaterialDigest != intent.PublicationMaterialDigest ||
		disposition.AcceptedFinalDigest != intent.AcceptedFinalDigest || disposition.EventManifestDigest != intent.EventManifestDigest ||
		disposition.AuthorityKeyID != intent.AuthorityKeyID || disposition.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("turn terminal disposition does not bind the exact intent")
	}
	return nil
}

func ValidateTurnTerminalDispositionForAuthoritiesV1(disposition TurnTerminalDispositionV1, intent TurnTerminalIntentV1,
	closure domaincachetelemetry.ProviderTurnClosureV1, acceptedDisposition domainevidence.AcceptedFinalDispositionRecord) error {
	if err := ValidateTurnTerminalDispositionForIntentV1(disposition, intent); err != nil {
		return err
	}
	if err := domaincachetelemetry.ValidateProviderTurnClosureV1(closure); err != nil {
		return err
	}
	if err := domainevidence.ValidateAcceptedFinalDispositionRecord(acceptedDisposition); err != nil {
		return err
	}
	closureBody, _ := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
	if closure.TerminalReasonCode != intent.TerminalReasonCode || closure.TurnBindingHMAC != disposition.ProviderTurnBindingHMAC ||
		closure.ClosureID != disposition.ProviderClosureID || domainsecurity.SHA256Hex(closureBody) != disposition.ProviderClosureDigest ||
		closure.AggregateDigest != disposition.ProviderAggregateDigest || closure.ClosedAt != disposition.ProviderClosedAt ||
		closure.AuthorityKeyID != intent.AuthorityKeyID || closure.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("turn terminal disposition does not bind the exact provider closure")
	}
	if acceptedDisposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		acceptedDisposition.State != domainevidence.AcceptedFinalCommitted ||
		acceptedDisposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		acceptedDisposition.AcceptedFinalDigest != intent.AcceptedFinalDigest ||
		acceptedDisposition.WinnerDigest != intent.AcceptedFinalDigest ||
		acceptedDisposition.EventManifestDigest != intent.EventManifestDigest ||
		acceptedDisposition.RecordDigest != disposition.AcceptedFinalDispositionDigest ||
		acceptedDisposition.AuthorityKeyID != intent.AuthorityKeyID ||
		acceptedDisposition.AuthorityPublicKey != intent.AuthorityPublicKey ||
		acceptedDisposition.DecidedAt != disposition.DisposedAt {
		return errors.New("turn terminal disposition does not bind the committed accepted-final disposition")
	}
	preparedAt, preparedErr := parseCanonicalTime(intent.PreparedAt)
	closedAt, closedErr := parseCanonicalTime(closure.ClosedAt)
	decidedAt, decidedErr := parseCanonicalTime(acceptedDisposition.DecidedAt)
	if preparedErr != nil || closedErr != nil || decidedErr != nil || closedAt.Before(preparedAt) || decidedAt.Before(closedAt) {
		return errors.New("turn terminal authority order is invalid")
	}
	return nil
}

func ParseTurnTerminalIntentV1(body []byte) (TurnTerminalIntentV1, error) {
	var intent TurnTerminalIntentV1
	if err := decodeStrict(body, &intent, "turn terminal intent"); err != nil {
		return TurnTerminalIntentV1{}, err
	}
	canonical, err := TurnTerminalIntentV1Bytes(intent)
	if err != nil || !bytes.Equal(body, canonical) {
		return TurnTerminalIntentV1{}, errors.New("turn terminal intent is not canonical JSON")
	}
	return intent, nil
}

func ParseTurnTerminalDispositionV1(body []byte) (TurnTerminalDispositionV1, error) {
	var disposition TurnTerminalDispositionV1
	if err := decodeStrict(body, &disposition, "turn terminal disposition"); err != nil {
		return TurnTerminalDispositionV1{}, err
	}
	canonical, err := TurnTerminalDispositionV1Bytes(disposition)
	if err != nil || !bytes.Equal(body, canonical) {
		return TurnTerminalDispositionV1{}, errors.New("turn terminal disposition is not canonical JSON")
	}
	return disposition, nil
}

func TurnTerminalIntentV1Bytes(intent TurnTerminalIntentV1) ([]byte, error) {
	if err := ValidateTurnTerminalIntentV1(intent); err != nil {
		return nil, err
	}
	return json.Marshal(intent)
}

func TurnTerminalDispositionV1Bytes(disposition TurnTerminalDispositionV1) ([]byte, error) {
	if err := ValidateTurnTerminalDispositionV1(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func TurnTerminalIntentV1SigningBytes(intent TurnTerminalIntentV1) []byte {
	intent.AuthoritySignature = ""
	body, _ := json.Marshal(intent)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), turnTerminalIntentSignatureDomain...), digest[:]...)
}

func TurnTerminalDispositionV1SigningBytes(disposition TurnTerminalDispositionV1) []byte {
	disposition.AuthoritySignature = ""
	body, _ := json.Marshal(disposition)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), turnTerminalDispositionSignatureDomain...), digest[:]...)
}

func TurnTerminalIntentV1AuthorityMaterial(intent TurnTerminalIntentV1) (string, []byte, []byte, error) {
	if err := ValidateTurnTerminalIntentV1(intent); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(intent.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(intent.AuthoritySignature)
	return intent.AuthorityKeyID, publicKey, signature, nil
}

func TurnTerminalDispositionV1AuthorityMaterial(disposition TurnTerminalDispositionV1) (string, []byte, []byte, error) {
	if err := ValidateTurnTerminalDispositionV1(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func publicationMaterialDigestV1(intent TurnTerminalIntentV1) string {
	body := struct {
		ContextDigest           string                                            `json:"contextDigest"`
		TerminalReasonCode      domaincachetelemetry.ProviderTurnTerminalReasonV1 `json:"terminalReasonCode"`
		AcceptedFinalDigest     string                                            `json:"acceptedFinalDigest"`
		PrivateFinalStoreDigest string                                            `json:"privateFinalStoreDigest"`
		EventManifestDigest     string                                            `json:"eventManifestDigest"`
	}{intent.SecurityContext.ContextDigest, intent.TerminalReasonCode, intent.AcceptedFinalDigest,
		intent.PrivateFinalStoreDigest, intent.EventManifestDigest}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), turnTerminalMaterialDomain...), encoded...))
}

func turnTerminalIntentIDV1(intent TurnTerminalIntentV1) string {
	intent.IntentID = ""
	intent.AuthoritySignature = ""
	body, _ := json.Marshal(intent)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), turnTerminalIntentIDDomain...), body...))
}

func turnTerminalDispositionIDV1(disposition TurnTerminalDispositionV1) string {
	disposition.DispositionID = ""
	disposition.AuthoritySignature = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), turnTerminalDispositionIDDomain...), body...))
}

func acceptedFinalUsesAuthority(record domainevidence.AcceptedFinalRecord, keyID string, publicKey []byte) bool {
	return domainevidence.ValidateAcceptedFinalRecord(record) == nil && record.AuthorityKeyID == keyID &&
		record.AuthorityPublicKey == base64.RawURLEncoding.EncodeToString(publicKey)
}

func intentUsesAuthority(intent TurnTerminalIntentV1, keyID string, publicKey []byte) bool {
	return intent.AuthorityKeyID == keyID && intent.AuthorityPublicKey == base64.RawURLEncoding.EncodeToString(publicKey)
}

func providerClosureUsesAuthority(closure domaincachetelemetry.ProviderTurnClosureV1, keyID string, publicKey []byte) bool {
	return closure.AuthorityKeyID == keyID && closure.AuthorityPublicKey == base64.RawURLEncoding.EncodeToString(publicKey)
}

func acceptedFinalDispositionUsesAuthority(disposition domainevidence.AcceptedFinalDispositionRecord, keyID string, publicKey []byte) bool {
	return disposition.AuthorityKeyID == keyID && disposition.AuthorityPublicKey == base64.RawURLEncoding.EncodeToString(publicKey)
}

func validAuthorityInput(keyID string, publicKey []byte, sign SignFunc) bool {
	return len(publicKey) == ed25519.PublicKeySize && keyID == domainsecurity.SHA256Hex(publicKey) && sign != nil
}

func validateSignature(keyID, encodedPublicKey, encodedSignature string, signingBytes []byte, name string) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != encodedPublicKey || base64.RawURLEncoding.EncodeToString(signature) != encodedSignature ||
		keyID != domainsecurity.SHA256Hex(publicKey) || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return fmt.Errorf("%s authority material is invalid", name)
	}
	return nil
}

func canonicalTime(value string) (string, error) {
	_, err := parseCanonicalTime(value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func parseCanonicalTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != value {
		return time.Time{}, errors.New("timestamp is not canonical UTC")
	}
	return parsed, nil
}

func decodeStrict(body []byte, destination any, name string) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 256 * 1024, MaxDepth: 64, MaxTokens: 32 * 1024, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
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
