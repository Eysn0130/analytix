package cachetelemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

func TestServiceAggregatesEveryLogicalCallAndAttempt(t *testing.T) {
	service := NewService()
	first := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 40, 60)
	second := testObservation("call-a", 2, "2026-07-13T00:00:01Z", "2026-07-13T00:00:02Z", domaincache.ProviderCallStatusSucceeded, 80, 20)
	third := testObservation("call-b", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:03Z", domaincache.ProviderCallStatusStreamAborted, 0, 100)

	for _, observation := range []domaincache.ProviderCallObservationV1{first, second, third} {
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Observations) != 3 || snapshot.Observations[0].Shape.LogicalCallHMAC != testLogicalCallHMAC("call-a") || snapshot.Observations[0].Shape.Attempt != 1 ||
		snapshot.Observations[1].Shape.LogicalCallHMAC != testLogicalCallHMAC("call-a") || snapshot.Observations[1].Shape.Attempt != 2 ||
		snapshot.Observations[2].Shape.LogicalCallHMAC != testLogicalCallHMAC("call-b") {
		t.Fatalf("snapshot is not deterministically ordered: %#v", snapshot.Observations)
	}
	aggregate := snapshot.Aggregate
	if aggregate.LogicalCallCount != 2 || aggregate.AttemptCount != 3 {
		t.Fatalf("unexpected call counts: %#v", aggregate)
	}
	if aggregate.Statuses.Failed != 1 || aggregate.Statuses.Succeeded != 1 || aggregate.Statuses.StreamAborted != 1 {
		t.Fatalf("unexpected status counts: %#v", aggregate.Statuses)
	}
	if !aggregate.CacheHitTokens.Complete || aggregate.CacheHitTokens.Value != 120 || aggregate.CacheHitTokens.KnownObservationCount != 3 {
		t.Fatalf("unexpected cache-hit aggregate: %#v", aggregate.CacheHitTokens)
	}
	if !aggregate.CacheMissTokens.Complete || aggregate.CacheMissTokens.Value != 180 {
		t.Fatalf("unexpected cache-miss aggregate: %#v", aggregate.CacheMissTokens)
	}
	if !aggregate.CacheRate.Known || aggregate.CacheRate.Numerator != 120 || aggregate.CacheRate.Denominator != 300 {
		t.Fatalf("cache rate must be an unsmoothed integer ratio, got %#v", aggregate.CacheRate)
	}
	if !aggregate.InputTokens.Complete || aggregate.InputTokens.Value != 300 {
		t.Fatalf("all attempts must contribute to input token totals: %#v", aggregate.InputTokens)
	}
}

func TestServicePreservesUnknownVersusExplicitZero(t *testing.T) {
	service := NewService()
	explicitZero := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 0, 0)
	unknown := testObservation("call-b", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 0, 0)
	unknown.Usage.CacheHitTokens = domaincache.TokenCountV1{Known: false, Value: 0}
	unknown.Usage.CacheMissTokens = domaincache.TokenCountV1{Known: false, Value: 0}

	for _, observation := range []domaincache.ProviderCallObservationV1{explicitZero, unknown} {
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
	}
	aggregate, err := service.Aggregate()
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.CacheHitTokens.Value != 0 || aggregate.CacheHitTokens.KnownObservationCount != 1 ||
		aggregate.CacheHitTokens.ObservationCount != 2 || aggregate.CacheHitTokens.Complete {
		t.Fatalf("unknown counter was collapsed into explicit zero: %#v", aggregate.CacheHitTokens)
	}
	if aggregate.CacheRate.Known {
		t.Fatalf("cache rate must remain unknown when one attempt has unknown usage: %#v", aggregate.CacheRate)
	}
}

func TestServiceRefusesToAggregateIncompatibleDigestEpochs(t *testing.T) {
	service := NewService()
	first := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
	second := testObservation("call-b", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
	second.Shape.DigestEpoch = 2
	for _, observation := range []domaincache.ProviderCallObservationV1{first, second} {
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Aggregate(); !errors.Is(err, ErrIncompatibleShape) {
		t.Fatalf("expected incompatible digest epochs to reject aggregation, got %v", err)
	}
}

func TestServiceExactDuplicateDeliveryIsIdempotent(t *testing.T) {
	service := NewService()
	observation := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
	for range 3 {
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
	}
	aggregate, err := service.Aggregate()
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.AttemptCount != 1 || aggregate.CacheHitTokens.Value != 90 {
		t.Fatalf("idempotent duplicate was counted more than once: %#v", aggregate)
	}
}

func TestServiceConflictPoisonsLedger(t *testing.T) {
	t.Run("shape conflict", func(t *testing.T) {
		service := NewService()
		observation := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		conflict := observation.Shape
		conflict.WireBodyHMAC = strings.Repeat("c", 64)
		err := service.Begin(conflict)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerConflict) {
			t.Fatalf("expected poisoned conflict, got %v", err)
		}
		if _, err := service.Aggregate(); !errors.Is(err, ErrLedgerPoisoned) {
			t.Fatalf("poisoned ledger later aggregated: %v", err)
		}
	})

	t.Run("settlement conflict", func(t *testing.T) {
		service := NewService()
		observation := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
		conflict := observation
		conflict.Status = domaincache.ProviderCallStatusFailed
		err := service.Settle(conflict)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerConflict) {
			t.Fatalf("expected poisoned conflict, got %v", err)
		}
	})

	t.Run("unregistered settlement", func(t *testing.T) {
		service := NewService()
		err := service.Settle(testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 0, 0))
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerConflict) {
			t.Fatalf("expected unregistered settlement to poison ledger, got %v", err)
		}
	})
}

func TestServiceRequiresCompleteOrderedSettlement(t *testing.T) {
	t.Run("pending attempt blocks aggregate but can settle", func(t *testing.T) {
		service := NewService()
		observation := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 0, 100)
		if err := service.Begin(observation.Shape); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Aggregate(); !errors.Is(err, ErrLedgerIncomplete) || errors.Is(err, ErrLedgerPoisoned) {
			t.Fatalf("expected recoverable incomplete ledger, got %v", err)
		}
		if err := service.Settle(observation); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Aggregate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("skipped first attempt poisons ledger", func(t *testing.T) {
		service := NewService()
		shape := testShape("call-a", 2, "2026-07-13T00:00:00Z")
		err := service.Begin(shape)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrAttemptSequence) {
			t.Fatalf("expected skipped attempt to poison ledger, got %v", err)
		}
	})

	t.Run("later attempt cannot start while prior is pending", func(t *testing.T) {
		service := NewService()
		first := testShape("call-a", 1, "2026-07-13T00:00:00Z")
		if err := service.Begin(first); err != nil {
			t.Fatal(err)
		}
		second := testShape("call-a", 2, "2026-07-13T00:00:01Z")
		err := service.Begin(second)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerIncomplete) {
			t.Fatalf("expected pending prior attempt to poison ledger, got %v", err)
		}
	})

	t.Run("logical call binding cannot change between attempts", func(t *testing.T) {
		service := NewService()
		first := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 0, 100)
		if err := service.Begin(first.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(first); err != nil {
			t.Fatal(err)
		}
		second := testShape("call-a", 2, "2026-07-13T00:00:01Z")
		second.ModelHMAC = strings.Repeat("f", 64)
		err := service.Begin(second)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerConflict) {
			t.Fatalf("expected changed binding to poison ledger, got %v", err)
		}
	})

	t.Run("logical call wire body cannot change between attempts", func(t *testing.T) {
		service := NewService()
		first := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusFailed, 0, 100)
		if err := service.Begin(first.Shape); err != nil {
			t.Fatal(err)
		}
		if err := service.Settle(first); err != nil {
			t.Fatal(err)
		}
		second := testShape("call-a", 2, "2026-07-13T00:00:01Z")
		second.WireBodyHMAC = strings.Repeat("c", 64)
		err := service.Begin(second)
		if !errors.Is(err, ErrLedgerPoisoned) || !errors.Is(err, ErrLedgerConflict) {
			t.Fatalf("expected changed wire body to poison logical call, got %v", err)
		}
	})
}

func TestServiceConcurrentExactDuplicatesRemainOneAttempt(t *testing.T) {
	service := NewService()
	observation := testObservation("call-a", 1, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z", domaincache.ProviderCallStatusSucceeded, 90, 10)
	var beginGroup sync.WaitGroup
	beginErrors := make(chan error, 32)
	for range 32 {
		beginGroup.Add(1)
		go func() {
			defer beginGroup.Done()
			beginErrors <- service.Begin(observation.Shape)
		}()
	}
	beginGroup.Wait()
	close(beginErrors)
	for err := range beginErrors {
		if err != nil {
			t.Fatal(err)
		}
	}

	var settleGroup sync.WaitGroup
	settleErrors := make(chan error, 32)
	for range 32 {
		settleGroup.Add(1)
		go func() {
			defer settleGroup.Done()
			settleErrors <- service.Settle(observation)
		}()
	}
	settleGroup.Wait()
	close(settleErrors)
	for err := range settleErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	aggregate, err := service.Aggregate()
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.AttemptCount != 1 {
		t.Fatalf("concurrent duplicate was counted more than once: %#v", aggregate)
	}
}

func testObservation(logicalCallID string, attempt uint32, startedAt, settledAt string, status domaincache.ProviderCallStatusV1, hit, miss uint64) domaincache.ProviderCallObservationV1 {
	return domaincache.ProviderCallObservationV1{
		SchemaVersion: domaincache.ProviderCallObservationV1SchemaVersion,
		Shape:         testShape(logicalCallID, attempt, startedAt),
		Status:        status,
		Usage: domaincache.ProviderUsageV1{
			InputTokens:     domaincache.TokenCountV1{Known: true, Value: hit + miss},
			OutputTokens:    domaincache.TokenCountV1{Known: true, Value: 10},
			CacheHitTokens:  domaincache.TokenCountV1{Known: true, Value: hit},
			CacheMissTokens: domaincache.TokenCountV1{Known: true, Value: miss},
			ReasoningTokens: domaincache.TokenCountV1{Known: false, Value: 0},
		},
		SettledAt: settledAt,
	}
}

func testShape(logicalCallID string, attempt uint32, startedAt string) domaincache.CacheVisibleShapeV1 {
	return domaincache.CacheVisibleShapeV1{
		SchemaVersion:       domaincache.CacheVisibleShapeV1SchemaVersion,
		LogicalCallHMAC:     testLogicalCallHMAC(logicalCallID),
		Attempt:             attempt,
		ProviderFamily:      domaincache.ProviderFamilyDeepSeek,
		ModelHMAC:           strings.Repeat("c", 64),
		Endpoint:            domaincache.EndpointFormatChatCompletions,
		EndpointHMAC:        strings.Repeat("a", 64),
		WireBodyHMAC:        strings.Repeat("b", 64),
		CredentialScopeHMAC: strings.Repeat("d", 64),
		ProviderConfigHMAC:  strings.Repeat("e", 64),
		DigestEpoch:         1,
		StartedAt:           startedAt,
	}
}

func testLogicalCallHMAC(value string) string {
	digest := sha256.Sum256([]byte("cache-telemetry-service-test-hmac:" + value))
	return hex.EncodeToString(digest[:])
}
