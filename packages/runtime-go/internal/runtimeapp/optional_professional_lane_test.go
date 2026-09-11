//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

var runtimeOptionalProfessionalTestOwners = []struct {
	owner    string
	leaf     string
	maxBytes int
}{
	{"case-entity", "bindings-v1", 16 << 10},
	{"evidence-registry", "capsules", 16 << 20},
	{"evidence-authority", "bundles", 512 << 10},
	{"dataset-snapshot-authority", "authority-bundles-v2", 1 << 20},
	{"pii-authorization", "grants", 2 << 20},
	{"report-publication", "attempts", 512 << 10},
	{"controlled-artifact-access", "access-receipts", 256 << 10},
	{"controlled-artifact-access-v2", "access-receipts", 256 << 10},
}

func TestRuntimeOptionalProfessionalSemanticFailureUsesOriginalAuthorityBoundary(t *testing.T) {
	for _, fixture := range runtimeOptionalProfessionalTestOwners {
		t.Run(fixture.owner, func(t *testing.T) {
			_, enrolled := runtimeWitnessedRegistryConfigV2(t)
			if fixture.owner == "report-publication" {
				// An unattributable committed attempt is outside the proved
				// optional-domain closure; the complete report inventory gate
				// must refuse before any recovery or ordinary activation.
				enrolled.DurableTempDir = t.TempDir()
				initial, err := NewRuntimeServerHandlerE(enrolled)
				if err != nil {
					t.Fatal(err)
				}
				shutdownOwnedRuntimeHandler(t, initial)
				access, err := privatecastest.NewAccessAuthority(filepath.Join(enrolled.DataDir, "private"))
				if err != nil {
					t.Fatal(err)
				}
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(enrolled.DataDir, "private", fixture.owner, fixture.leaf), fixture.maxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				address := domainsecurity.SHA256Hex([]byte("unattributable-report-attempt"))
				if err := errors.Join(cas.PutIfAbsent(context.Background(), address, []byte(`{"domainCanary":"unattributable-report"}`)), cas.Close()); err != nil {
					t.Fatal(err)
				}
				before := startupWholeTreeDigest(t, enrolled.DataDir, enrolled.DurableTempDir)
				handler, err := NewRuntimeServerHandlerE(enrolled)
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
				if !errors.Is(err, errRuntimeReportRestartReconciliationRequired) {
					t.Fatalf("unattributable report did not retain complete-inventory denial: %v", err)
				}
				if after := startupWholeTreeDigest(t, enrolled.DataDir, enrolled.DurableTempDir); before != after {
					t.Fatal("unattributable report refusal changed persistent state")
				}
				return
			}
			runRuntimePlanThenProtectedOrdinaryRestartV1(t, func(t *testing.T, config Config) {
				initial, err := NewRuntimeServerHandlerE(config)
				if err != nil {
					t.Fatal(err)
				}
				shutdownOwnedRuntimeHandler(t, initial)
				access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
				if err != nil {
					t.Fatal(err)
				}
				ownerRoot := filepath.Join(config.DataDir, "private", fixture.owner)
				root := filepath.Join(ownerRoot, fixture.leaf)
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, fixture.maxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				// The secure CAS authenticates these bytes; the domain record is
				// deliberately invalid. This is not a physical corruption fixture.
				address := domainsecurity.SHA256Hex([]byte("optional-domain-semantics-" + fixture.owner))
				if err := cas.PutIfAbsent(context.Background(), address, []byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`)); err != nil {
					_ = cas.Close()
					t.Fatal(err)
				}
				if err := cas.Close(); err != nil {
					t.Fatal(err)
				}
				before := startupWholeTreeDigest(t, ownerRoot)
				t.Cleanup(func() {
					if after := startupWholeTreeDigest(t, ownerRoot); after != before {
						t.Error("unusable optional owner bytes changed during ordinary work or restart")
					}
				})
			}, func(config *Config) {
				config.DataDir = enrolled.DataDir
				config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
				config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
				config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
				config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
			})
		})
	}
}

func TestRuntimeOptionalProfessionalPhysicalFaultStillBlocksStartup(t *testing.T) {
	for _, fixture := range runtimeOptionalProfessionalTestOwners {
		t.Run(fixture.owner, func(t *testing.T) {
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(config.DataDir, "private", fixture.owner, fixture.leaf)
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, fixture.maxBytes, access)
			if err != nil {
				t.Fatal(err)
			}
			if err := cas.Close(); err != nil {
				t.Fatal(err)
			}
			// This bypasses the CAS writer and violates its physical inventory.
			if err := os.WriteFile(filepath.Join(root, "unexpected-entry"), []byte("synthetic physical fault"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir)
			handler, err := NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("optional owner physical corruption did not block startup")
			}
			if after := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir); after != before {
				t.Fatal("physical startup rejection changed persistent state")
			}
		})
	}
}
