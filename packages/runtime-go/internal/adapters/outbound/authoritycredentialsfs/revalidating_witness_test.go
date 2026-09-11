package authoritycredentialsfs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type revalidationAnchorStubV1 struct {
	mu      sync.Mutex
	current authorityanchorport.AnchorV1
}

func (stub *revalidationAnchorStubV1) Load(ctx context.Context) (authorityanchorport.AnchorV1, error) {
	if err := ctx.Err(); err != nil {
		return authorityanchorport.AnchorV1{}, err
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	current := stub.current
	current.AuthorityPublicKey = append([]byte(nil), current.AuthorityPublicKey...)
	return current, nil
}

func (stub *revalidationAnchorStubV1) set(current authorityanchorport.AnchorV1) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.current = current
}

type revalidationClockStubV1 struct {
	mu      sync.Mutex
	current time.Time
}

func (clock *revalidationClockStubV1) now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.current
}

func (clock *revalidationClockStubV1) set(current time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.current = current
}

type revalidationClientStubV1 struct {
	observeCalls int
	advanceCalls int
	resolveCalls int
	observe      func(context.Context) (domainsecurity.MonotonicHeadObservationV1, error)
	advance      func(context.Context) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error)
	resolve      func(context.Context) (domainsecurity.MonotonicHeadMutationResolutionV1, error)
}

type cancelingAnchorStubV1 struct {
	cancel context.CancelFunc
}

func (stub cancelingAnchorStubV1) Load(ctx context.Context) (authorityanchorport.AnchorV1, error) {
	stub.cancel()
	return authorityanchorport.AnchorV1{}, ctx.Err()
}

func (stub *revalidationClientStubV1) Observe(
	ctx context.Context,
	_ domainsecurity.MonotonicHeadObserveRequestV1,
) (domainsecurity.MonotonicHeadObservationV1, error) {
	stub.observeCalls++
	if stub.observe == nil {
		return domainsecurity.MonotonicHeadObservationV1{}, nil
	}
	return stub.observe(ctx)
}

func (stub *revalidationClientStubV1) Advance(
	ctx context.Context,
	_ domainsecurity.MonotonicHeadAdvanceRequestV1,
) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	stub.advanceCalls++
	if stub.advance == nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, nil
	}
	return stub.advance(ctx)
}

func (stub *revalidationClientStubV1) ResolveMutation(
	ctx context.Context,
	_ domainsecurity.MonotonicHeadMutationResolveRequestV1,
) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
	stub.resolveCalls++
	if stub.resolve == nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, nil
	}
	return stub.resolve(ctx)
}

func TestRevalidatingWitnessPreservesUnderlyingDefiniteCancellation(t *testing.T) {
	witness, client, _, _ := newRevalidatingWitnessFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	client.advance = func(context.Context) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
		cancel()
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, context.Canceled
	}
	_, err := witness.Advance(ctx, domainsecurity.MonotonicHeadAdvanceRequestV1{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, monotonicheadport.ErrIndeterminate) {
		t.Fatalf("pre-dispatch cancellation classification = %v", err)
	}
	if client.advanceCalls != 1 {
		t.Fatalf("advance calls = %d", client.advanceCalls)
	}
}

func TestRevalidatingWitnessPreservesCancellationDuringAnchorLookup(t *testing.T) {
	witness, client, _, _ := newRevalidatingWitnessFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	witness.anchor = cancelingAnchorStubV1{cancel: cancel}
	_, err := witness.Observe(ctx, domainsecurity.MonotonicHeadObserveRequestV1{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, monotonicheadport.ErrUnavailable) {
		t.Fatalf("anchor lookup cancellation classification = %v", err)
	}
	if client.observeCalls != 0 {
		t.Fatalf("anchor lookup cancellation reached client: calls=%d", client.observeCalls)
	}
}

func TestRevalidatingWitnessObserveSuccessThenCancellationPreservesContextError(t *testing.T) {
	witness, client, _, _ := newRevalidatingWitnessFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	client.observe = func(context.Context) (domainsecurity.MonotonicHeadObservationV1, error) {
		cancel()
		return domainsecurity.MonotonicHeadObservationV1{}, nil
	}
	_, err := witness.Observe(ctx, domainsecurity.MonotonicHeadObserveRequestV1{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, monotonicheadport.ErrUnavailable) {
		t.Fatalf("post-observe cancellation classification = %v", err)
	}
}

func TestRevalidatingWitnessDiscardsObservationAfterAuthorityChange(t *testing.T) {
	t.Run("anchor rotated", func(t *testing.T) {
		witness, client, anchor, _ := newRevalidatingWitnessFixtureV1(t)
		current, err := anchor.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		client.observe = func(context.Context) (domainsecurity.MonotonicHeadObservationV1, error) {
			current.CurrentManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			anchor.set(current)
			return domainsecurity.MonotonicHeadObservationV1{}, nil
		}
		_, err = witness.Observe(context.Background(), domainsecurity.MonotonicHeadObserveRequestV1{})
		if !errors.Is(err, monotonicheadport.ErrUnavailable) {
			t.Fatalf("post-observe anchor rotation classification = %v", err)
		}
	})
	t.Run("credential expired", func(t *testing.T) {
		witness, client, _, clock := newRevalidatingWitnessFixtureV1(t)
		client.observe = func(context.Context) (domainsecurity.MonotonicHeadObservationV1, error) {
			clock.set(time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC))
			return domainsecurity.MonotonicHeadObservationV1{}, nil
		}
		_, err := witness.Observe(context.Background(), domainsecurity.MonotonicHeadObserveRequestV1{})
		if !errors.Is(err, monotonicheadport.ErrUnavailable) {
			t.Fatalf("post-observe credential expiry classification = %v", err)
		}
	})
}

func TestRevalidatingWitnessDiscardsMutationResolutionAfterAuthorityChange(t *testing.T) {
	witness, client, anchor, _ := newRevalidatingWitnessFixtureV1(t)
	current, err := anchor.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	client.resolve = func(context.Context) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
		current.CurrentManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		anchor.set(current)
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, nil
	}
	_, err = witness.ResolveMutation(context.Background(), domainsecurity.MonotonicHeadMutationResolveRequestV1{})
	if !errors.Is(err, monotonicheadport.ErrUnavailable) || errors.Is(err, monotonicheadport.ErrIndeterminate) {
		t.Fatalf("post-resolution anchor rotation classification = %v", err)
	}
	if client.resolveCalls != 1 {
		t.Fatalf("mutation resolution client calls = %d", client.resolveCalls)
	}
}

func TestRevalidatingWitnessPostAdvanceAuthorityFailureIsIndeterminate(t *testing.T) {
	witness, client, anchor, clock := newRevalidatingWitnessFixtureV1(t)
	t.Run("anchor rotated", func(t *testing.T) {
		original := anchor.current
		client.advance = func(context.Context) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
			changed := original
			changed.CurrentManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			anchor.set(changed)
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, nil
		}
		_, err := witness.Advance(context.Background(), domainsecurity.MonotonicHeadAdvanceRequestV1{})
		if !errors.Is(err, monotonicheadport.ErrIndeterminate) {
			t.Fatalf("post-advance anchor rotation classification = %v", err)
		}
	})

	witness, client, _, clock = newRevalidatingWitnessFixtureV1(t)
	t.Run("credential expired", func(t *testing.T) {
		client.advance = func(context.Context) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
			clock.set(time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC))
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, nil
		}
		_, err := witness.Advance(context.Background(), domainsecurity.MonotonicHeadAdvanceRequestV1{})
		if !errors.Is(err, monotonicheadport.ErrIndeterminate) {
			t.Fatalf("post-advance credential expiry classification = %v", err)
		}
	})
}

func TestRevalidatingWitnessExpiredCredentialBlocksBeforeClient(t *testing.T) {
	witness, client, _, clock := newRevalidatingWitnessFixtureV1(t)
	clock.set(time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC))
	_, err := witness.Observe(context.Background(), domainsecurity.MonotonicHeadObserveRequestV1{})
	if !errors.Is(err, monotonicheadport.ErrUnavailable) {
		t.Fatalf("pre-observe credential expiry classification = %v", err)
	}
	if client.observeCalls != 0 {
		t.Fatalf("expired credential reached client: calls=%d", client.observeCalls)
	}
}

func newRevalidatingWitnessFixtureV1(
	t *testing.T,
) (*revalidatingWitnessV1, *revalidationClientStubV1, *revalidationAnchorStubV1, *revalidationClockStubV1) {
	t.Helper()
	current := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	anchorValue := authorityanchorport.AnchorV1{
		InstallationID: "installation", AuthorityKeyID: "authority-key",
		AuthorityPublicKey:    []byte("authority-public-key"),
		CurrentManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	anchor := &revalidationAnchorStubV1{current: anchorValue}
	clock := &revalidationClockStubV1{current: current}
	client := &revalidationClientStubV1{}
	witness := &revalidatingWitnessV1{
		client: client, anchor: anchor, expected: anchorValue, clock: clock.now,
		validFrom: current.Add(-time.Hour), validUntil: current.Add(24 * time.Hour),
	}
	return witness, client, anchor, clock
}
