package nativecomponent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

const (
	HealthOutcomeSchemaVersion = 1
	HealthStatusReady          = "ready"
	HealthStatusUnavailable    = "unavailable"
	HealthCodeReady            = "native_health_ready"
	HealthCodeUnavailable      = "native_health_unavailable"
	healthGrantTTL             = 30 * time.Second
)

var (
	ErrHealthUnavailableSettled = errors.New("native_component_health_unavailable_settled")
	ErrHealthAuthorityInvalid   = errors.New("native_component_health_authority_invalid")
	ErrHealthDuplicate          = errors.New("native_component_health_duplicate")
	ErrHealthSettlement         = errors.New("native_component_health_settlement_invalid")
	ErrHealthExecutionUnsafe    = errors.New("native_component_health_execution_unsafe")
)

// HealthOutcome is deliberately metadata-only. In particular, it never
// exposes the native registry digest, executable path, process identity, raw
// output, or a value that can be mistaken for case evidence.
type HealthOutcome struct {
	SchemaVersion int
	Status        string
	Code          string
}

type HealthStore interface {
	GetThread(string) (map[string]any, error)
	AppendItemToTurn(string, string, map[string]any) error
	PatchTurnItemStatus(string, string, string, string) error
}

type HealthDependencies struct {
	Store            HealthStore
	DurableAuthority DurableAuthority
	AcquireEffect    AcquireEffect
	Service          *Service
	Now              func() time.Time
}

type HealthCoordinator struct {
	dependencies HealthDependencies
	flightMu     sync.Mutex
	flights      map[string]struct{}
}

func NewHealthCoordinator(dependencies HealthDependencies) *HealthCoordinator {
	return &HealthCoordinator{dependencies: dependencies}
}

func (coordinator *HealthCoordinator) Available() bool {
	return coordinator != nil && !dependencyIsNil(coordinator.dependencies.Store) && !dependencyIsNil(coordinator.dependencies.DurableAuthority) &&
		coordinator.dependencies.AcquireEffect != nil && coordinator.dependencies.Now != nil
}

func (coordinator *HealthCoordinator) executionAvailable() bool {
	return coordinator != nil && !dependencyIsNil(coordinator.dependencies.Service) && coordinator.dependencies.Service.Available()
}

// ProbeDataEngine executes at most one current-run native health operation for
// a frozen case turn. The host persists the grant before execution and a
// metadata-only result afterward, without emitting tool events. Readiness is
// returned only after the exact durable registry transition is read back as
// active -> settled.
func (coordinator *HealthCoordinator) ProbeDataEngine(
	ctx context.Context,
	expected domainsecurity.TurnSecurityContext,
) (HealthOutcome, error) {
	return coordinator.probeDataEngine(ctx, expected, false)
}

// ensureDataEngineReady executes the first exact health probe and otherwise
// admits only a strictly verified settled-ready record for the same frozen
// context. An open, failed, unavailable, torn, or foreign record is never
// retried or upgraded into readiness.
func (coordinator *HealthCoordinator) ensureDataEngineReady(
	ctx context.Context,
	expected domainsecurity.TurnSecurityContext,
) (HealthOutcome, error) {
	return coordinator.probeDataEngine(ctx, expected, true)
}

func (coordinator *HealthCoordinator) probeDataEngine(
	ctx context.Context,
	expected domainsecurity.TurnSecurityContext,
	allowSettledReady bool,
) (HealthOutcome, error) {
	unavailable := HealthOutcome{SchemaVersion: HealthOutcomeSchemaVersion, Status: HealthStatusUnavailable, Code: HealthCodeUnavailable}
	if !coordinator.Available() || ctx == nil {
		return unavailable, ErrHealthAuthorityInvalid
	}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(expected) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(expected) {
		return unavailable, ErrHealthAuthorityInvalid
	}
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok || policy.MaxDuration <= 0 {
		return unavailable, ErrHealthAuthorityInvalid
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, policy.MaxDuration)
	defer cancelProbe()
	releaseFlight, err := coordinator.beginHealthFlight(expected.ContextDigest)
	if err != nil {
		return unavailable, err
	}
	defer releaseFlight()
	threadID := expected.ThreadID
	turnID := expected.TurnID
	before, _, err := coordinator.currentHealthContext(threadID, turnID)
	if err != nil || before != expected {
		return unavailable, ErrHealthAuthorityInvalid
	}
	if err := probeCtx.Err(); err != nil {
		return unavailable, err
	}
	effectCtx, release, err := coordinator.dependencies.AcquireEffect(probeCtx, before)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		if contextErr := healthContextError(probeCtx, err); contextErr != nil {
			return unavailable, contextErr
		}
		return unavailable, ErrHealthAuthorityInvalid
	}
	defer release()

	current, turn, err := coordinator.currentHealthContext(threadID, turnID)
	if err != nil || current != before {
		return unavailable, ErrHealthAuthorityInvalid
	}
	if err := probeCtx.Err(); err != nil {
		return unavailable, err
	}
	canonicalArguments, ok := domainnative.CanonicalArgumentsV1(policy.ComponentID, policy.Operation)
	if !ok {
		return unavailable, ErrHealthAuthorityInvalid
	}
	call := healthToolCall(current, policy, canonicalArguments)
	callItemID := healthToolCallItemID(turnID, call)
	if healthCallAlreadyExists(turn, call, callItemID) {
		if allowSettledReady {
			return coordinator.readDurableHealthOutcome(current, turn, policy, call, callItemID)
		}
		return unavailable, ErrHealthDuplicate
	}
	issuedAt := coordinator.dependencies.Now().UTC()
	if issuedAt.IsZero() {
		return unavailable, ErrHealthAuthorityInvalid
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: current, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: call.Name, ToolCallID: call.ID, ConnectionEpoch: 0,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(current, policy.ComponentID, policy.Operation), ReadOnly: policy.ReadOnly,
		ApprovalState: "not_required", IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(healthGrantTTL),
	})
	callItem, _, err := appturn.ToolCallReadyRecords(appturn.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: callItemID, CreatedAt: issuedAt.Format(time.RFC3339Nano),
		Call: call, ToolKind: toolcatalogapp.ToolKind(call.Name), Context: current, Grant: grant,
	})
	if err != nil {
		return unavailable, ErrHealthAuthorityInvalid
	}
	appendErr := coordinator.dependencies.Store.AppendItemToTurn(threadID, turnID, callItem)
	if verifyErr := coordinator.verifyActiveHealthCall(current, call, callItemID, grant); verifyErr != nil {
		if appendErr != nil {
			return unavailable, ErrHealthSettlement
		}
		return unavailable, ErrHealthAuthorityInvalid
	}

	runErr := error(ErrUnavailable)
	executionAvailable := coordinator.executionAvailable()
	if err := probeCtx.Err(); err != nil {
		runErr = err
	} else if executionAvailable {
		_, runErr = coordinator.dependencies.Service.Execute(effectCtx, ExecuteInput{
			ThreadID: threadID, TurnID: turnID, GrantID: grant.GrantID,
		})
	}
	if runErr == nil && probeCtx.Err() != nil {
		runErr = probeCtx.Err()
	}
	resultCode := healthToolResultCode(effectCtx, runErr)
	if err := coordinator.settleHealthCall(current, call, callItemID, grant, resultCode, runErr != nil); err != nil {
		return unavailable, ErrHealthSettlement
	}
	if runErr != nil {
		return unavailable, classifySettledHealthRunError(runErr, executionAvailable)
	}
	return HealthOutcome{SchemaVersion: HealthOutcomeSchemaVersion, Status: HealthStatusReady, Code: HealthCodeReady}, nil
}

func (coordinator *HealthCoordinator) readDurableHealthOutcome(
	securityContext domainsecurity.TurnSecurityContext,
	turn map[string]any,
	policy domainnative.OperationPolicy,
	call domainmodel.ToolCall,
	callItemID string,
) (HealthOutcome, error) {
	unavailable := HealthOutcome{SchemaVersion: HealthOutcomeSchemaVersion, Status: HealthStatusUnavailable, Code: HealthCodeUnavailable}
	resultItemID := toolcatalogapp.ToolResultItemID(securityContext.TurnID, call.ID)
	var callItem map[string]any
	var resultItem map[string]any
	callCount := 0
	resultCount := 0
	for _, raw := range healthTurnItems(turn) {
		item, ok := raw.(map[string]any)
		if !ok {
			return unavailable, ErrHealthSettlement
		}
		itemID := strings.TrimSpace(contracts.StringField(item, "id"))
		toolName := strings.TrimSpace(contracts.StringField(item, "toolName"))
		callID := strings.TrimSpace(contracts.StringField(item, "callId"))
		touchesHealth := itemID == callItemID || itemID == resultItemID || toolName == call.Name || callID == call.ID
		if !touchesHealth {
			continue
		}
		if toolName != call.Name || callID != call.ID {
			return unavailable, ErrHealthSettlement
		}
		switch itemID {
		case callItemID:
			callCount++
			callItem = item
		case resultItemID:
			resultCount++
			resultItem = item
		default:
			return unavailable, ErrHealthSettlement
		}
	}
	if callCount != 1 || resultCount != 1 || callItem == nil || resultItem == nil {
		return unavailable, ErrHealthSettlement
	}
	grant, err := domainsecurity.ParseExecutionGrant(callItem["executionGrant"])
	if err != nil || validateDurableHealthGrant(securityContext, grant, call) != nil ||
		grant.Provider != domainnative.NativeProvider || grant.ServerIdentity != domainnative.NativeServerIdentity ||
		grant.ConnectionEpoch != 0 || grant.SchemaHash != policy.SchemaHash ||
		grant.ScopeHash != domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation) ||
		grant.ReadOnly != policy.ReadOnly || grant.ApprovalState != "not_required" {
		return unavailable, ErrHealthSettlement
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(resultItem["output"])
	isError, isErrorOK := resultItem["isError"].(bool)
	if err != nil || !isErrorOK {
		return unavailable, ErrHealthSettlement
	}
	records := toolcatalogapp.ToolResultRecords{
		Status:       strings.TrimSpace(contracts.StringField(resultItem, "status")),
		ResultItem:   resultItem,
		ResultItemID: resultItemID,
	}
	if err := coordinator.verifyDurableHealthSettlement(
		securityContext, call, callItemID, grant, records, projection.Code, isError,
	); err != nil {
		return unavailable, err
	}
	switch projection.Code {
	case "tool_completed":
		if isError {
			return unavailable, ErrHealthSettlement
		}
		return HealthOutcome{SchemaVersion: HealthOutcomeSchemaVersion, Status: HealthStatusReady, Code: HealthCodeReady}, nil
	case "tool_source_unavailable":
		if !isError {
			return unavailable, ErrHealthSettlement
		}
		return unavailable, ErrHealthUnavailableSettled
	case "tool_cancelled", "tool_timeout", "tool_failed":
		if !isError {
			return unavailable, ErrHealthSettlement
		}
		return unavailable, ErrHealthExecutionUnsafe
	case "execution_grant_invalid":
		if !isError {
			return unavailable, ErrHealthSettlement
		}
		return unavailable, errors.Join(ErrHealthExecutionUnsafe, ErrGrantInvalid)
	default:
		return unavailable, ErrHealthSettlement
	}
}

func validateDurableHealthGrant(
	securityContext domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	call domainmodel.ToolCall,
) error {
	if domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		grant.ToolName != strings.TrimSpace(call.Name) ||
		grant.ToolCallID != strings.TrimSpace(call.ID) ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(call.Arguments) {
		return ErrHealthSettlement
	}
	return nil
}

func (coordinator *HealthCoordinator) beginHealthFlight(key string) (func(), error) {
	if coordinator == nil || !domainsecurity.IsSHA256Hex(key) {
		return nil, ErrHealthAuthorityInvalid
	}
	coordinator.flightMu.Lock()
	if coordinator.flights == nil {
		coordinator.flights = map[string]struct{}{}
	}
	if _, exists := coordinator.flights[key]; exists {
		coordinator.flightMu.Unlock()
		return nil, ErrHealthDuplicate
	}
	coordinator.flights[key] = struct{}{}
	coordinator.flightMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			coordinator.flightMu.Lock()
			delete(coordinator.flights, key)
			coordinator.flightMu.Unlock()
		})
	}, nil
}

func classifySettledHealthRunError(runErr error, executionAvailable bool) error {
	if runErr == nil {
		return nil
	}
	if contextErr := healthContextError(nil, runErr); contextErr != nil {
		return contextErr
	}
	if !executionAvailable || runErr == nativecomponentport.ErrUnavailable {
		return ErrHealthUnavailableSettled
	}
	if errors.Is(runErr, ErrAuthorityInvalid) || errors.Is(runErr, ErrGrantInvalid) ||
		errors.Is(runErr, ErrRequestInvalid) {
		return runErr
	}
	return errors.Join(ErrHealthExecutionUnsafe, runErr)
}

func healthContextError(ctx context.Context, err error) error {
	if ctx != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}

func (coordinator *HealthCoordinator) currentHealthContext(threadID, turnID string) (domainsecurity.TurnSecurityContext, map[string]any, error) {
	thread, err := coordinator.dependencies.Store.GetThread(threadID)
	if err != nil || thread == nil || strings.TrimSpace(contracts.StringField(thread, "id")) != threadID {
		return domainsecurity.TurnSecurityContext{}, nil, ErrHealthAuthorityInvalid
	}
	securityContext, err := coordinator.dependencies.DurableAuthority.ValidateCurrent(threadID, thread)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) || securityContext.ThreadID != threadID || securityContext.TurnID != turnID {
		return domainsecurity.TurnSecurityContext{}, nil, ErrHealthAuthorityInvalid
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	status := strings.TrimSpace(contracts.StringField(turn, "status"))
	turnContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if !ok || (status != "running" && status != "waiting") || contextErr != nil || turnContext != securityContext {
		return domainsecurity.TurnSecurityContext{}, nil, ErrHealthAuthorityInvalid
	}
	if _, ok := turn["items"].([]any); !ok {
		return domainsecurity.TurnSecurityContext{}, nil, ErrHealthAuthorityInvalid
	}
	if _, err := executiongrantapp.RegistryFromThread(threadID, thread, turnID); err != nil {
		return domainsecurity.TurnSecurityContext{}, nil, ErrHealthAuthorityInvalid
	}
	return securityContext, turn, nil
}

func (coordinator *HealthCoordinator) verifyActiveHealthCall(
	securityContext domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	callItemID string,
	grant domainsecurity.ExecutionGrant,
) error {
	current, turn, err := coordinator.currentHealthContext(securityContext.ThreadID, securityContext.TurnID)
	if err != nil || current != securityContext {
		return ErrHealthAuthorityInvalid
	}
	callCount := 0
	for _, raw := range healthTurnItems(turn) {
		item, ok := raw.(map[string]any)
		if !ok {
			return ErrHealthAuthorityInvalid
		}
		if strings.TrimSpace(contracts.StringField(item, "id")) != callItemID {
			continue
		}
		parsedGrant, parseErr := domainsecurity.ParseExecutionGrant(item["executionGrant"])
		if parseErr != nil || parsedGrant != grant || contracts.StringField(item, "kind") != "tool_call" ||
			contracts.StringField(item, "toolName") != call.Name || contracts.StringField(item, "callId") != call.ID ||
			contracts.StringField(item, "executionGrantId") != grant.GrantID {
			return ErrHealthAuthorityInvalid
		}
		callCount++
	}
	if callCount != 1 {
		return ErrHealthAuthorityInvalid
	}
	thread, err := coordinator.dependencies.Store.GetThread(securityContext.ThreadID)
	if err != nil {
		return ErrHealthAuthorityInvalid
	}
	return executiongrantapp.VerifyThreadGrantMembership(
		securityContext.ThreadID, thread, securityContext.TurnID, grant, domainsecurity.GrantRegistryActive,
	)
}

func (coordinator *HealthCoordinator) settleHealthCall(
	securityContext domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	callItemID string,
	grant domainsecurity.ExecutionGrant,
	code string,
	isError bool,
) error {
	settledAt := coordinator.dependencies.Now().UTC()
	issuedAt, err := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	if err != nil {
		return ErrHealthSettlement
	}
	if settledAt.Before(issuedAt) {
		settledAt = issuedAt
	}
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CreatedAt: settledAt.Format(time.RFC3339Nano), FinishedAt: settledAt.Format(time.RFC3339Nano),
		Call: call, Projection: toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, map[string]any{"code": code}, isError),
		IsError: isError, ContextDigest: securityContext.ContextDigest, ContextEpoch: securityContext.ContextEpoch,
		ExecutionGrantID: grant.GrantID,
	})
	if err != nil {
		return ErrHealthSettlement
	}
	appendErr := coordinator.dependencies.Store.AppendItemToTurn(securityContext.ThreadID, securityContext.TurnID, records.ResultItem)
	if appendErr != nil {
		thread, readErr := coordinator.dependencies.Store.GetThread(securityContext.ThreadID)
		if readErr == nil {
			_, readErr = executiongrantapp.DurableSettlementFromThread(
				securityContext.ThreadID, thread, securityContext.TurnID, records.ResultItemID, grant,
			)
		}
		if readErr == nil {
			appendErr = nil
		}
	}
	if appendErr != nil {
		return ErrHealthSettlement
	}
	patchErr := coordinator.dependencies.Store.PatchTurnItemStatus(
		securityContext.ThreadID, securityContext.TurnID, callItemID, records.Status,
	)
	if err := coordinator.verifyDurableHealthSettlement(securityContext, call, callItemID, grant, records, code, isError); err != nil {
		if patchErr != nil {
			return ErrHealthSettlement
		}
		return err
	}
	return nil
}

func (coordinator *HealthCoordinator) verifyDurableHealthSettlement(
	securityContext domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	callItemID string,
	grant domainsecurity.ExecutionGrant,
	records toolcatalogapp.ToolResultRecords,
	code string,
	isError bool,
) error {
	thread, err := coordinator.dependencies.Store.GetThread(securityContext.ThreadID)
	if err != nil {
		return ErrHealthSettlement
	}
	authority, err := executiongrantapp.DurableSettlementFromThread(
		securityContext.ThreadID, thread, securityContext.TurnID, records.ResultItemID, grant,
	)
	if err != nil || authority.ResultItem == nil || authority.ResultItem["hostEvidenceSettlement"] != nil ||
		contracts.StringField(authority.ResultItem, "toolName") != call.Name ||
		contracts.StringField(authority.ResultItem, "callId") != call.ID ||
		contracts.StringField(authority.ResultItem, "executionGrantId") != grant.GrantID {
		return ErrHealthSettlement
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(authority.ResultItem["output"])
	resultIsError, resultIsErrorOK := authority.ResultItem["isError"].(bool)
	expectedProjection := toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, map[string]any{"code": code}, isError)
	if err != nil || !resultIsErrorOK || resultIsError != isError ||
		projection.SchemaVersion != expectedProjection.SchemaVersion ||
		projection.ProjectionKind != expectedProjection.ProjectionKind ||
		projection.Disclosure != expectedProjection.Disclosure ||
		projection.MessageKey != expectedProjection.MessageKey ||
		projection.Status != expectedProjection.Status ||
		projection.Code != expectedProjection.Code ||
		projection.PrivatePayloadWithheld != expectedProjection.PrivatePayloadWithheld ||
		projection.FactAnswerAllowed || projection.EvidenceAuthority ||
		projection.Plan != nil || projection.RPCError != nil {
		return ErrHealthSettlement
	}
	turn, ok := appmodel.TurnByID(thread, securityContext.TurnID)
	if !ok {
		return ErrHealthSettlement
	}
	callCount := 0
	resultCount := 0
	for _, raw := range healthTurnItems(turn) {
		item, ok := raw.(map[string]any)
		if !ok {
			return ErrHealthSettlement
		}
		switch strings.TrimSpace(contracts.StringField(item, "id")) {
		case callItemID:
			_, argumentsErr := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(item["arguments"])
			// Older durable health calls were written with the internal "host"
			// kind, which the closed durable projection correctly withheld. Keep
			// those settled records readable, while every new write and every
			// present value must use the canonical catalog tool kind.
			callToolKind := strings.TrimSpace(contracts.StringField(item, "toolKind"))
			if !healthRecordHasOnlyKeys(item,
				"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "toolName", "callId",
				"toolKind", "arguments", "contextDigest", "contextEpoch", "executionGrantId", "executionGrant",
			) || argumentsErr != nil ||
				contracts.StringField(item, "kind") != "tool_call" || contracts.StringField(item, "role") != "tool" ||
				contracts.StringField(item, "status") != records.Status || contracts.StringField(item, "threadId") != securityContext.ThreadID ||
				contracts.StringField(item, "turnId") != securityContext.TurnID || contracts.StringField(item, "toolName") != call.Name ||
				contracts.StringField(item, "callId") != call.ID ||
				(callToolKind != "" && callToolKind != toolcatalogapp.ToolKind(call.Name)) ||
				contracts.StringField(item, "contextDigest") != securityContext.ContextDigest ||
				healthRecordUint64(item["contextEpoch"]) != securityContext.ContextEpoch ||
				contracts.StringField(item, "executionGrantId") != grant.GrantID {
				return ErrHealthSettlement
			}
			callCount++
		case records.ResultItemID:
			if !healthRecordHasOnlyKeys(item,
				"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "toolName", "callId",
				"toolKind", "output", "isError", "contextDigest", "contextEpoch", "executionGrantId",
			) || contracts.StringField(item, "kind") != "tool_result" || contracts.StringField(item, "role") != "tool" ||
				contracts.StringField(item, "threadId") != securityContext.ThreadID || contracts.StringField(item, "turnId") != securityContext.TurnID ||
				contracts.StringField(item, "toolName") != call.Name || contracts.StringField(item, "callId") != call.ID ||
				contracts.StringField(item, "toolKind") != toolcatalogapp.ToolKind(call.Name) ||
				contracts.StringField(item, "contextDigest") != securityContext.ContextDigest ||
				healthRecordUint64(item["contextEpoch"]) != securityContext.ContextEpoch ||
				contracts.StringField(item, "executionGrantId") != grant.GrantID {
				return ErrHealthSettlement
			}
			resultCount++
		}
	}
	if callCount != 1 || resultCount != 1 {
		return ErrHealthSettlement
	}
	return nil
}

func healthRecordHasOnlyKeys(record map[string]any, allowed ...string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range record {
		if _, ok := allowedSet[key]; !ok {
			return false
		}
	}
	return true
}

func healthRecordUint64(value any) uint64 {
	switch typed := value.(type) {
	case uint64:
		return typed
	case uint:
		return uint64(typed)
	case int:
		if typed >= 0 {
			return uint64(typed)
		}
	case int64:
		if typed >= 0 {
			return uint64(typed)
		}
	case float64:
		converted := uint64(typed)
		if typed >= 0 && float64(converted) == typed {
			return converted
		}
	}
	return 0
}

func healthToolCall(securityContext domainsecurity.TurnSecurityContext, policy domainnative.OperationPolicy, canonicalArguments string) domainmodel.ToolCall {
	entropy := sha256.Sum256([]byte("analytix.native-health-tool-call/v1\x00" + securityContext.ContextDigest))
	identity, _ := domainmodel.NewHostToolCallIDV1(entropy[:])
	return domainmodel.ToolCall{
		ID:        identity,
		Name:      domainnative.ToolName(policy.ComponentID, policy.Operation),
		Arguments: json.RawMessage(canonicalArguments),
	}
}

func healthToolCallItemID(turnID string, call domainmodel.ToolCall) string {
	return domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
}

func healthCallAlreadyExists(turn map[string]any, call domainmodel.ToolCall, callItemID string) bool {
	resultItemID := toolcatalogapp.ToolResultItemID(strings.TrimSpace(contracts.StringField(turn, "id")), call.ID)
	for _, raw := range healthTurnItems(turn) {
		item, ok := raw.(map[string]any)
		if !ok {
			return true
		}
		if strings.TrimSpace(contracts.StringField(item, "id")) == callItemID ||
			strings.TrimSpace(contracts.StringField(item, "id")) == resultItemID ||
			strings.TrimSpace(contracts.StringField(item, "toolName")) == call.Name ||
			strings.TrimSpace(contracts.StringField(item, "callId")) == call.ID {
			return true
		}
	}
	return false
}

func healthTurnItems(turn map[string]any) []any {
	items, _ := turn["items"].([]any)
	return items
}

func healthToolResultCode(ctx context.Context, runErr error) string {
	if runErr == nil {
		return "tool_completed"
	}
	if errors.Is(runErr, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)) {
		return "tool_cancelled"
	}
	if errors.Is(runErr, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return "tool_timeout"
	}
	if errors.Is(runErr, ErrAuthorityInvalid) || errors.Is(runErr, ErrGrantInvalid) || errors.Is(runErr, ErrRequestInvalid) {
		return "execution_grant_invalid"
	}
	if runErr == ErrUnavailable {
		return "tool_source_unavailable"
	}
	if runErr == nativecomponentport.ErrUnavailable {
		return "tool_source_unavailable"
	}
	return "tool_failed"
}
