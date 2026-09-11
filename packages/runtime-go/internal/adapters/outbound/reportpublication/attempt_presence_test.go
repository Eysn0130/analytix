package reportpublication

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPreparedAttemptInventoryCanonicalCompleteObservation(t *testing.T) {
	ctx := context.Background()
	fixture := newPublicationRecoveryFixture(t)
	body, err := domainpublication.PublicationAttemptV1Bytes(fixture.attempt)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "noncanonical", "wrong-identity", "bad-additional-record"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "report-publication")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(root, "attempts"), maxPublicationAttemptBytes, access)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cas.Close() })
			digest, stored := fixture.attempt.AttemptID, append([]byte(nil), body...)
			if scenario == "noncanonical" {
				stored = append(stored, '\n')
			}
			if scenario == "wrong-identity" {
				digest = domainsecurity.SHA256Hex([]byte("other attempt identity"))
			}
			if err := cas.PutIfAbsent(ctx, digest, stored); err != nil {
				t.Fatal(err)
			}
			if scenario == "bad-additional-record" {
				if err := cas.PutIfAbsent(ctx, domainsecurity.SHA256Hex([]byte("bad additional attempt")), []byte(`{}`)); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := PrepareAttemptInventoryV1(ctx, root, access)
			if scenario != "valid" {
				if err == nil || snapshot != nil {
					t.Fatal("invalid full attempt inventory returned a usable snapshot")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				count := 0
				if err := snapshot.VisitAttempts(ctx, func(attempt domainpublication.PublicationAttemptV1) error {
					count++
					current, err := domainpublication.PublicationAttemptV1Bytes(attempt)
					if err != nil || !bytes.Equal(current, body) {
						t.Fatal("snapshot did not retain the original canonical attempt")
					}
					attempt.ReportStageWorkID = "visitor mutation"
					return nil
				}); err != nil || count != 1 {
					t.Fatalf("fresh parsed complete observation failed: count=%d err=%v", count, err)
				}
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if err := snapshot.VisitAttempts(cancelled, func(domainpublication.PublicationAttemptV1) error {
				t.Fatal("cancelled observation called its visitor")
				return nil
			}); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled snapshot returned %v", err)
			}
			// Drift during an otherwise successful visitor cannot produce a
			// successful observation. No CAS recovery or cleanup is invoked.
			path := filepath.Join(root, "attempts", digest[:2], digest+".json")
			if err := snapshot.VisitAttempts(ctx, func(domainpublication.PublicationAttemptV1) error {
				return os.WriteFile(path, append(append([]byte(nil), body...), '\n'), 0o600)
			}); err == nil {
				t.Fatal("attempt bytes changed during a successful observation")
			}
			if err := snapshot.Revalidate(ctx); err == nil {
				t.Fatal("stale prepared attempt remained usable")
			}
		})
	}
}

func TestPreparedAttemptInventoryMissingLeafDoesNotCreateState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "report-publication")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := PrepareAttemptInventoryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VisitAttempts(context.Background(), func(domainpublication.PublicationAttemptV1) error {
		t.Fatal("missing attempt leaf invented a record")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("observation created the missing publication owner")
	}
}
