package cachetelemetry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

const (
	ProviderAttemptIntentV1SchemaVersion     = "provider-attempt-intent.v1"
	ProviderAttemptSettlementV1SchemaVersion = "provider-attempt-settlement.v1"
	ProviderTurnClosureV1SchemaVersion       = "provider-turn-closure.v1"

	ProviderAttemptIntentV1Purpose     = "analytix.provider-attempt-intent/v1"
	ProviderAttemptSettlementV1Purpose = "analytix.provider-attempt-settlement/v1"
	ProviderTurnClosureV1Purpose       = "analytix.provider-turn-closure/v1"
	ProviderLedgerAuthorityAlgorithm   = "Ed25519"

	providerIntentIDDomain            = "analytix/provider-attempt-intent-id/v1\x00"
	providerIntentSignatureDomain     = "analytix/provider-attempt-intent-signature/v1\x00"
	providerSettlementIDDomain        = "analytix/provider-attempt-settlement-id/v1\x00"
	providerSettlementSignatureDomain = "analytix/provider-attempt-settlement-signature/v1\x00"
	providerClosureIDDomain           = "analytix/provider-turn-closure-id/v1\x00"
	providerClosureSignatureDomain    = "analytix/provider-turn-closure-signature/v1\x00"

	maxProviderLedgerOrdinal uint32 = 1_000_000
)

var providerLedgerReasonCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,95}$`)

type ProviderUsageSourceV1 string

const (
	ProviderUsageSourceTurn     ProviderUsageSourceV1 = "turn"
	ProviderUsageSourceSubagent ProviderUsageSourceV1 = "subagent"
)

func NormalizeProviderUsageSourceV1(value string) ProviderUsageSourceV1 {
	switch strings.TrimSpace(value) {
	case "", string(ProviderUsageSourceTurn):
		return ProviderUsageSourceTurn
	case string(ProviderUsageSourceSubagent):
		return ProviderUsageSourceSubagent
	default:
		return ProviderUsageSourceV1(strings.TrimSpace(value))
	}
}

type ProviderChannelV1 string

const (
	ProviderChannelPrimary          ProviderChannelV1 = "primary"
	ProviderChannelAttachmentVision ProviderChannelV1 = "attachment_vision"
	ProviderChannelToolResultVision ProviderChannelV1 = "tool_result_vision"
)

type ProviderDispatchStateV1 string

const (
	ProviderDispatchStateNotSent       ProviderDispatchStateV1 = "not_sent"
	ProviderDispatchStateSent          ProviderDispatchStateV1 = "sent"
	ProviderDispatchStateIndeterminate ProviderDispatchStateV1 = "indeterminate"
)

// ProviderTurnTerminalReasonV1 is the closed host-authored terminal
// disposition sealed into a provider turn closure. It intentionally mirrors
// the Final Evidence Gate terminal taxonomy without importing the app layer.
// Provider or model text must never be converted into this authority.
type ProviderTurnTerminalReasonV1 string

const (
	ProviderTurnTerminalSuccessV1              ProviderTurnTerminalReasonV1 = "success"
	ProviderTurnTerminalSourceUnavailableV1    ProviderTurnTerminalReasonV1 = "source_unavailable"
	ProviderTurnTerminalSemanticFailureV1      ProviderTurnTerminalReasonV1 = "semantic_failure"
	ProviderTurnTerminalProviderFailureV1      ProviderTurnTerminalReasonV1 = "provider_failure"
	ProviderTurnTerminalCancelV1               ProviderTurnTerminalReasonV1 = "cancel"
	ProviderTurnTerminalTimeoutV1              ProviderTurnTerminalReasonV1 = "timeout"
	ProviderTurnTerminalStreamAbortV1          ProviderTurnTerminalReasonV1 = "stream_abort"
	ProviderTurnTerminalRecoveryV1             ProviderTurnTerminalReasonV1 = "recovery"
	ProviderTurnTerminalApprovalV1             ProviderTurnTerminalReasonV1 = "approval"
	ProviderTurnTerminalUserInputV1            ProviderTurnTerminalReasonV1 = "user_input"
	ProviderTurnTerminalResumeV1               ProviderTurnTerminalReasonV1 = "resume"
	ProviderTurnTerminalRestartV1              ProviderTurnTerminalReasonV1 = "restart"
	ProviderTurnTerminalReportFallbackV1       ProviderTurnTerminalReasonV1 = "report_fallback"
	ProviderTurnTerminalStepLimitV1            ProviderTurnTerminalReasonV1 = "step_limit"
	ProviderTurnTerminalBackgroundCompletionV1 ProviderTurnTerminalReasonV1 = "background_completion"
	ProviderTurnTerminalToolFailureV1          ProviderTurnTerminalReasonV1 = "tool_failure"
	ProviderTurnTerminalApprovalDeniedV1       ProviderTurnTerminalReasonV1 = "approval_denied"
	ProviderTurnTerminalInputCancelledV1       ProviderTurnTerminalReasonV1 = "input_cancelled"
)

func AllProviderTurnTerminalReasonsV1() []ProviderTurnTerminalReasonV1 {
	dispositions := domainterminal.AllDispositionsV1()
	reasons := make([]ProviderTurnTerminalReasonV1, 0, len(dispositions))
	for _, disposition := range dispositions {
		reasons = append(reasons, ProviderTurnTerminalReasonV1(disposition.Reason))
	}
	return reasons
}

func ValidateProviderTurnTerminalReasonV1(reason ProviderTurnTerminalReasonV1) error {
	if _, ok := domainterminal.LookupV1(string(reason)); ok {
		return nil
	}
	return errors.New("provider turn terminal reason is invalid")
}

func providerTurnTerminalReasonAllowsBenchmarkV1(reason ProviderTurnTerminalReasonV1) bool {
	return domainterminal.CandidateAllowedV1(string(reason))
}

// ProviderAttemptIntentV1 is the durable authority that must exist before a
// physical provider request may cross the HTTP send boundary. All sensitive
// identities are represented only by installation-keyed HMACs.
type ProviderAttemptIntentV1 struct {
	SchemaVersion      string                `json:"schemaVersion"`
	Purpose            string                `json:"purpose"`
	IntentID           string                `json:"intentId"`
	TurnBindingHMAC    string                `json:"turnBindingHmac"`
	UsageSource        ProviderUsageSourceV1 `json:"usageSource"`
	ChildRunHMAC       string                `json:"childRunHmac"`
	Channel            ProviderChannelV1     `json:"channel"`
	ChannelOrdinal     uint32                `json:"channelOrdinal"`
	LogicalCallOrdinal uint32                `json:"logicalCallOrdinal"`
	Shape              CacheVisibleShapeV1   `json:"shape"`
	WireHeadersHMAC    string                `json:"wireHeadersHmac"`
	IssuedAt           string                `json:"issuedAt"`
	AuthorityAlgorithm string                `json:"authorityAlgorithm"`
	AuthorityKeyID     string                `json:"authorityKeyId"`
	AuthorityPublicKey string                `json:"authorityPublicKey"`
	AuthoritySignature string                `json:"authoritySignature"`
}

type ProviderAttemptIntentInputV1 struct {
	TurnBindingHMAC    string
	UsageSource        ProviderUsageSourceV1
	ChildRunHMAC       string
	Channel            ProviderChannelV1
	ChannelOrdinal     uint32
	LogicalCallOrdinal uint32
	Shape              CacheVisibleShapeV1
	WireHeadersHMAC    string
	IssuedAt           time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

// ProviderAttemptSettlementV1 is the unique terminal settlement for one
// intent. SafeReasonCode is a bounded host enum-like diagnostic, never raw
// provider text.
type ProviderAttemptSettlementV1 struct {
	SchemaVersion      string                    `json:"schemaVersion"`
	Purpose            string                    `json:"purpose"`
	SettlementID       string                    `json:"settlementId"`
	IntentID           string                    `json:"intentId"`
	TurnBindingHMAC    string                    `json:"turnBindingHmac"`
	DispatchState      ProviderDispatchStateV1   `json:"dispatchState"`
	Observation        ProviderCallObservationV1 `json:"observation"`
	SafeReasonCode     string                    `json:"safeReasonCode"`
	AuthorityAlgorithm string                    `json:"authorityAlgorithm"`
	AuthorityKeyID     string                    `json:"authorityKeyId"`
	AuthorityPublicKey string                    `json:"authorityPublicKey"`
	AuthoritySignature string                    `json:"authoritySignature"`
}

type ProviderAttemptSettlementInputV1 struct {
	Intent             ProviderAttemptIntentV1
	DispatchState      ProviderDispatchStateV1
	Observation        ProviderCallObservationV1
	SafeReasonCode     string
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

// ProviderTurnClosureV1 seals the exact intent/settlement inventory accepted
// for one turn binding. Its aggregate digest is computed from namespace-local
// numeric aggregates so incompatible cache namespaces cannot be mixed.
type ProviderTurnClosureV1 struct {
	SchemaVersion       string                       `json:"schemaVersion"`
	Purpose             string                       `json:"purpose"`
	ClosureID           string                       `json:"closureId"`
	TurnBindingHMAC     string                       `json:"turnBindingHmac"`
	IntentCount         uint32                       `json:"intentCount"`
	SettlementCount     uint32                       `json:"settlementCount"`
	IntentSetDigest     string                       `json:"intentSetDigest"`
	SettlementSetDigest string                       `json:"settlementSetDigest"`
	AggregateDigest     string                       `json:"aggregateDigest"`
	TerminalReasonCode  ProviderTurnTerminalReasonV1 `json:"terminalReasonCode"`
	BenchmarkEligible   bool                         `json:"benchmarkEligible"`
	ClosedAt            string                       `json:"closedAt"`
	AuthorityAlgorithm  string                       `json:"authorityAlgorithm"`
	AuthorityKeyID      string                       `json:"authorityKeyId"`
	AuthorityPublicKey  string                       `json:"authorityPublicKey"`
	AuthoritySignature  string                       `json:"authoritySignature"`
}

type ProviderTurnClosureInputV1 struct {
	TurnBindingHMAC    string
	Intents            []ProviderAttemptIntentV1
	Settlements        []ProviderAttemptSettlementV1
	TerminalReasonCode ProviderTurnTerminalReasonV1
	ClosedAt           time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type ProviderLedgerSignFunc func([]byte) ([]byte, error)

func NewProviderAttemptIntentV1(input ProviderAttemptIntentInputV1, sign ProviderLedgerSignFunc) (ProviderAttemptIntentV1, error) {
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		return ProviderAttemptIntentV1{}, errors.New("provider attempt intent issued time is required")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	intent := ProviderAttemptIntentV1{
		SchemaVersion: ProviderAttemptIntentV1SchemaVersion, Purpose: ProviderAttemptIntentV1Purpose,
		TurnBindingHMAC: strings.TrimSpace(input.TurnBindingHMAC), UsageSource: input.UsageSource,
		ChildRunHMAC: strings.TrimSpace(input.ChildRunHMAC), Channel: input.Channel,
		ChannelOrdinal: input.ChannelOrdinal, LogicalCallOrdinal: input.LogicalCallOrdinal,
		Shape: input.Shape, WireHeadersHMAC: strings.TrimSpace(input.WireHeadersHMAC),
		IssuedAt: issuedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: ProviderLedgerAuthorityAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	intent.IntentID = computeProviderAttemptIntentIDV1(intent)
	if !validProviderLedgerAuthorityInput(intent.AuthorityKeyID, publicKey, sign) {
		return ProviderAttemptIntentV1{}, errors.New("provider attempt intent authority is invalid")
	}
	signature, err := sign(ProviderAttemptIntentV1SigningBytes(intent))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ProviderAttemptIntentV1{}, errors.New("provider attempt intent signing failed")
	}
	intent.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateProviderAttemptIntentV1(intent); err != nil {
		return ProviderAttemptIntentV1{}, err
	}
	return intent, nil
}

func NewProviderAttemptSettlementV1(input ProviderAttemptSettlementInputV1, sign ProviderLedgerSignFunc) (ProviderAttemptSettlementV1, error) {
	if err := ValidateProviderAttemptIntentV1(input.Intent); err != nil {
		return ProviderAttemptSettlementV1{}, fmt.Errorf("provider attempt settlement intent is invalid: %w", err)
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	settlement := ProviderAttemptSettlementV1{
		SchemaVersion: ProviderAttemptSettlementV1SchemaVersion, Purpose: ProviderAttemptSettlementV1Purpose,
		IntentID: input.Intent.IntentID, TurnBindingHMAC: input.Intent.TurnBindingHMAC,
		DispatchState: input.DispatchState, Observation: input.Observation,
		SafeReasonCode: strings.TrimSpace(input.SafeReasonCode), AuthorityAlgorithm: ProviderLedgerAuthorityAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	settlement.SettlementID = computeProviderAttemptSettlementIDV1(settlement)
	if !validProviderLedgerAuthorityInput(settlement.AuthorityKeyID, publicKey, sign) {
		return ProviderAttemptSettlementV1{}, errors.New("provider attempt settlement authority is invalid")
	}
	signature, err := sign(ProviderAttemptSettlementV1SigningBytes(settlement))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ProviderAttemptSettlementV1{}, errors.New("provider attempt settlement signing failed")
	}
	settlement.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateProviderAttemptSettlementForIntentV1(settlement, input.Intent); err != nil {
		return ProviderAttemptSettlementV1{}, err
	}
	return settlement, nil
}

func NewProviderTurnClosureV1(input ProviderTurnClosureInputV1, sign ProviderLedgerSignFunc) (ProviderTurnClosureV1, error) {
	closedAt := input.ClosedAt.UTC()
	if closedAt.IsZero() {
		return ProviderTurnClosureV1{}, errors.New("provider turn closure time is required")
	}
	intentSetDigest, settlementSetDigest, aggregateDigest, eligible, err := providerLedgerInventoryDigestsV1(
		strings.TrimSpace(input.TurnBindingHMAC), input.Intents, input.Settlements, closedAt,
	)
	if err != nil {
		return ProviderTurnClosureV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	closure := ProviderTurnClosureV1{
		SchemaVersion: ProviderTurnClosureV1SchemaVersion, Purpose: ProviderTurnClosureV1Purpose,
		TurnBindingHMAC: strings.TrimSpace(input.TurnBindingHMAC), IntentCount: uint32(len(input.Intents)),
		SettlementCount: uint32(len(input.Settlements)), IntentSetDigest: intentSetDigest,
		SettlementSetDigest: settlementSetDigest, AggregateDigest: aggregateDigest,
		TerminalReasonCode: input.TerminalReasonCode,
		BenchmarkEligible:  eligible && providerTurnTerminalReasonAllowsBenchmarkV1(input.TerminalReasonCode),
		ClosedAt:           closedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: ProviderLedgerAuthorityAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	closure.ClosureID = computeProviderTurnClosureIDV1(closure)
	if !validProviderLedgerAuthorityInput(closure.AuthorityKeyID, publicKey, sign) {
		return ProviderTurnClosureV1{}, errors.New("provider turn closure authority is invalid")
	}
	signature, err := sign(ProviderTurnClosureV1SigningBytes(closure))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ProviderTurnClosureV1{}, errors.New("provider turn closure signing failed")
	}
	closure.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateProviderTurnClosureForInventoryV1(closure, input.Intents, input.Settlements); err != nil {
		return ProviderTurnClosureV1{}, err
	}
	return closure, nil
}

func ValidateProviderAttemptIntentV1(intent ProviderAttemptIntentV1) error {
	if intent.SchemaVersion != ProviderAttemptIntentV1SchemaVersion || intent.Purpose != ProviderAttemptIntentV1Purpose ||
		!isLowerHexDigest(intent.IntentID) || !isLowerHexDigest(intent.TurnBindingHMAC) || !intent.UsageSource.valid() ||
		!isLowerHexDigest(intent.ChildRunHMAC) || !intent.Channel.valid() || intent.ChannelOrdinal == 0 ||
		intent.ChannelOrdinal > maxProviderLedgerOrdinal || intent.LogicalCallOrdinal == 0 ||
		intent.LogicalCallOrdinal > maxProviderLedgerOrdinal || !isLowerHexDigest(intent.WireHeadersHMAC) ||
		intent.AuthorityAlgorithm != ProviderLedgerAuthorityAlgorithm || !isLowerHexDigest(intent.AuthorityKeyID) {
		return errors.New("provider attempt intent identity is invalid")
	}
	if err := intent.Shape.Validate(); err != nil {
		return fmt.Errorf("provider attempt intent shape is invalid: %w", err)
	}
	issuedAt, err := parseCanonicalTimestamp(intent.IssuedAt)
	if err != nil {
		return fmt.Errorf("provider attempt intent issued time is invalid: %w", err)
	}
	startedAt, _ := parseCanonicalTimestamp(intent.Shape.StartedAt)
	if !issuedAt.Equal(startedAt) {
		return errors.New("provider attempt intent issuance does not bind shape start")
	}
	if intent.IntentID != computeProviderAttemptIntentIDV1(intent) {
		return errors.New("provider attempt intent integrity is invalid")
	}
	return validateProviderLedgerSignature(intent.AuthorityKeyID, intent.AuthorityPublicKey, intent.AuthoritySignature, ProviderAttemptIntentV1SigningBytes(intent), "provider attempt intent")
}

func ValidateProviderAttemptSettlementV1(settlement ProviderAttemptSettlementV1) error {
	if settlement.SchemaVersion != ProviderAttemptSettlementV1SchemaVersion || settlement.Purpose != ProviderAttemptSettlementV1Purpose ||
		!isLowerHexDigest(settlement.SettlementID) || !isLowerHexDigest(settlement.IntentID) ||
		!isLowerHexDigest(settlement.TurnBindingHMAC) || !settlement.DispatchState.valid() ||
		!providerLedgerReasonCodePattern.MatchString(settlement.SafeReasonCode) ||
		settlement.AuthorityAlgorithm != ProviderLedgerAuthorityAlgorithm || !isLowerHexDigest(settlement.AuthorityKeyID) {
		return errors.New("provider attempt settlement identity is invalid")
	}
	if err := settlement.Observation.Validate(); err != nil {
		return fmt.Errorf("provider attempt settlement observation is invalid: %w", err)
	}
	if err := validateProviderDispatchSemanticsV1(settlement.DispatchState, settlement.Observation); err != nil {
		return err
	}
	if settlement.SettlementID != computeProviderAttemptSettlementIDV1(settlement) {
		return errors.New("provider attempt settlement integrity is invalid")
	}
	return validateProviderLedgerSignature(settlement.AuthorityKeyID, settlement.AuthorityPublicKey, settlement.AuthoritySignature, ProviderAttemptSettlementV1SigningBytes(settlement), "provider attempt settlement")
}

func ValidateProviderAttemptSettlementForIntentV1(settlement ProviderAttemptSettlementV1, intent ProviderAttemptIntentV1) error {
	if err := ValidateProviderAttemptIntentV1(intent); err != nil {
		return err
	}
	if err := ValidateProviderAttemptSettlementV1(settlement); err != nil {
		return err
	}
	if settlement.IntentID != intent.IntentID || settlement.TurnBindingHMAC != intent.TurnBindingHMAC ||
		settlement.Observation.Shape != intent.Shape || settlement.AuthorityKeyID != intent.AuthorityKeyID ||
		settlement.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("provider attempt settlement does not bind the exact intent")
	}
	return nil
}

func ValidateProviderTurnClosureV1(closure ProviderTurnClosureV1) error {
	if closure.SchemaVersion != ProviderTurnClosureV1SchemaVersion || closure.Purpose != ProviderTurnClosureV1Purpose ||
		!isLowerHexDigest(closure.ClosureID) || !isLowerHexDigest(closure.TurnBindingHMAC) ||
		closure.IntentCount > maxProviderLedgerOrdinal || closure.SettlementCount > maxProviderLedgerOrdinal ||
		!isLowerHexDigest(closure.IntentSetDigest) || !isLowerHexDigest(closure.SettlementSetDigest) ||
		!isLowerHexDigest(closure.AggregateDigest) || ValidateProviderTurnTerminalReasonV1(closure.TerminalReasonCode) != nil ||
		closure.AuthorityAlgorithm != ProviderLedgerAuthorityAlgorithm || !isLowerHexDigest(closure.AuthorityKeyID) {
		return errors.New("provider turn closure identity is invalid")
	}
	if _, err := parseCanonicalTimestamp(closure.ClosedAt); err != nil {
		return fmt.Errorf("provider turn closure time is invalid: %w", err)
	}
	if closure.ClosureID != computeProviderTurnClosureIDV1(closure) {
		return errors.New("provider turn closure integrity is invalid")
	}
	return validateProviderLedgerSignature(closure.AuthorityKeyID, closure.AuthorityPublicKey, closure.AuthoritySignature, ProviderTurnClosureV1SigningBytes(closure), "provider turn closure")
}

func ParseProviderAttemptIntentV1(body []byte) (ProviderAttemptIntentV1, error) {
	var intent ProviderAttemptIntentV1
	if err := json.Unmarshal(body, &intent); err != nil {
		return ProviderAttemptIntentV1{}, err
	}
	return intent, ValidateProviderAttemptIntentV1(intent)
}

func ParseProviderAttemptSettlementV1(body []byte) (ProviderAttemptSettlementV1, error) {
	var settlement ProviderAttemptSettlementV1
	if err := json.Unmarshal(body, &settlement); err != nil {
		return ProviderAttemptSettlementV1{}, err
	}
	return settlement, ValidateProviderAttemptSettlementV1(settlement)
}

func ParseProviderTurnClosureV1(body []byte) (ProviderTurnClosureV1, error) {
	var closure ProviderTurnClosureV1
	if err := json.Unmarshal(body, &closure); err != nil {
		return ProviderTurnClosureV1{}, err
	}
	return closure, ValidateProviderTurnClosureV1(closure)
}

func ProviderAttemptIntentV1Bytes(intent ProviderAttemptIntentV1) ([]byte, error) {
	if err := ValidateProviderAttemptIntentV1(intent); err != nil {
		return nil, err
	}
	return json.Marshal(intent)
}

func ProviderAttemptSettlementV1Bytes(settlement ProviderAttemptSettlementV1) ([]byte, error) {
	if err := ValidateProviderAttemptSettlementV1(settlement); err != nil {
		return nil, err
	}
	return json.Marshal(settlement)
}

func ProviderTurnClosureV1Bytes(closure ProviderTurnClosureV1) ([]byte, error) {
	if err := ValidateProviderTurnClosureV1(closure); err != nil {
		return nil, err
	}
	return json.Marshal(closure)
}

func ProviderAttemptIntentV1SigningBytes(intent ProviderAttemptIntentV1) []byte {
	value := intent
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(providerIntentSignatureDomain), body...)
}

func ProviderAttemptSettlementV1SigningBytes(settlement ProviderAttemptSettlementV1) []byte {
	value := settlement
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(providerSettlementSignatureDomain), body...)
}

func ProviderTurnClosureV1SigningBytes(closure ProviderTurnClosureV1) []byte {
	value := closure
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(providerClosureSignatureDomain), body...)
}

func ProviderAttemptIntentV1AuthorityMaterial(intent ProviderAttemptIntentV1) (string, []byte, []byte, error) {
	if err := ValidateProviderAttemptIntentV1(intent); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(intent.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(intent.AuthoritySignature)
	return intent.AuthorityKeyID, publicKey, signature, nil
}

func ProviderAttemptSettlementV1AuthorityMaterial(settlement ProviderAttemptSettlementV1) (string, []byte, []byte, error) {
	if err := ValidateProviderAttemptSettlementV1(settlement); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(settlement.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(settlement.AuthoritySignature)
	return settlement.AuthorityKeyID, publicKey, signature, nil
}

func ProviderTurnClosureV1AuthorityMaterial(closure ProviderTurnClosureV1) (string, []byte, []byte, error) {
	if err := ValidateProviderTurnClosureV1(closure); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(closure.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(closure.AuthoritySignature)
	return closure.AuthorityKeyID, publicKey, signature, nil
}

func (intent *ProviderAttemptIntentV1) UnmarshalJSON(body []byte) error {
	type wireIntent ProviderAttemptIntentV1
	var decoded wireIntent
	if err := decodeStrictObject(body, &decoded, []string{
		"schemaVersion", "purpose", "intentId", "turnBindingHmac", "usageSource", "childRunHmac",
		"channel", "channelOrdinal", "logicalCallOrdinal", "shape", "wireHeadersHmac", "issuedAt",
		"authorityAlgorithm", "authorityKeyId", "authorityPublicKey", "authoritySignature",
	}); err != nil {
		return fmt.Errorf("decode provider attempt intent: %w", err)
	}
	candidate := ProviderAttemptIntentV1(decoded)
	if err := ValidateProviderAttemptIntentV1(candidate); err != nil {
		return err
	}
	*intent = candidate
	return nil
}

func (settlement *ProviderAttemptSettlementV1) UnmarshalJSON(body []byte) error {
	type wireSettlement ProviderAttemptSettlementV1
	var decoded wireSettlement
	if err := decodeStrictObject(body, &decoded, []string{
		"schemaVersion", "purpose", "settlementId", "intentId", "turnBindingHmac", "dispatchState",
		"observation", "safeReasonCode", "authorityAlgorithm", "authorityKeyId", "authorityPublicKey", "authoritySignature",
	}); err != nil {
		return fmt.Errorf("decode provider attempt settlement: %w", err)
	}
	candidate := ProviderAttemptSettlementV1(decoded)
	if err := ValidateProviderAttemptSettlementV1(candidate); err != nil {
		return err
	}
	*settlement = candidate
	return nil
}

func (closure *ProviderTurnClosureV1) UnmarshalJSON(body []byte) error {
	type wireClosure ProviderTurnClosureV1
	var decoded wireClosure
	if err := decodeStrictObject(body, &decoded, []string{
		"schemaVersion", "purpose", "closureId", "turnBindingHmac", "intentCount", "settlementCount",
		"intentSetDigest", "settlementSetDigest", "aggregateDigest", "terminalReasonCode", "benchmarkEligible",
		"closedAt", "authorityAlgorithm", "authorityKeyId", "authorityPublicKey", "authoritySignature",
	}); err != nil {
		return fmt.Errorf("decode provider turn closure: %w", err)
	}
	candidate := ProviderTurnClosureV1(decoded)
	if err := ValidateProviderTurnClosureV1(candidate); err != nil {
		return err
	}
	*closure = candidate
	return nil
}

func computeProviderAttemptIntentIDV1(intent ProviderAttemptIntentV1) string {
	value := intent
	value.IntentID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(providerIntentIDDomain), body...))
}

func computeProviderAttemptSettlementIDV1(settlement ProviderAttemptSettlementV1) string {
	value := settlement
	value.SettlementID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(providerSettlementIDDomain), body...))
}

func computeProviderTurnClosureIDV1(closure ProviderTurnClosureV1) string {
	value := closure
	value.ClosureID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(providerClosureIDDomain), body...))
}

func validProviderLedgerAuthorityInput(keyID string, publicKey []byte, sign ProviderLedgerSignFunc) bool {
	return len(publicKey) == ed25519.PublicKeySize && strings.TrimSpace(keyID) == domainsecurity.SHA256Hex(publicKey) && sign != nil
}

func validateProviderLedgerSignature(keyID, encodedPublicKey, encodedSignature string, signingBytes []byte, name string) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return fmt.Errorf("%s authority material is invalid", name)
	}
	return nil
}

func validateProviderDispatchSemanticsV1(state ProviderDispatchStateV1, observation ProviderCallObservationV1) error {
	if observation.Status == ProviderCallStatusSucceeded && state != ProviderDispatchStateSent {
		return errors.New("successful provider observation requires sent dispatch")
	}
	if observation.Status == ProviderCallStatusStreamAborted && state != ProviderDispatchStateSent {
		return errors.New("stream-aborted provider observation requires sent dispatch")
	}
	if observation.Status == ProviderCallStatusRestartInterrupted && state != ProviderDispatchStateIndeterminate {
		return errors.New("restart-interrupted provider observation requires indeterminate dispatch")
	}
	if state != ProviderDispatchStateSent && !providerUsageAllUnknownV1(observation.Usage) {
		return errors.New("non-sent provider observation requires unknown usage")
	}
	if state == ProviderDispatchStateIndeterminate && observation.Status == ProviderCallStatusSucceeded {
		return errors.New("indeterminate provider dispatch cannot succeed")
	}
	return nil
}

func providerUsageAllUnknownV1(usage ProviderUsageV1) bool {
	return !usage.InputTokens.Known && !usage.OutputTokens.Known && !usage.CacheHitTokens.Known &&
		!usage.CacheMissTokens.Known && !usage.ReasoningTokens.Known
}

func (source ProviderUsageSourceV1) valid() bool {
	return source == ProviderUsageSourceTurn || source == ProviderUsageSourceSubagent
}

func (channel ProviderChannelV1) valid() bool {
	switch channel {
	case ProviderChannelPrimary, ProviderChannelAttachmentVision, ProviderChannelToolResultVision:
		return true
	default:
		return false
	}
}

func (state ProviderDispatchStateV1) valid() bool {
	switch state {
	case ProviderDispatchStateNotSent, ProviderDispatchStateSent, ProviderDispatchStateIndeterminate:
		return true
	default:
		return false
	}
}
