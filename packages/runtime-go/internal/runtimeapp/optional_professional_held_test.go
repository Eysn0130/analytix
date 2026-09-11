//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	reportpublicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	"analytix.local/runtime-go/internal/contracts"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRuntimeHeldUnknownAndUnavailableProfessionalLaneAllowOrdinaryHTTP(t *testing.T) {
	for _, fixture := range runtimeOptionalProfessionalTestOwners {
		if fixture.owner != "evidence-registry" && fixture.owner != "pii-authorization" {
			continue
		}
		t.Run(fixture.owner, func(t *testing.T) {
			_, enrolled := runtimeWitnessedRegistryConfigV2(t)
			var original map[string]string
			var retainedRoots []string
			checkOriginal := func() {
				if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, retainedRoots...)) {
					t.Fatal("ordinary operation or restart changed original held or unavailable owner inventory")
				}
			}
			config, ordinaryThreadID, _ := runRuntimePlanThenProtectedOrdinaryRestartWithObservationV1(t, func(t *testing.T, config Config) {
				initial, err := NewRuntimeServerHandlerE(config)
				if err != nil {
					t.Fatal(err)
				}
				shutdownOwnedRuntimeHandler(t, initial)
				roots, err := resolveRuntimePersistenceRoots(config)
				if err != nil {
					t.Fatal(err)
				}
				core, primary := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
				access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
				if err != nil {
					t.Fatal(err)
				}
				owner := filepath.Join(config.DataDir, "private", fixture.owner)
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(owner, fixture.leaf), fixture.maxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				address := domainsecurity.SHA256Hex([]byte("held-optional-domain-" + fixture.owner))
				if err := cas.PutIfAbsent(context.Background(), address, []byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`)); err != nil {
					_ = cas.Close()
					t.Fatal(err)
				}
				if err := cas.Close(); err != nil {
					t.Fatal(err)
				}
				retainedRoots = []string{owner, filepath.Dir(primary)}
				if fixture.owner == "pii-authorization" {
					attempt := runtimeOriginalReportAttemptFixtureV1(t, core, primary)
					body, err := domainpublication.PublicationAttemptV1Bytes(attempt)
					if err != nil {
						t.Fatal(err)
					}
					attempts, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "report-publication", "attempts"), 512<<10, access)
					if err != nil {
						t.Fatal(err)
					}
					if err := errors.Join(attempts.PutIfAbsent(context.Background(), attempt.AttemptID, body), attempts.Close()); err != nil {
						t.Fatal(err)
					}
					residue := filepath.Join(config.DataDir, "private", "report-publication", "attempts", attempt.AttemptID[:2], "."+attempt.AttemptID+".json-original.tmp")
					if err := os.WriteFile(residue, []byte("synthetic original uncommitted report bytes"), 0o600); err != nil {
						t.Fatal(err)
					}
					for _, name := range runtimePublicationOwnersV1 {
						retainedRoots = append(retainedRoots, filepath.Join(config.DataDir, "private", name))
					}
				}
				for _, receipt := range core.pendingInventory.Receipts {
					workID := receipt.WorkID
					for _, leaf := range []string{"receipts", "dispositions"} {
						retainedRoots = append(retainedRoots, filepath.Join(config.DataDir, "private", "pending-work", leaf, workID[:2], workID+".json"))
					}
				}
				original = startupWholeTreeRecordMapForTest(t, retainedRoots...)
			}, func(t *testing.T, client *http.Client, url string) {
				checkOriginal()
				// The helper's exact provider count also proves these held
				// requests never invoke a provider, before or after restart.
				status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, url, http.MethodPost, "/v1/threads/thread-report-held/turns", map[string]any{"prompt": "Read the local ordinary file.", "mode": "agent"})
				if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
					t.Fatalf("held execution was not explicitly refused: status=%d", status)
				}
				for _, path := range []string{"/v1/threads/thread-report-held", "/v1/usage?thread_id=thread-report-held"} {
					status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, url, http.MethodGet, path, nil)
					if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
						t.Fatalf("held public observation returned empty success: path=%s status=%d", path, status)
					}
				}
				checkOriginal()
			}, func(config *Config) {
				config.DataDir = enrolled.DataDir
				config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
				config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
				config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
				config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
			})
			checkOriginal()
			// An orphan candidate is safely recoverable ordinary state.
			orphan := filepath.Join(config.ProductionDurableRoot, "threads", ordinaryThreadID, ".events-bundle-v1.tmp")
			if err := os.WriteFile(orphan, []byte("R129_SYNTHETIC_ORPHAN_CANDIDATE"), 0o600); err != nil {
				t.Fatal(err)
			}
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatalf("ordinary orphan recovery failed: %v", err)
			}
			shutdownOwnedRuntimeHandler(t, handler)
			checkOriginal()
			// A malformed actual ordinary primary is a Core failure and
			// must be refused before further startup effects.
			primary := filepath.Join(config.ProductionDurableRoot, "threads", ordinaryThreadID, "thread.json")
			if err := os.WriteFile(primary, []byte("R129_SYNTHETIC_INVALID_CORE_PRIMARY"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
			handler, err = NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("malformed Core primary was degraded into optional availability")
			}
			if startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot) != before {
				t.Fatal("Core rejection ran after startup effects")
			}
			checkOriginal()
		})
	}
}

func TestRuntimeUnavailableReportPreservationRequiresCurrentAbsence(t *testing.T) {
	for _, scenario := range []string{"late-attempt", "v2-record", "missing-scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			roots, err := persistencefs.ResolveRootSet(config.DataDir, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			core, _ := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
			core.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, config, core.verification)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			installation, err := loadRuntimeOptionalDomainInstallation(ctx, config, roots, rootAuthority)
			if err != nil {
				t.Fatal(err)
			}
			reportRoot := filepath.Join(roots.DataDir, "private", "report-publication")
			present, err := reportpublicationstore.HasPreparedAttemptsV1(ctx, reportRoot, core.access)
			if err != nil || present {
				t.Fatalf("initial attempt denominator is not empty: %v", err)
			}
			// An attempt can appear after the old probe but before the new
			// complete optional-domain observation. That observation must not
			// turn the earlier false into permission to pass the scope gate.
			if scenario != "missing-scope" {
				owner, leaf, limit := "report-publication", "attempts", 512<<10
				if scenario == "v2-record" {
					owner, leaf, limit = "controlled-artifact-access-v2", "access-receipts", 256<<10
				}
				for _, root := range runtimePrivateCASExpectedRoots(roots.DataDir, owner) {
					if err := os.MkdirAll(root, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", owner, leaf), limit, core.access)
				if err != nil {
					t.Fatal(err)
				}
				if err := cas.PutIfAbsent(ctx, domainsecurity.SHA256Hex([]byte(scenario)), []byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`)); err != nil {
					_ = cas.Close()
					t.Fatal(err)
				}
				if err := cas.Close(); err != nil {
					t.Fatal(err)
				}
			}
			domains, err := prepareRuntimeOptionalDomainCapabilities(ctx, roots.DataDir, core.access, installation)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-scope" {
				preserved = runtimeReportRestartPreservationV1{}
			}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			if err := validateRuntimeUnavailableReportPreservationV1(ctx, preserved, core.pendingInventory, domains); !errors.Is(err, errRuntimeReportRestartReconciliationRequired) {
				t.Fatalf("scope gate accepted missing current proof: scenario=%s err=%v", scenario, err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("scope gate changed original state")
			}
		})
	}
}
