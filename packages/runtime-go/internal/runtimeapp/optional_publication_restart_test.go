//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	reportpublicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestUnavailablePublicationPreservesTrustedUnknownReportWork(t *testing.T) {
	for _, disposedUnknown := range []bool{false, true} {
		name := "open report work"
		if disposedUnknown {
			name = "persisted outcome unknown"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, handler)
			authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: "thread-report-held", TurnID: "turn-report-held", WorkspaceRealPath: "/synthetic/report-held",
				CaseID: "case-report-held", CaseBindingHash: domainsecurity.SHA256Hex([]byte("report-held-binding")),
				ContextEpoch: 2, IssuedAt: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
			receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
				Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
				GrantRegistrySequence: 1, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("report-held-registry")),
				GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("report-held-grant")), RegistrySequence: 1, RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("report-held-entry"))}},
				PayloadHash:  domainsecurity.SHA256Hex([]byte("report-held-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("report-held-route")),
				IssuedAt: now, ExpiresAt: now.Add(time.Hour), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
			}, sign)
			if err != nil {
				t.Fatal(err)
			}
			put := func(relative string, maxBytes int, digest string, body []byte) {
				t.Helper()
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", relative), maxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				writeErr := cas.PutIfAbsent(ctx, digest, body)
				if err := errors.Join(writeErr, cas.Close()); err != nil {
					t.Fatal(err)
				}
			}
			receiptBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
			if err != nil {
				t.Fatal(err)
			}
			put("pending-work/receipts", 1<<20, receipt.WorkID, receiptBody)
			if disposedUnknown {
				disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", now.Add(time.Minute), authority.KeyID(), authority.PublicKey(), sign)
				if err != nil {
					t.Fatal(err)
				}
				body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
				if err != nil {
					t.Fatal(err)
				}
				put("pending-work/dispositions", 1<<20, receipt.WorkID, body)
			}
			put("pii-authorization/grants", 2<<20, domainsecurity.SHA256Hex([]byte("unavailable-pii")), []byte(`{}`))
			// An authenticated earlier owner is otherwise permitted to remove
			// this exact retired writer residue. The report gate must precede
			// that recovery as well as semantic-stage activation.
			digest := domainsecurity.SHA256Hex([]byte("synthetic-early-residue"))
			shard := filepath.Join(config.DataDir, "private", "accepted-finals", "records", digest[:2])
			if err := os.MkdirAll(shard, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(shard, "."+digest+".json-recovery.tmp"), []byte("prepared-residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
			handler, err = NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("unresolved report work reached runtime activation")
			}
			if !errors.Is(err, errRuntimeReportRestartReconciliationRequired) {
				t.Fatalf("trusted report work did not reach the preserve-unknown gate: %v", err)
			}
			if after := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot); after != before {
				t.Fatal("unresolved report startup changed durable state")
			}
		})
	}
}

func TestRuntimeReportEarlyGateAllowsUnrelatedAuthenticatedRecovery(t *testing.T) {
	_, config := runtimeWitnessedRegistryConfigV2(t)
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	digest := domainsecurity.SHA256Hex([]byte("synthetic-early-residue"))
	shard := filepath.Join(config.DataDir, "private", "accepted-finals", "records", digest[:2])
	if err := os.MkdirAll(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(shard, "."+digest+".json-recovery.tmp")
	if err := os.WriteFile(residue, []byte("prepared-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	reportResidue := filepath.Join(config.DataDir, "private", "report-publication", domainprivatecas.CreateDirectoryResidueNameV1("artifacts"))
	if err := os.Mkdir(reportResidue, 0o700); err != nil {
		t.Fatal(err)
	}
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)
	if _, err := os.Lstat(residue); !os.IsNotExist(err) {
		t.Fatal("unrelated authenticated recovery did not consume its exact residue")
	}
	if _, err := os.Lstat(reportResidue); !os.IsNotExist(err) {
		t.Fatal("attempt presence probe blocked unrelated empty create recovery")
	}
}

func TestRuntimeReportAttemptGatePrecedesRecoveryForOpenAndTerminalStages(t *testing.T) {
	for _, scenario := range []string{"reserved", "aborted"} {
		t.Run(scenario, func(t *testing.T) { testRuntimeReportAttemptGateV1(t, scenario) })
	}
}

func TestRuntimeReportAttemptCoreLinkagePrecedesRecovery(t *testing.T) {
	for _, scenario := range []string{"orphan", "foreign-key", "changed-stage-bytes"} {
		t.Run(scenario, func(t *testing.T) { testRuntimeReportAttemptGateV1(t, scenario) })
	}
}

func testRuntimeReportAttemptGateV1(t *testing.T, scenario string) {
	t.Helper()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	ctx := context.Background()
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	stage, attempt := runtimeReservedReportAttemptFixtureV1(t, authority)
	switch scenario {
	case "foreign-key":
		foreign, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "foreign.json"), false)
		if err != nil {
			t.Fatal(err)
		}
		_, attempt = runtimeReservedReportAttemptFixtureV1(t, foreign)
	case "changed-stage-bytes":
		original := stage
		stage, _ = runtimeReservedReportAttemptFixtureWithStageDelayV1(t, authority, time.Minute)
		if stage.WorkID != original.WorkID || stage.ReceiptID == original.ReceiptID {
			t.Fatal("fixture did not preserve work identity while changing signed receipt bytes")
		}
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	put := func(relative string, maxBytes int, digest string, body []byte, bodyErr error) {
		t.Helper()
		if bodyErr != nil {
			t.Fatal(bodyErr)
		}
		cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", relative), maxBytes, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := errors.Join(cas.PutIfAbsent(ctx, digest, body), cas.Close()); err != nil {
			t.Fatal(err)
		}
	}
	body, err := domainpendingwork.PendingWorkReceiptV1Bytes(stage)
	if scenario != "orphan" {
		put("pending-work/receipts", 1<<20, stage.WorkID, body, err)
	}
	body, err = domainpublication.PublicationAttemptV1Bytes(attempt)
	put("report-publication/attempts", 512<<10, attempt.AttemptID, body, err)
	if scenario == "aborted" {
		disposition, err := domainpendingwork.NewPendingWorkDispositionV1(stage, domainpendingwork.StatusFailed, "synthetic_stage_failed", time.Date(2026, 7, 13, 8, 1, 0, 0, time.UTC), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
		if err != nil {
			t.Fatal(err)
		}
		body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
		put("pending-work/dispositions", 1<<20, stage.WorkID, body, err)
	}
	capabilities, err := prepareRuntimeOptionalDomainCapabilities(ctx, config.DataDir, access, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"} {
		if !capabilities[name].available {
			t.Fatalf("fixture publication graph unavailable: %s", name)
		}
	}
	residue := filepath.Join(config.DataDir, "private", "accepted-finals", domainprivatecas.CreateDirectoryResidueNameV1("records"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir)
	handler, err = NewRuntimeServerHandlerE(config)
	if handler != nil {
		shutdownOwnedRuntimeHandler(t, handler)
	}
	if scenario == "reserved" || scenario == "aborted" {
		if !errors.Is(err, errRuntimeReportRestartReconciliationRequired) {
			t.Errorf("valid reserved/aborted attempt missed original gate: %v", err)
		}
	} else if scenario == "foreign-key" {
		// This fixture also lacks independent enrollment. The complete
		// Original current-key guard must reject that fault before attempts
		// can be treated as trusted facts for Core linkage.
		if !errors.Is(err, errRuntimeOptionalDomainInstallationRequired) {
			t.Errorf("foreign attempt without independent installation missed the original trust gate: %v", err)
		}
	} else if !errors.Is(err, reportpublicationapp.ErrAttemptCoreLinkageV1) {
		t.Errorf("attempt Core linkage was not rejected before the optional gate: %v", err)
	}
	if startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir) != before {
		t.Fatal("report attempt gate ran after startup changed state")
	}
}

func runtimeReservedReportAttemptFixtureV1(t *testing.T, authority authorityport.Authority) (domainpendingwork.PendingWorkReceiptV1, domainpublication.PublicationAttemptV1) {
	return runtimeReservedReportAttemptFixtureWithStageDelayV1(t, authority, 0)
}

func runtimeReservedReportAttemptFixtureWithStageDelayV1(t *testing.T, authority authorityport.Authority, delay time.Duration) (domainpendingwork.PendingWorkReceiptV1, domainpublication.PublicationAttemptV1) {
	t.Helper()
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication-store", TurnID: "turn-publication-store", WorkspaceRealPath: "/workspace/publication-store",
		CaseID: "case-publication-store", CaseBindingHash: domainsecurity.SHA256Hex([]byte("publication-store-binding")), ContextEpoch: 3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, keyID := authority.PublicKey(), authority.KeyID()
	stageReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
		GrantRegistrySequence: 3, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("publication-store-grant-registry")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("publication-store-grant")), RegistrySequence: 3,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("publication-store-grant-entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("publication-store-stage-payload")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("publication-store-stage-route")),
		IssuedAt:    now.Add(delay), ExpiresAt: now.Add(time.Hour), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x53}, domainsecurity.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	return stageReceipt, runtimeReportAttemptForStageFixtureV1(t, authority, securityContext, stageReceipt, toolCallID)
}

func runtimeReportAttemptForStageFixtureV1(t *testing.T, authority authorityport.Authority, securityContext domainsecurity.TurnSecurityContext, stageReceipt domainpendingwork.PendingWorkReceiptV1, toolCallID string) domainpublication.PublicationAttemptV1 {
	t.Helper()
	now, err := time.Parse(time.RFC3339Nano, securityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("publication-store-no-hit"))
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: nil, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("verified no-hit report")
	reportSHA := domainsecurity.SHA256Hex(artifact)
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionOrdinaryMasked, RulesetHash: domainsecurity.SHA256Hex([]byte("publication-store-rules")),
		ProjectedContentSHA256: reportSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA,
		ReportByteLength: uint64(len(artifact)), MediaType: "application/pdf", Passed: true, IssueCodes: []string{}, InspectedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKey := authority.PublicKey()
	keyID := authority.KeyID()
	installationID := domainsecurity.SHA256Hex([]byte("publication-store-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("publication-store-enrollment"))
	target := domainsecurity.SHA256Hex([]byte("publication-store-target"))
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Context: securityContext,
		ReportVariant: domainpublication.VerifiedNoHitReport, EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("publication-store-bundle")),
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("publication-store-registry-index")), EvidenceRegistryCount: 1,
		EvidenceRegistrySequence: 1, EvidenceRegistryStateDigest: domainsecurity.SHA256Hex([]byte("publication-store-registry-state")),
		ClaimLedger: ledger, ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)), MediaType: "application/pdf",
		PIIProjection: projection, RenderInspection: inspection, Publisher: "analytix-host", PublisherVersion: "1.0.0",
		TargetIdentityDigest: target, IssuedAt: now, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	attemptID := domainpublication.PublicationAttemptIDV1(installationID, enrollmentID, stageReceipt.WorkID)
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(),
		MutationID:          domainpublication.PublicationIndexMutationIDForAttemptV1(attemptID),
		ReceiptID:           receipt.ReceiptID, ReceiptRecordDigest: receipt.RecordDigest, TargetIdentityDigest: target,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	attemptInput := domainpublication.PublicationAttemptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, ReportStageReceipt: stageReceipt,
		ToolCallID: toolCallID, StageInputHash: domainsecurity.SHA256Hex([]byte("publication-store-stage-input")),
		Candidate: receipt, Index: index, ExpectedEvidenceBundleDigest: receipt.EvidenceAuthorityBundleDigest,
		ExpectedPublicationIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(), ExpectedPublicationCount: 0,
		NextEvidenceBundleDigest:     domainsecurity.SHA256Hex([]byte("publication-store-next-bundle")),
		AuthorityAdvanceIntentDigest: domainsecurity.SHA256Hex([]byte("publication-store-advance-intent")),
		AuthorityKeyID:               keyID, AuthorityPublicKey: publicKey,
	}
	attempt, err := domainpublication.NewPublicationAttemptV1(attemptInput, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}

	return attempt
}

// The attempt uses the full context and call identity read from the actual
// primary that supplied the trusted stage, rather than a hash-only Core stub.
func runtimeOriginalReportAttemptFixtureV1(t *testing.T, core *runtimeChildIdentityStartupV1, primary string, callOverride ...string) domainpublication.PublicationAttemptV1 {
	t.Helper()
	var stored struct {
		SecurityState domainsecurity.TurnSecurityContext `json:"securityState"`
		Turns         []struct {
			Items []struct {
				CallID string `json:"callId"`
			} `json:"items"`
		} `json:"turns"`
	}
	body, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &stored); err != nil {
		t.Fatal(err)
	}
	if len(core.pendingInventory.Receipts) != 1 || len(stored.Turns) != 1 || len(stored.Turns[0].Items) == 0 {
		t.Fatal("ambiguous actual report attempt fixture")
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	callID := stored.Turns[0].Items[0].CallID
	if len(callOverride) > 0 {
		callID = callOverride[0]
	}
	return runtimeReportAttemptForStageFixtureV1(t, authority, stored.SecurityState, core.pendingInventory.Receipts[0], callID)
}

func TestUnavailablePublicationOriginalAttemptRetainsHeldCoreAcrossRestarts(t *testing.T) {
	for _, scenario := range []string{"unknown", "missing-disposition", "wrong-call", "noncanonical", "additional-orphan", "terminal-same-held-thread"} {
		t.Run(scenario, func(t *testing.T) {
			_, config, _, _, _, _, _ := runtimePublicationOriginalFixtureV1(t, true)
			roots, err := resolveRuntimePersistenceRoots(config)
			if err != nil {
				t.Fatal(err)
			}
			core, primary := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, scenario != "missing-disposition", false)
			attempt := runtimeOriginalReportAttemptFixtureV1(t, core, primary)
			if scenario == "wrong-call" {
				differentCall, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x67}, domainsecurity.HostToolCallIDEntropyBytesV1))
				if err != nil {
					t.Fatal(err)
				}
				attempt = runtimeOriginalReportAttemptFixtureV1(t, core, primary, differentCall)
			}
			body, err := domainpublication.PublicationAttemptV1Bytes(attempt)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "noncanonical" {
				body = append(body, '\n')
			}
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "report-publication", "attempts"), 512<<10, core.access)
			if err != nil {
				t.Fatal(err)
			}
			if err := cas.PutIfAbsent(context.Background(), attempt.AttemptID, body); err != nil {
				t.Fatal(err)
			}
			if scenario == "additional-orphan" {
				authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
				if err != nil {
					t.Fatal(err)
				}
				_, orphan := runtimeReservedReportAttemptFixtureV1(t, authority)
				body, err := domainpublication.PublicationAttemptV1Bytes(orphan)
				if err != nil {
					t.Fatal(err)
				}
				if err := cas.PutIfAbsent(context.Background(), orphan.AttemptID, body); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "terminal-same-held-thread" {
				authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
				if err != nil {
					t.Fatal(err)
				}
				var stored struct {
					SecurityState domainsecurity.TurnSecurityContext `json:"securityState"`
				}
				primaryBody, err := os.ReadFile(primary)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(primaryBody, &stored); err != nil {
					t.Fatal(err)
				}
				original := core.pendingInventory.Receipts[0]
				now, err := time.Parse(time.RFC3339Nano, original.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				var thread map[string]any
				if err := json.Unmarshal(primaryBody, &thread); err != nil {
					t.Fatal(err)
				}
				turn := thread["turns"].([]any)[0].(map[string]any)
				firstItem := turn["items"].([]any)[0].(map[string]any)
				previousGrant, err := domainsecurity.ParseExecutionGrant(firstItem["executionGrant"])
				if err != nil {
					t.Fatal(err)
				}
				terminalCall, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x75}, domainsecurity.HostToolCallIDEntropyBytesV1))
				if err != nil {
					t.Fatal(err)
				}
				grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
					Context: stored.SecurityState, Provider: previousGrant.Provider, ServerIdentity: previousGrant.ServerIdentity,
					ToolName: previousGrant.ToolName, ToolCallID: terminalCall, ArgsHash: previousGrant.ArgsHash,
					SchemaHash: previousGrant.SchemaHash, ScopeHash: previousGrant.ScopeHash, ReadOnly: false,
					ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
				})
				if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
					t.Fatal(err)
				}
				secondItem := map[string]any{}
				for key, value := range firstItem {
					secondItem[key] = value
				}
				secondItem["id"] = domaintoolcall.ToolCallItemIDV1(stored.SecurityState.TurnID, terminalCall)
				secondItem["callId"], secondItem["executionGrantId"], secondItem["executionGrant"], secondItem["createdAt"] = terminalCall, grant.GrantID, grant, grant.IssuedAt
				turn["items"] = append(turn["items"].([]any), secondItem)
				updatedPrimary, err := json.Marshal(thread)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(primary, updatedPrimary, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(updatedPrimary, &thread); err != nil {
					t.Fatal(err)
				}
				registry, err := executiongrantapp.RegistryFromThread(stored.SecurityState.ThreadID, thread, stored.SecurityState.TurnID)
				if err != nil {
					t.Fatal(err)
				}
				member, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
				if !found {
					t.Fatal("second actual issued grant is missing")
				}
				sign := func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) }
				terminal, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
					Kind: original.Kind, SecurityContext: stored.SecurityState, GrantRegistrySequence: registry.Sequence,
					GrantRegistryDigest: registry.StateDigest, GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: member.Sequence, RegistryEntryDigest: member.EntryDigest}},
					PayloadHash: domainsecurity.SHA256Hex([]byte("different-terminal-report-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("different-terminal-report-route")),
					IssuedAt: now, ExpiresAt: now.Add(time.Hour), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
				}, sign)
				if err != nil {
					t.Fatal(err)
				}
				disposition, err := domainpendingwork.NewPendingWorkDispositionV1(terminal, domainpendingwork.StatusFailed, "synthetic_stage_failed", now.Add(time.Minute), authority.KeyID(), authority.PublicKey(), sign)
				if err != nil {
					t.Fatal(err)
				}
				receiptBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(terminal)
				if err != nil {
					t.Fatal(err)
				}
				dispositionBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
				if err != nil {
					t.Fatal(err)
				}
				for leaf, body := range map[string][]byte{"receipts": receiptBody, "dispositions": dispositionBody} {
					pending, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pending-work", leaf), 1<<20, core.access)
					if err != nil {
						t.Fatal(err)
					}
					if err := errors.Join(pending.PutIfAbsent(context.Background(), terminal.WorkID, body), pending.Close()); err != nil {
						t.Fatal(err)
					}
				}
				copied := *core
				copied.pendingInventory.Receipts = []domainpendingwork.PendingWorkReceiptV1{terminal}
				terminalAttempt := runtimeOriginalReportAttemptFixtureV1(t, &copied, primary, terminalCall)
				body, err := domainpublication.PublicationAttemptV1Bytes(terminalAttempt)
				if err != nil {
					t.Fatal(err)
				}
				if err := cas.PutIfAbsent(context.Background(), terminalAttempt.AttemptID, body); err != nil {
					t.Fatal(err)
				}
			}
			if err := cas.Close(); err != nil {
				t.Fatal(err)
			}
			if scenario == "unknown" {
				residue := filepath.Join(roots.DataDir, "private", "report-publication", "attempts", attempt.AttemptID[:2], "."+attempt.AttemptID+".json-original.tmp")
				if err := os.WriteFile(residue, []byte("synthetic original uncommitted attempt bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			retained := []string{filepath.Dir(primary), filepath.Join(roots.DataDir, "private", "pending-work")}
			for _, owner := range runtimePublicationOwnersV1 {
				retained = append(retained, filepath.Join(roots.DataDir, "private", owner))
			}
			before := startupWholeTreeRecordMapForTest(t, retained...)
			whole := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			for i := 0; i < 2; i++ {
				handler, err := NewRuntimeServerHandlerE(config)
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, retained...)) {
					t.Fatal("ordinary restart changed original attempt, held Core or unavailable publication closure")
				}
				if scenario != "unknown" && scenario != "missing-disposition" {
					if err == nil {
						t.Fatal("invalid original attempt opened ordinary startup")
					}
					if scenario == "terminal-same-held-thread" && !errors.Is(err, errRuntimeReportRestartReconciliationRequired) {
						t.Fatalf("terminal attempt was not kept behind its exact unresolved-scope gate: %v", err)
					}
					if scenario != "noncanonical" && scenario != "terminal-same-held-thread" && !errors.Is(err, reportpublicationapp.ErrAttemptCoreLinkageV1) {
						t.Fatalf("invalid original attempt missed exact Core linkage refusal: %v", err)
					}
					if !reflect.DeepEqual(whole, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
						t.Fatal("invalid attempt refused after startup effects")
					}
					return
				}
				if err != nil {
					t.Fatalf("original attempt linked to the held Core blocked ordinary restart %d: %v", i, err)
				}
			}
		})
	}
}
