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
