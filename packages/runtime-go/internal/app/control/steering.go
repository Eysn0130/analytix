package control

import (
	"errors"
	"strings"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

const (
	SteerBlockerCaseRiskRaise       = "case_risk_raise"
	SteerBlockerContextChangingData = "context_changing_input"
)

type SteerSecurityAdmissionInput struct {
	Context           domainsecurity.TurnSecurityContext
	RiskIntent        string
	LexicalCaseRisk   bool
	ProtectedCaseData bool
	AttachmentCount   int
	FileRefCount      int
}

type SteerSecurityAdmission struct {
	RequiresNewTurn bool
	BlockerCode     string
}

// EvaluateSteerSecurityAdmission prevents an untrusted mid-turn input from
// changing the authority frozen for an already admitted turn. Attachments and
// file references are always new context. Case text is admissible only when
// the host already froze an executable case context for this turn.
func EvaluateSteerSecurityAdmission(input SteerSecurityAdmissionInput) (SteerSecurityAdmission, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(input.Context); err != nil {
		return SteerSecurityAdmission{}, errors.New("steer security context is invalid")
	}
	if input.AttachmentCount < 0 || input.FileRefCount < 0 {
		return SteerSecurityAdmission{}, errors.New("steer context input count is invalid")
	}
	if input.AttachmentCount > 0 || input.FileRefCount > 0 {
		return SteerSecurityAdmission{RequiresNewTurn: true, BlockerCode: SteerBlockerContextChangingData}, nil
	}
	caseRisk := NormalizeRiskIntent(input.RiskIntent) == domainsecurity.RiskClassCase || input.LexicalCaseRisk || input.ProtectedCaseData
	if caseRisk && !domainsecurity.TurnSecurityContextAllowsCaseEvidence(input.Context) {
		return SteerSecurityAdmission{RequiresNewTurn: true, BlockerCode: SteerBlockerCaseRiskRaise}, nil
	}
	return SteerSecurityAdmission{}, nil
}

// SteeringLogicalEffectBindingV1 binds raw host intent before privacy
// projection. The resulting effect and ordinary-work bit are the only values
// persisted with the projected steering entry.
func SteeringLogicalEffectBindingV1(rawText string, admission SteerSecurityAdmissionInput) domainsteering.EntryLogicalEffectBinding {
	ordinaryWork := apploop.IndependentOrdinaryPromptV1(rawText) != ""
	boundFundsFollowup := domainsecurity.TurnSecurityContextIsCaseSensitive(admission.Context) &&
		apploop.PromptLooksLikeBoundCaseFundFollowupV1(rawText)
	if apploop.PromptExplicitlyRequestsCaseFundAnalysis(rawText) || boundFundsFollowup {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectFundsData,
			OrdinaryWork:  ordinaryWork,
		}
	}
	if apploop.PromptLooksLikeLocalFilesystemTask(rawText) &&
		!admission.ProtectedCaseData && !apploop.PromptRequiresCaseRiskAdmission(rawText) {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork:  true,
		}
	}
	caseRisk := NormalizeRiskIntent(admission.RiskIntent) == domainsecurity.RiskClassCase ||
		admission.LexicalCaseRisk || admission.ProtectedCaseData
	if caseRisk {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectCaseData,
			OrdinaryWork:  ordinaryWork,
		}
	}
	return domainsteering.EntryLogicalEffectBinding{
		LogicalEffect: domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork:  true,
	}
}

// TaskJobSteerLogicalEffectBindingV1 applies the same host classifier to a
// child turn, retaining a bound parent funds follow-up while that child has not
// yet frozen its own security context.
func TaskJobSteerLogicalEffectBindingV1(
	rawText string,
	frozen domainsecurity.TurnSecurityContext,
	childTurnStarted bool,
	parentCaseBound bool,
	lexicalCaseRisk bool,
	protectedCaseData bool,
) domainsteering.EntryLogicalEffectBinding {
	admissionContext := domainsecurity.TurnSecurityContext{}
	if childTurnStarted {
		admissionContext = frozen
	}
	binding := SteeringLogicalEffectBindingV1(rawText, SteerSecurityAdmissionInput{
		Context: admissionContext, LexicalCaseRisk: lexicalCaseRisk, ProtectedCaseData: protectedCaseData,
	})
	if !childTurnStarted && parentCaseBound && apploop.PromptLooksLikeBoundCaseFundFollowupV1(rawText) {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectFundsData,
			OrdinaryWork:  apploop.IndependentOrdinaryPromptV1(rawText) != "",
		}
	}
	return binding
}

// StricterTaskJobSteerLogicalEffectBindingV1 prevents a valid queue-time
// binding from lowering the risk discovered under the child's later frozen
// context. Independent ordinary work survives only when both classifications
// agree it is present.
func StricterTaskJobSteerLogicalEffectBindingV1(
	current domainsteering.EntryLogicalEffectBinding,
	queued domainsteering.EntryLogicalEffectBinding,
) domainsteering.EntryLogicalEffectBinding {
	effect := current.LogicalEffect
	if steeringLogicalEffectRankV1(queued.LogicalEffect) > steeringLogicalEffectRankV1(effect) {
		effect = queued.LogicalEffect
	}
	ordinaryWork := current.OrdinaryWork && queued.OrdinaryWork
	if effect == domainsecurity.LogicalEffectOrdinary {
		ordinaryWork = true
	}
	return domainsteering.EntryLogicalEffectBinding{LogicalEffect: effect, OrdinaryWork: ordinaryWork}
}

func steeringLogicalEffectRankV1(effect domainsecurity.LogicalEffect) int {
	switch effect {
	case domainsecurity.LogicalEffectFundsData:
		return 3
	case domainsecurity.LogicalEffectCaseData:
		return 2
	case domainsecurity.LogicalEffectOrdinary:
		return 1
	default:
		return 0
	}
}

func NewTurnRequiredResponse(threadID, turnID, blockerCode string) map[string]any {
	return map[string]any{
		"code":        "new_turn_required",
		"message":     "This input requires a newly admitted turn under current host security authority.",
		"threadId":    strings.TrimSpace(threadID),
		"turnId":      strings.TrimSpace(turnID),
		"blockerCode": strings.TrimSpace(blockerCode),
	}
}

type SteeringEntryInput struct {
	TurnID              string
	Text                string
	DisplayText         string
	ClientUserMessageID string
	AttachmentIDs       []string
	FileReferences      []any
	EffectBinding       domainsteering.EntryLogicalEffectBinding
	Now                 time.Time
}

type TurnSteeredEventInput struct {
	ThreadID            string
	TurnID              string
	Text                string
	DisplayText         string
	ClientUserMessageID string
}

type SteeringStore interface {
	AdmitSteeringEntryForContext(threadID, turnID, expectedTurnID, expectedContextDigest string, entry map[string]any) (map[string]any, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type AdmitSteeringInput struct {
	Store                 SteeringStore
	Request               SteerTurnRequest
	ExpectedContextDigest string
	EffectBinding         domainsteering.EntryLogicalEffectBinding
	Now                   time.Time
}

func ValidateSteeringEffectBinding(binding domainsteering.EntryLogicalEffectBinding) error {
	if err := domainsecurity.ValidateLogicalEffect(binding.LogicalEffect); err != nil {
		return errors.New("steering logical effect binding is invalid")
	}
	if binding.LogicalEffect == domainsecurity.LogicalEffectOrdinary && !binding.OrdinaryWork {
		return errors.New("ordinary steering input must retain ordinary work")
	}
	return nil
}

func BuildSteeringEntry(input SteeringEntryInput) (string, map[string]any, error) {
	if err := ValidateSteeringEffectBinding(input.EffectBinding); err != nil {
		return "", nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	text := strings.TrimSpace(input.Text)
	clientUserMessageID := strings.TrimSpace(input.ClientUserMessageID)
	if !IsOpaqueClientUserMessageID(clientUserMessageID) {
		clientUserMessageID = newOpaqueClientUserMessageID(input.TurnID, now)
	}
	entry := map[string]any{
		"id":                  domainsteering.EntryIDV1(input.TurnID, clientUserMessageID),
		"clientUserMessageId": clientUserMessageID,
		"text":                text,
		"admittedAt":          now.UTC().Format(time.RFC3339Nano),
		"delivery":            "steer",
		"logicalEffect":       string(input.EffectBinding.LogicalEffect),
		"ordinaryWork":        input.EffectBinding.OrdinaryWork,
	}
	if displayText := strings.TrimSpace(input.DisplayText); displayText != "" && displayText != text {
		entry["displayText"] = displayText
	}
	if len(input.AttachmentIDs) > 0 {
		entry["attachmentIds"] = append([]string(nil), input.AttachmentIDs...)
	}
	if len(input.FileReferences) > 0 {
		entry["fileReferences"] = contracts.CloneValue(input.FileReferences)
	}
	return clientUserMessageID, entry, nil
}

func BuildTurnSteeredEvent(input TurnSteeredEventInput) map[string]any {
	return map[string]any{
		"kind":                "turn_steered",
		"threadId":            strings.TrimSpace(input.ThreadID),
		"turnId":              strings.TrimSpace(input.TurnID),
		"text":                strings.TrimSpace(input.Text),
		"displayText":         strings.TrimSpace(input.DisplayText),
		"clientUserMessageId": strings.TrimSpace(input.ClientUserMessageID),
	}
}

func SteeringAcceptedResponse(threadID, turnID, clientUserMessageID string, seq any) map[string]any {
	response := map[string]any{
		"ok":                  true,
		"threadId":            strings.TrimSpace(threadID),
		"turnId":              strings.TrimSpace(turnID),
		"clientUserMessageId": strings.TrimSpace(clientUserMessageID),
	}
	if value, ok := NumericAny(seq); ok {
		response["admittedSeq"] = float64(value)
	}
	return response
}

func AdmitSteeringTurn(input AdmitSteeringInput) (map[string]any, error) {
	if input.Store == nil {
		return nil, errors.New("steering store is required")
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(input.ExpectedContextDigest)) {
		return nil, errors.New("steering context digest is invalid")
	}
	request := input.Request
	clientUserMessageID := strings.TrimSpace(request.ClientUserMessageID)
	if clientUserMessageID != "" && !IsOpaqueClientUserMessageID(clientUserMessageID) {
		return nil, ErrInvalidSteerClientID
	}
	clientUserMessageID, entry, err := BuildSteeringEntry(SteeringEntryInput{
		TurnID:              request.TurnID,
		Text:                request.Text,
		DisplayText:         request.DisplayText,
		ClientUserMessageID: clientUserMessageID,
		AttachmentIDs:       request.AttachmentIDs,
		FileReferences:      request.FileReferences,
		EffectBinding:       input.EffectBinding,
		Now:                 input.Now,
	})
	if err != nil {
		return nil, err
	}
	admitted, err := input.Store.AdmitSteeringEntryForContext(
		request.ThreadID, request.TurnID, strings.TrimSpace(request.ExpectedTurnID), strings.TrimSpace(input.ExpectedContextDigest), entry,
	)
	if err != nil {
		return nil, err
	}
	event, _, err := input.Store.RecordEvent(BuildTurnSteeredEvent(TurnSteeredEventInput{
		ThreadID:            request.ThreadID,
		TurnID:              request.TurnID,
		Text:                request.Text,
		DisplayText:         stringField(admitted, "displayText"),
		ClientUserMessageID: clientUserMessageID,
	}))
	if err != nil {
		return nil, err
	}
	return SteeringAcceptedResponse(request.ThreadID, request.TurnID, clientUserMessageID, event["seq"]), nil
}

// IsOpaqueClientUserMessageID accepts only canonical UUIDv4 correlation IDs.
// Client-provided identifiers are public metadata, so free-form strings must
// never become a side channel for account numbers or other private content.
func IsOpaqueClientUserMessageID(value string) bool {
	return contracts.IsCanonicalOpaqueUUIDV4(value)
}

func newOpaqueClientUserMessageID(turnID string, now time.Time) string {
	return contracts.NewOpaqueUUIDV4("analytix/steering-client-message-id/v1", contracts.SafeRecordID(turnID), now)
}
