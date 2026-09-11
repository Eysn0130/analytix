package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"unicode/utf8"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

var (
	ErrToolExecutionObservationInvalid   = errors.New("tool execution observation request is invalid")
	ErrToolExecutionNotObserved          = errors.New("successful tool execution was not observed")
	ErrToolExecutionObservationAmbiguous = errors.New("tool execution observation is ambiguous")
)

const maxSuccessfulToolExecutionObservationReadPathBytesV1 = 4096

// SuccessfulToolExecutionObservationInputV1 binds a caller's expected
// ordinary read or foreground bash request to one frozen thread workspace.
// Arguments and workspace are used only for host-side comparison and never
// enter the returned projection. SemanticIdentity and ReadPathResolved are
// host-derived adapter inputs, never caller-controlled wire fields.
type SuccessfulToolExecutionObservationInputV1 struct {
	ThreadID          string
	TurnID            string
	ToolName          string
	ExpectedWorkspace string
	Arguments         json.RawMessage
	SemanticIdentity  domainsideeffectidentity.IdentityV1
	ReadPathResolved  bool
}

// ObserveSuccessfulToolExecutionV1 derives the same semantic effect identity
// used at dispatch, then proves a signed completed pending-work receipt, its
// exact settled grant, and its successful metadata-only durable tool result.
// It is deliberately limited to ordinary read and foreground bash effects
// admitted by a V2 turn context, so a case-sensitive thread can prove that its
// permanent ordinary capability base stayed available without this seam
// becoming case-fact or protected-tool authority.
func (service *Service) ObserveSuccessfulToolExecutionV1(
	ctx context.Context,
	input SuccessfulToolExecutionObservationInputV1,
) (domainpendingwork.SuccessfulToolExecutionObservationV1, error) {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	toolName := strings.TrimSpace(input.ToolName)
	expectedWorkspace := strings.TrimSpace(input.ExpectedWorkspace)
	if service == nil || !service.Available() || threadID == "" || threadID != input.ThreadID ||
		turnID == "" || turnID != input.TurnID || !toolExecutionObservationToolAllowedV1(toolName) || toolName != input.ToolName ||
		expectedWorkspace == "" || expectedWorkspace != input.ExpectedWorkspace ||
		len(input.Arguments) == 0 {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
	}
	arguments, err := domainsecurity.DecodeCanonicalJSONObject(input.Arguments)
	if err != nil || (toolName == "bash" && backgroundBashRequestedV1(arguments)) ||
		(toolName == "read" && (!input.ReadPathResolved || !validReadExpectationArgumentsV1(arguments))) {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
	}
	if toolName == "bash" && (domainsideeffectidentity.ValidateV1(input.SemanticIdentity) != nil ||
		input.SemanticIdentity.ToolName != "bash") {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
	}
	if toolName == "read" && input.SemanticIdentity != (domainsideeffectidentity.IdentityV1{}) {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
	}
	argumentHash := domainsecurity.CanonicalJSONHash(input.Arguments)
	if argumentHash == "" {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
	}

	thread, err := service.grants.GetThread(threadID)
	if err != nil || thread == nil || mapString(thread, "id") != threadID {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionNotObserved
	}
	turn, found := appmodel.TurnByID(thread, turnID)
	if !found {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionNotObserved
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || securityContext.ThreadID != threadID || securityContext.TurnID != turnID ||
		securityContext.WorkspaceRealPath != expectedWorkspace ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionNotObserved
	}
	expectedPayloadHash := ""
	if toolName == "bash" {
		payload, payloadErr := sideEffectIntentPayloadV1(input.SemanticIdentity)
		if payloadErr != nil {
			return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationInvalid
		}
		expectedPayloadHash, err = service.KeyedPayloadHash(ctx, sideEffectIntentPayloadPurpose, payload)
		if err != nil {
			return domainpendingwork.SuccessfulToolExecutionObservationV1{}, err
		}
	}

	inventory, err := service.TrustedInventoryV1(ctx)
	if err != nil {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, err
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionNotObserved
	}

	matches := make([]domainpendingwork.SuccessfulToolExecutionObservationV1, 0, 1)
	for _, receipt := range inventory.Receipts {
		if !receiptMatchesContext(receipt, securityContext) || !toolExecutionReceiptMatchesV1(receipt, toolName, expectedPayloadHash) {
			continue
		}
		disposition, closed := inventory.Dispositions[receipt.WorkID]
		if !closed || disposition.Status != domainpendingwork.StatusCompleted ||
			(toolName == "bash" && disposition.ReasonCode != "tool_outcome_durable") ||
			(toolName == "read" && disposition.ReasonCode != "batch_completed") {
			continue
		}
		for _, member := range receipt.GrantMembers {
			entry, present := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
			if !present || !toolExecutionGrantMatchesV1(
				securityContext, receipt, member, entry, thread, toolName, argumentHash, input.Arguments,
			) {
				continue
			}
			resultItemID, resultItem, present := exactDurableResultItem(thread, securityContext, entry.Grant)
			if !present || !successfulMetadataOnlyToolResultV1(resultItem) {
				continue
			}
			durable, settlementErr := executiongrantapp.DurableSettlementFromThread(
				threadID, thread, turnID, resultItemID, entry.Grant,
			)
			resultDigest := canonicalRecordHash(resultItem)
			if settlementErr != nil || resultDigest == "" || canonicalRecordHash(durable.ResultItem) != resultDigest {
				continue
			}
			observation := domainpendingwork.SuccessfulToolExecutionObservationV1{
				SchemaVersion: domainpendingwork.SuccessfulToolExecutionObservationSchemaVersionV1,
				Disclosure:    domainpendingwork.ToolExecutionObservationDisclosureV1, PrivatePayloadWithheld: true,
				ThreadID: threadID, TurnID: turnID, ToolName: toolName, Status: domainpendingwork.StatusCompleted,
				WorkID: receipt.WorkID, ReceiptID: receipt.ReceiptID, DispositionID: disposition.DispositionID,
				ExecutionGrantID: entry.Grant.GrantID, ResultItemID: resultItemID, ResultItemDigest: resultDigest,
			}
			if domainpendingwork.ValidateSuccessfulToolExecutionObservationV1(observation) != nil {
				return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationAmbiguous
			}
			matches = append(matches, observation)
		}
	}
	if len(matches) == 0 {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionNotObserved
	}
	if len(matches) != 1 {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, ErrToolExecutionObservationAmbiguous
	}
	return matches[0], nil
}

func toolExecutionObservationToolAllowedV1(toolName string) bool {
	return toolName == "bash" || toolName == "read"
}

func toolExecutionReceiptMatchesV1(
	receipt domainpendingwork.PendingWorkReceiptV1,
	toolName string,
	expectedPayloadHash string,
) bool {
	switch toolName {
	case "bash":
		return receipt.Kind == domainpendingwork.KindSideEffectIntent && len(receipt.GrantMembers) == 1 &&
			receipt.PayloadHash == expectedPayloadHash
	case "read":
		return receipt.Kind == domainpendingwork.KindToolBatch && len(receipt.GrantMembers) > 0 &&
			receipt.RouteHash == domainsecurity.SHA256Hex([]byte(toolBatchRouteIdentity))
	default:
		return false
	}
}

func toolExecutionGrantMatchesV1(
	securityContext domainsecurity.TurnSecurityContext,
	receipt domainpendingwork.PendingWorkReceiptV1,
	member domainpendingwork.GrantMemberV1,
	entry domainsecurity.ExecutionGrantRegistryEntry,
	thread map[string]any,
	toolName string,
	argumentHash string,
	arguments json.RawMessage,
) bool {
	expectedReadOnly := toolName == "read"
	if entry.Status != domainsecurity.GrantRegistrySettled || entry.Sequence != member.RegistrySequence ||
		entry.Grant.GrantID != member.GrantID ||
		entry.Grant.ToolName != toolName || entry.Grant.ServerIdentity != "host:builtin" || entry.Grant.ConnectionEpoch != 0 ||
		entry.Grant.ReadOnly != expectedReadOnly || entry.Grant.ArgsHash != argumentHash ||
		executiongrantapp.CallUsesCaseDataAuthority(domainmodel.ToolCall{Name: toolName}) {
		return false
	}
	call := domainmodel.ToolCall{ID: entry.Grant.ToolCallID, Name: toolName, Arguments: arguments}
	if executiongrantapp.ValidateExecutionGrantForCall(securityContext, entry.Grant, call) != nil ||
		!receiptMemberMatchesIssuedRegistryV1(receipt, thread, member, entry.Grant) {
		return false
	}
	if toolName == "bash" {
		routeHash, err := sideEffectIntentRouteHashV1(entry.Grant)
		return err == nil && receipt.RouteHash == routeHash
	}
	return true
}

func receiptMemberMatchesIssuedRegistryV1(
	receipt domainpendingwork.PendingWorkReceiptV1,
	thread map[string]any,
	member domainpendingwork.GrantMemberV1,
	grant domainsecurity.ExecutionGrant,
) bool {
	prefix, err := executiongrantapp.RegistrySnapshotFromThread(
		receipt.Context.ThreadID,
		thread,
		receipt.Context.TurnID,
		receipt.GrantRegistrySequence,
		receipt.GrantRegistryDigest,
	)
	if err != nil || member.RegistrySequence == 0 || member.RegistrySequence > uint64(len(prefix.Entries)) {
		return false
	}
	issued := prefix.Entries[member.RegistrySequence-1]
	return issued.Grant == grant && issued.Grant.GrantID == member.GrantID &&
		issued.EntryDigest == member.RegistryEntryDigest && issued.Status == domainsecurity.GrantRegistryActive
}

func successfulMetadataOnlyToolResultV1(resultItem map[string]any) bool {
	if mapString(resultItem, "status") != domainpendingwork.StatusCompleted || mapBool(resultItem, "isError") {
		return false
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(resultItem["output"])
	return err == nil && projection.ProjectionKind == domaintoolresult.ProjectionHostStatus &&
		projection.Status == domainpendingwork.StatusCompleted && projection.MessageKey == "tool_completed" &&
		projection.Code == "tool_completed" && projection.PrivatePayloadWithheld &&
		!projection.FactAnswerAllowed && !projection.EvidenceAuthority
}

func validReadExpectationArgumentsV1(arguments map[string]any) bool {
	if len(arguments) == 0 || len(arguments) > 3 {
		return false
	}
	for key := range arguments {
		if key != "path" && key != "offset" && key != "limit" {
			return false
		}
	}
	path, ok := arguments["path"].(string)
	if !ok || !ValidSuccessfulToolExecutionObservationReadPathV1(path) {
		return false
	}
	for _, key := range []string{"offset", "limit"} {
		value, present := arguments[key]
		if !present {
			continue
		}
		number, ok := value.(float64)
		if !ok || number < 1 || number != float64(int64(number)) {
			return false
		}
	}
	return true
}

// ValidSuccessfulToolExecutionObservationReadPathV1 limits this metadata-only
// release-evidence seam to one canonical workspace-relative slash path. The
// inbound adapter must additionally resolve that path under the exact frozen
// workspace; the service then binds the unchanged arguments to the settled
// execution grant.
func ValidSuccessfulToolExecutionObservationReadPathV1(value string) bool {
	if value == "" || value != strings.TrimSpace(value) ||
		len(value) > maxSuccessfulToolExecutionObservationReadPathBytesV1 ||
		!utf8.ValidString(value) || strings.ContainsRune(value, '\x00') ||
		strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return false
	}
	cleaned := path.Clean(value)
	return cleaned == value && cleaned != "." && cleaned != ".." &&
		!strings.HasPrefix(cleaned, "../")
}

func backgroundBashRequestedV1(arguments map[string]any) bool {
	for _, key := range []string{"run_in_background", "runInBackground"} {
		value, present := arguments[key]
		if !present {
			continue
		}
		flag, valid := value.(bool)
		if !valid || flag {
			return true
		}
	}
	return false
}

func sideEffectIntentPayloadV1(identity domainsideeffectidentity.IdentityV1) ([]byte, error) {
	if domainsideeffectidentity.ValidateV1(identity) != nil {
		return nil, ErrOperationMismatch
	}
	payload := struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		ToolName      string `json:"toolName"`
		ArgsHash      string `json:"argsHash"`
	}{identity.SchemaVersion, domainpendingwork.KindSideEffectIntent, identity.ToolName, identity.ArgsHash}
	return strictCanonicalPendingWorkValue(payload)
}

func sideEffectIntentRouteHashV1(grant domainsecurity.ExecutionGrant) (string, error) {
	route := struct {
		SchemaVersion   int    `json:"schemaVersion"`
		Kind            string `json:"kind"`
		Provider        string `json:"provider"`
		ServerIdentity  string `json:"serverIdentity"`
		ConnectionEpoch uint64 `json:"connectionEpoch"`
		ToolName        string `json:"toolName"`
		SchemaHash      string `json:"schemaHash"`
		ScopeHash       string `json:"scopeHash"`
		ApprovalState   string `json:"approvalState"`
	}{
		sideEffectIntentRouteVersion, domainpendingwork.KindSideEffectIntent, grant.Provider,
		grant.ServerIdentity, grant.ConnectionEpoch, grant.ToolName, grant.SchemaHash, grant.ScopeHash, grant.ApprovalState,
	}
	routeBytes, err := strictCanonicalPendingWorkValue(route)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(routeBytes), nil
}
