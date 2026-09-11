package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestOriginalReadersRetainCompleteDetachedPublication(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "projected"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newPublicationRecoveryFixture(t)
			if rejected {
				_, fixture.deliveryOutcome = fixture.evidenceChangedRejection(t)
			}
			observation, files, check := originalReaderFixtureV1(t, fixture)
			readers, err := ParseOriginalReadersV1(ctx, files, nil, check, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Retain a fresh native snapshot before attacking caller-owned input.
			before, err := observation.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for name, entry := range files {
				clear(entry.Body)
				delete(files, name)
			}
			for range 2 {
				attempt, err := readers.Attempts.Resolve(ctx, fixture.attempt.AttemptID)
				if err != nil || !reflect.DeepEqual(attempt, fixture.attempt) {
					t.Fatalf("original attempt changed: %v", err)
				}
				count := 0
				if err := readers.Attempts.VisitAttempts(ctx, func(record domainpublication.PublicationAttemptV1) error {
					count++
					record.ReportStageWorkID = "caller mutation"
					return nil
				}); err != nil || count != 1 {
					t.Fatalf("complete attempts: count=%d err=%v", count, err)
				}
				count = 0
				if err := readers.DeliveryOutcomes.VisitDeliveryOutcomes(ctx, func(record domainpublication.ReportDeliveryOutcomeV1) error {
					count++
					if !reflect.DeepEqual(record, fixture.deliveryOutcome) {
						t.Fatal("outcome differs from Original")
					}
					return nil
				}); err != nil || count != 1 {
					t.Fatalf("complete outcomes: count=%d err=%v", count, err)
				}
				ledger, err := readers.Ledgers.Resolve(ctx, fixture.ledger.LedgerDigest)
				if err != nil || !reflect.DeepEqual(ledger, fixture.ledger) {
					t.Fatalf("original ledger changed: %v", err)
				}
				ledger.EvidenceReceiptIDs[0] = "caller mutation"
				artifact, err := readers.Artifacts.ResolveExact(ctx, fixture.target)
				if err != nil || !bytes.Equal(artifact, fixture.artifact) {
					t.Fatalf("original artifact changed: %v", err)
				}
				clear(artifact)
			}
			if _, err := readers.Attempts.Resolve(ctx, domainsecurity.SHA256Hex([]byte("missing attempt"))); !errors.Is(err, publicationport.ErrNotFound) {
				t.Fatalf("missing selector: %v", err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if err := readers.Attempts.VisitAttempts(cancelled, func(domainpublication.PublicationAttemptV1) error { t.Fatal("cancelled read visited"); return nil }); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled read: %v", err)
			}
			sentinel := errors.New("visitor failure")
			if err := readers.DeliveryOutcomes.VisitDeliveryOutcomes(ctx, func(domainpublication.ReportDeliveryOutcomeV1) error { return sentinel }); !errors.Is(err, sentinel) {
				t.Fatalf("visitor cause lost: %v", err)
			}
			if err := observation.RevalidatePhysicalV1(ctx); err != nil {
				t.Fatal(err)
			}
			after, err := observation.SnapshotOriginalFilesV1(ctx)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("readers changed native owner: %v", err)
			}
		})
	}
}

func TestOriginalReadersRejectIncompleteOrUnauthenticatedOwner(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t)
	observation, _, check := originalReaderFixtureV1(t, fixture)
	for _, scenario := range []string{"missing-material", "noncanonical-additional-attempt", "foreign-key", "no-key"} {
		t.Run(scenario, func(t *testing.T) {
			files, err := observation.SnapshotOriginalFilesV1(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			localCheck := check
			switch scenario {
			case "missing-material":
				digest := fixture.ledger.LedgerDigest
				delete(files, "claim-ledgers/"+digest[:2]+"/"+digest+".json")
			case "noncanonical-additional-attempt":
				digest := domainsecurity.SHA256Hex([]byte("unselected corrupt attempt"))
				files["attempts/"+digest[:2]] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0700}
				files["attempts/"+digest[:2]+"/"+digest+".json"] = finalauthority.SecurePrivateCASOriginalEntryV1{Mode: 0600, Body: []byte(`{}`)}
			case "foreign-key":
				localCheck = func(string, string) error { return errors.New("not current installation key") }
			case "no-key":
				localCheck = nil
			}
			readers, err := ParseOriginalReadersV1(context.Background(), files, nil, localCheck, nil)
			if err == nil || readers != nil {
				t.Fatal("invalid full owner returned readers")
			}
		})
	}
	if err := observation.RevalidatePhysicalV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func originalReaderFixtureV1(t *testing.T, fixture publicationRecoveryFixture) (*finalauthority.OriginalFixedOwnerObservationV1, map[string]finalauthority.SecurePrivateCASOriginalEntryV1, func(string, string) error) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "report-publication")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := NewStores(root, access)
	if err != nil {
		t.Fatal(err)
	}
	fixture.install(t, stores, fixture.artifact)
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	observation, err := PrepareOriginalObservationV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := observation.SnapshotOriginalFilesV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	public := fixture.authorityKey.Public().(ed25519.PublicKey)
	keyID, publicText := domainsecurity.SHA256Hex(public), base64.RawURLEncoding.EncodeToString(public)
	return observation, files, func(id, key string) error {
		if id != keyID || key != publicText {
			return errors.New("record is not signed by observed current key")
		}
		return nil
	}
}
