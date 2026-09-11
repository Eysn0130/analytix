package continuation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

const (
	ContractVersionV1          = 1
	ContractVersionV2          = 2
	ContractVersionV3          = 3
	ContractVersion            = ContractVersionV3
	AuthorityAlgorithm         = "Ed25519"
	KindApproval               = "approval"
	KindUserInput              = "user_input"
	StatusAllowed              = "allowed"
	StatusDenied               = "denied"
	StatusSubmitted            = "submitted"
	StatusCancelled            = "cancelled"
	StatusRejected             = "rejected"
	StatusInterrupted          = "interrupted"
	StatusRestartInvalid       = "restart_invalid"
	maxGUIPlanBytes            = 256 * 1024
	maxToolArgumentsBytes      = 4 * 1024 * 1024
	maxContinuationTextBytes   = 4096
	receiptSigningDomainV2     = "analytix/gate-continuation-receipt/v2\x00"
	receiptSigningDomain       = "analytix/gate-continuation-receipt/v3\x00"
	dispositionSigningDomainV2 = "analytix/gate-continuation-disposition/v2\x00"
	dispositionSigningDomain   = "analytix/gate-continuation-disposition/v3\x00"
)

type Payload struct {
	Version                     int                                   `json:"version"`
	Kind                        string                                `json:"kind"`
	GateID                      string                                `json:"gateId"`
	ThreadID                    string                                `json:"threadId"`
	TurnID                      string                                `json:"turnId"`
	ItemID                      string                                `json:"itemId"`
	CallID                      string                                `json:"callId"`
	ToolName                    string                                `json:"toolName"`
	ToolCallItemID              string                                `json:"toolCallItemId"`
	Arguments                   json.RawMessage                       `json:"arguments"`
	ProviderID                  string                                `json:"providerId"`
	Model                       string                                `json:"model"`
	ProviderRouteHash           string                                `json:"providerRouteHash"`
	WorkspaceCheckpointID       string                                `json:"workspaceCheckpointId,omitempty"`
	ApprovalPolicy              string                                `json:"approvalPolicy"`
	SandboxMode                 string                                `json:"sandboxMode"`
	SubagentDepth               int                                   `json:"subagentDepth"`
	ReasoningEffort             string                                `json:"reasoningEffort,omitempty"`
	Mode                        string                                `json:"mode,omitempty"`
	GUIPlan                     json.RawMessage                       `json:"guiPlan,omitempty"`
	DisableUserInput            bool                                  `json:"disableUserInput"`
	MaxModelSteps               *int                                  `json:"maxModelSteps,omitempty"`
	EffectiveMaxModelSteps      *int                                  `json:"effectiveMaxModelSteps,omitempty"`
	ToolScope                   []string                              `json:"toolScope"`
	PriorSettledToolRefs        []domainsecurity.SettledToolReference `json:"priorSettledToolRefs,omitempty"`
	LogicalEffect               domainsecurity.LogicalEffect          `json:"logicalEffect,omitempty"`
	OrdinaryWork                bool                                  `json:"ordinaryWork,omitempty"`
	ProviderStepExact           bool                                  `json:"providerStepExact,omitempty"`
	CaseSourceUnavailable       bool                                  `json:"caseSourceUnavailable,omitempty"`
	OrdinaryResultInputIsolated bool                                  `json:"ordinaryResultInputIsolated,omitempty"`
	PrivateProtocolObserved     bool                                  `json:"privateProtocolObserved,omitempty"`
	ProviderStepPromptSHA256    string                                `json:"providerStepPromptSha256,omitempty"`
	ProviderNamespace           ProviderContinuationNamespaceV1       `json:"providerNamespace"`
	TerminalRecoveryKind        TerminalRecoveryKindV1                `json:"terminalRecoveryKind"`
	SecurityContext             domainsecurity.TurnSecurityContext    `json:"turnSecurityContext"`
	ExecutionGrant              domainsecurity.ExecutionGrant         `json:"executionGrant"`
	IssuedAt                    string                                `json:"issuedAt"`
}

type Receipt struct {
	Version            int     `json:"version"`
	ReceiptID          string  `json:"receiptId"`
	Payload            Payload `json:"payload"`
	AuthorityAlgorithm string  `json:"authorityAlgorithm"`
	AuthorityKeyID     string  `json:"authorityKeyId"`
	AuthorityPublicKey string  `json:"authorityPublicKey"`
	AuthoritySignature string  `json:"authoritySignature"`
}

type Disposition struct {
	Version            int    `json:"version"`
	DispositionID      string `json:"dispositionId"`
	GateID             string `json:"gateId"`
	ReceiptID          string `json:"receiptId"`
	Kind               string `json:"kind"`
	Status             string `json:"status"`
	ReasonCode         string `json:"reasonCode"`
	DisposedAt         string `json:"disposedAt"`
	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
}

type SignFunc func([]byte) ([]byte, error)

func GateID(kind, threadID, turnID, contextDigest, grantID, callID string) string {
	prefix := "gate"
	switch strings.TrimSpace(kind) {
	case KindApproval, "appr":
		prefix = "appr"
	case KindUserInput, "input":
		prefix = "input"
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		prefix, strings.TrimSpace(threadID), strings.TrimSpace(turnID), strings.TrimSpace(contextDigest), strings.TrimSpace(grantID), strings.TrimSpace(callID),
	}, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:])
}

func CanonicalGUIPlan(value map[string]any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > maxGUIPlanBytes {
		return nil, errors.New("continuation GUI plan is invalid")
	}
	decoded, err := domainjsonstrict.DecodeValue(body, domainjsonstrict.Options{
		MaxBytes: maxGUIPlanBytes, MaxTokens: 50_000, MaxStringBytes: maxGUIPlanBytes,
	})
	if err != nil {
		return nil, errors.New("continuation GUI plan is invalid")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || len(canonical) > maxGUIPlanBytes {
		return nil, errors.New("continuation GUI plan is invalid")
	}
	return json.RawMessage(canonical), nil
}

// CanonicalPrivateToolArgumentsV2 preserves the exact JSON value while
// producing the byte-stable representation bound by ExecutionGrant.ArgsHash
// and the private continuation receipt. The name is retained for contract
// compatibility with the V2 canonicalization rule.
func CanonicalPrivateToolArgumentsV2(raw json.RawMessage) (json.RawMessage, error) {
	return canonicalToolArguments(raw)
}

// CanonicalProviderStepPromptSHA256 binds a continuation to the exact
// provider-step prompt without making the prompt itself durable. Whitespace
// outside the provider step is not semantic; an empty trimmed prompt has no
// executable binding and therefore returns the zero value.
func CanonicalProviderStepPromptSHA256(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	return domainsecurity.SHA256Hex([]byte(prompt))
}

func NewReceipt(payload Payload, keyID string, publicKey []byte, sign SignFunc) (Receipt, error) {
	if err := domainmodel.ValidateReasoningEffortV1(payload.ReasoningEffort); err != nil {
		return Receipt{}, errors.New("continuation receipt reasoning effort is invalid")
	}
	payload = sealPayload(payload)
	if err := ValidatePayloadForExecution(payload); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{
		Version: ContractVersion, Payload: payload, ReceiptID: payloadDigest(payload),
		AuthorityAlgorithm: AuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(keyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if !domainsecurity.IsSHA256Hex(receipt.AuthorityKeyID) || domainsecurity.SHA256Hex(publicKey) != receipt.AuthorityKeyID || sign == nil {
		return Receipt{}, errors.New("continuation receipt authority is invalid")
	}
	signature, err := sign(ReceiptSigningBytes(receipt))
	if err != nil || len(signature) == 0 {
		return Receipt{}, errors.New("continuation receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateReceipt(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func NewDisposition(receipt Receipt, status, reasonCode string, disposedAt time.Time, keyID string, publicKey []byte, sign SignFunc) (Disposition, error) {
	if err := ValidateReceipt(receipt); err != nil {
		return Disposition{}, err
	}
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	disposition := Disposition{
		Version: ContractVersion, GateID: receipt.Payload.GateID, ReceiptID: receipt.ReceiptID, Kind: receipt.Payload.Kind,
		Status: strings.TrimSpace(status), ReasonCode: strings.TrimSpace(reasonCode), DisposedAt: disposedAt.UTC().Format(time.RFC3339Nano),
		AuthorityAlgorithm: AuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(keyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = dispositionDigest(disposition)
	if !domainsecurity.IsSHA256Hex(disposition.AuthorityKeyID) || domainsecurity.SHA256Hex(publicKey) != disposition.AuthorityKeyID || sign == nil {
		return Disposition{}, errors.New("continuation disposition authority is invalid")
	}
	signature, err := sign(DispositionSigningBytes(disposition))
	if err != nil || len(signature) == 0 {
		return Disposition{}, errors.New("continuation disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateDisposition(disposition); err != nil {
		return Disposition{}, err
	}
	return disposition, nil
}

func ParseReceipt(body []byte) (Receipt, error) {
	var receipt Receipt
	if err := decodeStrict(body, &receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, ValidateReceipt(receipt)
}

func ParseDisposition(body []byte) (Disposition, error) {
	var disposition Disposition
	if err := decodeStrict(body, &disposition); err != nil {
		return Disposition{}, err
	}
	return disposition, ValidateDisposition(disposition)
}

func ValidatePayload(payload Payload) error {
	if !validContractVersion(payload.Version) || !validKind(payload.Kind) || !validText(payload.ThreadID, true) || !validText(payload.TurnID, true) ||
		!validText(payload.ItemID, true) || !validText(payload.CallID, true) || !validText(payload.ToolName, true) || !validText(payload.ToolCallItemID, true) ||
		!validText(payload.ProviderID, true) || !validText(payload.Model, true) || !domainsecurity.IsSHA256Hex(payload.ProviderRouteHash) ||
		!validApprovalPolicy(payload.ApprovalPolicy) || !validSandboxMode(payload.SandboxMode) || payload.SubagentDepth < 0 || payload.SubagentDepth > 32 ||
		!validOptionalEffort(payload.ReasoningEffort) || !validOptionalMode(payload.Mode) || !validOptionalText(payload.WorkspaceCheckpointID) ||
		!validSteps(payload.MaxModelSteps) || !validSteps(payload.EffectiveMaxModelSteps) || len(payload.ToolScope) == 0 || len(payload.ToolScope) > 512 {
		return errors.New("continuation payload identity or policy is invalid")
	}
	if payload.GateID != GateID(payload.Kind, payload.ThreadID, payload.TurnID, payload.SecurityContext.ContextDigest, payload.ExecutionGrant.GrantID, payload.CallID) ||
		payload.ItemID != "item_"+payload.GateID || payload.SecurityContext.ThreadID != payload.ThreadID || payload.SecurityContext.TurnID != payload.TurnID ||
		payload.ExecutionGrant.TurnID != payload.TurnID || payload.ExecutionGrant.ContextDigest != payload.SecurityContext.ContextDigest ||
		payload.ExecutionGrant.ToolCallID != payload.CallID || payload.ExecutionGrant.ToolName != payload.ToolName || payload.ExecutionGrant.Provider != payload.ProviderID {
		return errors.New("continuation payload authority binding is invalid")
	}
	canonicalArguments, argumentsErr := canonicalToolArguments(payload.Arguments)
	if argumentsErr != nil || !bytes.Equal(canonicalArguments, payload.Arguments) ||
		domainsecurity.CanonicalJSONHash(payload.Arguments) != payload.ExecutionGrant.ArgsHash {
		return errors.New("continuation private tool arguments are invalid or grant-mismatched")
	}
	if err := domainsecurity.ValidateTurnSecurityContext(payload.SecurityContext); err != nil {
		return errors.New("continuation turn security context is invalid")
	}
	if err := domainsecurity.ValidateExecutionGrantForAudit(payload.ExecutionGrant); err != nil {
		return errors.New("continuation execution grant is invalid")
	}
	if err := domainsecurity.ValidateSettledToolReferences(payload.PriorSettledToolRefs); err != nil {
		return errors.New("continuation prior settled tool authority is invalid")
	}
	if payload.Version == ContractVersionV2 {
		if hasProviderStepBindingFields(payload) || payload.PrivateProtocolObserved ||
			payload.ProviderNamespace != (ProviderContinuationNamespaceV1{}) ||
			payload.TerminalRecoveryKind != TerminalRecoveryNoneV1 {
			return errors.New("legacy continuation payload contains unsupported authority")
		}
	} else {
		providerStepBound, err := validateProviderStepBinding(payload)
		if err != nil {
			return err
		}
		if payload.PrivateProtocolObserved && !providerStepBound {
			return errors.New("continuation private-protocol observation lacks provider-step authority")
		}
		if ValidateProviderContinuationNamespaceV1(payload.ProviderNamespace, payload.SubagentDepth, payload.ToolScope) != nil ||
			ValidateTerminalRecoveryKindV1(payload.TerminalRecoveryKind) != nil {
			return errors.New("continuation provider or recovery authority is invalid")
		}
	}
	if payload.ExecutionGrant.ScopeHash != toolScopeHash(payload.ToolScope) {
		return errors.New("continuation tool scope does not match execution grant")
	}
	if payload.Kind == KindApproval {
		if payload.ExecutionGrant.ApprovalState != "pending" || payload.ToolName == "user_input" || payload.ToolName == "request_user_input" {
			return errors.New("approval continuation grant state is invalid")
		}
	} else if payload.ExecutionGrant.ApprovalState != "not_required" || (payload.ToolName != "user_input" && payload.ToolName != "request_user_input") {
		return errors.New("user-input continuation grant state is invalid")
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	grantIssuedAt, grantIssuedErr := time.Parse(time.RFC3339Nano, payload.ExecutionGrant.IssuedAt)
	grantExpiresAt, grantExpiresErr := time.Parse(time.RFC3339Nano, payload.ExecutionGrant.ExpiresAt)
	if issuedErr != nil || grantIssuedErr != nil || grantExpiresErr != nil || issuedAt.Before(grantIssuedAt) || !issuedAt.Before(grantExpiresAt) {
		return errors.New("continuation payload time is invalid")
	}
	seen := map[string]bool{}
	for _, name := range payload.ToolScope {
		if !validText(name, true) || seen[name] {
			return errors.New("continuation tool scope is invalid")
		}
		seen[name] = true
	}
	if len(payload.GUIPlan) > 0 {
		if len(payload.GUIPlan) > maxGUIPlanBytes {
			return errors.New("continuation GUI plan is too large")
		}
		value, err := domainjsonstrict.DecodeValue(payload.GUIPlan, domainjsonstrict.Options{MaxBytes: maxGUIPlanBytes, MaxTokens: 50_000, MaxStringBytes: maxGUIPlanBytes})
		canonical, marshalErr := json.Marshal(value)
		if err != nil || marshalErr != nil || !bytes.Equal(canonical, payload.GUIPlan) {
			return errors.New("continuation GUI plan is not canonical JSON")
		}
	}
	return nil
}

// ValidatePayloadForExecution keeps strict historical decoding separate from
// the V3 live-record floor. The domain contract binds the exact call and grant
// to an ordinary-effect-capable V2 context; the application execution
// boundary applies the canonical per-call classifier before issue, resume, or
// consumption so a protected call still requires case-fact authority. V1
// contexts remain audit-only.
func ValidatePayloadForExecution(payload Payload) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	if payload.Version != ContractVersion || !HasExactProviderStepBinding(payload) ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(payload.SecurityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForOrdinaryContext(payload.ExecutionGrant, payload.SecurityContext) != nil ||
		!domainsecurity.IsHostToolCallIDV1(payload.CallID) ||
		!domainsecurity.IsHostToolCallIDV1(payload.ExecutionGrant.ToolCallID) ||
		payload.ToolCallItemID != domaintoolcall.ToolCallItemIDV1(payload.TurnID, payload.CallID) {
		return errors.New("continuation payload lacks current V3 execution authority")
	}
	return nil
}

// HasExactProviderStepBinding reports whether a structurally valid V3 payload
// carries the complete live provider-step binding. A V3 payload with every
// binding field absent remains readable for audit, but cannot execute.
func HasExactProviderStepBinding(payload Payload) bool {
	bound, err := validateProviderStepBinding(payload)
	return payload.Version == ContractVersionV3 && bound && err == nil
}

func validateProviderStepBinding(payload Payload) (bool, error) {
	if !hasProviderStepBindingFields(payload) {
		return false, nil
	}
	if !payload.ProviderStepExact || domainsecurity.ValidateLogicalEffect(payload.LogicalEffect) != nil ||
		!domainsecurity.IsSHA256Hex(payload.ProviderStepPromptSHA256) ||
		payload.ProviderStepPromptSHA256 == domainsecurity.SHA256Hex(nil) {
		return false, errors.New("continuation provider-step binding is incomplete or invalid")
	}
	if payload.LogicalEffect == domainsecurity.LogicalEffectOrdinary && !payload.OrdinaryWork {
		return false, errors.New("continuation ordinary provider-step binding lacks ordinary work")
	}
	if payload.CaseSourceUnavailable &&
		(payload.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !payload.OrdinaryWork) {
		return false, errors.New("continuation unavailable-source provider-step binding is invalid")
	}
	if payload.OrdinaryResultInputIsolated &&
		(payload.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !payload.OrdinaryWork) {
		return false, errors.New("continuation ordinary-result input isolation is invalid")
	}
	return true, nil
}

func hasProviderStepBindingFields(payload Payload) bool {
	return payload.LogicalEffect != "" || payload.OrdinaryWork || payload.ProviderStepExact ||
		payload.CaseSourceUnavailable || payload.OrdinaryResultInputIsolated ||
		strings.TrimSpace(payload.ProviderStepPromptSHA256) != ""
}

func ValidateReceiptForExecution(receipt Receipt) error {
	if err := ValidateReceipt(receipt); err != nil {
		return err
	}
	return ValidatePayloadForExecution(receipt.Payload)
}

func ValidateReceipt(receipt Receipt) error {
	if !validContractVersion(receipt.Version) || receipt.Version != receipt.Payload.Version || receipt.ReceiptID != payloadDigest(receipt.Payload) || !domainsecurity.IsSHA256Hex(receipt.ReceiptID) ||
		receipt.AuthorityAlgorithm != AuthorityAlgorithm || !domainsecurity.IsSHA256Hex(receipt.AuthorityKeyID) {
		return errors.New("continuation receipt identity is invalid")
	}
	if err := ValidatePayload(receipt.Payload); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != 32 || len(signature) != 64 || domainsecurity.SHA256Hex(publicKey) != receipt.AuthorityKeyID {
		return errors.New("continuation receipt authority material is invalid")
	}
	return nil
}

func ValidateDisposition(disposition Disposition) error {
	if !validContractVersion(disposition.Version) || !domainsecurity.IsSHA256Hex(disposition.DispositionID) || disposition.DispositionID != dispositionDigest(disposition) ||
		!validGateID(disposition.GateID, disposition.Kind) || !domainsecurity.IsSHA256Hex(disposition.ReceiptID) || !validKind(disposition.Kind) ||
		!validDispositionStatus(disposition.Status) || !validReasonCode(disposition.ReasonCode) || disposition.AuthorityAlgorithm != AuthorityAlgorithm ||
		!domainsecurity.IsSHA256Hex(disposition.AuthorityKeyID) {
		return errors.New("continuation disposition identity is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, disposition.DisposedAt); err != nil {
		return errors.New("continuation disposition time is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != 32 || len(signature) != 64 || domainsecurity.SHA256Hex(publicKey) != disposition.AuthorityKeyID {
		return errors.New("continuation disposition authority material is invalid")
	}
	return nil
}

func ReceiptSigningBytes(receipt Receipt) []byte {
	if receipt.Version == ContractVersionV2 {
		value := struct {
			Version            int       `json:"version"`
			ReceiptID          string    `json:"receiptId"`
			Payload            payloadV2 `json:"payload"`
			AuthorityAlgorithm string    `json:"authorityAlgorithm"`
			AuthorityKeyID     string    `json:"authorityKeyId"`
			AuthorityPublicKey string    `json:"authorityPublicKey"`
		}{receipt.Version, receipt.ReceiptID, legacyPayloadV2(receipt.Payload), receipt.AuthorityAlgorithm, receipt.AuthorityKeyID, receipt.AuthorityPublicKey}
		body, _ := json.Marshal(value)
		return append([]byte(receiptSigningDomainV2), body...)
	}
	value := struct {
		Version            int     `json:"version"`
		ReceiptID          string  `json:"receiptId"`
		Payload            Payload `json:"payload"`
		AuthorityAlgorithm string  `json:"authorityAlgorithm"`
		AuthorityKeyID     string  `json:"authorityKeyId"`
		AuthorityPublicKey string  `json:"authorityPublicKey"`
	}{receipt.Version, receipt.ReceiptID, receipt.Payload, receipt.AuthorityAlgorithm, receipt.AuthorityKeyID, receipt.AuthorityPublicKey}
	body, _ := json.Marshal(value)
	return append([]byte(receiptSigningDomain), body...)
}

func DispositionSigningBytes(disposition Disposition) []byte {
	value := disposition
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(dispositionSigningDomainForVersion(disposition.Version)), body...)
}

func ReceiptBytes(receipt Receipt) ([]byte, error) {
	if err := ValidateReceipt(receipt); err != nil {
		return nil, err
	}
	if receipt.Version == ContractVersionV2 {
		return json.Marshal(legacyReceiptV2(receipt))
	}
	return json.Marshal(receipt)
}

func DispositionBytes(disposition Disposition) ([]byte, error) {
	if err := ValidateDisposition(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func AuthorityMaterial(receipt Receipt) (string, []byte, []byte, error) {
	if err := ValidateReceipt(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func DispositionAuthorityMaterial(disposition Disposition) (string, []byte, []byte, error) {
	if err := ValidateDisposition(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func sealPayload(payload Payload) Payload {
	payload.Version = ContractVersion
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.ThreadID = strings.TrimSpace(payload.ThreadID)
	payload.TurnID = strings.TrimSpace(payload.TurnID)
	// Authority identities are never normalized before signing. Any whitespace
	// or alternate spelling is a different, invalid identity.
	payload.ToolName = strings.TrimSpace(payload.ToolName)
	if arguments, err := canonicalToolArguments(payload.Arguments); err == nil {
		payload.Arguments = arguments
	}
	payload.ProviderID = strings.TrimSpace(payload.ProviderID)
	payload.Model = strings.TrimSpace(payload.Model)
	payload.ProviderRouteHash = strings.TrimSpace(payload.ProviderRouteHash)
	payload.WorkspaceCheckpointID = strings.TrimSpace(payload.WorkspaceCheckpointID)
	payload.ApprovalPolicy = strings.TrimSpace(payload.ApprovalPolicy)
	payload.SandboxMode = strings.TrimSpace(payload.SandboxMode)
	payload.Mode = strings.TrimSpace(payload.Mode)
	payload.IssuedAt = strings.TrimSpace(payload.IssuedAt)
	for index := range payload.ToolScope {
		payload.ToolScope[index] = strings.TrimSpace(payload.ToolScope[index])
	}
	payload.ToolScope = append([]string(nil), payload.ToolScope...)
	payload.PriorSettledToolRefs = append([]domainsecurity.SettledToolReference(nil), payload.PriorSettledToolRefs...)
	payload.ProviderNamespace = CloneProviderContinuationNamespaceV1(payload.ProviderNamespace)
	payload.GateID = GateID(payload.Kind, payload.ThreadID, payload.TurnID, payload.SecurityContext.ContextDigest, payload.ExecutionGrant.GrantID, payload.CallID)
	payload.ItemID = "item_" + payload.GateID
	return payload
}

func payloadDigest(payload Payload) string {
	var body []byte
	if payload.Version == ContractVersionV2 {
		body, _ = json.Marshal(legacyPayloadV2(payload))
	} else {
		body, _ = json.Marshal(payload)
	}
	return domainsecurity.SHA256Hex(append([]byte(receiptSigningDomainForVersion(payload.Version)), body...))
}

func canonicalToolArguments(raw json.RawMessage) (json.RawMessage, error) {
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		MaxBytes: maxToolArgumentsBytes, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("continuation tool arguments must be an object")
	}
	body, err := json.Marshal(object)
	if err != nil || len(body) == 0 || len(body) > maxToolArgumentsBytes {
		return nil, errors.New("continuation tool arguments are invalid")
	}
	return json.RawMessage(body), nil
}

func dispositionDigest(disposition Disposition) string {
	value := disposition
	value.DispositionID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(dispositionSigningDomainForVersion(disposition.Version)), body...))
}

func validContractVersion(version int) bool {
	return version == ContractVersionV2 || version == ContractVersionV3
}

func receiptSigningDomainForVersion(version int) string {
	if version == ContractVersionV2 {
		return receiptSigningDomainV2
	}
	return receiptSigningDomain
}

func dispositionSigningDomainForVersion(version int) string {
	if version == ContractVersionV2 {
		return dispositionSigningDomainV2
	}
	return dispositionSigningDomain
}

func toolScopeHash(scope []string) string {
	values := append([]string(nil), scope...)
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	sort.Strings(values)
	body, _ := json.Marshal(values)
	return domainsecurity.SHA256Hex(body)
}

func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("continuation record contains trailing JSON")
	}
	return nil
}

func validKind(value string) bool { return value == KindApproval || value == KindUserInput }

func validGateID(value, kind string) bool {
	prefix := "appr_"
	if kind == KindUserInput {
		prefix = "input_"
	}
	return strings.HasPrefix(value, prefix) && domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, prefix))
}

func validDispositionStatus(value string) bool {
	switch value {
	case StatusAllowed, StatusDenied, StatusSubmitted, StatusCancelled, StatusRejected, StatusInterrupted, StatusRestartInvalid:
		return true
	default:
		return false
	}
}

func validApprovalPolicy(value string) bool {
	switch value {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return true
	default:
		return false
	}
}

func validSandboxMode(value string) bool {
	switch value {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return true
	default:
		return false
	}
}

func validOptionalEffort(value string) bool {
	return domainmodel.ValidateReasoningEffortV1(value) == nil
}

func validOptionalMode(value string) bool { return value == "" || value == "agent" || value == "plan" }

func validSteps(value *int) bool { return value == nil || (*value > 0 && *value <= 100_000) }

func validOptionalText(value string) bool { return value == "" || validText(value, false) }

func validText(value string, nonEmpty bool) bool {
	value = strings.TrimSpace(value)
	if nonEmpty && value == "" {
		return false
	}
	if len(value) > maxContinuationTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func validReasonCode(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}
