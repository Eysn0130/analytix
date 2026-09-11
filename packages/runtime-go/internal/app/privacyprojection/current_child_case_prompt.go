package privacyprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type HostChildCasePromptCurrentAttemptWitnessV1 interface {
	UseCurrentAttemptV1(func(
		string,
		domainsecurity.TurnSecurityContext,
		domainjob.Record,
		string,
		string,
		string,
		*domainjob.SecurityBinding,
		*domainjob.CaseDelegationContextV1,
	) error) error
}

type BindHostChildCasePromptCurrentAttemptInputV1 struct {
	Witness         HostChildCasePromptCurrentAttemptWitnessV1
	SecurityContext domainsecurity.TurnSecurityContext
	CurrentRecord   domainjob.Record
	Thread          map[string]any
	ThreadID        string
	TurnID          string
	UserItemID      string
	Aliases         []domaincaseentity.ModelEntityAliasV1
	Messages        *[]domainmodel.Message
}

// boundHostChildCasePromptV1 is an attempt-private provenance sidecar. Its
// concrete type and fields never cross JSON, history, compaction, or provider
// serialization; only this package's final provider projector accepts it.
type boundHostChildCasePromptV1 struct {
	context               domainsecurity.TurnSecurityContext
	recordDigest          string
	durableUserItemDigest string
	messageDigest         string
	childRunID            string
	childThreadID         string
	childTurnID           string
	userItemID            string
	userRole              string
	binding               *domainjob.SecurityBinding
	delegation            *domainjob.CaseDelegationContextV1
	aliases               []domaincaseentity.ModelEntityAliasV1
	attempt               *providerCurrentAttemptUseV1
}

// BindHostChildCasePromptCurrentAttemptV1 consumes one frozen witness and
// replaces no durable content. It binds only the exact final current user
// message after proving that the current durable item is the host's ordinary
// projection of the canonical prompt and the child record is byte-canonical
// with the record validated under the transition writer.
func BindHostChildCasePromptCurrentAttemptV1(input BindHostChildCasePromptCurrentAttemptInputV1) error {
	if input.Witness == nil || input.Messages == nil || input.Thread == nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(input.SecurityContext) != nil {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return input.Witness.UseCurrentAttemptV1(func(
		canonical string,
		frozen domainsecurity.TurnSecurityContext,
		expectedRecord domainjob.Record,
		expectedRecordDigest string,
		childRunID string,
		childThreadID string,
		binding *domainjob.SecurityBinding,
		delegation *domainjob.CaseDelegationContextV1,
	) error {
		if validateHostChildCurrentRecordTransitionV1(
			expectedRecord, input.CurrentRecord, expectedRecordDigest, input.TurnID,
		) != nil {
			return errors.New("host child prompt current record mismatch")
		}
		if frozen != input.SecurityContext || childRunID != input.CurrentRecord.ID ||
			childThreadID != input.CurrentRecord.ChildThreadID || childThreadID != input.ThreadID ||
			input.SecurityContext.ThreadID != input.ThreadID || input.SecurityContext.TurnID != input.TurnID {
			return errors.New("host child prompt current scope mismatch")
		}
		if strings.TrimSpace(input.CurrentRecord.Status) != string(domainjob.StatusRunning) ||
			input.CurrentRecord.ChildTurnID != input.TurnID || input.CurrentRecord.Background ||
			canonical != input.CurrentRecord.Prompt {
			return errors.New("host child prompt current state mismatch")
		}
		if domainjob.ValidateSecurityBinding(binding) != nil ||
			domainjob.ValidateCaseDelegationContextV1(delegation, binding) != nil ||
			!domainjob.CaseDelegationContextsEqualV1(delegation, input.CurrentRecord.CaseDelegation) ||
			input.CurrentRecord.SecurityBinding == nil ||
			input.CurrentRecord.SecurityBinding.BindingDigest != binding.BindingDigest {
			return errors.New("host child prompt current binding mismatch")
		}
		canonicalAliases, err := canonicalProviderCaseAliasesV1(input.Aliases)
		if err != nil || !sameProviderCaseAliasesV1(canonicalAliases, domainjob.CaseDelegationAliasesV1(delegation)) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		projected := ProjectOrdinaryText(canonical)
		item, itemDigest, err := exactCurrentHostChildUserItemV1(
			input.Thread, input.ThreadID, input.TurnID, input.UserItemID, projected,
		)
		if err != nil || item["role"] != "user" {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		messages := *input.Messages
		if len(messages) == 0 {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		message := &messages[len(messages)-1]
		if message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
			len(message.Parts) != 0 || len(message.ToolCalls) != 0 || message.Content != canonical ||
			message.PrivateProviderSemanticBinding != nil || message.PrivateProviderReferenceBinding != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		messageDigest, err := providerReferenceMessageSHA256V1(*message)
		if err != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		message.PrivateProviderSemanticBinding = boundHostChildCasePromptV1{
			context: frozen, recordDigest: expectedRecordDigest, durableUserItemDigest: itemDigest,
			messageDigest: messageDigest, childRunID: childRunID, childThreadID: childThreadID,
			childTurnID: input.TurnID, userItemID: input.UserItemID, userRole: "user",
			binding: domainjob.CloneSecurityBinding(binding), delegation: domainjob.CloneCaseDelegationContextV1(delegation),
			aliases: append([]domaincaseentity.ModelEntityAliasV1(nil), canonicalAliases...),
			attempt: &providerCurrentAttemptUseV1{},
		}
		*input.Messages = messages
		return nil
	})
}

func exactCurrentHostChildUserItemV1(
	thread map[string]any,
	threadID string,
	turnID string,
	itemID string,
	projectedPrompt string,
) (map[string]any, string, error) {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(turnID) == "" || strings.TrimSpace(itemID) == "" ||
		thread["id"] != threadID {
		return nil, "", ErrProviderPrivacyAuthorityUnavailable
	}
	var currentTurn map[string]any
	turnMatches := 0
	for _, rawTurn := range currentAttemptAnyListV1(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn != nil && turn["id"] == turnID {
			currentTurn = turn
			turnMatches++
		}
	}
	if turnMatches != 1 || currentTurn["threadId"] != threadID || currentTurn["prompt"] != projectedPrompt {
		return nil, "", ErrProviderPrivacyAuthorityUnavailable
	}
	var exact map[string]any
	userItems := 0
	itemMatches := 0
	for _, rawItem := range currentAttemptAnyListV1(currentTurn["items"]) {
		item, _ := rawItem.(map[string]any)
		if item == nil {
			continue
		}
		if item["kind"] == "user_message" || item["role"] == "user" {
			userItems++
		}
		if item["id"] == itemID {
			exact = item
			itemMatches++
		}
	}
	if userItems != 1 || itemMatches != 1 || exact["id"] != itemID || exact["threadId"] != threadID ||
		exact["turnId"] != turnID || exact["kind"] != "user_message" || exact["role"] != "user" ||
		exact["status"] != "completed" || exact["text"] != projectedPrompt {
		return nil, "", ErrProviderPrivacyAuthorityUnavailable
	}
	body, err := json.Marshal(exact)
	if err != nil || len(body) == 0 {
		return nil, "", ErrProviderPrivacyAuthorityUnavailable
	}
	digest := domainsecurity.SHA256Hex(body)
	if !domainsecurity.IsSHA256Hex(digest) {
		return nil, "", ErrProviderPrivacyAuthorityUnavailable
	}
	return exact, digest, nil
}

func currentAttemptAnyListV1(value any) []any {
	list, _ := value.([]any)
	return list
}

func validateHostChildCurrentRecordTransitionV1(
	expected domainjob.Record,
	current domainjob.Record,
	expectedDigest string,
	turnID string,
) error {
	expectedBytes, expectedErr := json.Marshal(expected)
	if expectedErr != nil || domainsecurity.SHA256Hex(expectedBytes) != expectedDigest ||
		expected.ChildTurnID != turnID || current.ChildTurnID != turnID || strings.TrimSpace(turnID) == "" ||
		current.LeaseOwner != expected.LeaseOwner || strings.TrimSpace(current.LeaseOwner) == "" {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	expectedHeartbeat, expectedHeartbeatErr := time.Parse(time.RFC3339Nano, expected.LastHeartbeatAt)
	currentHeartbeat, currentHeartbeatErr := time.Parse(time.RFC3339Nano, current.LastHeartbeatAt)
	currentUpdated, currentUpdatedErr := time.Parse(time.RFC3339Nano, current.UpdatedAt)
	currentExpires, currentExpiresErr := time.Parse(time.RFC3339Nano, current.LeaseExpiresAt)
	if expectedHeartbeatErr != nil || currentHeartbeatErr != nil || currentUpdatedErr != nil || currentExpiresErr != nil ||
		currentHeartbeat.Before(expectedHeartbeat) || currentUpdated.Before(currentHeartbeat) || !currentExpires.After(currentUpdated) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	normalized := current
	normalized.LastHeartbeatAt = expected.LastHeartbeatAt
	normalized.LeaseExpiresAt = expected.LeaseExpiresAt
	normalized.UpdatedAt = expected.UpdatedAt
	normalizedBytes, err := json.Marshal(normalized)
	if err != nil || !bytes.Equal(expectedBytes, normalizedBytes) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return nil
}

func validateBoundHostChildCasePromptV1(
	context domainsecurity.TurnSecurityContext,
	message domainmodel.Message,
) (validatedAccountFlowProviderSemanticV1, error) {
	binding, ok := message.PrivateProviderSemanticBinding.(boundHostChildCasePromptV1)
	if !ok || binding.context != context ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainsecurity.IsSHA256Hex(binding.recordDigest) ||
		!domainsecurity.IsSHA256Hex(binding.durableUserItemDigest) ||
		!domainsecurity.IsSHA256Hex(binding.messageDigest) ||
		binding.attempt == nil ||
		strings.TrimSpace(binding.childRunID) == "" || binding.childThreadID != context.ThreadID ||
		binding.childTurnID != context.TurnID || strings.TrimSpace(binding.userItemID) == "" ||
		binding.userRole != "user" || domainjob.ValidateSecurityBinding(binding.binding) != nil ||
		domainjob.ValidateCaseDelegationContextV1(binding.delegation, binding.binding) != nil ||
		!domainmodel.IsHostToolCallIDV1(binding.binding.ParentToolCallID) ||
		!domainjob.SecurityBindingMatchesCaseEpochScope(binding.binding, context) ||
		message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
		len(message.Parts) != 0 || len(message.ToolCalls) != 0 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonicalAliases, err := canonicalProviderCaseAliasesV1(binding.aliases)
	if err != nil || !sameProviderCaseAliasesV1(canonicalAliases, domainjob.CaseDelegationAliasesV1(binding.delegation)) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := domainjob.CaseDelegationProviderPromptV1(binding.delegation, binding.binding)
	messageDigest, digestErr := providerReferenceMessageSHA256V1(message)
	if err != nil || digestErr != nil || canonical != message.Content || messageDigest != binding.messageDigest {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	return validatedAccountFlowProviderSemanticV1{
		canonical: []byte(canonical), digest: domainsecurity.SHA256Hex([]byte(canonical)),
		references: []domaincaseentity.ReferenceV1{}, kind: validatedProviderSemanticHostChildPromptV1,
		currentAttempt: binding.attempt,
	}, nil
}
