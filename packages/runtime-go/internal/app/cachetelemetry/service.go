package cachetelemetry

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

var (
	ErrLedgerConflict    = errors.New("cache telemetry ledger conflict")
	ErrLedgerIncomplete  = errors.New("cache telemetry ledger is incomplete")
	ErrLedgerPoisoned    = errors.New("cache telemetry ledger is poisoned")
	ErrAttemptSequence   = errors.New("cache telemetry attempt sequence is invalid")
	ErrAggregateOverflow = errors.New("cache telemetry aggregate overflows uint64")
	ErrIncompatibleShape = errors.New("cache telemetry shapes are not comparable")
)

type attemptKey struct {
	logicalCallHMAC string
	attempt         uint32
}

type attemptEntry struct {
	shape       domaincache.CacheVisibleShapeV1
	observation *domaincache.ProviderCallObservationV1
}

// Service is an in-memory fail-closed ledger core. Production adapters are
// expected to register an attempt after serializing its wire body and settle it
// on every terminal path. Aggregate and Snapshot reject any pending attempt.
type Service struct {
	mu       sync.Mutex
	entries  map[attemptKey]*attemptEntry
	poisoned error
}

type AggregatedTokenCountV1 struct {
	Value                 uint64
	KnownObservationCount uint64
	ObservationCount      uint64
	Complete              bool
}

type ProviderStatusCountsV1 struct {
	Succeeded          uint64
	Failed             uint64
	Cancelled          uint64
	TimedOut           uint64
	StreamAborted      uint64
	RestartInterrupted uint64
}

type CacheRateV1 struct {
	Known       bool
	Numerator   uint64
	Denominator uint64
}

type AggregateV1 struct {
	CostKnownAttempts, CostUSDNanos, CostCNYNanos uint64
	CostKnownUSDAttempts, CostKnownCNYAttempts    uint64
	LogicalCallCount                              uint64
	AttemptCount                                  uint64
	Statuses                                      ProviderStatusCountsV1
	InputTokens                                   AggregatedTokenCountV1
	OutputTokens                                  AggregatedTokenCountV1
	CacheHitTokens                                AggregatedTokenCountV1
	CacheMissTokens                               AggregatedTokenCountV1
	ReasoningTokens                               AggregatedTokenCountV1
	CacheRate                                     CacheRateV1
}

type SnapshotV1 struct {
	Observations []domaincache.ProviderCallObservationV1
	Aggregate    AggregateV1
}

func NewService() *Service {
	return &Service{entries: make(map[attemptKey]*attemptEntry)}
}

// Begin registers exactly one safe wire shape. An exact duplicate is
// idempotent. A conflicting duplicate, a skipped attempt, or starting a later
// attempt before the previous one settles permanently poisons the ledger.
func (service *Service) Begin(shape domaincache.CacheVisibleShapeV1) error {
	if err := shape.Validate(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.poisonedErrorLocked(); err != nil {
		return err
	}
	service.ensureEntriesLocked()
	key := attemptKey{logicalCallHMAC: shape.LogicalCallHMAC, attempt: shape.Attempt}
	if existing, ok := service.entries[key]; ok {
		if existing.shape == shape {
			return nil
		}
		return service.poisonLocked(fmt.Errorf("%w: shape changed for %s attempt %d", ErrLedgerConflict, shape.LogicalCallHMAC, shape.Attempt))
	}

	var previous *attemptEntry
	var maxAttempt uint32
	for candidateKey, entry := range service.entries {
		if candidateKey.logicalCallHMAC != shape.LogicalCallHMAC || candidateKey.attempt <= maxAttempt {
			continue
		}
		maxAttempt = candidateKey.attempt
		previous = entry
	}
	if shape.Attempt != maxAttempt+1 {
		return service.poisonLocked(fmt.Errorf("%w: %s expected attempt %d, got %d", ErrAttemptSequence, shape.LogicalCallHMAC, maxAttempt+1, shape.Attempt))
	}
	if previous != nil {
		if previous.observation == nil {
			return service.poisonLocked(fmt.Errorf("%w: %s attempt %d is still pending", ErrLedgerIncomplete, shape.LogicalCallHMAC, maxAttempt))
		}
		if !sameLogicalCallBinding(previous.shape, shape) {
			return service.poisonLocked(fmt.Errorf("%w: logical call binding changed for %s", ErrLedgerConflict, shape.LogicalCallHMAC))
		}
		previousSettledAt, _ := time.Parse(time.RFC3339Nano, previous.observation.SettledAt)
		startedAt, _ := time.Parse(time.RFC3339Nano, shape.StartedAt)
		if startedAt.Before(previousSettledAt) {
			return service.poisonLocked(fmt.Errorf("%w: %s attempt %d starts before attempt %d settles", ErrAttemptSequence, shape.LogicalCallHMAC, shape.Attempt, maxAttempt))
		}
	}
	service.entries[key] = &attemptEntry{shape: shape}
	return nil
}

// Settle records a terminal observation only for a previously registered
// attempt. Exact duplicate delivery is idempotent; any divergent replay poisons
// the ledger and prevents aggregation.
func (service *Service) Settle(observation domaincache.ProviderCallObservationV1) error {
	if err := observation.Validate(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.poisonedErrorLocked(); err != nil {
		return err
	}
	service.ensureEntriesLocked()
	key := attemptKey{logicalCallHMAC: observation.Shape.LogicalCallHMAC, attempt: observation.Shape.Attempt}
	entry, ok := service.entries[key]
	if !ok {
		return service.poisonLocked(fmt.Errorf("%w: settlement has no registered shape for %s attempt %d", ErrLedgerConflict, key.logicalCallHMAC, key.attempt))
	}
	if entry.shape != observation.Shape {
		return service.poisonLocked(fmt.Errorf("%w: settlement shape changed for %s attempt %d", ErrLedgerConflict, key.logicalCallHMAC, key.attempt))
	}
	if entry.observation != nil {
		if *entry.observation == observation {
			return nil
		}
		return service.poisonLocked(fmt.Errorf("%w: settlement changed for %s attempt %d", ErrLedgerConflict, key.logicalCallHMAC, key.attempt))
	}
	copy := observation
	entry.observation = &copy
	return nil
}

func (service *Service) Aggregate() (AggregateV1, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.poisonedErrorLocked(); err != nil {
		return AggregateV1{}, err
	}
	observations, err := service.completeObservationsLocked()
	if err != nil {
		return AggregateV1{}, err
	}
	return aggregateObservations(observations)
}

func (service *Service) Snapshot() (SnapshotV1, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.poisonedErrorLocked(); err != nil {
		return SnapshotV1{}, err
	}
	observations, err := service.completeObservationsLocked()
	if err != nil {
		return SnapshotV1{}, err
	}
	aggregate, err := aggregateObservations(observations)
	if err != nil {
		return SnapshotV1{}, err
	}
	return SnapshotV1{Observations: observations, Aggregate: aggregate}, nil
}

func (service *Service) completeObservationsLocked() ([]domaincache.ProviderCallObservationV1, error) {
	observations := make([]domaincache.ProviderCallObservationV1, 0, len(service.entries))
	for key, entry := range service.entries {
		if entry.observation == nil {
			return nil, fmt.Errorf("%w: %s attempt %d has no terminal settlement", ErrLedgerIncomplete, key.logicalCallHMAC, key.attempt)
		}
		observations = append(observations, *entry.observation)
	}
	sort.Slice(observations, func(left, right int) bool {
		if observations[left].Shape.LogicalCallHMAC == observations[right].Shape.LogicalCallHMAC {
			return observations[left].Shape.Attempt < observations[right].Shape.Attempt
		}
		return observations[left].Shape.LogicalCallHMAC < observations[right].Shape.LogicalCallHMAC
	})
	return observations, nil
}

func aggregateObservations(observations []domaincache.ProviderCallObservationV1) (AggregateV1, error) {
	aggregate := AggregateV1{AttemptCount: uint64(len(observations))}
	logicalCalls := make(map[string]struct{})
	var comparisonShape *domaincache.CacheVisibleShapeV1
	for _, observation := range observations {
		if comparisonShape == nil {
			shape := observation.Shape
			comparisonShape = &shape
		} else if !sameComparisonBinding(*comparisonShape, observation.Shape) {
			return AggregateV1{}, fmt.Errorf(
				"%w: %s attempt %d changes provider, model, endpoint, or digest epoch",
				ErrIncompatibleShape,
				observation.Shape.LogicalCallHMAC,
				observation.Shape.Attempt,
			)
		}
		if cost := observation.Usage.EstimatedCost; cost.Known {
			aggregate.CostKnownAttempts++
			total := &aggregate.CostUSDNanos
			if cost.Currency == "CNY" {
				total = &aggregate.CostCNYNanos
				aggregate.CostKnownCNYAttempts++
			} else {
				aggregate.CostKnownUSDAttempts++
			}
			if *total > 9007199254740991-cost.NanoUnits {
				return AggregateV1{}, ErrAggregateOverflow
			}
			*total += cost.NanoUnits
		}
		logicalCalls[observation.Shape.LogicalCallHMAC] = struct{}{}
		if err := addStatus(&aggregate.Statuses, observation.Status); err != nil {
			return AggregateV1{}, err
		}
		if err := addTokenCount(&aggregate.InputTokens, observation.Usage.InputTokens); err != nil {
			return AggregateV1{}, err
		}
		if err := addTokenCount(&aggregate.OutputTokens, observation.Usage.OutputTokens); err != nil {
			return AggregateV1{}, err
		}
		if err := addTokenCount(&aggregate.CacheHitTokens, observation.Usage.CacheHitTokens); err != nil {
			return AggregateV1{}, err
		}
		if err := addTokenCount(&aggregate.CacheMissTokens, observation.Usage.CacheMissTokens); err != nil {
			return AggregateV1{}, err
		}
		if err := addTokenCount(&aggregate.ReasoningTokens, observation.Usage.ReasoningTokens); err != nil {
			return AggregateV1{}, err
		}
	}
	aggregate.LogicalCallCount = uint64(len(logicalCalls))
	finalizeTokenCount(&aggregate.InputTokens)
	finalizeTokenCount(&aggregate.OutputTokens)
	finalizeTokenCount(&aggregate.CacheHitTokens)
	finalizeTokenCount(&aggregate.CacheMissTokens)
	finalizeTokenCount(&aggregate.ReasoningTokens)
	if aggregate.CacheHitTokens.Complete && aggregate.CacheMissTokens.Complete {
		denominator, ok := checkedAdd(aggregate.CacheHitTokens.Value, aggregate.CacheMissTokens.Value)
		if !ok {
			return AggregateV1{}, ErrAggregateOverflow
		}
		if denominator > 0 {
			aggregate.CacheRate = CacheRateV1{
				Known:       true,
				Numerator:   aggregate.CacheHitTokens.Value,
				Denominator: denominator,
			}
		}
	}
	return aggregate, nil
}

func addTokenCount(aggregate *AggregatedTokenCountV1, count domaincache.TokenCountV1) error {
	if aggregate.ObservationCount == math.MaxUint64 {
		return ErrAggregateOverflow
	}
	aggregate.ObservationCount++
	if !count.Known {
		return nil
	}
	value, ok := checkedAdd(aggregate.Value, count.Value)
	if !ok || aggregate.KnownObservationCount == math.MaxUint64 {
		return ErrAggregateOverflow
	}
	aggregate.Value = value
	aggregate.KnownObservationCount++
	return nil
}

func finalizeTokenCount(count *AggregatedTokenCountV1) {
	count.Complete = count.KnownObservationCount == count.ObservationCount
}

func addStatus(counts *ProviderStatusCountsV1, status domaincache.ProviderCallStatusV1) error {
	var target *uint64
	switch status {
	case domaincache.ProviderCallStatusSucceeded:
		target = &counts.Succeeded
	case domaincache.ProviderCallStatusFailed:
		target = &counts.Failed
	case domaincache.ProviderCallStatusCancelled:
		target = &counts.Cancelled
	case domaincache.ProviderCallStatusTimedOut:
		target = &counts.TimedOut
	case domaincache.ProviderCallStatusStreamAborted:
		target = &counts.StreamAborted
	case domaincache.ProviderCallStatusRestartInterrupted:
		target = &counts.RestartInterrupted
	default:
		return errors.New("cache telemetry observation has unknown terminal status")
	}
	if *target == math.MaxUint64 {
		return ErrAggregateOverflow
	}
	(*target)++
	return nil
}

func checkedAdd(left, right uint64) (uint64, bool) {
	if math.MaxUint64-left < right {
		return 0, false
	}
	return left + right, true
}

func sameLogicalCallBinding(left, right domaincache.CacheVisibleShapeV1) bool {
	return left.LogicalCallHMAC == right.LogicalCallHMAC &&
		left.WireBodyHMAC == right.WireBodyHMAC &&
		left.StepBindingHMAC == right.StepBindingHMAC && left.StepStartedAt == right.StepStartedAt &&
		left.PublicAttribution == right.PublicAttribution &&
		sameComparisonBinding(left, right)
}

func sameComparisonBinding(left, right domaincache.CacheVisibleShapeV1) bool {
	return left.ProviderFamily == right.ProviderFamily &&
		left.ModelHMAC == right.ModelHMAC &&
		left.Endpoint == right.Endpoint &&
		left.EndpointHMAC == right.EndpointHMAC &&
		left.CredentialScopeHMAC == right.CredentialScopeHMAC &&
		left.ProviderConfigHMAC == right.ProviderConfigHMAC &&
		left.DigestEpoch == right.DigestEpoch
}

func (service *Service) ensureEntriesLocked() {
	if service.entries == nil {
		service.entries = make(map[attemptKey]*attemptEntry)
	}
}

func (service *Service) poisonLocked(cause error) error {
	if service.poisoned == nil {
		service.poisoned = cause
	}
	return errors.Join(ErrLedgerPoisoned, service.poisoned)
}

func (service *Service) poisonedErrorLocked() error {
	if service.poisoned == nil {
		return nil
	}
	return errors.Join(ErrLedgerPoisoned, service.poisoned)
}

// ReportedTokenCountV1 retains the reported subtotal even when other started
// attempts have no usage. Complete is coverage, not a substitute for presence.
type ReportedTokenCountV1 struct {
	QualifiedAttemptCount uint64 `json:"-"`
	ReportedValue         uint64 `json:"reportedValue"`
	KnownAttemptCount     uint64 `json:"knownAttemptCount"`
	AttemptCount          uint64 `json:"attemptCount"`
	Complete              bool   `json:"complete"`
}

// ProviderBillingV1 is a numeric read projection of the existing signed ledger.
// It does not establish a cache-comparable namespace or change legacy totals.
type ProviderBillingV1 struct {
	InventoryComplete   bool                                      `json:"inventoryComplete"`
	Closed              bool                                      `json:"closed"`
	AttemptCount        uint64                                    `json:"attemptCount"`
	SettledAttemptCount uint64                                    `json:"settledAttemptCount"`
	InputTokens         ReportedTokenCountV1                      `json:"inputTokens"`
	OutputTokens        ReportedTokenCountV1                      `json:"outputTokens"`
	UncachedInputTokens ReportedTokenCountV1                      `json:"uncachedInputTokens"`
	CacheReadTokens     ReportedTokenCountV1                      `json:"cacheReadTokens"`
	CacheWriteTokens    ReportedTokenCountV1                      `json:"cacheWriteTokens"`
	ReasoningTokens     ReportedTokenCountV1                      `json:"reasoningTokens"`
	TotalTokens         ReportedTokenCountV1                      `json:"totalTokens"`
	Routes              []domaincache.ProviderPublicAttributionV1 `json:"routes,omitempty"`
}

// TurnInventoryV1 can only be authored by the durable service after a fresh,
// installation-trusted inventory read. The zero value is unavailable, not a
// verified empty ledger. No private identities have a public representation.
type TurnInventoryV1 struct {
	observed    bool
	turnBinding string
	threadID    string
	turnID      string
	intents     []domaincache.ProviderAttemptIntentV1
	settlements []domaincache.ProviderAttemptSettlementV1
	closure     domaincache.ProviderTurnClosureV1
	closed      bool
}

func (inventory TurnInventoryV1) BillingV1() (ProviderBillingV1, error) {
	if !inventory.observed {
		return ProviderBillingV1{}, errors.New("provider billing inventory is unavailable")
	}
	if err := domaincache.ValidateProviderLedgerOpenInventoryV1(inventory.turnBinding, inventory.intents, inventory.settlements); err != nil {
		return ProviderBillingV1{}, err
	}
	if inventory.closed {
		if err := domaincache.ValidateProviderTurnClosureForInventoryV1(inventory.closure, inventory.intents, inventory.settlements); err != nil {
			return ProviderBillingV1{}, err
		}
	}
	n := uint64(len(inventory.intents))
	result := ProviderBillingV1{InventoryComplete: true, Closed: inventory.closed, AttemptCount: n, SettledAttemptCount: uint64(len(inventory.settlements))}
	seenRoutes := map[domaincache.ProviderPublicAttributionV1]bool{}
	routesComplete := true
	for _, intent := range inventory.intents {
		route := intent.Shape.PublicAttribution
		if route == (domaincache.ProviderPublicAttributionV1{}) {
			routesComplete = false
		}
		if route != (domaincache.ProviderPublicAttributionV1{}) && !seenRoutes[route] {
			seenRoutes[route] = true
			result.Routes = append(result.Routes, route)
		}
	}
	sort.Slice(result.Routes, func(i, j int) bool {
		if result.Routes[i].Provider == result.Routes[j].Provider {
			return result.Routes[i].Model < result.Routes[j].Model
		}
		return result.Routes[i].Provider < result.Routes[j].Provider
	})
	if !routesComplete {
		result.Routes = nil
	}
	for _, settlement := range inventory.settlements {
		usage := settlement.Observation.Usage
		if usage.ReasoningTokens.Known && usage.OutputTokens.Known && usage.ReasoningTokens.Value > usage.OutputTokens.Value {
			return ProviderBillingV1{}, errors.New("provider reasoning exceeds output tokens")
		}
		read := usage.MessagesInput.Read
		if usage.MessagesInput.SchemaVersion == "" {
			read = usage.CacheHitTokens
		}
		pairs := []struct {
			target *ReportedTokenCountV1
			source domaincache.TokenCountV1
		}{
			{&result.InputTokens, usage.InputTokens}, {&result.OutputTokens, usage.OutputTokens},
			{&result.UncachedInputTokens, usage.MessagesInput.Uncached}, {&result.CacheReadTokens, read},
			{&result.CacheWriteTokens, usage.MessagesInput.Created}, {&result.ReasoningTokens, usage.ReasoningTokens},
		}
		for _, pair := range pairs {
			qualified := true
			if pair.target == &result.OutputTokens {
				qualified = usage.OutputTokensFinality == domaincache.ProviderOutputTokensFinalV1
			}
			if pair.target == &result.ReasoningTokens {
				qualified = usage.ReasoningTokensFinality == domaincache.ProviderOutputTokensFinalV1
			}
			if err := addReportedTokenV1(pair.target, pair.source, qualified); err != nil {
				return ProviderBillingV1{}, err
			}
		}
		// The intersection is per attempt: independent input/output subtotals
		// must not be paired across different attempts. Reasoning is a subset.
		if usage.InputTokens.Known && usage.OutputTokens.Known {
			if usage.InputTokens.Value > 9007199254740991-usage.OutputTokens.Value {
				return ProviderBillingV1{}, ErrAggregateOverflow
			}
			if err := addReportedTokenV1(&result.TotalTokens, domaincache.TokenCountV1{Known: true, Value: usage.InputTokens.Value + usage.OutputTokens.Value}, usage.OutputTokensFinality == domaincache.ProviderOutputTokensFinalV1); err != nil {
				return ProviderBillingV1{}, err
			}
		}
	}
	for _, count := range []*ReportedTokenCountV1{&result.InputTokens, &result.OutputTokens, &result.UncachedInputTokens, &result.CacheReadTokens, &result.CacheWriteTokens, &result.ReasoningTokens, &result.TotalTokens} {
		count.AttemptCount = n
		// A fresh open inventory can cover its current nonempty attempt set.
		// Only an exact zero-intent closure establishes an explicitly empty turn.
		count.Complete = count.QualifiedAttemptCount == n && (n > 0 || inventory.closed)
	}
	return result, nil
}

func addReportedTokenV1(target *ReportedTokenCountV1, source domaincache.TokenCountV1, qualified bool) error {
	if !source.Known {
		return nil
	}
	const maxPublicInteger uint64 = 9007199254740991
	if source.Value > maxPublicInteger || target.ReportedValue > maxPublicInteger-source.Value {
		return ErrAggregateOverflow
	}
	target.ReportedValue += source.Value
	target.KnownAttemptCount++
	if qualified {
		target.QualifiedAttemptCount++
	}
	return nil
}

// VisitAttemptsV1 supplies internal metadata to a read projection. The visitor
// receives copies, and a nil settlement explicitly marks a pending intent.
func (inventory TurnInventoryV1) VisitAttemptsV1(visit func(domaincache.ProviderAttemptIntentV1, *domaincache.ProviderAttemptSettlementV1) error) error {
	if visit == nil {
		return errors.New("provider billing visitor is unavailable")
	}
	if _, err := inventory.BillingV1(); err != nil {
		return err
	}
	settlements := make(map[string]domaincache.ProviderAttemptSettlementV1, len(inventory.settlements))
	for _, settlement := range inventory.settlements {
		settlements[settlement.IntentID] = settlement
	}
	for _, intent := range inventory.intents {
		var settled *domaincache.ProviderAttemptSettlementV1
		if value, ok := settlements[intent.IntentID]; ok {
			copy := value
			settled = &copy
		}
		if err := visit(intent, settled); err != nil {
			return err
		}
	}
	return nil
}

// ProjectionScopeV1 is an internal membership proof, never a public DTO.
func (inventory TurnInventoryV1) ProjectionScopeV1() (threadID, turnID, binding string, available bool) {
	return inventory.threadID, inventory.turnID, inventory.turnBinding, inventory.observed
}
