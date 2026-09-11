package thread

import (
	"errors"
	"fmt"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var ErrCaseDerivationAdmissionAuthorityRequired = errors.New("case thread derivation requires host admission authority")

type ForkInput struct {
	Source                   map[string]any
	ForkID                   string
	ParentThreadID           string
	Now                      string
	Relation                 string
	Title                    string
	TurnID                   string
	caseDerivationAuthorized bool
}

type ResumeInput struct {
	Source                   map[string]any
	ThreadID                 string
	SessionID                string
	Now                      string
	Workspace                string
	Model                    string
	Mode                     string
	caseDerivationAuthorized bool
}

func AuthorizeCaseForkV1(input ForkInput) ForkInput {
	input.caseDerivationAuthorized = true
	return input
}

func AuthorizeCaseResumeV1(input ResumeInput) ResumeInput {
	input.caseDerivationAuthorized = true
	return input
}

func BuildFork(input ForkInput) (map[string]any, error) {
	source := input.Source
	if source == nil {
		source = map[string]any{}
	}
	relation := strings.TrimSpace(input.Relation)
	if relation == "" {
		relation = "fork"
	}
	forkID := strings.TrimSpace(input.ForkID)
	now := strings.TrimSpace(input.Now)
	parentThreadID := strings.TrimSpace(input.ParentThreadID)
	sourceCaseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(source)
	if err != nil {
		return nil, err
	}
	if sourceCaseSensitive {
		sourceThreadID := strings.TrimSpace(stringField(source, "id"))
		if !input.caseDerivationAuthorized || parentThreadID != sourceThreadID ||
			(strings.TrimSpace(input.Relation) != "" && relation != "fork") {
			return nil, ErrCaseDerivationAdmissionAuthorityRequired
		}
	}
	if !sourceCaseSensitive {
		if _, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(source); err != nil {
			return nil, err
		}
	}
	turns, _ := source["turns"].([]any)
	sourceTurns, includesLatestTurn, err := forkSourceTurns(turns, strings.TrimSpace(input.TurnID))
	if err != nil {
		return nil, err
	}
	clonedTurns := make([]any, 0, len(sourceTurns))
	for _, turnValue := range sourceTurns {
		turn, _ := turnValue.(map[string]any)
		if turn == nil {
			continue
		}
		cloned := CloneTurnForFork(turn, forkID, now, relation)
		if sourceCaseSensitive {
			cloned = isolateCaseDerivedTurn(cloned, forkID)
		}
		clonedTurns = append(clonedTurns, cloned)
	}
	title := fmt.Sprintf("%s fork", stringField(source, "title"))
	if relation == "side" {
		title = fmt.Sprintf("%s · side", stringField(source, "title"))
	}
	if requested := strings.TrimSpace(input.Title); requested != "" {
		title = requested
	}
	fork := contracts.CloneMap(source)
	fork["id"] = forkID
	fork["title"] = title
	fork["relation"] = relation
	fork["parentThreadId"] = parentThreadID
	fork["forkedFromThreadId"] = parentThreadID
	fork["forkedFromTitle"] = stringField(source, "title")
	fork["forkedFromMessageCount"] = float64(CountUserMessagesInTurns(clonedTurns))
	fork["forkedFromTurnCount"] = float64(len(clonedTurns))
	fork["forkedAt"] = now
	fork["mode"] = "agent"
	fork["status"] = "idle"
	fork["createdAt"] = now
	fork["updatedAt"] = now
	fork["turns"] = clonedTurns
	delete(fork, "goal")
	delete(fork, "securityState")
	delete(fork, "contextEpochState")
	stripDerivedExecutionAuthorityFields(fork)
	if sourceCaseSensitive {
		delete(fork, "todos")
		delete(fork, "preview")
		fork["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	} else if todos, _ := source["todos"].(map[string]any); todos != nil && includesLatestTurn {
		fork["todos"] = CloneTodoListForThread(todos, forkID, now)
	} else {
		delete(fork, "todos")
	}
	return fork, nil
}

func BuildResume(input ResumeInput) (map[string]any, error) {
	source := input.Source
	if source == nil {
		source = map[string]any{}
	}
	threadID := strings.TrimSpace(input.ThreadID)
	sessionID := strings.TrimSpace(input.SessionID)
	now := strings.TrimSpace(input.Now)
	sourceCaseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(source)
	if err != nil {
		return nil, err
	}
	if sourceCaseSensitive {
		sourceWorkspace := strings.TrimSpace(stringField(source, "workspace"))
		requestedWorkspace := strings.TrimSpace(input.Workspace)
		if !input.caseDerivationAuthorized ||
			(requestedWorkspace != "" && requestedWorkspace != sourceWorkspace) {
			return nil, ErrCaseDerivationAdmissionAuthorityRequired
		}
	}
	if !sourceCaseSensitive {
		if _, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(source); err != nil {
			return nil, err
		}
	}
	resumed := contracts.CloneMap(source)
	resumed["id"] = threadID
	resumed["title"] = fmt.Sprintf("%s resumed", stringField(source, "title"))
	if workspace := strings.TrimSpace(input.Workspace); workspace != "" {
		resumed["workspace"] = workspace
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		resumed["model"] = model
	}
	if mode := strings.TrimSpace(input.Mode); mode != "" {
		resumed["mode"] = mode
	}
	resumed["relation"] = "primary"
	resumed["forkedFromThreadId"] = sessionID
	resumed["forkedFromTitle"] = stringField(source, "title")
	resumed["forkedAt"] = now
	resumed["status"] = "idle"
	resumed["createdAt"] = now
	resumed["updatedAt"] = now
	sourceTurns, _ := source["turns"].([]any)
	clonedTurns := make([]any, 0, len(sourceTurns))
	for _, turnValue := range sourceTurns {
		turn, _ := turnValue.(map[string]any)
		if turn == nil {
			continue
		}
		cloned := CloneTurnForThread(turn, threadID, now)
		if sourceCaseSensitive {
			cloned = isolateCaseDerivedTurn(cloned, threadID)
		}
		clonedTurns = append(clonedTurns, cloned)
	}
	resumed["turns"] = clonedTurns
	resumed["forkedFromMessageCount"] = float64(CountUserMessagesInTurns(clonedTurns))
	resumed["forkedFromTurnCount"] = float64(len(clonedTurns))
	delete(resumed, "goal")
	delete(resumed, "securityState")
	delete(resumed, "contextEpochState")
	stripDerivedExecutionAuthorityFields(resumed)
	if sourceCaseSensitive {
		delete(resumed, "todos")
		delete(resumed, "preview")
		resumed["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	} else if todos, _ := source["todos"].(map[string]any); todos != nil {
		resumed["todos"] = CloneTodoListForThread(todos, threadID, now)
	}
	return resumed, nil
}

var caseDerivedUserHistoryFieldsV1 = []string{
	"id", "status", "createdAt", "finishedAt", "kind", "text", "displayText", "delivery", "clientUserMessageId",
	"attachmentIds", "fileReferences", "workspaceCheckpointId", "activeSkillIds", "injectedMemoryIds", "skillInjectionBytes",
}

func isolateCaseDerivedTurn(turn map[string]any, threadID string) map[string]any {
	isolated := contracts.CloneMap(turn)
	turnID := strings.TrimSpace(stringField(isolated, "id"))
	items, _ := isolated["items"].([]any)
	userItems := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "user_message" {
			continue
		}
		projected := selectPublicFields(item, caseDerivedUserHistoryFieldsV1)
		projected["threadId"] = threadID
		projected["turnId"] = turnID
		projected["role"] = "user"
		userItems = append(userItems, projected)
	}
	stripCaseAuthorityFields(isolated)
	// The source compaction marker is bound to the source frozen context.
	// A new derived user shell carries neither that context nor its authority.
	delete(isolated, "caseHistoryProjection")
	delete(isolated, "acceptedFinalView")
	isolated["threadId"] = threadID
	isolated["items"] = userItems
	isolated["attachmentIds"] = AttachmentIDsFromItems(userItems)
	return isolated
}

func CountTurns(thread map[string]any) int {
	turns, _ := thread["turns"].([]any)
	return len(turns)
}

func CountUserMessagesInThread(thread map[string]any) int {
	turns, _ := thread["turns"].([]any)
	return CountUserMessagesInTurns(turns)
}

func CountUserMessagesInTurns(turns []any) int {
	count := 0
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, itemValue := range items {
			item, _ := itemValue.(map[string]any)
			if stringField(item, "kind") == "user_message" {
				count++
			}
		}
	}
	return count
}

func CloneTurnForFork(turn map[string]any, threadID string, now string, relation string) map[string]any {
	if relation == "side" && isTurnInFlight(turn) {
		cloned := contracts.CloneMap(turn)
		cloned["threadId"] = threadID
		cloned["status"] = "aborted"
		if strings.TrimSpace(stringField(cloned, "finishedAt")) == "" {
			cloned["finishedAt"] = now
		}
		if userMessage := firstUserMessageItem(turn); userMessage != nil {
			clonedUser := CloneItemForThread(userMessage, threadID, now)
			stripCaseAuthorityFields(clonedUser)
			cloned["items"] = []any{clonedUser}
			if len(listAny(cloned["attachmentIds"])) == 0 {
				cloned["attachmentIds"] = AttachmentIDsFromItems([]any{clonedUser})
			}
		} else {
			cloned["items"] = []any{}
			if _, ok := cloned["attachmentIds"]; !ok {
				cloned["attachmentIds"] = []any{}
			}
		}
		stripCaseAuthorityFields(cloned)
		return cloned
	}
	return CloneTurnForThread(turn, threadID, now)
}

func CloneTurnForThread(turn map[string]any, threadID string, now string) map[string]any {
	caseAuthority := turnRequiresCaseAuthorityIsolation(turn)
	privateProtocolCallIDs := privateProtocolObservedCallIDsV1(turn)
	cloned := contracts.CloneMap(turn)
	cloned["threadId"] = threadID
	if isTurnInFlight(cloned) {
		cloned["status"] = "completed"
	}
	if strings.TrimSpace(stringField(cloned, "finishedAt")) == "" {
		cloned["finishedAt"] = now
	}
	items, _ := cloned["items"].([]any)
	clonedItems := make([]any, 0, len(items))
	for _, itemValue := range items {
		item, _ := itemValue.(map[string]any)
		if item == nil {
			continue
		}
		if privateProtocolCallIDs[stringField(item, "callId")] {
			switch stringField(item, "kind") {
			case "tool_call", "tool_result":
				// Fork/resume starts a new provider attempt after stripping
				// source execution authority. Drop only host-marked private
				// protocol pairs; replay without ephemeral thinking bytes is
				// invalid, while ordinary pairs remain native.
				continue
			}
		}
		if caseAuthority && stringField(item, "kind") != "user_message" {
			continue
		}
		clonedItem := CloneItemForThread(item, threadID, now)
		if clonedItem == nil {
			continue
		}
		if caseAuthority {
			stripCaseAuthorityFields(clonedItem)
		}
		clonedItems = append(clonedItems, clonedItem)
	}
	cloned["items"] = clonedItems
	stripCaseAuthorityFields(cloned)
	if caseAuthority || len(listAny(cloned["attachmentIds"])) == 0 {
		cloned["attachmentIds"] = AttachmentIDsFromItems(clonedItems)
	}
	return cloned
}

func privateProtocolObservedCallIDsV1(turn map[string]any) map[string]bool {
	callIDs := map[string]bool{}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if item == nil || stringField(item, "kind") != "tool_result" {
			continue
		}
		if _, present := item["privateProtocolObserved"]; !present {
			continue
		}
		if callID := stringField(item, "callId"); callID != "" {
			// Canonical durable history accepts only boolean true. Treat any
			// in-memory malformed presence as private so it cannot degrade
			// into provider-native replay during derivation.
			callIDs[callID] = true
		}
	}
	return callIDs
}

func turnRequiresCaseAuthorityIsolation(turn map[string]any) bool {
	if turn == nil {
		return false
	}
	if turn["acceptedFinal"] != nil {
		return true
	}
	value := turn["securityContext"]
	if value == nil {
		return false
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(value)
	return err != nil || domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext)
}

func stripCaseAuthorityFields(record map[string]any) {
	stripDerivedExecutionAuthorityFields(record)
	for _, key := range []string{
		"acceptedFinal", "providerResponse", "assistantText", "output",
	} {
		delete(record, key)
	}
}

func stripDerivedExecutionAuthorityFields(record map[string]any) {
	for _, key := range []string{
		"securityContext", "contextEpochSnapshot", "contextDigest", "contextEpoch", "datasetSnapshotId", "caseId", "caseBindingHash",
		"sourceManifestHash", "executionGrant", "executionGrantId", "grantId", "parentGrantId", "hostEvidenceSettlement",
		"privateProtocolObserved",
		"approvalTransition", "approvalTransitionId", "approvalItemId", "continuationDispositionId",
		"continuationReceiptId", "approvalId", "inputId", "pendingApprovalIds", "pendingUserInputIds",
		"generalTerminalPublication", "generalTerminalPublicationArchive", "generalTerminalCASBinding", "generalTerminalCASBindingDigest",
		"generalTerminalCommitId", "generalTerminalEventId", "generalTerminalSlot", "generalTerminalPayloadDigest",
		"generalTerminalAuthorityKind", "generalTerminalAuthorityDigest",
	} {
		delete(record, key)
	}
}

func CloneItemForThread(item map[string]any, threadID string, now string) map[string]any {
	var cloned map[string]any
	switch strings.TrimSpace(stringField(item, "kind")) {
	case "execution_grant_transition", "assistant_text", "error":
		return nil
	case "compaction":
		if !domainevent.ValidGeneralCompactionProviderHistoryItemV3(item) {
			return nil
		}
		cloned = contracts.CloneMap(item)
	case "tool_call":
		cloned = domaintoolcall.PublicToolCallItemRecordV1(item)
	case "tool_result":
		cloned = domaintoolresult.PublicToolResultItemRecordV1(item)
		if projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(cloned["output"]); err == nil &&
			projection.ProjectionKind == domaintoolresult.ProjectionCaseSourceStatus {
			cloned["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.LegacyWithheldProjectionV1())
		}
	default:
		cloned = contracts.CloneMap(item)
	}
	cloned["threadId"] = threadID
	stripDerivedExecutionAuthorityFields(cloned)
	if isItemInFlight(cloned) {
		switch stringField(cloned, "kind") {
		case "approval":
			cloned["status"] = "expired"
		case "user_input":
			cloned["status"] = "cancelled"
		default:
			cloned["status"] = "completed"
		}
		if strings.TrimSpace(stringField(cloned, "finishedAt")) == "" {
			cloned["finishedAt"] = now
		}
	}
	return cloned
}

func AttachmentIDsFromItems(items []any) []any {
	seen := map[string]bool{}
	ids := []any{}
	for _, itemValue := range items {
		item, _ := itemValue.(map[string]any)
		if stringField(item, "kind") != "user_message" {
			continue
		}
		for _, rawID := range listAny(item["attachmentIds"]) {
			id, _ := rawID.(string)
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func CloneTodoListForThread(todos map[string]any, threadID string, now string) map[string]any {
	cloned := contracts.CloneMap(todos)
	cloned["threadId"] = threadID
	cloned["updatedAt"] = now
	return cloned
}

func forkSourceTurns(turns []any, targetTurnID string) ([]any, bool, error) {
	if targetTurnID == "" {
		return turns, true, nil
	}
	for index, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		if stringField(turn, "id") == targetTurnID {
			return turns[:index+1], index == len(turns)-1, nil
		}
	}
	return nil, false, fmt.Errorf("%w: %s", ErrTurnNotFound, targetTurnID)
}

func firstUserMessageItem(turn map[string]any) map[string]any {
	items, _ := turn["items"].([]any)
	for _, itemValue := range items {
		item, _ := itemValue.(map[string]any)
		if stringField(item, "kind") == "user_message" {
			return item
		}
	}
	return nil
}

func isTurnInFlight(turn map[string]any) bool {
	status := stringField(turn, "status")
	return status == "queued" || status == "running"
}

func isItemInFlight(item map[string]any) bool {
	status := stringField(item, "status")
	return status == "pending" || status == "running"
}

func listAny(value any) []any {
	items, _ := value.([]any)
	return items
}
