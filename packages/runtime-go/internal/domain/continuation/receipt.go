package continuation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
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
	ContractVersionV1                           = 1
	ContractVersionV2                           = 2
	ContractVersionV3                           = 3
	ContractVersion                             = ContractVersionV3
	AuthorityAlgorithm                          = "Ed25519"
	KindApproval                                = "approval"
	KindUserInput                               = "user_input"
	PlanReviewPromptV1                          = "Approve this plan and leave plan mode?"
	StatusAllowed                               = "allowed"
	StatusDenied                                = "denied"
	StatusSubmitted                             = "submitted"
	StatusContinued                             = "continued"
	StatusCancelled                             = "cancelled"
	StatusRejected                              = "rejected"
	StatusInterrupted                           = "interrupted"
	StatusRestartInvalid                        = "restart_invalid"
	ReasonUserInputContinuedV1                  = "user_input_continued"
	ReasonUserInputOutcomeUnknownAfterRestartV1 = "user_input_outcome_unknown_after_restart"
	maxGUIPlanBytes                             = 256 * 1024
	maxToolArgumentsBytes                       = 4 * 1024 * 1024
	MaxUserInputToolArgumentsBytesV1            = maxToolArgumentsBytes
	maxContinuationTextBytes                    = 4096
	receiptSigningDomainV2                      = "analytix/gate-continuation-receipt/v2\x00"
	receiptSigningDomain                        = "analytix/gate-continuation-receipt/v3\x00"
	dispositionSigningDomainV2                  = "analytix/gate-continuation-disposition/v2\x00"
	dispositionSigningDomain                    = "analytix/gate-continuation-disposition/v3\x00"
)

// UserInputTimedWaitV1 is immutable request policy, not mutable timer state.
// Nil means legacy blocking; -1 binds an unlimited wait to this exact call.
type UserInputTimedWaitV1 struct {
	TimeoutSeconds int `json:"timeoutSeconds"`
}

func ValidUserInputTimeoutSeconds(seconds int) bool {
	return seconds == -1 || (seconds >= 1 && seconds <= 2147483)
}

// Unknown-after-restart is a failed interrupted original tool outcome, not
// a foreground timeout. Its closed signed reason permits a -1 request to
// retain the original late-answer relation after its old waiter is gone.
func ValidUserInputContinuedReasonV1(wait *UserInputTimedWaitV1, reason string) bool {
	return wait != nil && ValidUserInputTimeoutSeconds(wait.TimeoutSeconds) &&
		(reason == ReasonUserInputOutcomeUnknownAfterRestartV1 || reason == ReasonUserInputContinuedV1 && wait.TimeoutSeconds != -1)
}

func CloneUserInputTimedWaitV1(wait *UserInputTimedWaitV1) *UserInputTimedWaitV1 {
	if wait == nil {
		return nil
	}
	copied := *wait
	return &copied
}

// UserInputReplyCaptureV1 identifies a private, audit-only late submission.
// It reuses the receipt/disposition CAS, but can never grant execution.
type UserInputReplyCaptureV1 struct {
	SourceInputID       string `json:"sourceInputId"`
	SourceReceiptID     string `json:"sourceReceiptId"`
	ClientUserMessageID string `json:"clientUserMessageId"`
	CapturedAt          string `json:"capturedAt"`
}

func UserInputReplyCaptureGateIDV1(sourceInputID, sourceReceiptID, clientUserMessageID string) string {
	digest := sha256.Sum256([]byte("analytix/user-input-reply-capture/v1\x00" + sourceInputID + "\x00" + sourceReceiptID + "\x00" + clientUserMessageID))
	return "input_" + hex.EncodeToString(digest[:])
}

func IsUserInputReplyAttemptIDV1(value string) bool { return validReplyAttemptIDV1(value) }

func validReplyAttemptIDV1(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' || !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	for index, c := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func ValidateUserInputReplyCaptureSourceV1(capture, source Receipt) error {
	link := capture.Payload.UserInputReplyCapture
	if link == nil || source.Payload.UserInputReplyCapture != nil || source.Payload.Kind != KindUserInput || source.Payload.UserInputTimedWait == nil || link.SourceInputID != source.Payload.GateID || link.SourceReceiptID != source.ReceiptID {
		return errors.New("late user input capture has no original timed request authority")
	}
	payload := capture.Payload
	payload.UserInputReplyCapture = nil
	payload.GateID, payload.ItemID = source.Payload.GateID, source.Payload.ItemID
	left, leftErr := json.Marshal(payload)
	right, rightErr := json.Marshal(source.Payload)
	if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
		return errors.New("late user input capture changed original request facts")
	}
	return nil
}

type Payload struct {
	UserInputDraft              *UserInputDraftV1                     `json:"userInputDraft,omitempty"`
	UserInputReplyCapture       *UserInputReplyCaptureV1              `json:"userInputReplyCapture,omitempty"`
	UserInputTimedWait          *UserInputTimedWaitV1                 `json:"userInputTimedWait,omitempty"`
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
	ModelVisibleToolDefinition  *domainmodel.ToolDefinitionSnapshotV1 `json:"modelVisibleToolDefinition,omitempty"`
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
	OutputCapLogicalSequence    uint64                                `json:"outputCapLogicalSequence,omitempty"`
	TerminalRecoveryKind        TerminalRecoveryKindV1                `json:"terminalRecoveryKind"`
	SecurityContext             domainsecurity.TurnSecurityContext    `json:"turnSecurityContext"`
	ExecutionGrant              domainsecurity.ExecutionGrant         `json:"executionGrant"`
	IssuedAt                    string                                `json:"issuedAt"`
}

// Private draft records have no executable or model authority fields. Keep the
// ordinary payload wire unchanged, including its historical signing bytes.
type userInputDraftPayloadWireV1 struct {
	UserInputDraft *UserInputDraftV1 `json:"userInputDraft"`
	Version        int               `json:"version"`
	Kind           string            `json:"kind"`
	GateID         string            `json:"gateId"`
	ThreadID       string            `json:"threadId"`
	TurnID         string            `json:"turnId"`
	ItemID         string            `json:"itemId"`
	CallID         string            `json:"callId"`
	IssuedAt       string            `json:"issuedAt"`
}

func (payload Payload) MarshalJSON() ([]byte, error) {
	type ordinaryPayload Payload
	if payload.UserInputDraft == nil && payload.Kind != KindUserInputDraft {
		return json.Marshal(ordinaryPayload(payload))
	}
	if err := validateUserInputDraftPayloadV1(payload); err != nil {
		return nil, err
	}
	return json.Marshal(userInputDraftPayloadWireV1{
		UserInputDraft: payload.UserInputDraft, Version: payload.Version, Kind: payload.Kind,
		GateID: payload.GateID, ThreadID: payload.ThreadID, TurnID: payload.TurnID,
		ItemID: payload.ItemID, CallID: payload.CallID, IssuedAt: payload.IssuedAt,
	})
}

func (payload *Payload) UnmarshalJSON(body []byte) error {
	type ordinaryPayload Payload
	var header struct {
		Kind           string          `json:"kind"`
		UserInputDraft json.RawMessage `json:"userInputDraft"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return err
	}
	if header.Kind != KindUserInputDraft && header.UserInputDraft == nil {
		var value ordinaryPayload
		if err := decodeStrict(body, &value); err != nil {
			return err
		}
		*payload = Payload(value)
		return nil
	}
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: MaxUserInputDraftBytesV1 + 4096, MaxDepth: 6})
	if err != nil || len(fields) != 9 {
		return errors.New("private draft payload is invalid")
	}
	for _, key := range []string{"userInputDraft", "version", "kind", "gateId", "threadId", "turnId", "itemId", "callId", "issuedAt"} {
		if raw, ok := fields[key]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("private draft payload is incomplete")
		}
	}
	var wire userInputDraftPayloadWireV1
	if err := decodeStrict(body, &wire); err != nil {
		return err
	}
	value := Payload{UserInputDraft: wire.UserInputDraft, Version: wire.Version, Kind: wire.Kind, GateID: wire.GateID, ThreadID: wire.ThreadID, TurnID: wire.TurnID, ItemID: wire.ItemID, CallID: wire.CallID, IssuedAt: wire.IssuedAt}
	if err := validateUserInputDraftPayloadV1(value); err != nil {
		return err
	}
	*payload = value
	return nil
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
	UserInputReplySourceID string                 `json:"userInputReplySourceId,omitempty"`
	Version                int                    `json:"version"`
	DispositionID          string                 `json:"dispositionId"`
	GateID                 string                 `json:"gateId"`
	ReceiptID              string                 `json:"receiptId"`
	Kind                   string                 `json:"kind"`
	Status                 string                 `json:"status"`
	ReasonCode             string                 `json:"reasonCode"`
	DisposedAt             string                 `json:"disposedAt"`
	UserInputSubmission    *UserInputSubmissionV1 `json:"userInputSubmission,omitempty"`
	AuthorityAlgorithm     string                 `json:"authorityAlgorithm"`
	AuthorityKeyID         string                 `json:"authorityKeyId"`
	AuthorityPublicKey     string                 `json:"authorityPublicKey"`
	AuthoritySignature     string                 `json:"authoritySignature"`
}

// UserInputSubmissionV1 is private local-display authority. Answers retain the
// validated client values before trimming or ordinary-text projection. It is
// never a public event, persisted tool output, or model carrier.
type UserInputSubmissionV1 struct {
	SchemaVersion int             `json:"schemaVersion"`
	Format        string          `json:"format"`
	Answers       json.RawMessage `json:"answers"`
}

const MaxUserInputSubmissionBytesV1 = maxToolArgumentsBytes

type UserInputStructuredAnswerV1 struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	OptionIDs  *[]string `json:"optionIds,omitempty"`
	CustomText *string   `json:"customText,omitempty"`
}

type UserInputLegacyAnswerV1 struct {
	ID         string  `json:"id"`
	Label      *string `json:"label,omitempty"`
	Value      string  `json:"value"`
	QuestionID string  `json:"questionId"`
}

func (submission *UserInputSubmissionV1) UnmarshalJSON(body []byte) error {
	type closedSubmission UserInputSubmissionV1
	var value closedSubmission
	if domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: MaxUserInputSubmissionBytesV1 + 4096, MaxDepth: 6}) != nil || decodeStrict(body, &value) != nil {
		return errors.New("private user input submission is invalid")
	}
	if err := ValidateUserInputSubmissionV1(UserInputSubmissionV1(value)); err != nil {
		return err
	}
	*submission = UserInputSubmissionV1(value)
	return nil
}

func CloneUserInputSubmissionV1(submission UserInputSubmissionV1) UserInputSubmissionV1 {
	submission.Answers = append(json.RawMessage(nil), submission.Answers...)
	return submission
}

func ValidateUserInputSubmissionV1(submission UserInputSubmissionV1) error {
	invalid := func() error { return errors.New("private user input submission is invalid") }
	if submission.SchemaVersion != 1 || (submission.Format != "structured" && submission.Format != "legacy") {
		return invalid()
	}
	// RawMessage is escaped by the signed record's JSON marshaler. Bound those
	// actual wire bytes too, so a valid source cannot outgrow its own reader.
	wireAnswers, err := json.Marshal(submission.Answers)
	if err != nil || len(wireAnswers) > MaxUserInputSubmissionBytesV1 {
		return invalid()
	}
	value, err := domainjsonstrict.DecodeValue(submission.Answers, domainjsonstrict.Options{MaxBytes: MaxUserInputSubmissionBytesV1, MaxDepth: 4, MaxTokens: 100_000, MaxStringBytes: MaxUserInputSubmissionBytesV1})
	records, ok := value.([]any)
	if err != nil || !ok || len(records) == 0 || len(records) > 3 {
		return invalid()
	}
	seen := map[string]bool{}
	for _, raw := range records {
		record, ok := raw.(map[string]any)
		if !ok {
			return invalid()
		}
		id, ok := record["id"].(string)
		if !ok || !validText(id, true) {
			return invalid()
		}
		bindingID := id
		if submission.Format == "legacy" {
			for key := range record {
				if key != "id" && key != "label" && key != "value" && key != "questionId" {
					return invalid()
				}
			}
			questionID, ok := record["questionId"].(string)
			if !ok || !validText(questionID, true) {
				return invalid()
			}
			bindingID = questionID
			answer, ok := record["value"].(string)
			if !ok || strings.TrimSpace(answer) == "" {
				return invalid()
			}
			if label, present := record["label"]; present {
				if _, ok := label.(string); !ok {
					return invalid()
				}
			}
		} else {
			for key := range record {
				if key != "id" && key != "kind" && key != "optionIds" && key != "customText" {
					return invalid()
				}
			}
			kind, _ := record["kind"].(string)
			if kind == "skip" {
				if len(record) != 2 {
					return invalid()
				}
			} else if kind == "answer" {
				options, ok := record["optionIds"].([]any)
				if !ok || len(options) > 3 {
					return invalid()
				}
				optionIDs := map[string]bool{}
				for _, rawOption := range options {
					optionID, ok := rawOption.(string)
					if !ok || !validText(optionID, true) || optionIDs[optionID] {
						return invalid()
					}
					optionIDs[optionID] = true
				}
				custom := ""
				if rawCustom, present := record["customText"]; present {
					var ok bool
					custom, ok = rawCustom.(string)
					if !ok || strings.TrimSpace(custom) == "" {
						return invalid()
					}
				}
				if len(options) == 0 && custom == "" {
					return invalid()
				}
			} else {
				return invalid()
			}
		}
		if seen[bindingID] {
			return invalid()
		}
		seen[bindingID] = true
	}
	return nil
}

func validateUserInputSubmissionGateIDs(gateID string, submission UserInputSubmissionV1) error {
	if err := ValidateUserInputSubmissionV1(submission); err != nil {
		return err
	}
	var records []map[string]any
	if json.Unmarshal(submission.Answers, &records) != nil {
		return errors.New("private user input submission is invalid")
	}
	for _, record := range records {
		questionID, _ := record["id"].(string)
		if submission.Format == "legacy" {
			questionID, _ = record["questionId"].(string)
		}
		matches := false
		for index := 1; index <= 3; index++ {
			if questionID == fmt.Sprintf("%s_%d", gateID, index) {
				matches = true
			}
		}
		if !matches {
			return errors.New("private user input submission crossed gate identity")
		}
		if options, ok := record["optionIds"].([]any); ok {
			for _, rawOption := range options {
				optionID, _ := rawOption.(string)
				matches := false
				for index := 1; index <= 3; index++ {
					if optionID == fmt.Sprintf("%s_option_%d", questionID, index) {
						matches = true
					}
				}
				if !matches {
					return errors.New("private user input submission crossed option identity")
				}
			}
		}
	}
	return nil
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
	if payload.UserInputDraft != nil || payload.UserInputReplyCapture != nil {
		return Receipt{}, errors.New("late capture is audit-only")
	}
	if err := domainmodel.ValidateReasoningEffortV1(payload.ReasoningEffort); err != nil {
		return Receipt{}, errors.New("continuation receipt reasoning effort is invalid")
	}
	payload = sealPayload(payload)
	if err := ValidatePayloadForExecution(payload); err != nil {
		return Receipt{}, err
	}
	return signReceipt(payload, keyID, publicKey, sign)
}

// NewUserInputReplyCaptureReceiptV1 preserves the original signed request
// facts while assigning a disjoint audit record identity for one stable attempt.
func NewUserInputReplyCaptureReceiptV1(source Receipt, attemptID string, capturedAt time.Time, keyID string, publicKey []byte, sign SignFunc) (Receipt, error) {
	if ValidateReceipt(source) != nil || source.Payload.UserInputReplyCapture != nil || source.Payload.Kind != KindUserInput || source.Payload.UserInputTimedWait == nil || !validReplyAttemptIDV1(attemptID) || capturedAt.IsZero() {
		return Receipt{}, errors.New("late user input capture is invalid")
	}
	payload := sealPayload(source.Payload)
	payload.UserInputReplyCapture = &UserInputReplyCaptureV1{SourceInputID: source.Payload.GateID, SourceReceiptID: source.ReceiptID, ClientUserMessageID: attemptID, CapturedAt: capturedAt.UTC().Format(time.RFC3339Nano)}
	payload.GateID = UserInputReplyCaptureGateIDV1(source.Payload.GateID, source.ReceiptID, attemptID)
	payload.ItemID = "item_" + payload.GateID
	if err := ValidatePayload(payload); err != nil {
		return Receipt{}, err
	}
	return signReceipt(payload, keyID, publicKey, sign)
}

func signReceipt(payload Payload, keyID string, publicKey []byte, sign SignFunc) (Receipt, error) {
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
	return newDisposition(receipt, status, reasonCode, disposedAt, keyID, publicKey, sign, nil)
}

func NewUserInputSubmittedDisposition(receipt Receipt, submission UserInputSubmissionV1, disposedAt time.Time, keyID string, publicKey []byte, sign SignFunc) (Disposition, error) {
	copy := CloneUserInputSubmissionV1(submission)
	return newDisposition(receipt, StatusSubmitted, "user_input_submitted", disposedAt, keyID, publicKey, sign, &copy)
}

func newDisposition(receipt Receipt, status, reasonCode string, disposedAt time.Time, keyID string, publicKey []byte, sign SignFunc, submission *UserInputSubmissionV1) (Disposition, error) {
	if receipt.Payload.UserInputDraft != nil || receipt.Payload.Kind == KindUserInputDraft {
		return Disposition{}, errors.New("private draft cannot receive a gate disposition")
	}
	if status == StatusContinued && !ValidUserInputContinuedReasonV1(receipt.Payload.UserInputTimedWait, reasonCode) {
		return Disposition{}, errors.New("continued disposition requires a finite timed request")
	}
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
	disposition.UserInputSubmission = submission
	submissionGateID := receipt.Payload.GateID
	if link := receipt.Payload.UserInputReplyCapture; link != nil {
		if submission == nil || status != StatusSubmitted || reasonCode != "user_input_submitted" || disposition.DisposedAt != link.CapturedAt {
			return Disposition{}, errors.New("late capture must contain its exact private submission")
		}
		disposition.UserInputReplySourceID, submissionGateID = link.SourceInputID, link.SourceInputID
	}
	if submission != nil && (receipt.Payload.Kind != KindUserInput || validateUserInputSubmissionGateIDs(submissionGateID, *submission) != nil) {
		return Disposition{}, errors.New("private user input submission does not match its receipt")
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
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: MaxUserInputSubmissionBytesV1 + 64*1024, MaxDepth: 8})
	if err != nil {
		return Disposition{}, err
	}
	if value, present := fields["userInputSubmission"]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return Disposition{}, errors.New("private user input submission must be absent or complete")
	}
	var disposition Disposition
	if err := decodeStrict(body, &disposition); err != nil {
		return Disposition{}, err
	}
	return disposition, ValidateDisposition(disposition)
}

func ValidatePayload(payload Payload) error {
	if payload.UserInputDraft != nil || payload.Kind == KindUserInputDraft {
		return validateUserInputDraftPayloadV1(payload)
	}
	if domainmodel.ValidateToolDefinitionSnapshotV1(payload.ModelVisibleToolDefinition, payload.ToolName) != nil || payload.Version != ContractVersionV3 && payload.ModelVisibleToolDefinition != nil {
		return errors.New("continuation model-visible tool definition is invalid")
	}
	if wait := payload.UserInputTimedWait; wait != nil && (payload.Version != ContractVersionV3 || payload.Kind != KindUserInput || payload.ToolName == "exit_plan_mode" || !ValidUserInputTimeoutSeconds(wait.TimeoutSeconds)) {
		return errors.New("timed user input request policy is invalid")
	}
	if !validContractVersion(payload.Version) || !validKind(payload.Kind) || !validText(payload.ThreadID, true) || !validText(payload.TurnID, true) ||
		!validText(payload.ItemID, true) || !validText(payload.CallID, true) || !validText(payload.ToolName, true) || !validText(payload.ToolCallItemID, true) ||
		!validText(payload.ProviderID, true) || !validText(payload.Model, true) || !domainsecurity.IsSHA256Hex(payload.ProviderRouteHash) ||
		!validApprovalPolicy(payload.ApprovalPolicy) || !validSandboxMode(payload.SandboxMode) || payload.SubagentDepth < 0 || payload.SubagentDepth > 32 ||
		!validOptionalEffort(payload.ReasoningEffort) || !validOptionalMode(payload.Mode) || !validOptionalText(payload.WorkspaceCheckpointID) ||
		!validSteps(payload.MaxModelSteps) || !validSteps(payload.EffectiveMaxModelSteps) || len(payload.ToolScope) == 0 || len(payload.ToolScope) > 512 {
		return errors.New("continuation payload identity or policy is invalid")
	}
	expectedGateID := GateID(payload.Kind, payload.ThreadID, payload.TurnID, payload.SecurityContext.ContextDigest, payload.ExecutionGrant.GrantID, payload.CallID)
	if link := payload.UserInputReplyCapture; link != nil {
		capturedAt, capturedErr := time.Parse(time.RFC3339Nano, link.CapturedAt)
		issuedAt, issuedErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
		if payload.Version != ContractVersionV3 || payload.Kind != KindUserInput || payload.UserInputTimedWait == nil || link.SourceInputID != expectedGateID || !domainsecurity.IsSHA256Hex(link.SourceReceiptID) || !validReplyAttemptIDV1(link.ClientUserMessageID) || capturedErr != nil || issuedErr != nil || capturedAt.UTC().Format(time.RFC3339Nano) != link.CapturedAt || capturedAt.Before(issuedAt) {
			return errors.New("late user input capture linkage is invalid")
		}
		expectedGateID = UserInputReplyCaptureGateIDV1(link.SourceInputID, link.SourceReceiptID, link.ClientUserMessageID)
	}
	if payload.GateID != expectedGateID ||
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
			payload.TerminalRecoveryKind != TerminalRecoveryNoneV1 || payload.OutputCapLogicalSequence != 0 {
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
			ValidateTerminalRecoveryKindV1(payload.TerminalRecoveryKind) != nil ||
			payload.OutputCapLogicalSequence > payload.ProviderNamespace.ProviderCallSequence {
			return errors.New("continuation provider or recovery authority is invalid")
		}
	}
	if payload.ExecutionGrant.ScopeHash != toolScopeHash(payload.ToolScope) {
		return errors.New("continuation tool scope does not match execution grant")
	}
	if payload.Kind == KindApproval {
		if payload.ExecutionGrant.ApprovalState != "pending" || payload.ToolName == "user_input" || payload.ToolName == "request_user_input" || payload.ToolName == "exit_plan_mode" {
			return errors.New("approval continuation grant state is invalid")
		}
	} else if payload.ExecutionGrant.ApprovalState != "not_required" || (payload.ToolName != "user_input" && payload.ToolName != "request_user_input" && payload.ToolName != "exit_plan_mode") {
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
	if payload.UserInputDraft != nil || payload.UserInputReplyCapture != nil || payload.Version != ContractVersion || !HasExactProviderStepBinding(payload) ||
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
	submissionGateID := disposition.GateID
	if disposition.UserInputReplySourceID != "" {
		if disposition.Version != ContractVersionV3 || disposition.Kind != KindUserInput || !validGateID(disposition.UserInputReplySourceID, KindUserInput) || disposition.UserInputSubmission == nil {
			return errors.New("late capture disposition is incomplete")
		}
		submissionGateID = disposition.UserInputReplySourceID
	}
	if disposition.Status == StatusContinued && (disposition.Version != ContractVersionV3 || disposition.Kind != KindUserInput || (disposition.ReasonCode != ReasonUserInputContinuedV1 && disposition.ReasonCode != ReasonUserInputOutcomeUnknownAfterRestartV1) || disposition.UserInputReplySourceID != "" || disposition.UserInputSubmission != nil) {
		return errors.New("continued disposition is not a timed user input foreground close")
	}
	if submission := disposition.UserInputSubmission; submission != nil {
		if disposition.Version != ContractVersionV3 || disposition.Kind != KindUserInput || disposition.Status != StatusSubmitted || disposition.ReasonCode != "user_input_submitted" || validateUserInputSubmissionGateIDs(submissionGateID, *submission) != nil {
			return errors.New("private user input submission has no submitted disposition authority")
		}
	}
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
	if receipt.Payload.UserInputDraft != nil {
		return append([]byte(userInputDraftSigningDomainV1), body...)
	}
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
	payload.ModelVisibleToolDefinition = domainmodel.CloneToolDefinitionSnapshotV1(payload.ModelVisibleToolDefinition)
	payload.UserInputTimedWait = CloneUserInputTimedWaitV1(payload.UserInputTimedWait)
	if payload.UserInputReplyCapture != nil {
		copy := *payload.UserInputReplyCapture
		payload.UserInputReplyCapture = &copy
	}
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
	if payload.UserInputDraft != nil {
		return domainsecurity.SHA256Hex(append([]byte(userInputDraftSigningDomainV1), body...))
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
	case StatusAllowed, StatusDenied, StatusSubmitted, StatusCancelled, StatusContinued, StatusRejected, StatusInterrupted, StatusRestartInvalid:
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

// UserInputDraftSnapshotV1 is unfinished private UI state. It never represents
// a submitted answer, queue admission, execution grant or model input.
type UserInputDraftAnswerV1 struct {
	QuestionID string   `json:"questionId"`
	OptionIDs  []string `json:"optionIds"`
	CustomText string   `json:"customText"`
	Skipped    bool     `json:"skipped"`
}

type UserInputDraftSnapshotV1 struct {
	Wait        *string                  `json:"wait,omitempty"`
	CurrentPage int                      `json:"currentPage"`
	Answers     []UserInputDraftAnswerV1 `json:"answers"`
}

type UserInputDraftV1 struct {
	CaptureSetSHA256 string                   `json:"captureSetSha256"`
	SourceInputID    string                   `json:"sourceInputId"`
	SourceReceiptID  string                   `json:"sourceReceiptId"`
	Revision         string                   `json:"revision"`
	ParentRevision   *string                  `json:"parentRevision"`
	Snapshot         UserInputDraftSnapshotV1 `json:"snapshot"`
}

const (
	KindUserInputDraft               = "user_input_draft"
	MaxUserInputDraftBytesV1         = MaxUserInputSubmissionBytesV1
	MaxUserInputDraftRevisionsV1     = 4096
	MaxUserInputDraftRetainedBytesV1 = 8 << 20
	userInputDraftSigningDomainV1    = "analytix/private-question-draft/v1\x00"
)

func IsUserInputDraftRevisionV1(value string) bool { return validReplyAttemptIDV1(value) }

func UserInputDraftSlotGateIDV1(sourceReceiptID string, parentRevision *string) string {
	parent := ""
	if parentRevision != nil {
		parent = *parentRevision
	}
	digest := sha256.Sum256([]byte("analytix/private-question-draft-slot/v1\x00" + sourceReceiptID + "\x00" + parent))
	return "input_" + hex.EncodeToString(digest[:])
}

func CloneUserInputDraftSnapshotV1(snapshot UserInputDraftSnapshotV1) UserInputDraftSnapshotV1 {
	copy := snapshot
	if snapshot.Wait != nil {
		value := *snapshot.Wait
		copy.Wait = &value
	}
	if snapshot.Answers != nil {
		copy.Answers = make([]UserInputDraftAnswerV1, len(snapshot.Answers))
		for i, answer := range snapshot.Answers {
			copy.Answers[i] = answer
			if answer.OptionIDs != nil {
				copy.Answers[i].OptionIDs = append([]string{}, answer.OptionIDs...)
			}
		}
	}
	return copy
}

func (link *UserInputDraftV1) UnmarshalJSON(body []byte) error {
	type closed UserInputDraftV1
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: MaxUserInputDraftBytesV1 + 2048, MaxDepth: 5})
	if err != nil || len(fields) != 6 {
		return errors.New("private draft linkage is invalid")
	}
	for _, key := range []string{"captureSetSha256", "sourceInputId", "sourceReceiptId", "revision", "parentRevision", "snapshot"} {
		raw, ok := fields[key]
		if !ok || (key != "parentRevision" && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return errors.New("private draft linkage is incomplete")
		}
	}
	var value closed
	if err := decodeStrict(body, &value); err != nil {
		return err
	}
	*link = UserInputDraftV1(value)
	return nil
}

func (answer *UserInputDraftAnswerV1) UnmarshalJSON(body []byte) error {
	type closed UserInputDraftAnswerV1
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: MaxUserInputDraftBytesV1, MaxDepth: 2})
	if err != nil || len(fields) != 4 {
		return errors.New("private draft answer is invalid")
	}
	for _, key := range []string{"questionId", "optionIds", "customText", "skipped"} {
		if raw, ok := fields[key]; !ok || bytes.Equal(raw, []byte("null")) {
			return errors.New("private draft answer is incomplete")
		}
	}
	var value closed
	if decodeStrict(body, &value) != nil {
		return errors.New("private draft answer is invalid")
	}
	*answer = UserInputDraftAnswerV1(value)
	return nil
}

func (snapshot *UserInputDraftSnapshotV1) UnmarshalJSON(body []byte) error {
	type closed UserInputDraftSnapshotV1
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: MaxUserInputDraftBytesV1, MaxDepth: 4})
	if err != nil || (len(fields) != 2 && len(fields) != 3) {
		return errors.New("private draft snapshot is invalid")
	}
	for _, key := range []string{"currentPage", "answers"} {
		if raw, ok := fields[key]; !ok || bytes.Equal(raw, []byte("null")) {
			return errors.New("private draft snapshot is incomplete")
		}
	}
	if raw, present := fields["wait"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("private draft wait choice is invalid")
	}
	var value closed
	if decodeStrict(body, &value) != nil {
		return errors.New("private draft snapshot is invalid")
	}
	*snapshot = UserInputDraftSnapshotV1(value)
	return nil
}

func ValidateUserInputDraftSnapshotV1(snapshot UserInputDraftSnapshotV1, inputID string) error {
	if snapshot.Wait != nil && *snapshot.Wait != "waiting" && *snapshot.Wait != "editing" {
		return errors.New("private draft wait choice is invalid")
	}
	if !validGateID(inputID, KindUserInput) || snapshot.CurrentPage < 0 || snapshot.CurrentPage > 2 || snapshot.Answers == nil || len(snapshot.Answers) > 3 {
		return errors.New("private draft snapshot is invalid")
	}
	seen := map[string]bool{}
	for _, answer := range snapshot.Answers {
		ordinal := strings.TrimPrefix(answer.QuestionID, inputID+"_")
		if (ordinal != "1" && ordinal != "2" && ordinal != "3") || answer.QuestionID != inputID+"_"+ordinal || seen[answer.QuestionID] || answer.OptionIDs == nil || len(answer.OptionIDs) > 3 || !utf8.ValidString(answer.CustomText) || len(answer.CustomText) > MaxUserInputDraftBytesV1 || (answer.Skipped && (len(answer.OptionIDs) != 0 || answer.CustomText != "")) {
			return errors.New("private draft answer is invalid")
		}
		seen[answer.QuestionID] = true
		options := map[string]bool{}
		for _, option := range answer.OptionIDs {
			index := strings.TrimPrefix(option, answer.QuestionID+"_option_")
			if (index != "1" && index != "2" && index != "3") || option != answer.QuestionID+"_option_"+index || options[option] {
				return errors.New("private draft option is invalid")
			}
			options[option] = true
		}
	}
	body, err := json.Marshal(snapshot)
	if err != nil || len(body) > MaxUserInputDraftBytesV1 {
		return errors.New("private draft snapshot exceeds its size limit")
	}
	return nil
}

func validateUserInputDraftPayloadV1(payload Payload) error {
	link := payload.UserInputDraft
	if link == nil || payload.Version != ContractVersionV3 || payload.Kind != KindUserInputDraft || !validText(payload.ThreadID, true) || !validText(payload.TurnID, true) || !validText(payload.CallID, true) || !domainsecurity.IsSHA256Hex(link.SourceReceiptID) || !domainsecurity.IsSHA256Hex(link.CaptureSetSHA256) || !IsUserInputDraftRevisionV1(link.Revision) || (link.ParentRevision != nil && (!IsUserInputDraftRevisionV1(*link.ParentRevision) || *link.ParentRevision == link.Revision)) || ValidateUserInputDraftSnapshotV1(link.Snapshot, link.SourceInputID) != nil || payload.GateID != UserInputDraftSlotGateIDV1(link.SourceReceiptID, link.ParentRevision) || payload.ItemID != "item_"+payload.GateID {
		return errors.New("private draft receipt linkage is invalid")
	}
	at, err := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != payload.IssuedAt {
		return errors.New("private draft receipt time is invalid")
	}
	// A draft has no copied tool arguments, provider route, security context or
	// execution fields. Only its source's signed receipt can supply those facts.
	closed := Payload{Version: payload.Version, Kind: payload.Kind, GateID: payload.GateID, ThreadID: payload.ThreadID, TurnID: payload.TurnID, ItemID: payload.ItemID, CallID: payload.CallID, IssuedAt: payload.IssuedAt, UserInputDraft: link}
	if !reflect.DeepEqual(payload, closed) {
		return errors.New("private draft receipt carries unsupported authority")
	}
	return nil
}

func ValidateUserInputDraftSourceV1(draft, source Receipt) error {
	link := draft.Payload.UserInputDraft
	if ValidateReceipt(draft) != nil || ValidateReceipt(source) != nil || link == nil || source.Payload.Kind != KindUserInput || source.Payload.UserInputReplyCapture != nil || source.Payload.UserInputDraft != nil || link.SourceInputID != source.Payload.GateID || link.SourceReceiptID != source.ReceiptID || draft.Payload.ThreadID != source.Payload.ThreadID || draft.Payload.TurnID != source.Payload.TurnID || draft.Payload.CallID != source.Payload.CallID {
		return errors.New("private draft has no exact original request authority")
	}
	savedAt, savedErr := time.Parse(time.RFC3339Nano, draft.Payload.IssuedAt)
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, source.Payload.IssuedAt)
	if savedErr != nil || issuedErr != nil || savedAt.Before(issuedAt) {
		return errors.New("private draft predates its request")
	}
	return nil
}

func NewUserInputDraftReceiptV1(source Receipt, revision string, parent *string, snapshot UserInputDraftSnapshotV1, captureSet string, at time.Time, keyID string, publicKey []byte, sign SignFunc) (Receipt, error) {
	if ValidateReceipt(source) != nil || source.Payload.Kind != KindUserInput || source.Payload.UserInputReplyCapture != nil || source.Payload.UserInputDraft != nil || at.IsZero() {
		return Receipt{}, errors.New("private draft source is invalid")
	}
	var copiedParent *string
	if parent != nil {
		value := *parent
		copiedParent = &value
	}
	link := &UserInputDraftV1{CaptureSetSHA256: captureSet, SourceInputID: source.Payload.GateID, SourceReceiptID: source.ReceiptID, Revision: revision, ParentRevision: copiedParent, Snapshot: CloneUserInputDraftSnapshotV1(snapshot)}
	gateID := UserInputDraftSlotGateIDV1(source.ReceiptID, parent)
	payload := Payload{Version: ContractVersionV3, Kind: KindUserInputDraft, GateID: gateID, ItemID: "item_" + gateID, ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, CallID: source.Payload.CallID, IssuedAt: at.UTC().Format(time.RFC3339Nano), UserInputDraft: link}
	if validateUserInputDraftPayloadV1(payload) != nil {
		return Receipt{}, errors.New("private draft is invalid")
	}
	receipt, err := signReceipt(payload, keyID, publicKey, sign)
	if err != nil || ValidateUserInputDraftSourceV1(receipt, source) != nil {
		return Receipt{}, errors.New("private draft signing failed")
	}
	return receipt, nil
}
