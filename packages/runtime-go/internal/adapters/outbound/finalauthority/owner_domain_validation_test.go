package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPrivateCASDomainValidationRetainsPhysicalAndControlFailures(t *testing.T) {
	for _, scenario := range []string{"domain", "later physical fault", "cancelled", "unknown leaf", "material domain"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			leaves := []SecurePrivateCASOwnerLeafV1{{Name: "records", MaxBytes: 4096}, {Name: "related", MaxBytes: 4096}}
			for _, leaf := range leaves {
				cas, err := OpenSecurePrivateCASWithAccessAuthority(filepath.Join(root, leaf.Name), leaf.MaxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				body := []byte(`{}`)
				digest := sha256.Sum256(body)
				writeErr := cas.PutIfAbsent(context.Background(), hex.EncodeToString(digest[:]), body)
				if err := errors.Join(writeErr, cas.Close()); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root, leaves, access)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			domainFailure := errors.New("synthetic private domain canary")
			err = prepared.ValidateDomainSemantics(ctx, func(visit PrivateCASDomainVisitor, materials PrivateCASDomainMaterialVisitor) error {
				if scenario == "unknown leaf" {
					_ = visit("unknown", func(SecurePrivateCASFile) error { return nil })
					return domainFailure
				}
				if scenario == "material domain" {
					return materials("records", func(SecurePrivateCASPreparedMaterialV1) error { return domainFailure })
				}
				return visit("records", func(SecurePrivateCASFile) error {
					if scenario == "later physical fault" {
						// A hostile callback cannot hide drift in a leaf whose
						// domain records are never reached after the first error.
						if err := os.WriteFile(filepath.Join(root, "related", "unexpected"), []byte("physical fault"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					if scenario == "cancelled" {
						cancel()
					}
					return domainFailure
				})
			})
			_, domainOnly := err.(*DomainRecordUnavailableError)
			wantDomain := scenario == "domain" || scenario == "material domain"
			if err == nil || domainOnly != wantDomain {
				t.Fatalf("domain classification=%t want=%t error=%v", domainOnly, wantDomain, err)
			}
			if wantDomain && (!errors.Is(err, domainFailure) || err.Error() != "private domain records are unavailable") {
				t.Fatal("domain result lost its private cause or exposed it in the bounded error")
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("domain validation lost cancellation")
			}
		})
	}
}

func TestPrivateCASDomainInstallationRetainsSharedKeyFailures(t *testing.T) {
	for _, scenario := range []string{"record mismatch", "key changed during domain failure", "cancelled during domain failure"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newExistingFileAuthorityAnchorFixture(t)
			authority, err := fixture.anchor.Open(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(fixture.dataRoot, "private", "optional-owner")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root, []SecurePrivateCASOwnerLeafV1{{Name: "records", MaxBytes: 4096}}, access)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = owner.ValidateDomainInstallation(ctx, authority, func(PrivateCASDomainVisitor, PrivateCASDomainMaterialVisitor, func(string, string) error) error {
				if scenario == "key changed during domain failure" {
					if err := os.WriteFile(fixture.path, []byte(`{}`), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "cancelled during domain failure" {
					cancel()
				}
				return errors.New("synthetic domain identity failure")
			})
			_, domainOnly := err.(*DomainRecordUnavailableError)
			if err == nil || domainOnly != (scenario == "record mismatch") {
				t.Fatalf("installation classification=%t err=%v", domainOnly, err)
			}
			if scenario == "cancelled during domain failure" && !errors.Is(err, context.Canceled) {
				t.Fatal("installation validation lost cancellation")
			}
		})
	}
}

func TestPrivateCASSelectedBodiesKeepCompletePhysicalBoundary(t *testing.T) {
	for _, scenario := range []string{"selected only", "unselected physical drift", "cancelled selection"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(root, "artifacts")
			cas, err := OpenSecurePrivateCASWithAccessAuthority(leaf, 4096, access)
			if err != nil {
				t.Fatal(err)
			}
			var selected string
			for index, body := range [][]byte{[]byte("controlled synthetic artifact"), []byte("ordinary synthetic artifact")} {
				digest := sha256.Sum256(body)
				address := hex.EncodeToString(digest[:])
				if index == 0 {
					selected = address
				}
				if err := cas.PutIfAbsent(context.Background(), address, body); err != nil {
					t.Fatal(err)
				}
			}
			if err := cas.Close(); err != nil {
				t.Fatal(err)
			}
			owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root, []SecurePrivateCASOwnerLeafV1{{Name: "artifacts", MaxBytes: 4096}}, access)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			visited := 0
			err = owner.SecurePrivateCASRecoveryPlansV2()[0].VisitSelectedCommittedFiles(ctx, func(material SecurePrivateCASPreparedMaterialV1) bool {
				if material.Digest != selected {
					if scenario == "unselected physical drift" {
						if err := os.WriteFile(filepath.Join(leaf, "unexpected"), []byte("synthetic physical drift"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					if scenario == "cancelled selection" {
						cancel()
					}
				}
				return material.Digest == selected
			}, func(file SecurePrivateCASFile) error {
				visited++
				if file.Digest != selected || string(file.Body) != "controlled synthetic artifact" {
					t.Fatal("unselected body was materialized to the domain callback")
				}
				return nil
			})
			if scenario == "selected only" {
				if err != nil || visited != 1 {
					t.Fatalf("selected body inventory: visited=%d err=%v", visited, err)
				}
			} else if err == nil {
				t.Fatal("selection hid a physical or context fault")
			}
			if scenario == "cancelled selection" && !errors.Is(err, context.Canceled) {
				t.Fatal("selection lost cancellation")
			}
		})
	}
}
