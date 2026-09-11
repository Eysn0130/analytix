package casethread

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCommittedContextSnapshotRequiresExactCommittedFullInventory(t *testing.T) {
	ctx := context.Background()
	authority, store := newTestAuthority(68), newMemoryStore()
	registry, err := NewRegistry(ctx, authority, store)
	if err != nil {
		t.Fatal(err)
	}
	committed := testSecurityContext("thread-committed", "turn-committed", 1)
	prepared := testSecurityContext("thread-prepared", "turn-prepared", 1)
	for _, frozen := range []domainsecurity.TurnSecurityContext{committed, prepared} {
		if err := registry.Register(ctx, frozen); err != nil {
			t.Fatal(err)
		}
	}
	state, err := contextepochapp.BootstrapState(committed.ThreadID, committed.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(committed)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Commit(ctx, committed, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	if err := registry.Derive(ctx, committed.ThreadID, "thread-derived", "fork"); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"none", "prepared_only", "foreign_key", "duplicate", "orphan", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			input := append([]domainsecurity.CaseThreadAuthorityRecord{}, records...)
			key := authority
			operationCtx := ctx
			switch fault {
			case "prepared_only":
				input = nil
				for _, record := range records {
					if record.SecurityContext != nil && !domainsecurity.CaseThreadAuthorityIsCommittedContext(record) {
						input = append(input, record)
					}
				}
			case "foreign_key":
				key = newTestAuthority(69)
			case "duplicate":
				input = append(input, input[0])
			case "orphan":
				for _, record := range records {
					if domainsecurity.CaseThreadAuthorityIsLineage(record) {
						input = []domainsecurity.CaseThreadAuthorityRecord{record}
						break
					}
				}
			case "cancelled":
				var cancel context.CancelFunc
				operationCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			observed, err := VerifyCommittedContextInventoryV1(operationCtx, input, key)
			if fault != "none" && fault != "prepared_only" {
				if observed != nil || err == nil {
					t.Fatal("invalid complete inventory became historical context authority")
				}
				if fault == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Errorf("snapshot lost cancellation: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, found := observed.CommittedContextV1(committed.ThreadID, committed.TurnID)
			if fault == "prepared_only" {
				if found {
					t.Fatal("prepared record became committed authority")
				}
			} else if !found || actual != committed {
				t.Fatal("exact committed context was not observed")
			}
			if _, found := observed.CommittedContextV1(prepared.ThreadID, prepared.TurnID); found {
				t.Fatal("uncommitted context became historical authority")
			}
		})
	}
	for _, sentinel := range []error{errors.New("synthetic inventory verification I/O failure"), context.Canceled} {
		if _, err := VerifyCommittedContextInventoryV1(ctx, records, caseInventoryVerificationFailureV1{testAuthority: authority, err: sentinel}); !errors.Is(err, sentinel) {
			t.Errorf("snapshot lost verifier failure: %v", err)
		}
	}
	observed, err := VerifyCommittedContextInventoryV1(ctx, records, authority)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.SecurityContext != nil {
			*record.SecurityContext = domainsecurity.TurnSecurityContext{}
		}
	}
	actual, found := observed.CommittedContextV1(committed.ThreadID, committed.TurnID)
	if !found || !reflect.DeepEqual(actual, committed) {
		t.Fatal("caller mutation changed the verified snapshot")
	}
}
