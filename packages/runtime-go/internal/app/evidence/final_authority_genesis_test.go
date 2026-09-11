package evidence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// This reader has no unavailable-domain marker: a live outage cannot silently
// reclassify a trusted historical final as audit-only.
type failingLiveHistoricalReplayV1 struct{ calls int }

type originalBoundaryReplayTestV1 struct {
	failingLiveHistoricalReplayV1
	snapshot      domainevidence.EvidenceReceiptRegistry
	err           error
	originalCalls int
	unavailable   bool
}

func (reader *originalBoundaryReplayTestV1) ReplayOriginalAcceptedFinalV1(context.Context, domainevidence.PrivateAcceptedFinalRecord) (domainevidence.EvidenceReceiptRegistry, error) {
	reader.originalCalls++
	return reader.snapshot, reader.err
}

func (reader *originalBoundaryReplayTestV1) CaseEvidenceAuthorityUnavailableV1() bool {
	return reader.unavailable
}

type cancelAfterFinalVerificationV1 struct {
	authorityport.Authority
	cancel context.CancelFunc
}

func (authority cancelAfterFinalVerificationV1) VerifyTrusted(ctx context.Context, key string, public, body, signature []byte) error {
	if err := authority.Authority.VerifyTrusted(ctx, key, public, body, signature); err != nil {
		return err
	}
	authority.cancel()
	return nil
}

func (reader *failingLiveHistoricalReplayV1) ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	reader.calls++
	return domainevidence.EvidenceReceiptRegistry{}, errors.New("synthetic live witness unavailable")
}

func TestHistoricalGenesisFinalDoesNotDependOnLiveWitness(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		name := "signed-genesis"
		if nonempty {
			name = "nonzero-history-must-not-use-genesis"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			issuer, input := evidenceIssuerFixture(t)
			finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
			if nonempty {
				issuer.Registry = registry.memoryEvidenceRegistry
				seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
			}
			store := &caseTerminalStoreStub{}
			result, err := finalizer.PersistBoundary(ctx, PersistCaseBoundaryInput{Store: store, Context: input.Context, TerminalReason: TerminalSourceUnavailable, SourceUnavailable: true, ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime()})
			if err != nil || !result.Persistence.Changed {
				t.Fatalf("real boundary fixture failed: %v", err)
			}
			records, err := privateStore.List(ctx)
			if err != nil || len(records) != 1 || (records[0].RegistryHead.Sequence != 0) != nonempty {
				t.Fatal("boundary fixture has the wrong signed historical prefix")
			}
			reader := acceptedFinalReaderForStore(input.Context, store)
			live := &failingLiveHistoricalReplayV1{}
			beforeEvents := cloneEventMaps(store.events)
			beforePuts, beforeFinishes := privateStore.putCalls, store.finishCalls
			inventory, err := PreflightFinalAuthorityInventory(ctx, reader, reader, live, authority, privateStore)
			if nonempty {
				if err == nil || live.calls != 1 {
					t.Fatalf("nonzero history fell back to genesis: calls=%d err=%v", live.calls, err)
				}
			} else if err != nil || live.calls != 0 || len(inventory.Committed) != 1 || len(inventory.AuditOnlyPublicWinners) != 0 || inventory.Committed[0].AcceptedFinal.RecordDigest != records[0].AcceptedFinal.RecordDigest {
				t.Fatalf("signed genesis history depended on live witness or changed classification: calls=%d err=%v", live.calls, err)
			}
			if privateStore.putCalls != beforePuts || store.finishCalls != beforeFinishes || !reflect.DeepEqual(beforeEvents, store.events) {
				t.Fatal("historical validation wrote final authority")
			}
			if nonempty {
				snapshot, err := registry.ReplayAt(ctx, records[0].SecurityContext, records[0].RegistryHead.Sequence)
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"original-prefix", "current-unavailable", "original-failure-no-live-fallback", "wrong-returned-head"} {
					t.Run(name, func(t *testing.T) {
						original := &originalBoundaryReplayTestV1{snapshot: snapshot, unavailable: name == "current-unavailable"}
						if name == "original-failure-no-live-fallback" {
							original.err = errors.New("original proof unavailable")
						}
						if name == "wrong-returned-head" {
							original.snapshot, _ = domainevidence.NewEvidenceReceiptRegistry(records[0].SecurityContext)
						}
						inventory, err := PreflightFinalAuthorityInventory(ctx, reader, reader, original, authority, privateStore)
						if name == "original-prefix" || name == "current-unavailable" {
							if err != nil || len(inventory.Committed) != 1 || len(inventory.AuditOnlyPublicWinners) != 0 {
								t.Fatalf("Original boundary changed classification: %v", err)
							}
						} else if err == nil {
							t.Fatal("unsupported Original proof was accepted")
						}
						if original.calls != 0 || original.originalCalls != 1 {
							t.Fatal("Original proof fell back or was bypassed")
						}
					})
				}
				return
			}
			// A new current-key signature cannot make another digest the unique
			// genesis prefix. The ordinary signature/record grammar remains valid.
			original := records[0]
			wrongHead := original.RegistryHead
			wrongHead.StateDigest = domainsecurity.SHA256Hex([]byte("not the context-bound genesis prefix"))
			acceptedAt, err := time.Parse(time.RFC3339Nano, original.AcceptedFinal.AcceptedAt)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{Context: original.SecurityContext, Envelope: original.Envelope, RenderedText: original.RenderedText, RegistryHead: wrongHead, PrivateRecordDigest: original.PrivateRecordDigest, AcceptedAt: acceptedAt, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey()}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
			if err != nil {
				t.Fatal(err)
			}
			forged, err := domainevidence.NewPrivateAcceptedFinalRecord(original.SecurityContext, original.Envelope, original.RenderedText, wrongHead, original.PublicationIntent, accepted)
			if err != nil || verifyTrustedPrivateFinalInstallationV1(ctx, authority, forged) != nil {
				t.Fatalf("wrong-genesis control lost its genuine current-key signature: %v", err)
			}
			if err := verifyTrustedPrivateFinal(ctx, live, authority, forged); err == nil || !strings.Contains(err.Error(), "registry head is unavailable or inconsistent") || live.calls != 0 {
				t.Fatalf("wrong signed genesis did not reach exact prefix binding: calls=%d err=%v", live.calls, err)
			}
			if err := verifyTrustedPrivateFinal(ctx, live, newMemoryFinalAuthority(2), original); err == nil || live.calls != 0 {
				t.Fatal("historical genesis bypassed the independent current key")
			}
			if err := verifyTrustedPrivateFinal(nil, live, authority, original); err == nil || live.calls != 0 {
				t.Fatal("historical genesis accepted a missing context")
			}
			cancelled, cancel := context.WithCancel(ctx)
			defer cancel()
			if err := verifyTrustedPrivateFinal(cancelled, live, cancelAfterFinalVerificationV1{Authority: authority, cancel: cancel}, original); !errors.Is(err, context.Canceled) || live.calls != 0 {
				t.Fatalf("genesis verification lost cancellation after actual signature verification: %v", err)
			}
		})
	}
}
