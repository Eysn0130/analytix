package jobs

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func validateDurableChildRunRecord(ctx context.Context, record Record, verifier ChildCompletionReceiptVerifier) error {
	if err := domainjob.ValidateStatusV1(record.Status); err != nil {
		return err
	}
	if legacyTypeScriptTombstoneCandidateV1(record) {
		return validateLegacyTypeScriptTombstoneV1(record)
	}
	if err := domainmodel.ValidateReasoningEffortV1(record.Effort); err != nil {
		return err
	}
	if err := domainjob.ValidatePersistedOperationalStatusesV1(record); err != nil {
		return err
	}
	if err := validateDurableSteerMessages(record); err != nil {
		return err
	}
	if err := validateDurablePauseRequests(record); err != nil {
		return err
	}
	if !domainjob.ValidFailureCode(record.FailureCode) {
		return errors.New("child-run failure code is invalid")
	}
	if strings.TrimSpace(record.Error) != "" {
		return errors.New("child-run error text cannot be written to the job record")
	}
	if record.SecurityBinding == nil {
		if record.ChildCompletionReceipt != nil || record.ForegroundChildHandoffReceipt != nil {
			return errors.New("unbound child run cannot carry a completion receipt")
		}
		return nil
	}
	if err := domainjob.ValidateSecurityBinding(record.SecurityBinding); err != nil ||
		record.SecurityBinding.ParentThreadID != strings.TrimSpace(record.ParentThreadID) ||
		record.SecurityBinding.ParentTurnID != strings.TrimSpace(record.ParentTurnID) ||
		record.SecurityBinding.ParentToolCallID != strings.TrimSpace(record.ParentToolCallID) {
		return errors.New("child-run record security binding is invalid")
	}
	subagent := strings.TrimSpace(record.Kind) == "subagent"
	if subagent && record.Output != "" {
		return errors.New("security-bound child output cannot be written")
	}
	if subagent && record.Usage != nil {
		return errors.New("security-bound child usage cannot be written to the job record")
	}
	caseBound := subagent && record.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID
	if caseBound {
		if err := domainjob.ValidateExecutableCaseDelegationV1(
			record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
		); err != nil {
			return err
		}
	} else if record.CaseDelegation != nil {
		return errors.New("durable case delegation is outside a case-bound child")
	}
	foregroundHandoff := subagent && len(record.ToolScope) == 1 && strings.TrimSpace(record.ToolScope[0]) == "submit_child_result"
	if caseBound && strings.TrimSpace(record.Status) == string(domainjob.StatusCompleted) && record.ChildCompletionReceipt == nil {
		return errors.New("case-bound child completion requires a trusted completion receipt")
	}
	if foregroundHandoff && strings.TrimSpace(record.Status) == string(domainjob.StatusCompleted) && record.ForegroundChildHandoffReceipt == nil {
		return errors.New("foreground child completion requires a handoff receipt")
	}
	if record.ForegroundChildHandoffReceipt != nil {
		caseReceipt := record.ForegroundChildHandoffReceipt.CaseBinding != nil
		if !foregroundHandoff || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) || caseBound != caseReceipt ||
			(caseBound && record.ChildCompletionReceipt == nil) || (!caseBound && record.ChildCompletionReceipt != nil) ||
			validateForegroundChildHandoffReceiptForRecord(record, *record.ForegroundChildHandoffReceipt) != nil {
			return errors.New("foreground child handoff receipt is attached to an invalid durable state")
		}
		// V1 is audit-only. Restart never turns this durable value back into a
		// parent continuation capability; live consumption also requires the
		// process-local nonce registry.
		if !caseBound {
			return nil
		}
	}
	if record.ChildCompletionReceipt == nil {
		return nil
	}
	if !caseBound || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) {
		return errors.New("child completion receipt is attached to an invalid durable state")
	}
	if err := validateChildCompletionReceiptForRecord(record, *record.ChildCompletionReceipt, record.ChildTurnID); err != nil {
		return err
	}
	if verifier == nil {
		return errors.New("trusted child completion verifier is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := verifier.VerifyStoredChildCompletion(ctx, record); err != nil {
		return errors.Join(errors.New("child-run completion receipt is not host-trusted"), err)
	}
	return nil
}

func validateForegroundChildHandoffReceiptForRecord(record Record, receipt domainjob.ForegroundChildHandoffReceiptV1) error {
	if domainjob.ValidateForegroundChildHandoffReceiptV1(receipt) != nil || record.SecurityBinding == nil ||
		receipt.ParentThreadID != strings.TrimSpace(record.ParentThreadID) ||
		receipt.ParentTurnID != strings.TrimSpace(record.ParentTurnID) || receipt.ParentRunID != strings.TrimSpace(record.ParentTurnID) ||
		receipt.ChildThreadID != strings.TrimSpace(record.ChildThreadID) || receipt.ChildTurnID != strings.TrimSpace(record.ChildTurnID) ||
		receipt.ChildRunID != strings.TrimSpace(record.ID) || receipt.WorkspaceRealPath != strings.TrimSpace(record.Workspace) ||
		receipt.ParentContextEpoch != record.SecurityBinding.ParentContextEpoch ||
		receipt.ParentExecutionGrantID != record.SecurityBinding.ParentExecutionGrantID ||
		receipt.ParentToolCallID != record.SecurityBinding.ParentToolCallID {
		return errors.New("foreground child handoff receipt does not match the durable child run")
	}
	if receipt.CaseBinding != nil && !domainjob.ForegroundCaseHandoffBindingMatchesDurableV1(
		*receipt.CaseBinding, record.SecurityBinding, record.CaseDelegation,
		record.DelegatedToolManifest, record.ChildCompletionReceipt,
	) {
		return errors.New("foreground case handoff receipt does not match the durable child run")
	}
	return nil
}

func validateDurablePauseRequests(record Record) error {
	seen := make(map[string]struct{}, len(record.PauseRequests))
	for _, request := range record.PauseRequests {
		requestID := strings.TrimSpace(request.ID)
		if requestID == "" || len(requestID) > 256 {
			return errors.New("durable pause request identity is invalid")
		}
		if _, found := seen[requestID]; found {
			return errors.New("durable pause request identity is duplicated")
		}
		seen[requestID] = struct{}{}
		if err := domainjob.ValidatePauseRequestStatusV1(request.Status); err != nil {
			return err
		}
		if strings.TrimSpace(request.ParentThreadID) != strings.TrimSpace(record.ParentThreadID) ||
			strings.TrimSpace(request.ChildRunID) != strings.TrimSpace(record.ID) ||
			strings.TrimSpace(request.JobID) != strings.TrimSpace(record.ID) {
			return errors.New("durable pause request binding is invalid")
		}
		requestedAt, err := canonicalPauseTimeV1(request.RequestedAt)
		if err != nil {
			return errors.New("durable pause request time is invalid")
		}
		switch strings.TrimSpace(request.Status) {
		case "requested":
			if request.PausedAt != "" || request.ResumedAt != "" || request.ResumeToken != "" || request.ResumeTokenIssuedAt != "" || request.ResumeTokenExpiresAt != "" || request.RejectedReason != "" {
				return errors.New("requested durable pause carries settlement authority")
			}
		case "paused":
			if request.PausedAt == "" || request.ResumeToken == "" || request.ResumeTokenIssuedAt == "" || request.ResumeTokenExpiresAt == "" || request.ResumedAt != "" || request.RejectedReason != "" {
				return errors.New("paused durable request authority is incomplete")
			}
			if err := validateDurablePauseTokenTimesV1(request, requestedAt); err != nil {
				return err
			}
		case "resumed":
			if request.PausedAt == "" || request.ResumedAt == "" || request.ResumeToken != "" || request.ResumeTokenIssuedAt == "" || request.ResumeTokenExpiresAt == "" || request.RejectedReason != "" {
				return errors.New("resumed durable request authority is incomplete")
			}
			if err := validateDurablePauseTokenTimesV1(request, requestedAt); err != nil {
				return err
			}
			resumedAt, err := canonicalPauseTimeV1(request.ResumedAt)
			if err != nil || resumedAt.Before(requestedAt) {
				return errors.New("resumed durable request time is invalid")
			}
		case "rejected":
			if request.PausedAt != "" || request.ResumedAt != "" || request.ResumeToken != "" || request.ResumeTokenIssuedAt != "" || request.ResumeTokenExpiresAt != "" || strings.TrimSpace(request.RejectedReason) == "" {
				return errors.New("rejected durable pause authority is invalid")
			}
		case "expired":
			if request.ResumedAt != "" || request.ResumeToken != "" || strings.TrimSpace(request.RejectedReason) == "" {
				return errors.New("expired durable pause authority is invalid")
			}
			if request.PausedAt == "" {
				if request.ResumeTokenIssuedAt != "" || request.ResumeTokenExpiresAt != "" {
					return errors.New("unpaused expired request carries token metadata")
				}
			} else if err := validateDurablePauseTokenTimesV1(request, requestedAt); err != nil {
				return err
			}
		}
	}
	if status := strings.TrimSpace(record.PauseState.Status); status != "" {
		if err := domainjob.ValidatePauseRequestStatusV1(status); err != nil {
			return err
		}
	}
	expected := childRunPauseState(record)
	if !reflect.DeepEqual(record.PauseState, expected) {
		return errors.New("durable pause state does not match pause requests")
	}
	switch strings.TrimSpace(record.Status) {
	case string(domainjob.StatusPauseRequested):
		if record.PauseState.Status != "requested" {
			return errors.New("pause-requested job lacks a requested durable pause")
		}
	case string(domainjob.StatusPaused):
		if record.PauseState.Status != "paused" {
			return errors.New("paused job lacks paused durable authority")
		}
	case string(domainjob.StatusResumeRequested):
		if record.PauseState.Status != "paused" {
			return errors.New("resume-requested job lacks paused durable authority")
		}
	case string(domainjob.StatusResuming):
		if record.PauseState.Status != "resumed" {
			return errors.New("resuming job lacks resumed durable authority")
		}
	}
	return nil
}

func canonicalPauseTimeV1(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, errors.New("pause time is not canonical UTC")
	}
	return parsed, nil
}

func validateDurablePauseTokenTimesV1(request domainjob.PauseRequest, requestedAt time.Time) error {
	pausedAt, pausedErr := canonicalPauseTimeV1(request.PausedAt)
	issuedAt, issuedErr := canonicalPauseTimeV1(request.ResumeTokenIssuedAt)
	expiresAt, expiresErr := canonicalPauseTimeV1(request.ResumeTokenExpiresAt)
	if pausedErr != nil || issuedErr != nil || expiresErr != nil || pausedAt.Before(requestedAt) || !issuedAt.Equal(pausedAt) || !expiresAt.After(issuedAt) {
		return errors.New("durable pause token times are invalid")
	}
	return nil
}

func validateDurableSteerMessages(record Record) error {
	seen := make(map[string]struct{}, len(record.Steers))
	for _, message := range record.Steers {
		messageID := strings.TrimSpace(message.ID)
		if messageID == "" || len(messageID) > 256 {
			return errors.New("durable steer message identity is invalid")
		}
		if _, found := seen[messageID]; found {
			return errors.New("durable steer message identity is duplicated")
		}
		seen[messageID] = struct{}{}
		if strings.TrimSpace(message.ParentThreadID) != strings.TrimSpace(record.ParentThreadID) ||
			strings.TrimSpace(message.ChildRunID) != strings.TrimSpace(record.ID) || strings.TrimSpace(message.JobID) != strings.TrimSpace(record.ID) ||
			message.ProjectionVersion != domainjob.SteerMessageProjectionVersionV1 ||
			!domainsecurity.IsSHA256Hex(strings.TrimSpace(message.ContentDigest)) ||
			!domainsecurity.IsSHA256Hex(strings.TrimSpace(message.QueueAuthorityDigest)) {
			return errors.New("durable steer message binding is invalid")
		}
		if record.SecurityBinding != nil && !domainsecurity.IsSHA256Hex(strings.TrimSpace(message.AuthorityDigest)) {
			return errors.New("durable steer message authority is invalid")
		}
		if value := strings.TrimSpace(message.ContextDigest); value != "" && !domainsecurity.IsSHA256Hex(value) {
			return errors.New("durable steer message context is invalid")
		}
		switch strings.TrimSpace(message.Status) {
		case "queued":
			if domainjob.ValidateSteerTextProjectionV1(message.Text) != nil || message.ContentDigest != domainjob.SteerMessageContentDigestV1(message) {
				return errors.New("durable steer message content is invalid")
			}
			if message.AdmittedAt != "" || message.PromotionCommitID != "" || message.PromotionEntryID != "" {
				return errors.New("queued durable steer message carries promotion authority")
			}
		case "admitted":
			if domainjob.ValidateSteerTextProjectionV1(message.Text) != nil || message.ContentDigest != domainjob.SteerMessageContentDigestV1(message) {
				return errors.New("durable steer message content is invalid")
			}
			if message.PromotionCommitID != "" || message.PromotionEntryID != "" {
				if err := domainjob.ValidateSteerPromotionSettlementV1(domainjob.SteerPromotionSettlementV1{
					Version: domainjob.SteerPromotionSettlementVersionV1, PromotionCommitID: message.PromotionCommitID,
					PromotionEntryID: message.PromotionEntryID, ContextDigest: message.ContextDigest, PromotedAt: message.AdmittedAt,
				}, domainjob.SteerMessage{
					ID: message.ID, ParentThreadID: message.ParentThreadID, ChildRunID: message.ChildRunID, JobID: message.JobID,
					Text: message.Text, ProjectionVersion: message.ProjectionVersion, ContentDigest: message.ContentDigest,
					ContextDigest: message.ContextDigest, AuthorityDigest: message.AuthorityDigest,
					QueueAuthorityDigest: message.QueueAuthorityDigest, Status: "queued", CreatedAt: message.CreatedAt,
					SourceTurnID: message.SourceTurnID, SourceToolCallID: message.SourceToolCallID,
					LogicalEffect: message.LogicalEffect, OrdinaryWork: message.OrdinaryWork,
				}); err != nil {
					return errors.New("durable steer promotion authority is invalid")
				}
			}
		case "rejected", "expired":
			if message.Text != "" || message.SourceTurnID != "" || message.SourceToolCallID != "" ||
				message.PromotionCommitID != "" || message.PromotionEntryID != "" {
				return errors.New("settled durable steer message retained private content")
			}
		default:
			return errors.New("durable steer message status is invalid")
		}
	}
	expectedState := childRunState(record)
	if record.SteerState.PendingSteers != expectedState.PendingSteers ||
		record.SteerState.AdmittedSteers != expectedState.AdmittedSteers ||
		record.SteerState.LastSteerAt != expectedState.LastSteerAt ||
		record.SteerState.SteerCount != expectedState.SteerCount {
		return errors.New("durable steer state does not match messages")
	}
	return nil
}

// validateLiveSteerProjectionV1 runs before the live-store normalizer. A live
// caller may persist only text that was already projected by the host; unlike
// semantic startup, the live path must not silently convert untrusted text
// into a newly authoritative tombstone.
func validateLiveSteerProjectionV1(messages []domainjob.SteerMessage) error {
	for _, message := range messages {
		switch strings.TrimSpace(message.Status) {
		case "queued", "admitted":
			if err := domainjob.ValidateSteerTextProjectionV1(message.Text); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateChildCompletionReceiptForRecord(record Record, receipt domainjob.ChildCompletionReceiptV1, childTurnID string) error {
	if domainjob.ValidateChildCompletionReceiptV1(receipt) != nil || record.SecurityBinding == nil ||
		domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil || receipt.ChildRunID != strings.TrimSpace(record.ID) ||
		receipt.SecurityBindingDigest != record.SecurityBinding.BindingDigest ||
		receipt.ParentExecutionGrantID != record.SecurityBinding.ParentExecutionGrantID ||
		receipt.ParentToolCallID != record.SecurityBinding.ParentToolCallID ||
		receipt.ParentContext.ThreadID != strings.TrimSpace(record.ParentThreadID) ||
		receipt.ParentContext.TurnID != strings.TrimSpace(record.ParentTurnID) ||
		receipt.ChildContext.ThreadID != strings.TrimSpace(record.ChildThreadID) {
		return errors.New("child completion receipt does not match the durable child run")
	}
	expectedTurnID := strings.TrimSpace(childTurnID)
	if expectedTurnID == "" {
		expectedTurnID = strings.TrimSpace(record.ChildTurnID)
	}
	if expectedTurnID == "" || receipt.ChildContext.TurnID != expectedTurnID {
		return errors.New("child completion receipt turn does not match the durable child run")
	}
	if record.ChildCompletionReceipt != nil && *record.ChildCompletionReceipt != receipt {
		return errors.New("child completion receipt conflicts with the durable child run")
	}
	return nil
}

func exactChildCompletionReceiptReplay(record Record, request UpdateRequest) bool {
	if record.ChildCompletionReceipt == nil || request.ChildCompletionReceipt == nil ||
		strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) ||
		strings.TrimSpace(request.Status) != string(domainjob.StatusCompleted) ||
		*record.ChildCompletionReceipt != *request.ChildCompletionReceipt || request.Output != "" {
		return false
	}
	if value := strings.TrimSpace(request.ChildThreadID); value != "" && value != record.ChildThreadID {
		return false
	}
	if value := strings.TrimSpace(request.ChildTurnID); value != "" && value != record.ChildTurnID {
		return false
	}
	if value := strings.TrimSpace(request.Workspace); value != "" && value != record.Workspace {
		return false
	}
	if request.Usage != nil && !reflect.DeepEqual(request.Usage, record.Usage) {
		return false
	}
	if request.ToolInvocations != nil && *request.ToolInvocations != record.ToolInvocations {
		return false
	}
	remainder := request
	remainder.Status = ""
	remainder.ChildThreadID = ""
	remainder.ChildTurnID = ""
	remainder.ChildCompletionReceipt = nil
	remainder.Workspace = ""
	remainder.Output = ""
	remainder.Error = ""
	remainder.FailureCode = ""
	remainder.Usage = nil
	remainder.ToolInvocations = nil
	return reflect.DeepEqual(remainder, UpdateRequest{})
}

func exactForegroundChildHandoffReceiptReplay(record Record, request UpdateRequest) bool {
	if record.ForegroundChildHandoffReceipt == nil || request.ForegroundChildHandoffReceipt == nil ||
		strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) ||
		strings.TrimSpace(request.Status) != string(domainjob.StatusCompleted) ||
		!domainjob.ForegroundChildHandoffReceiptsEqualV1(record.ForegroundChildHandoffReceipt, request.ForegroundChildHandoffReceipt) || request.Output != "" {
		return false
	}
	if value := strings.TrimSpace(request.ChildThreadID); value != "" && value != record.ChildThreadID {
		return false
	}
	if value := strings.TrimSpace(request.ChildTurnID); value != "" && value != record.ChildTurnID {
		return false
	}
	if value := strings.TrimSpace(request.Workspace); value != "" && value != record.Workspace {
		return false
	}
	if request.Usage != nil && !reflect.DeepEqual(request.Usage, record.Usage) {
		return false
	}
	if request.ToolInvocations != nil && *request.ToolInvocations != record.ToolInvocations {
		return false
	}
	remainder := request
	remainder.Status = ""
	remainder.ChildThreadID = ""
	remainder.ChildTurnID = ""
	remainder.ForegroundChildHandoffReceipt = nil
	remainder.Workspace = ""
	remainder.Output = ""
	remainder.Error = ""
	remainder.FailureCode = ""
	remainder.Usage = nil
	remainder.ToolInvocations = nil
	return reflect.DeepEqual(remainder, UpdateRequest{})
}
