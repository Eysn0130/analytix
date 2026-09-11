package piiauthorization

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
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiport "analytix.local/runtime-go/internal/ports/piiauthorization"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

// This verifies native canonical records and independent fixture-key checks,
// not the cross-owner historical report graph or live release authority.
func TestOriginalPIIReadersRetainCompleteDetachedInventories(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "open"
		if closed {
			name = "closed"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			grantRoot := filepath.Join(root, "pii-authorization")
			accessRoot := filepath.Join(root, "controlled-artifact-access-v2")
			grants, err := NewStore(filepath.Join(grantRoot, "grants"), access)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = grants.Close() })
			grant := storeTestGrant(t)
			if err := grants.PutGrantIfAbsent(ctx, grant); err != nil {
				t.Fatal(err)
			}
			store, err := NewAccessStoreV2(accessRoot, access)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			receipt, disposition := accessStoreFixtureV2(t, "original-reader-outcome")
			if err := store.ReserveAccessReceiptV2(ctx, receipt); err != nil {
				t.Fatal(err)
			}
			if closed {
				if err := store.PutAccessDispositionIfAbsentV2(ctx, disposition); err != nil {
					t.Fatal(err)
				}
			}
			grantObservation, err := PrepareOriginalObservationV1(ctx, grantRoot, access)
			if err != nil {
				t.Fatal(err)
			}
			accessObservation, err := PrepareOriginalAccessObservationV2(ctx, accessRoot, access)
			if err != nil {
				t.Fatal(err)
			}
			grantFiles, err := grantObservation.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			accessFiles, err := accessObservation.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// Derive expected authority independently from the fixture key seed.
			public := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x45}, ed25519.SeedSize)).Public().(ed25519.PublicKey)
			check := func(id, key string) error {
				if id != domainsecurity.SHA256Hex(public) || key != base64.RawURLEncoding.EncodeToString(public) {
					return errors.New("foreign fixture key")
				}
				return nil
			}
			grantReader, err := ParseOriginalGrantReaderV1(ctx, grantFiles, nil, check, nil)
			if err != nil {
				t.Fatal(err)
			}
			callbackFiles, err := grantObservation.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			callbackReader, callbackErr := ParseOriginalGrantReaderV1(ctx, callbackFiles, nil, func(id, key string) error {
				if err := check(id, key); err != nil {
					return err
				}
				// A synchronous callback can mutate caller-owned bytes without
				// a race. Captured records must be the same bytes we validated.
				for _, entry := range callbackFiles {
					clear(entry.Body)
				}
				return nil
			}, nil)
			if callbackErr == nil {
				got, err := callbackReader.ResolveGrant(ctx, grant.RecordDigest)
				if err != nil || !reflect.DeepEqual(got, grant) {
					t.Fatal("key callback replaced the validated original grant bytes")
				}
			} else if callbackReader != nil {
				t.Fatal("failed callback validation returned a reader")
			}
			reader, err := ParseOriginalAccessReaderV2(ctx, accessFiles, nil, check, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, writable := any(grantReader).(piiport.Store); writable {
				t.Fatal("original grant reader has write authority")
			}
			if _, writable := any(reader).(piiport.AccessStoreV2); writable {
				t.Fatal("original access reader has release/reservation authority")
			}
			grantCopy, err := grantReader.ResolveGrant(ctx, grant.RecordDigest)
			if err != nil || !reflect.DeepEqual(grantCopy, grant) {
				t.Fatalf("original grant read: %v", err)
			}
			grantCopy.FieldBindings[0].ClaimID = "caller mutation"
			grantCopy, err = grantReader.ResolveGrant(ctx, grant.RecordDigest)
			if err != nil || !reflect.DeepEqual(grantCopy, grant) {
				t.Fatal("caller changed original nested grant fields")
			}
			count := 0
			if err := reader.VisitAccessReceiptsV2(ctx, func(got domainpii.ControlledArtifactAccessReceiptV2) error {
				count++
				if !reflect.DeepEqual(got, receipt) {
					t.Fatal("receipt inventory changed")
				}
				return nil
			}); err != nil || count != 1 {
				t.Fatalf("complete access inventory: count=%d err=%v", count, err)
			}
			gotDisposition, err := reader.ResolveAccessDispositionV2(ctx, receipt.AccessID)
			if closed && (err != nil || !reflect.DeepEqual(gotDisposition, disposition)) || !closed && !errors.Is(err, piiport.ErrNotFound) {
				t.Fatalf("original disposition state changed: %v", err)
			}
			visitorError := errors.New("actual visitor error")
			if err := reader.VisitAccessReceiptsV2(ctx, func(domainpii.ControlledArtifactAccessReceiptV2) error { return visitorError }); !errors.Is(err, visitorError) {
				t.Fatal("inventory lost visitor cause")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := reader.ResolveAccessReceiptV2(cancelled, receipt.AccessID); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled original read succeeded")
			}
			foreign := func(string, string) error { return visitorError }
			if candidate, err := ParseOriginalAccessReaderV2(ctx, accessFiles, nil, foreign, nil); err == nil || candidate != nil {
				t.Fatal("foreign key supplied an original access reader")
			}
			if candidate, err := ParseOriginalGrantReaderV1(ctx, grantFiles, nil, nil, nil); err == nil || candidate != nil {
				t.Fatal("embedded grant key supplied its own trust")
			}
			id := domainsecurity.SHA256Hex([]byte("malformed extra access"))
			accessFiles["access-receipts/"+id[:2]] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0700}
			accessFiles["access-receipts/"+id[:2]+"/"+id+".json"] = finalauthority.SecurePrivateCASOriginalEntryV1{Mode: 0600, Body: []byte("{}")}
			if candidate, err := ParseOriginalAccessReaderV2(ctx, accessFiles, nil, check, nil); err == nil || candidate != nil {
				t.Fatal("extra malformed inventory entry was silently omitted")
			}
			for name, entry := range accessFiles {
				if !entry.Directory {
					clear(entry.Body)
				}
				delete(accessFiles, name)
			}
			gotReceipt, err := reader.ResolveAccessReceiptV2(ctx, receipt.AccessID)
			if err != nil || !reflect.DeepEqual(gotReceipt, receipt) {
				t.Fatal("caller changed detached original access bytes")
			}
			if err := errors.Join(grantObservation.RevalidatePhysicalV1(ctx), accessObservation.RevalidatePhysicalV1(ctx)); err != nil {
				t.Fatalf("read-only adapter changed native owners: %v", err)
			}
		})
	}
}
