package cachetelemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
	cachetelemetrystoreport "analytix.local/runtime-go/internal/ports/cachetelemetrystore"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const (
	providerTelemetryKeyDerivationDomain = domaincachetelemetry.ProviderTelemetryKeyDerivationDomainV1
	providerTelemetryDigestEpochDomain   = "analytix/provider-attempt-telemetry-digest-epoch/v1\x00"
	maxProviderTelemetryWireBodyBytes    = 32 * 1024 * 1024
	maxProviderTelemetryHeadersBytes     = 1024 * 1024
	maxProviderTelemetryConfigBytes      = 1024 * 1024
	maxProviderTelemetryIdentityBytes    = 64 * 1024
)

var ErrDurableAuthority = errors.New("provider cache telemetry durable authority failed")

// RetryForbiddenError prevents an authority/storage failure from being
// mistaken for a retryable provider transport failure.
type RetryForbiddenError struct {
	Operation string
	Err       error
}

func (err RetryForbiddenError) Error() string {
	if err.Err == nil {
		return ErrDurableAuthority.Error()
	}
	if strings.TrimSpace(err.Operation) == "" {
		return fmt.Sprintf("%s: %v", ErrDurableAuthority, err.Err)
	}
	return fmt.Sprintf("%s during %s: %v", ErrDurableAuthority, err.Operation, err.Err)
}

func (err RetryForbiddenError) Unwrap() error {
	if err.Err == nil {
		return ErrDurableAuthority
	}
	return errors.Join(ErrDurableAuthority, err.Err)
}

func (RetryForbiddenError) ProviderRetryForbidden() bool { return true }

type DurableService struct {
	mu               sync.Mutex
	authority        finalauthorityport.Authority
	store            cachetelemetrystoreport.Store
	trustedInventory *trustedProviderInventoryV1
	restartPreserved map[string]struct{}
}

type trustedProviderInventoryV1 struct {
	intents     []domaincachetelemetry.ProviderAttemptIntentV1
	settlements []domaincachetelemetry.ProviderAttemptSettlementV1
	closures    []domaincachetelemetry.ProviderTurnClosureV1
}

type RestartSettlementPlanV1 struct {
	Settlements []domaincachetelemetry.ProviderAttemptSettlementV1
}

func NewDurableService(authority finalauthorityport.Authority, store cachetelemetrystoreport.Store) (*DurableService, error) {
	if authority == nil || store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(authority.KeyID())) || len(authority.PublicKey()) == 0 {
		return nil, errors.New("provider cache telemetry durable service authority is unavailable")
	}
	return &DurableService{authority: authority, store: store}, nil
}

func (service *DurableService) BeginAttempt(ctx context.Context, input cachetelemetryport.AttemptRegistrationInputV1) (cachetelemetryport.AttemptHandleV1, error) {
	if err := validateAttemptRegistrationInputV1(input); err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", err)
	}
	if service == nil || service.authority == nil || service.store == nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", errors.New("durable service is unavailable"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := contextErrorV1(ctx); err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", err)
	}
	digests, err := service.deriveAttemptDigestsV1(ctx, input)
	if err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", err)
	}
	if service.restartPreservesBindingV1(digests.turnBindingHMAC) {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", ErrRestartPreserved)
	}
	intents, _, err := service.turnInventoryV1(ctx, digests.turnBindingHMAC)
	if err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", err)
	}
	logicalOrdinal, channelOrdinal, err := allocateProviderAttemptOrdinalsV1(intents, digests, input.PhysicalAttempt)
	if err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("begin provider attempt", err)
	}
	shape := domaincachetelemetry.CacheVisibleShapeV1{
		SchemaVersion:   domaincachetelemetry.CacheVisibleShapeV1SchemaVersion,
		LogicalCallHMAC: digests.logicalCallHMAC, Attempt: input.PhysicalAttempt,
		ProviderFamily: input.ProviderFamily, ModelHMAC: digests.modelHMAC,
		Endpoint: input.EndpointFormat, EndpointHMAC: digests.endpointHMAC,
		WireBodyHMAC: digests.wireBodyHMAC, CredentialScopeHMAC: digests.credentialScopeHMAC,
		ProviderConfigHMAC: digests.providerConfigHMAC, DigestEpoch: providerTelemetryDigestEpochV1(service.authority.KeyID()),
		StartedAt: input.StartedAt.UTC().Format(time.RFC3339Nano),
	}
	intent, err := domaincachetelemetry.NewProviderAttemptIntentV1(domaincachetelemetry.ProviderAttemptIntentInputV1{
		TurnBindingHMAC: digests.turnBindingHMAC, UsageSource: input.UsageSource,
		ChildRunHMAC: digests.childRunHMAC, Channel: input.Channel, ChannelOrdinal: channelOrdinal,
		LogicalCallOrdinal: logicalOrdinal, Shape: shape, WireHeadersHMAC: digests.wireHeadersHMAC,
		IssuedAt: input.StartedAt, AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("sign provider attempt intent", err)
	}
	persisted, err := service.store.CommitIntentIfAbsent(ctx, intent)
	if err != nil {
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("commit provider attempt intent", err)
	}
	if persisted != intent || service.verifyIntentTrustedV1(ctx, persisted) != nil {
		service.trustedInventory = nil
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("verify provider attempt intent", errors.Join(err, errors.New("intent read-back differs from signed authority")))
	}
	if err := service.rememberIntentLockedV1(persisted); err != nil {
		service.trustedInventory = nil
		return cachetelemetryport.AttemptHandleV1{}, forbidProviderRetryV1("verify provider attempt intent", err)
	}
	return cachetelemetryport.AttemptHandleV1{Intent: persisted}, nil
}

func (service *DurableService) SettleAttempt(ctx context.Context, input cachetelemetryport.AttemptSettlementInputV1) error {
	if service == nil || service.authority == nil || service.store == nil {
		return forbidProviderRetryV1("settle provider attempt", errors.New("durable service is unavailable"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := contextErrorV1(ctx); err != nil {
		return forbidProviderRetryV1("settle provider attempt", err)
	}
	intent := input.Handle.Intent
	if err := domaincachetelemetry.ValidateProviderAttemptIntentV1(intent); err != nil {
		return forbidProviderRetryV1("settle provider attempt", err)
	}
	if service.verifyIntentTrustedV1(ctx, intent) != nil {
		return forbidProviderRetryV1("settle provider attempt", errors.New("intent authority changed before settlement"))
	}
	if service.restartPreservesBindingV1(intent.TurnBindingHMAC) {
		return forbidProviderRetryV1("settle provider attempt", ErrRestartPreserved)
	}
	settledAt := input.SettledAt.UTC()
	if settledAt.IsZero() {
		return forbidProviderRetryV1("settle provider attempt", errors.New("settlement time is required"))
	}
	observation := domaincachetelemetry.ProviderCallObservationV1{
		SchemaVersion: domaincachetelemetry.ProviderCallObservationV1SchemaVersion,
		Shape:         intent.Shape, Status: input.Status, Usage: input.Usage,
		SettledAt: settledAt.Format(time.RFC3339Nano),
	}
	settlement, err := domaincachetelemetry.NewProviderAttemptSettlementV1(domaincachetelemetry.ProviderAttemptSettlementInputV1{
		Intent: intent, DispatchState: input.DispatchState, Observation: observation,
		SafeReasonCode: strings.TrimSpace(input.SafeReasonCode), AuthorityKeyID: service.authority.KeyID(),
		AuthorityPublicKey: service.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return forbidProviderRetryV1("sign provider attempt settlement", err)
	}
	persisted, err := service.store.CommitSettlementIfAbsent(ctx, settlement)
	if err != nil {
		return forbidProviderRetryV1("commit provider attempt settlement", err)
	}
	if persisted != settlement || service.verifySettlementTrustedV1(ctx, persisted) != nil {
		service.trustedInventory = nil
		return forbidProviderRetryV1("verify provider attempt settlement", errors.Join(err, errors.New("settlement read-back differs from signed authority")))
	}
	if err := service.rememberSettlementLockedV1(persisted); err != nil {
		service.trustedInventory = nil
		return forbidProviderRetryV1("verify provider attempt settlement", err)
	}
	return nil
}

func (service *DurableService) CloseTurn(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	terminalReasonCode domaincachetelemetry.ProviderTurnTerminalReasonV1,
	closedAt time.Time,
) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	if service == nil || service.authority == nil || service.store == nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", errors.New("durable service is unavailable"))
	}
	if err := domainsecurity.ValidateTurnSecurityContext(securityContext); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
	}
	if securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", errors.New("turn security context V1 is audit-only"))
	}
	if err := domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(terminalReasonCode); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	turnBinding, err := service.turnBindingHMACV1(ctx, securityContext)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
	}
	if service.restartPreservesBindingV1(turnBinding) {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", ErrRestartPreserved)
	}
	inventory, err := service.trustedInventoryLockedV1(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
	}
	for _, existing := range inventory.closures {
		if existing.TurnBindingHMAC != turnBinding {
			continue
		}
		persisted, readErr := service.store.ReadClosure(ctx, turnBinding)
		if readErr != nil || persisted != existing || service.verifyClosureTrustedV1(ctx, persisted) != nil {
			service.trustedInventory = nil
			return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", errors.New("existing closure is untrusted"))
		}
		if persisted.TerminalReasonCode != terminalReasonCode {
			return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", errors.New("terminal reason conflicts with existing closure"))
		}
		if err := service.rememberClosureLockedV1(persisted); err != nil {
			service.trustedInventory = nil
			return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
		}
		return persisted, nil
	}
	intents, settlements, err := service.turnInventoryV1(ctx, turnBinding)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", err)
	}
	closedAt = closedAt.UTC()
	if closedAt.IsZero() {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("close provider telemetry turn", errors.New("closure time is required"))
	}
	closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC: turnBinding, Intents: intents, Settlements: settlements,
		TerminalReasonCode: terminalReasonCode, ClosedAt: closedAt,
		AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("sign provider telemetry turn closure", err)
	}
	persisted, err := service.store.CommitClosureIfAbsent(ctx, closure)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("commit provider telemetry turn closure", err)
	}
	if persisted.TurnBindingHMAC != turnBinding || service.verifyClosureTrustedV1(ctx, persisted) != nil {
		service.trustedInventory = nil
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("verify provider telemetry turn closure", errors.New("closure commit differs from trusted authority"))
	}
	if persisted.TerminalReasonCode != terminalReasonCode {
		service.trustedInventory = nil
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("verify provider telemetry turn closure", errors.New("terminal reason conflicts with committed closure"))
	}
	if err := service.rememberClosureLockedV1(persisted); err != nil {
		service.trustedInventory = nil
		return domaincachetelemetry.ProviderTurnClosureV1{}, forbidProviderRetryV1("verify provider telemetry turn closure", err)
	}
	return persisted, nil
}

// ObserveTurnClosureV1 derives the private turn binding from the immutable V2
// security context and returns only an already-durable, installation-trusted
// closure. It never creates or repairs provider authority.
func (service *DurableService) ObserveTurnClosureV1(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domaincachetelemetry.ProviderTurnClosureV1, bool, error) {
	if service == nil || service.authority == nil || service.store == nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, forbidProviderRetryV1("observe provider telemetry turn closure", errors.New("durable service is unavailable"))
	}
	if err := domainsecurity.ValidateTurnSecurityContext(securityContext); err != nil || securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 {
		if err == nil {
			err = errors.New("turn security context V1 is audit-only")
		}
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, forbidProviderRetryV1("observe provider telemetry turn closure", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	turnBinding, err := service.turnBindingHMACV1(ctx, securityContext)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, forbidProviderRetryV1("observe provider telemetry turn closure", err)
	}
	closure, err := service.store.ReadClosure(ctx, turnBinding)
	if errors.Is(err, cachetelemetrystoreport.ErrNotFound) {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, nil
	}
	if err != nil || service.verifyClosureTrustedV1(ctx, closure) != nil || closure.TurnBindingHMAC != turnBinding {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, forbidProviderRetryV1("observe provider telemetry turn closure", errors.Join(err, errors.New("provider turn closure is untrusted or mismatched")))
	}
	return closure, true, nil
}

// VisitTurnClosuresV1 exposes a complete, deterministic and
// installation-trusted closure inventory to the terminal recovery owner. It
// deliberately omits provider request material and never creates or repairs
// authority while visiting.
func (service *DurableService) VisitTurnClosuresV1(
	ctx context.Context,
	visit func(domaincachetelemetry.ProviderTurnClosureV1) error,
) error {
	if service == nil || service.authority == nil || service.store == nil || visit == nil {
		return forbidProviderRetryV1("visit provider telemetry turn closures", errors.New("durable service or visitor is unavailable"))
	}
	service.mu.Lock()
	if err := contextErrorV1(ctx); err != nil {
		service.mu.Unlock()
		return forbidProviderRetryV1("visit provider telemetry turn closures", err)
	}
	closures := make([]domaincachetelemetry.ProviderTurnClosureV1, 0)
	if err := service.store.VisitClosures(ctx, func(closure domaincachetelemetry.ProviderTurnClosureV1) error {
		if err := service.verifyClosureTrustedV1(ctx, closure); err != nil {
			return err
		}
		closures = append(closures, closure)
		return nil
	}); err != nil {
		service.mu.Unlock()
		return forbidProviderRetryV1("visit provider telemetry turn closures", err)
	}
	sort.Slice(closures, func(i, j int) bool { return closures[i].ClosureID < closures[j].ClosureID })
	service.mu.Unlock()
	for _, closure := range closures {
		if err := visit(closure); err != nil {
			return err
		}
	}
	return nil
}

func (service *DurableService) PlanRestartSettlements(ctx context.Context, observedAt time.Time) (RestartSettlementPlanV1, error) {
	if service == nil || service.authority == nil || service.store == nil {
		return RestartSettlementPlanV1{}, forbidProviderRetryV1("plan provider restart settlements", errors.New("durable service is unavailable"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := contextErrorV1(ctx); err != nil {
		return RestartSettlementPlanV1{}, forbidProviderRetryV1("plan provider restart settlements", err)
	}
	observedAt = observedAt.UTC()
	if observedAt.IsZero() {
		return RestartSettlementPlanV1{}, forbidProviderRetryV1("plan provider restart settlements", errors.New("restart observation time is required"))
	}
	intents, settlements, closures, err := service.fullInventoryV1(ctx)
	if err != nil {
		return RestartSettlementPlanV1{}, forbidProviderRetryV1("plan provider restart settlements", err)
	}
	settled := make(map[string]struct{}, len(settlements))
	for _, settlement := range settlements {
		settled[settlement.IntentID] = struct{}{}
	}
	closed := make(map[string]struct{}, len(closures))
	for _, closure := range closures {
		closed[closure.TurnBindingHMAC] = struct{}{}
	}
	plan := RestartSettlementPlanV1{Settlements: []domaincachetelemetry.ProviderAttemptSettlementV1{}}
	for _, intent := range intents {
		if _, exists := settled[intent.IntentID]; exists {
			continue
		}
		if _, exists := closed[intent.TurnBindingHMAC]; exists {
			return RestartSettlementPlanV1{}, forbidProviderRetryV1("plan provider restart settlements", errors.New("closed provider turn contains an open intent"))
		}
		if service.restartPreservesBindingV1(intent.TurnBindingHMAC) {
			continue
		}
		settledAt := observedAt
		startedAt, _ := time.Parse(time.RFC3339Nano, intent.Shape.StartedAt)
		if settledAt.Before(startedAt) {
			settledAt = startedAt
		}
		observation := domaincachetelemetry.ProviderCallObservationV1{
			SchemaVersion: domaincachetelemetry.ProviderCallObservationV1SchemaVersion,
			Shape:         intent.Shape, Status: domaincachetelemetry.ProviderCallStatusRestartInterrupted,
			Usage: domaincachetelemetry.ProviderUsageV1{}, SettledAt: settledAt.Format(time.RFC3339Nano),
		}
		settlement, err := domaincachetelemetry.NewProviderAttemptSettlementV1(domaincachetelemetry.ProviderAttemptSettlementInputV1{
			Intent: intent, DispatchState: domaincachetelemetry.ProviderDispatchStateIndeterminate,
			Observation: observation, SafeReasonCode: "restart_interrupted",
			AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
		}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
		if err != nil {
			return RestartSettlementPlanV1{}, forbidProviderRetryV1("sign provider restart settlement", err)
		}
		plan.Settlements = append(plan.Settlements, settlement)
	}
	sort.Slice(plan.Settlements, func(i, j int) bool { return plan.Settlements[i].IntentID < plan.Settlements[j].IntentID })
	return plan, nil
}

func (service *DurableService) ApplyRestartSettlements(ctx context.Context, plan RestartSettlementPlanV1) error {
	if service == nil || service.authority == nil || service.store == nil {
		return forbidProviderRetryV1("apply provider restart settlements", errors.New("durable service is unavailable"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := contextErrorV1(ctx); err != nil {
		return forbidProviderRetryV1("apply provider restart settlements", err)
	}
	// Check the whole signed plan before any commit. In particular a plan made
	// before preservation cannot write its unrelated prefix and then fail when
	// it reaches a held intent later in the plan.
	previousID := ""
	for _, settlement := range plan.Settlements {
		if settlement.IntentID <= previousID || settlement.DispatchState != domaincachetelemetry.ProviderDispatchStateIndeterminate ||
			settlement.Observation.Status != domaincachetelemetry.ProviderCallStatusRestartInterrupted ||
			service.verifySettlementTrustedV1(ctx, settlement) != nil {
			return forbidProviderRetryV1("apply provider restart settlements", errors.New("restart settlement plan is invalid"))
		}
		previousID = settlement.IntentID
		intent, err := service.store.ReadIntent(ctx, settlement.IntentID)
		if err != nil || domaincachetelemetry.ValidateProviderAttemptSettlementForIntentV1(settlement, intent) != nil || service.verifyIntentTrustedV1(ctx, intent) != nil {
			return forbidProviderRetryV1("apply provider restart settlements", errors.Join(err, errors.New("restart settlement intent changed")))
		}
		if service.restartPreservesBindingV1(intent.TurnBindingHMAC) {
			return forbidProviderRetryV1("apply provider restart settlements", ErrRestartPreserved)
		}
	}
	for _, settlement := range plan.Settlements {
		if existing, err := service.store.ReadSettlement(ctx, settlement.IntentID); err == nil {
			if existing != settlement || service.verifySettlementTrustedV1(ctx, existing) != nil {
				return forbidProviderRetryV1("apply provider restart settlements", errors.New("restart settlement conflicts with existing authority"))
			}
			continue
		} else if !errors.Is(err, cachetelemetrystoreport.ErrNotFound) {
			return forbidProviderRetryV1("apply provider restart settlements", err)
		}
		persisted, err := service.store.CommitSettlementIfAbsent(ctx, settlement)
		if err != nil {
			return forbidProviderRetryV1("apply provider restart settlements", err)
		}
		if persisted != settlement || service.verifySettlementTrustedV1(ctx, persisted) != nil {
			service.trustedInventory = nil
			return forbidProviderRetryV1("apply provider restart settlements", errors.New("restart settlement read-back differs from signed authority"))
		}
		if err := service.rememberSettlementLockedV1(persisted); err != nil {
			service.trustedInventory = nil
			return forbidProviderRetryV1("apply provider restart settlements", err)
		}
	}
	return nil
}

type providerAttemptDigestsV1 struct {
	turnBindingHMAC     string
	childRunHMAC        string
	logicalCallHMAC     string
	modelHMAC           string
	endpointHMAC        string
	wireBodyHMAC        string
	wireHeadersHMAC     string
	credentialScopeHMAC string
	providerConfigHMAC  string
	usageSource         domaincachetelemetry.ProviderUsageSourceV1
	channel             domaincachetelemetry.ProviderChannelV1
}

func (service *DurableService) deriveAttemptDigestsV1(ctx context.Context, input cachetelemetryport.AttemptRegistrationInputV1) (providerAttemptDigestsV1, error) {
	contextBody, err := json.Marshal(input.SecurityContext)
	if err != nil {
		return providerAttemptDigestsV1{}, errors.New("provider telemetry security context cannot be encoded")
	}
	key, err := service.authority.Sign(ctx, []byte(providerTelemetryKeyDerivationDomain))
	if err != nil || len(key) < sha256.Size {
		return providerAttemptDigestsV1{}, errors.New("provider telemetry HMAC key derivation failed")
	}
	defer zeroBytesV1(key)
	sequence := make([]byte, 8)
	binary.BigEndian.PutUint64(sequence, input.LogicalSequence)
	outerAttempt := make([]byte, 4)
	binary.BigEndian.PutUint32(outerAttempt, input.OuterAttempt)
	laneCallSequence := make([]byte, 4)
	binary.BigEndian.PutUint32(laneCallSequence, input.LaneCallSequence)
	turnBinding := providerTelemetryHMACV1(key, "turn-binding", contextBody)
	childRun := providerTelemetryHMACV1(key, "child-run", input.ChildRunID)
	logicalCall := providerTelemetryHMACV1(
		key, "logical-call", []byte(turnBinding), []byte(input.UsageSource), []byte(childRun), []byte(input.Channel), sequence, outerAttempt, laneCallSequence,
	)
	return providerAttemptDigestsV1{
		turnBindingHMAC: turnBinding, childRunHMAC: childRun, logicalCallHMAC: logicalCall,
		modelHMAC:           providerTelemetryHMACV1(key, "model", input.Model),
		endpointHMAC:        providerTelemetryHMACV1(key, "endpoint", input.Endpoint),
		wireBodyHMAC:        providerTelemetryHMACV1(key, "wire-body", input.WireBody),
		wireHeadersHMAC:     providerTelemetryHMACV1(key, "wire-headers", input.WireHeaders),
		credentialScopeHMAC: providerTelemetryHMACV1(key, "credential-scope", input.CredentialScope),
		providerConfigHMAC:  providerTelemetryHMACV1(key, "provider-config", input.ProviderConfig),
		usageSource:         input.UsageSource, channel: input.Channel,
	}, nil
}

func (service *DurableService) turnBindingHMACV1(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (string, error) {
	body, err := json.Marshal(securityContext)
	if err != nil {
		return "", err
	}
	key, err := service.authority.Sign(ctx, []byte(providerTelemetryKeyDerivationDomain))
	if err != nil || len(key) < sha256.Size {
		return "", errors.New("provider telemetry HMAC key derivation failed")
	}
	defer zeroBytesV1(key)
	return providerTelemetryHMACV1(key, "turn-binding", body), nil
}

func allocateProviderAttemptOrdinalsV1(intents []domaincachetelemetry.ProviderAttemptIntentV1, digests providerAttemptDigestsV1, physicalAttempt uint32) (uint32, uint32, error) {
	logicalOrdinals := make(map[string]uint32)
	channelOrdinals := make(map[string]uint32)
	var existing *domaincachetelemetry.ProviderAttemptIntentV1
	var maxPhysical uint32
	for index := range intents {
		intent := intents[index]
		logicalOrdinals[intent.Shape.LogicalCallHMAC] = intent.LogicalCallOrdinal
		if intent.UsageSource == digests.usageSource && intent.ChildRunHMAC == digests.childRunHMAC && intent.Channel == digests.channel {
			channelOrdinals[intent.Shape.LogicalCallHMAC] = intent.ChannelOrdinal
		}
		if intent.Shape.LogicalCallHMAC == digests.logicalCallHMAC {
			candidate := intent
			if candidate.Shape.Attempt > maxPhysical {
				existing = &candidate
				maxPhysical = candidate.Shape.Attempt
			}
		}
	}
	if existing != nil {
		if physicalAttempt != maxPhysical+1 || existing.UsageSource != digests.usageSource ||
			existing.ChildRunHMAC != digests.childRunHMAC || existing.Channel != digests.channel {
			return 0, 0, errors.New("provider telemetry physical attempt sequence is invalid")
		}
		return existing.LogicalCallOrdinal, existing.ChannelOrdinal, nil
	}
	if physicalAttempt != 1 {
		return 0, 0, errors.New("provider telemetry logical call must start at physical attempt one")
	}
	if len(logicalOrdinals) >= 1_000_000 || len(channelOrdinals) >= 1_000_000 {
		return 0, 0, errors.New("provider telemetry ordinal capacity is exhausted")
	}
	return uint32(len(logicalOrdinals) + 1), uint32(len(channelOrdinals) + 1), nil
}

func (service *DurableService) turnInventoryV1(ctx context.Context, turnBinding string) ([]domaincachetelemetry.ProviderAttemptIntentV1, []domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	inventory, err := service.trustedInventoryLockedV1(ctx)
	if err != nil {
		return nil, nil, err
	}
	turnIntents := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0)
	turnSettlements := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0)
	for _, intent := range inventory.intents {
		if intent.TurnBindingHMAC == turnBinding {
			turnIntents = append(turnIntents, intent)
		}
	}
	for _, settlement := range inventory.settlements {
		if settlement.TurnBindingHMAC == turnBinding {
			turnSettlements = append(turnSettlements, settlement)
		}
	}
	return turnIntents, turnSettlements, nil
}

// trustedInventoryLockedV1 avoids repeating a complete cross-leaf read during
// one live service lifetime. Every cached value was either installation-
// verified by fullInventoryV1 or returned by a secure CAS commit with exact
// read-back. Store commits still rescan the current cross-leaf ledger, so
// out-of-process drift fails closed instead of being authorized by this cache.
func (service *DurableService) trustedInventoryLockedV1(ctx context.Context) (*trustedProviderInventoryV1, error) {
	if service.trustedInventory != nil {
		return service.trustedInventory, nil
	}
	intents, settlements, closures, err := service.fullInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	service.trustedInventory = &trustedProviderInventoryV1{
		intents: intents, settlements: settlements, closures: closures,
	}
	return service.trustedInventory, nil
}

func (service *DurableService) rememberIntentLockedV1(intent domaincachetelemetry.ProviderAttemptIntentV1) error {
	if service.trustedInventory == nil {
		return nil
	}
	for _, existing := range service.trustedInventory.intents {
		if existing.IntentID != intent.IntentID {
			continue
		}
		if existing != intent {
			return errors.New("provider cache telemetry trusted intent cache conflicts with committed authority")
		}
		return nil
	}
	service.trustedInventory.intents = append(service.trustedInventory.intents, intent)
	return nil
}

func (service *DurableService) rememberSettlementLockedV1(settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
	if service.trustedInventory == nil {
		return nil
	}
	for _, existing := range service.trustedInventory.settlements {
		if existing.IntentID != settlement.IntentID {
			continue
		}
		if existing != settlement {
			return errors.New("provider cache telemetry trusted settlement cache conflicts with committed authority")
		}
		return nil
	}
	service.trustedInventory.settlements = append(service.trustedInventory.settlements, settlement)
	return nil
}

func (service *DurableService) rememberClosureLockedV1(closure domaincachetelemetry.ProviderTurnClosureV1) error {
	if service.trustedInventory == nil {
		return nil
	}
	for _, existing := range service.trustedInventory.closures {
		if existing.TurnBindingHMAC != closure.TurnBindingHMAC {
			continue
		}
		if existing != closure {
			return errors.New("provider cache telemetry trusted closure cache conflicts with committed authority")
		}
		return nil
	}
	service.trustedInventory.closures = append(service.trustedInventory.closures, closure)
	return nil
}

func (service *DurableService) fullInventoryV1(ctx context.Context) ([]domaincachetelemetry.ProviderAttemptIntentV1, []domaincachetelemetry.ProviderAttemptSettlementV1, []domaincachetelemetry.ProviderTurnClosureV1, error) {
	intents := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0)
	settlements := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0)
	closures := make([]domaincachetelemetry.ProviderTurnClosureV1, 0)
	if err := service.store.VisitInventory(ctx, func(intent domaincachetelemetry.ProviderAttemptIntentV1) error {
		if err := service.verifyIntentTrustedV1(ctx, intent); err != nil {
			return err
		}
		intents = append(intents, intent)
		return nil
	}, func(settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
		if err := service.verifySettlementTrustedV1(ctx, settlement); err != nil {
			return err
		}
		settlements = append(settlements, settlement)
		return nil
	}, func(closure domaincachetelemetry.ProviderTurnClosureV1) error {
		if err := service.verifyClosureTrustedV1(ctx, closure); err != nil {
			return err
		}
		closures = append(closures, closure)
		return nil
	}); err != nil {
		return nil, nil, nil, err
	}
	return intents, settlements, closures, nil
}

func (service *DurableService) verifyIntentTrustedV1(ctx context.Context, intent domaincachetelemetry.ProviderAttemptIntentV1) error {
	keyID, publicKey, signature, err := domaincachetelemetry.ProviderAttemptIntentV1AuthorityMaterial(intent)
	if err != nil {
		return err
	}
	return service.authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderAttemptIntentV1SigningBytes(intent), signature)
}

func (service *DurableService) verifySettlementTrustedV1(ctx context.Context, settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
	keyID, publicKey, signature, err := domaincachetelemetry.ProviderAttemptSettlementV1AuthorityMaterial(settlement)
	if err != nil {
		return err
	}
	return service.authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderAttemptSettlementV1SigningBytes(settlement), signature)
}

func (service *DurableService) verifyClosureTrustedV1(ctx context.Context, closure domaincachetelemetry.ProviderTurnClosureV1) error {
	keyID, publicKey, signature, err := domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
	if err != nil {
		return err
	}
	return service.authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature)
}

func validateAttemptRegistrationInputV1(input cachetelemetryport.AttemptRegistrationInputV1) error {
	validateSecurityContext := domainsecurity.ValidateTurnSecurityContextForExecution
	if input.OrdinaryEffect {
		validateSecurityContext = domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect
	}
	if err := validateSecurityContext(input.SecurityContext); err != nil {
		return err
	}
	if input.UsageSource != domaincachetelemetry.ProviderUsageSourceTurn && input.UsageSource != domaincachetelemetry.ProviderUsageSourceSubagent {
		return errors.New("provider telemetry usage source is invalid")
	}
	childBound := len(bytes.TrimSpace(input.ChildRunID)) > 0
	if (input.UsageSource == domaincachetelemetry.ProviderUsageSourceSubagent) != childBound {
		return errors.New("provider telemetry usage source and child run binding disagree")
	}
	switch input.Channel {
	case domaincachetelemetry.ProviderChannelPrimary:
		if input.LaneCallSequence != 0 {
			return errors.New("primary provider telemetry lane call sequence must be zero")
		}
	case domaincachetelemetry.ProviderChannelAttachmentVision, domaincachetelemetry.ProviderChannelToolResultVision:
		if input.LaneCallSequence == 0 {
			return errors.New("non-primary provider telemetry lane call sequence is required")
		}
	default:
		return errors.New("provider telemetry channel is invalid")
	}
	if input.LogicalSequence == 0 || input.OuterAttempt == 0 || input.PhysicalAttempt == 0 {
		return errors.New("provider telemetry attempt correlation is invalid")
	}
	switch input.ProviderFamily {
	case domaincachetelemetry.ProviderFamilyDeepSeek, domaincachetelemetry.ProviderFamilyOpenAICompatible,
		domaincachetelemetry.ProviderFamilyAnthropicCompatible, domaincachetelemetry.ProviderFamilyCustomEndpoint:
	default:
		return errors.New("provider telemetry family is invalid")
	}
	switch input.EndpointFormat {
	case domaincachetelemetry.EndpointFormatChatCompletions, domaincachetelemetry.EndpointFormatResponses,
		domaincachetelemetry.EndpointFormatMessages, domaincachetelemetry.EndpointFormatCustomEndpoint:
	default:
		return errors.New("provider telemetry endpoint format is invalid")
	}
	if input.StartedAt.IsZero() || len(input.Model) == 0 || len(input.Model) > maxProviderTelemetryIdentityBytes ||
		len(input.Endpoint) == 0 || len(input.Endpoint) > maxProviderTelemetryIdentityBytes ||
		len(input.WireBody) == 0 || len(input.WireBody) > maxProviderTelemetryWireBodyBytes ||
		len(input.WireHeaders) == 0 || len(input.WireHeaders) > maxProviderTelemetryHeadersBytes ||
		len(input.CredentialScope) == 0 || len(input.CredentialScope) > maxProviderTelemetryIdentityBytes ||
		len(input.ProviderConfig) == 0 || len(input.ProviderConfig) > maxProviderTelemetryConfigBytes ||
		len(input.ChildRunID) > maxProviderTelemetryIdentityBytes {
		return errors.New("provider telemetry wire identity is invalid")
	}
	return nil
}

func providerTelemetryHMACV1(key []byte, label string, parts ...[]byte) string {
	return domaincachetelemetry.ProviderTelemetryHMACV1(key, label, parts...)
}

func providerTelemetryDigestEpochV1(keyID string) uint64 {
	digest := sha256.Sum256([]byte(providerTelemetryDigestEpochDomain + strings.TrimSpace(keyID)))
	epoch := binary.BigEndian.Uint64(digest[:8])
	if epoch == 0 {
		return 1
	}
	return epoch
}

func zeroBytesV1(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return errors.New("provider telemetry context is required")
	}
	return ctx.Err()
}

func forbidProviderRetryV1(operation string, err error) error {
	return RetryForbiddenError{Operation: strings.TrimSpace(operation), Err: err}
}
