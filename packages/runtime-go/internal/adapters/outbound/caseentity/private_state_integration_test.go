package caseentity_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	caseentityadapter "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestBindingStableOrdinalsAreAtomicCaseScopedAndRestartStable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-ordinal", TurnID: "turn-ordinal", TenantID: "tenant-ordinal", UserID: "user-ordinal",
		CaseID: "case-ordinal", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "ordinal-a", ContextEpoch: 1,
	})

	const bindingCount = 100
	type bindingResult struct {
		index  int
		record domaincaseentity.CaseEntityBindingRecord
		err    error
	}
	results := make(chan bindingResult, bindingCount)
	var wait sync.WaitGroup
	for index := 0; index < bindingCount; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			reference, referenceErr := domaincaseentity.NewReferenceV1FromKeyedDigest(
				domainsecurity.SHA256Hex([]byte(fmt.Sprintf("ordinal-reference-%03d", index))),
			)
			if referenceErr != nil {
				results <- bindingResult{index: index, err: referenceErr}
				return
			}
			entityType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
			if index%2 == 1 {
				entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
			}
			writer := store
			if index%2 == 1 {
				writer = sibling
			}
			record, ensureErr := writer.EnsureBinding(
				ctx,
				domaincaseentity.NewCaseEntityBindingRecordInputV1(
					securityContext,
					entityType,
					reference,
					fmt.Sprintf("62220212%08d", index),
				),
			)
			results <- bindingResult{index: index, record: record, err: ensureErr}
		}()
	}
	wait.Wait()
	close(results)

	seen := make(map[uint32]domaincaseentity.CaseEntityBindingRecord, bindingCount)
	byIndex := make(map[int]domaincaseentity.CaseEntityBindingRecord, bindingCount)
	for result := range results {
		if result.err != nil {
			t.Fatalf("binding %d failed: %v", result.index, result.err)
		}
		if result.record.StableOrdinal == 0 || result.record.StableOrdinal > bindingCount {
			t.Fatalf("binding %d received invalid ordinal %d", result.index, result.record.StableOrdinal)
		}
		if previous, duplicate := seen[result.record.StableOrdinal]; duplicate {
			t.Fatalf("ordinal %d was shared by %s and %s", result.record.StableOrdinal, previous.Reference, result.record.Reference)
		}
		seen[result.record.StableOrdinal] = result.record
		byIndex[result.index] = result.record
	}
	if len(seen) != bindingCount {
		t.Fatalf("ordinal allocation coverage=%d want=%d", len(seen), bindingCount)
	}

	first := byIndex[0]
	var firstCanonical string
	if err := first.UseCanonicalValueV1(func(value string) error {
		firstCanonical = value
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	replayed, err := sibling.EnsureBinding(ctx, domaincaseentity.NewCaseEntityBindingRecordInputV1(
		securityContext, first.EntityType, first.Reference, firstCanonical,
	))
	if err != nil || replayed.RecordDigest != first.RecordDigest || replayed.StableOrdinal != first.StableOrdinal {
		t.Fatalf("idempotent ensure changed stable ordinal: first=%#v replayed=%#v err=%v", first, replayed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sibling.Close(); err != nil {
		t.Fatal(err)
	}
	prepared, err := caseentityadapter.PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatalf("sibling ordinal writes did not survive semantic recovery: %v", err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatalf("sibling ordinal recovery did not remain stable: %v", err)
	}

	restarted, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restartedRecord, err := restarted.ResolveBinding(ctx, first.BindingKey)
	if err != nil || restartedRecord.RecordDigest != first.RecordDigest || restartedRecord.StableOrdinal != first.StableOrdinal {
		t.Fatalf("restart changed stable ordinal: first=%#v restarted=%#v err=%v", first, restartedRecord, err)
	}

	otherCase := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-other", TurnID: "turn-other", TenantID: securityContext.TenantID, UserID: securityContext.UserID,
		CaseID: "case-other", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "ordinal-b", ContextEpoch: 1,
	})
	otherReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(domainsecurity.SHA256Hex([]byte("ordinal-other-case")))
	if err != nil {
		t.Fatal(err)
	}
	otherRecord, err := restarted.EnsureBinding(ctx, domaincaseentity.NewCaseEntityBindingRecordInputV1(
		otherCase,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		otherReference,
		"6217009876543210",
	))
	if err != nil || otherRecord.StableOrdinal != 1 {
		t.Fatalf("cross-case ordinal namespace was not isolated: record=%#v err=%v", otherRecord, err)
	}
}

func TestModelEntityAliasResolvesOnlyInsideCurrentCaseAndType(t *testing.T) {
	ctx := context.Background()
	store, service := openPersistentCaseEntityServiceV1(
		t,
		t.TempDir(),
		&recordingKeyedDigesterV1{key: []byte("installation-key-alias")},
	)
	defer store.Close()
	caseAThreadOne := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-alias-a-1", TurnID: "turn-alias-a-1", TenantID: "tenant-alias", UserID: "user-alias",
		CaseID: "case-alias-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "alias-a", ContextEpoch: 1,
	})
	caseAThreadTwo := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-alias-a-2", TurnID: "turn-alias-a-2", TenantID: "tenant-alias", UserID: "user-alias",
		CaseID: "case-alias-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "alias-a", ContextEpoch: 1,
	})
	caseB := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-alias-b", TurnID: "turn-alias-b", TenantID: "tenant-alias", UserID: "user-alias",
		CaseID: "case-alias-b", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "alias-b", ContextEpoch: 1,
	})

	const accountExact = "6222021234567890"
	accountRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		caseAThreadOne,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		accountExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	cardRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		caseAThreadOne,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		"6217009876543210",
	))
	if err != nil {
		t.Fatal(err)
	}
	caseBAccountRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		caseB,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		accountExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	caseBCardRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		caseB,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		"6217009876543210",
	))
	if err != nil {
		t.Fatal(err)
	}
	if caseBAccountRef == accountRef || caseBCardRef == cardRef {
		t.Fatalf("same canonical value linked authority refs across cases: account=%q/%q card=%q/%q", accountRef, caseBAccountRef, cardRef, caseBCardRef)
	}

	resolve := func(securityContext domainsecurity.TurnSecurityContext, alias string) (domaincaseentity.ReferenceV1, string, error) {
		var reference domaincaseentity.ReferenceV1
		var canonical string
		err := service.UseVerifiedBindingByAliasV1(
			ctx,
			caseentityapp.ResolveVerifiedBindingByAliasInputV1{
				SecurityContext: securityContext,
				Alias:           domaincaseentity.ModelEntityAliasV1(alias),
			},
			func(current domaincaseentity.ReferenceV1, value string, digest string) error {
				if !domainsecurity.IsSHA256Hex(digest) {
					t.Fatal("alias resolution returned an invalid private digest")
				}
				reference, canonical = current, value
				return nil
			},
		)
		return reference, canonical, err
	}

	for _, current := range []domainsecurity.TurnSecurityContext{caseAThreadOne, caseAThreadTwo} {
		reference, canonical, err := resolve(current, "acct:1")
		if err != nil || reference != accountRef || canonical != accountExact {
			t.Fatalf("same-case alias did not preserve identity: ref=%q canonical=%q err=%v", reference, canonical, err)
		}
		reference, _, err = resolve(current, "card:2")
		if err != nil || reference != cardRef {
			t.Fatalf("card alias did not resolve its exact type: ref=%q err=%v", reference, err)
		}
	}
	if reference, canonical, err := resolve(caseB, "acct:1"); err != nil ||
		reference != caseBAccountRef || reference == accountRef || canonical != accountExact {
		t.Fatalf("case-local acct:1 did not remain unlinkable: ref=%q canonical=%q err=%v", reference, canonical, err)
	}
	if reference, _, err := resolve(caseB, "card:2"); err != nil ||
		reference != caseBCardRef || reference == cardRef {
		t.Fatalf("case-local card:2 did not remain unlinkable: ref=%q err=%v", reference, err)
	}
	for name, test := range map[string]struct {
		context domainsecurity.TurnSecurityContext
		alias   string
	}{
		"wrong prefix": {caseAThreadOne, "card:1"},
		"missing":      {caseAThreadOne, "acct:99"},
	} {
		_, value, err := resolve(test.context, test.alias)
		if !errors.Is(err, caseentityapp.ErrReferenceNotFound) || value != "" ||
			strings.Contains(err.Error(), accountExact) || strings.Contains(err.Error(), string(accountRef)) {
			t.Fatalf("%s alias did not fail closed: value=%q err=%v", name, value, err)
		}
	}
}

func TestCaseLongitudinalContinuitySeparatesTransitionsAndSnapshotFacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-longitudinal")}
	store, service := openPersistentCaseEntityServiceV1(t, root, digester)
	base := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-origin", TurnID: "turn-origin", TenantID: "tenant-longitudinal", UserID: "user-longitudinal",
		CaseID: "case-longitudinal", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	accountRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		base, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, "6222021234567890",
	))
	if err != nil {
		t.Fatal(err)
	}
	cardRef, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		base, domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1, "6217009876543210",
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AppendCaseLongitudinalIngressV1(ctx, caseentityapp.AppendCaseLongitudinalIngressInputV1{
		SecurityContext: base, References: []domaincaseentity.ReferenceV1{accountRef, cardRef},
	}); err != nil {
		t.Fatal(err)
	}
	index, err := store.ResolveLatestCaseLongitudinalContext(ctx, base)
	if err != nil || index.Generation != 1 || len(index.EntityReferences) != 2 || len(index.EntityIdentities) != 2 {
		t.Fatalf("initial longitudinal index mismatch: index=%#v err=%v", index, err)
	}
	aliases := map[domaincaseentity.ModelEntityAliasV1]domaincaseentity.ReferenceV1{}
	for _, identity := range index.EntityIdentities {
		alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
		if aliasErr != nil {
			t.Fatal(aliasErr)
		}
		aliases[alias] = identity.Reference
	}
	if aliases["acct:1"] != accountRef || aliases["card:2"] != cardRef {
		t.Fatalf("longitudinal index did not bind refs and stable ordinals: %#v", index.EntityIdentities)
	}
	evidenceReference := "evr_" + domainsecurity.SHA256Hex([]byte("longitudinal-evidence-reference"))
	evidenceDigest := domainsecurity.SHA256Hex([]byte("longitudinal-evidence-digest"))
	claimReference := "claim_longitudinal_origin"
	claimDigest := domainsecurity.SHA256Hex([]byte("longitudinal-claim-digest"))
	continuationDigest := domainsecurity.SHA256Hex([]byte("longitudinal-continuation-digest"))
	ownerState := caseentityapp.AppendCaseLongitudinalOwnerStateInputV1{
		SecurityContext: base,
		Evidence: []caseentityapp.CaseLongitudinalEvidenceDigestV1{{
			EvidenceReference: evidenceReference,
			EvidenceDigest:    evidenceDigest,
		}},
		Claims: []caseentityapp.CaseLongitudinalClaimDigestV1{{
			ClaimReference: claimReference, ClaimDigest: claimDigest,
			InvestigationState: domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences: []string{evidenceReference}, CounterEvidenceReferences: []string{},
		}},
		ContinuationDigest: continuationDigest,
	}
	if err := service.AppendCaseLongitudinalOwnerStateV1(ctx, ownerState); err != nil {
		t.Fatalf("append typed longitudinal owner state: %v", err)
	}
	ownerIndex, err := store.ResolveLatestCaseLongitudinalContext(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	ownerThread, err := store.ResolveLatestThreadContextForScope(ctx, base, base.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AppendCaseLongitudinalOwnerStateV1(ctx, ownerState); err != nil {
		t.Fatalf("replay typed longitudinal owner state: %v", err)
	}
	replayedIndex, indexErr := store.ResolveLatestCaseLongitudinalContext(ctx, base)
	replayedThread, threadErr := store.ResolveLatestThreadContextForScope(ctx, base, base.ThreadID)
	if indexErr != nil || threadErr != nil || replayedIndex.RecordDigest != ownerIndex.RecordDigest ||
		replayedThread.RecordDigest != ownerThread.RecordDigest {
		t.Fatalf(
			"owner-state replay changed immutable index/thread generations: index=%d/%s -> %d/%s thread=%d/%s -> %d/%s indexErr=%v threadErr=%v",
			ownerIndex.Generation, ownerIndex.RecordDigest, replayedIndex.Generation, replayedIndex.RecordDigest,
			ownerThread.Generation, ownerThread.RecordDigest, replayedThread.Generation, replayedThread.RecordDigest,
			indexErr, threadErr,
		)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restartedStore, restarted := openPersistentCaseEntityServiceV1(t, root, digester)
	defer restartedStore.Close()
	use := func(
		securityContext domainsecurity.TurnSecurityContext,
		relation, sourceThreadID string,
		expected caseentityapp.CaseContinuityTransitionV1,
		aliases ...domaincaseentity.ModelEntityAliasV1,
	) (domaincaseentity.ThreadCaseContextRecord, caseentityapp.ProviderIngressLongitudinalStateV1) {
		t.Helper()
		calls := 0
		var selected caseentityapp.CaseLongitudinalAliasSelectionV1
		err := restarted.UseCaseLongitudinalAliasesV1(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
			SecurityContext: securityContext, Aliases: aliases, Relation: relation,
			SourceThreadID: sourceThreadID, ExpectedTransition: expected,
			ProviderText: "analyze " + strings.Join(caseEntityAliasStringsV1(aliases), " "),
		}, func(current caseentityapp.CaseLongitudinalAliasSelectionV1) error {
			calls++
			selected = current
			return nil
		})
		if err != nil || calls != 1 || selected.Transition != expected || len(selected.References) != len(aliases) {
			t.Fatalf("%s continuity failed: selected=%#v calls=%d err=%v", expected, selected, calls, err)
		}
		threadContext, resolveErr := restartedStore.ResolveLatestThreadContextForScope(
			ctx, securityContext, securityContext.ThreadID,
		)
		if resolveErr != nil || threadContext.RecordDigest != selected.Record.RecordDigest {
			t.Fatalf("%s thread context mismatch: context=%#v err=%v", expected, threadContext, resolveErr)
		}
		var providerState caseentityapp.ProviderIngressLongitudinalStateV1
		if err := selected.ProviderProjection.UseExactWithDescriptorsAndStateV1(func(
			_ string,
			_ uint32,
			useDescriptors caseentityapp.ProviderIngressDescriptorsUseV1,
			state caseentityapp.ProviderIngressLongitudinalStateV1,
		) error {
			providerState = state
			return useDescriptors(func(uint32, domaincaseentity.ModelEntityAliasV1, string, string, string, string) error {
				return nil
			})
		}); err != nil {
			t.Fatalf("%s provider longitudinal projection failed: %v", expected, err)
		}
		return threadContext, providerState
	}

	restartContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: base.ThreadID, TurnID: "turn-restart", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash, SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	restartRecord, _ := use(restartContext, "primary", "", caseentityapp.CaseContinuityRestartV1, "acct:1")
	if !containsCaseEntityReferenceV1(restartRecord.EntityReferences, accountRef) {
		t.Fatalf("restart changed account authority reference: %#v", restartRecord.EntityReferences)
	}

	independent := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-independent", TurnID: "turn-independent", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash, SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	independentRecord, independentState := use(independent, "primary", "", caseentityapp.CaseContinuityIndependentV1, "acct:1", "card:2")
	if len(independentRecord.Claims) != 1 || independentRecord.Claims[0].ClaimDigest != claimDigest ||
		len(independentRecord.Evidence) != 1 || independentRecord.Evidence[0].EvidenceDigest != evidenceDigest ||
		len(independentRecord.Continuations) != 1 || independentRecord.Continuations[0].ContinuationDigest != continuationDigest ||
		independentRecord.Continuations[0].Currentness != domaincaseentity.SnapshotCurrentV1 ||
		len(independentState.Claims) != 1 || independentState.Claims[0].Digest != claimDigest ||
		len(independentState.Evidence) != 1 || independentState.Evidence[0].Digest != evidenceDigest ||
		len(independentState.Continuations) != 1 || independentState.Continuations[0].Digest != continuationDigest ||
		independentState.Continuations[0].Currentness != domaincaseentity.SnapshotCurrentV1 ||
		!containsCaseEntityReferenceV1(independentRecord.EntityReferences, accountRef) ||
		!containsCaseEntityReferenceV1(independentRecord.EntityReferences, cardRef) {
		t.Fatalf("independent thread lost typed owner state or changed card reference: %#v", independentRecord)
	}

	fork := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-fork", TurnID: "turn-fork", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash, SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	_, _ = use(fork, "fork", base.ThreadID, caseentityapp.CaseContinuityForkV1, "acct:1")
	resume := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-resume", TurnID: "turn-resume", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash, SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	_, _ = use(resume, "primary", base.ThreadID, caseentityapp.CaseContinuityResumeV1, "acct:1")

	compaction, err := currentdatasettest.NewHarness().NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: base.ThreadID, TurnID: "turn-compaction", WorkspaceRealPath: base.WorkspaceRealPath,
		TenantID: base.TenantID, UserID: base.UserID, CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash,
		DatasetSnapshotID: base.DatasetSnapshotID, SourceManifestHash: base.SourceManifestHash,
		ContextEpoch: 2, IssuedAt: time.Unix(1_700_000_002, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, compactedState := use(compaction, "primary", "", caseentityapp.CaseContinuityCompactionV1, "acct:1")
	compactedIndex, err := restartedStore.ResolveLatestCaseLongitudinalContext(ctx, compaction)
	if err != nil || len(compactedIndex.Claims) != 1 || compactedIndex.Claims[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(compactedIndex.Evidence) != 1 || compactedIndex.Evidence[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(compactedState.Items) != 4 || compactedState.Items[0].Kind != caseentityapp.ProviderIngressLongitudinalHistoricalComparisonFactV1 {
		t.Fatalf("compaction promoted old snapshot facts: index=%#v err=%v", compactedIndex, err)
	}

	evolved := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-evolved-independent", TurnID: "turn-evolved", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash, SnapshotSeed: "longitudinal-b", ContextEpoch: 3,
	})
	evolvedThread, evolvedState := use(evolved, "primary", "", caseentityapp.CaseContinuityIndependentV1, "acct:1")
	evolvedIndex, err := restartedStore.ResolveLatestCaseLongitudinalContext(ctx, evolved)
	if err != nil || len(evolvedIndex.Claims) != 1 || evolvedIndex.Claims[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedIndex.Evidence) != 1 || evolvedIndex.Evidence[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedIndex.Continuations) != 1 || evolvedIndex.Continuations[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedThread.Claims) != 1 || evolvedThread.Claims[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedThread.Evidence) != 1 || evolvedThread.Evidence[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedThread.Continuations) != 1 || evolvedThread.Continuations[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(evolvedState.Claims) != 0 || len(evolvedState.Evidence) != 0 || len(evolvedState.Continuations) != 0 {
		t.Fatalf("snapshot evolution upgraded or copied old facts: index=%#v thread=%#v err=%v", evolvedIndex, evolvedThread, err)
	}
	if len(evolvedState.Items) < 4 ||
		evolvedState.Items[0].Kind != caseentityapp.ProviderIngressLongitudinalHistoricalComparisonFactV1 ||
		evolvedState.Items[0].Digest != claimDigest || evolvedState.Items[0].Currentness != domaincaseentity.SnapshotStaleV1 ||
		evolvedState.Items[len(evolvedState.Items)-1].Kind != caseentityapp.ProviderIngressLongitudinalSnapshotDifferenceV1 {
		t.Fatalf("new snapshot did not retain the old claim as bounded historical comparison: %#v", evolvedState)
	}
	for _, item := range evolvedState.Items {
		if item.Digest == claimDigest && item.Currentness == domaincaseentity.SnapshotCurrentV1 {
			t.Fatal("historical claim was selected as current support")
		}
	}

	currentEvidenceReference := "evr_" + domainsecurity.SHA256Hex([]byte("longitudinal-current-evidence-reference"))
	currentEvidenceDigest := domainsecurity.SHA256Hex([]byte("longitudinal-current-evidence-digest"))
	currentClaimDigest := domainsecurity.SHA256Hex([]byte("longitudinal-current-claim-digest"))
	currentContinuationDigest := domainsecurity.SHA256Hex([]byte("longitudinal-current-continuation-digest"))
	if err := restarted.AppendCaseLongitudinalOwnerStateV1(ctx, caseentityapp.AppendCaseLongitudinalOwnerStateInputV1{
		SecurityContext: evolved,
		Evidence: []caseentityapp.CaseLongitudinalEvidenceDigestV1{{
			EvidenceReference: currentEvidenceReference, EvidenceDigest: currentEvidenceDigest,
		}},
		Claims: []caseentityapp.CaseLongitudinalClaimDigestV1{{
			ClaimReference: "claim_longitudinal_current", ClaimDigest: currentClaimDigest, ClaimType: "entity",
			InvestigationState: domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences: []string{currentEvidenceReference}, CounterEvidenceReferences: []string{},
		}},
		ContinuationDigest: currentContinuationDigest,
	}); err != nil {
		t.Fatalf("append current-snapshot owner state: %v", err)
	}
	evolvedRestart := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: evolved.ThreadID, TurnID: "turn-evolved-restart", TenantID: evolved.TenantID, UserID: evolved.UserID,
		CaseID: evolved.CaseID, CaseBindingHash: evolved.CaseBindingHash, SnapshotSeed: "longitudinal-b", ContextEpoch: 3,
	})
	_, mixedState := use(evolvedRestart, "primary", "", caseentityapp.CaseContinuityRestartV1, "acct:1")
	if len(mixedState.Items) < 2 ||
		mixedState.Items[0].Kind != caseentityapp.ProviderIngressLongitudinalCurrentVerifiedFactV1 ||
		mixedState.Items[0].Digest != currentClaimDigest || mixedState.Items[0].Currentness != domaincaseentity.SnapshotCurrentV1 ||
		mixedState.Items[1].Kind != caseentityapp.ProviderIngressLongitudinalHistoricalComparisonFactV1 ||
		mixedState.Items[1].Digest != claimDigest || mixedState.Items[1].Currentness != domaincaseentity.SnapshotStaleV1 ||
		len(mixedState.Claims) != 1 || mixedState.Claims[0].Digest != currentClaimDigest {
		t.Fatalf("current and historical owner facts lost fixed priority or exact currentness: %#v", mixedState)
	}

	wrongCase := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-wrong-case", TurnID: "turn-wrong-case", TenantID: base.TenantID, UserID: base.UserID,
		CaseID: "case-other", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "longitudinal-a", ContextEpoch: 1,
	})
	if err := restarted.UseCaseLongitudinalAliasesV1(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
		SecurityContext: wrongCase, Aliases: []domaincaseentity.ModelEntityAliasV1{"acct:1"},
		ProviderText: "analyze acct:1",
	}, func(caseentityapp.CaseLongitudinalAliasSelectionV1) error { return nil }); err == nil ||
		strings.Contains(err.Error(), string(accountRef)) {
		t.Fatalf("cross-case alias selection did not fail closed: %v", err)
	}
	duplicateCalls := 0
	if err := restarted.UseCaseLongitudinalAliasesV1(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
		SecurityContext: independent,
		Aliases:         []domaincaseentity.ModelEntityAliasV1{"acct:1", "acct:1"},
		ProviderText:    "analyze acct:1",
	}, func(caseentityapp.CaseLongitudinalAliasSelectionV1) error {
		duplicateCalls++
		return nil
	}); err == nil || duplicateCalls != 0 || strings.Contains(err.Error(), string(accountRef)) {
		t.Fatalf("duplicate alias selection did not fail closed: calls=%d err=%v", duplicateCalls, err)
	}
}

func caseEntityAliasStringsV1(aliases []domaincaseentity.ModelEntityAliasV1) []string {
	values := make([]string, len(aliases))
	for index, alias := range aliases {
		values[index] = string(alias)
	}
	return values
}

func containsCaseEntityReferenceV1(
	references []domaincaseentity.ReferenceV1,
	want domaincaseentity.ReferenceV1,
) bool {
	for _, reference := range references {
		if reference == want {
			return true
		}
	}
	return false
}

func TestPrivateStateSurvivesRestartAndKeepsReferenceStableAcrossSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-a")}
	firstStore, firstService := openPersistentCaseEntityServiceV1(t, root, digester)
	firstContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 7,
	})
	secondContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-b", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 8,
	})

	reference, err := firstService.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		firstContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222-0212 3456-7890",
	))
	if err != nil {
		t.Fatal(err)
	}
	ingressReference, err := persistAccountIngressTestV1(
		ctx,
		firstService,
		firstContext,
		reference,
		"6222-0212 3456-7890",
	)
	if err != nil {
		t.Fatal(err)
	}
	threadReference, err := firstService.PersistThreadCaseContextV1(
		ctx,
		domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext:  firstContext,
			Generation:       1,
			EntityReferences: []domaincaseentity.ReferenceV1{reference},
			Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
				DatasetSnapshotID: firstContext.DatasetSnapshotID,
				ContextEpoch:      firstContext.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}},
			Claims:                 []domaincaseentity.CaseClaimStateV1{},
			Evidence:               []domaincaseentity.CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{"question_next_period"},
			DataGapReferences:      []string{"gap_counterparty_name"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatal(err)
	}

	secondStore, secondService := openPersistentCaseEntityServiceV1(t, root, digester)
	defer secondStore.Close()
	var resolved string
	resolveCalls := 0
	err = secondService.UseBoundReferenceV1(ctx, caseentityapp.ResolveBoundReferenceInputV1{
		SecurityContext:      secondContext,
		FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		Reference:            reference,
	}, func(value string) error {
		resolveCalls++
		resolved = value
		return nil
	})
	if err != nil || resolveCalls != 1 || resolved != "6222021234567890" {
		t.Fatalf("restart did not restore the exact private binding: err=%v", err)
	}
	rebound, err := secondService.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		secondContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	))
	if err != nil || rebound != reference {
		t.Fatalf("snapshot evolution changed the stable reference: rebound=%q err=%v", rebound, err)
	}
	var ingress domaincaseentity.CaseIngressRecord
	var ingressRaw string
	ingressCalls := 0
	err = secondService.UseIngressPrivateV1(
		ctx,
		firstContext,
		ingressReference.RecordID,
		func(record domaincaseentity.CaseIngressRecord, rawText string) error {
			ingressCalls++
			ingress = record
			ingressRaw = rawText
			return nil
		},
	)
	if err != nil || ingressCalls != 1 || ingress.RecordDigest != ingressReference.RecordDigest ||
		!strings.Contains(ingressRaw, "6222-0212 3456-7890") {
		t.Fatalf("restart under the same TSC did not restore exact private ingress: err=%v", err)
	}
	for _, test := range []struct {
		name            string
		securityContext domainsecurity.TurnSecurityContext
	}{
		{
			name: "new turn",
			securityContext: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-a", TurnID: "turn-b", TenantID: "tenant-a", UserID: "user-a",
				CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 7,
			}),
		},
		{
			name: "new snapshot",
			securityContext: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
				CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 7,
			}),
		},
		{
			name: "new epoch",
			securityContext: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
				CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 8,
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mismatchedCalls := 0
			err := secondService.UseIngressPrivateV1(
				ctx,
				test.securityContext,
				ingressReference.RecordID,
				func(domaincaseentity.CaseIngressRecord, string) error {
					mismatchedCalls++
					return nil
				},
			)
			if !errors.Is(err, caseentityapp.ErrPrivateStateIntegrity) || mismatchedCalls != 0 {
				t.Fatalf("restart restored ingress under a non-exact TSC: calls=%d err=%v", mismatchedCalls, err)
			}
		})
	}
	threadContext, err := secondService.ResolveThreadCaseContextPrivateV1(ctx, secondContext, 1)
	if err != nil || threadContext.RecordDigest != threadReference.RecordDigest ||
		len(threadContext.EntityReferences) != 1 || threadContext.EntityReferences[0] != reference {
		t.Fatalf("restart did not restore typed thread context: err=%v", err)
	}
}

func TestPrivateBindingCollisionAndCrossCaseLookupFailClosedWithoutEcho(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	constantDigester := &recordingKeyedDigesterV1{forcedDigest: strings.Repeat("0", 64)}
	store, service := openPersistentCaseEntityServiceV1(t, root, constantDigester)
	defer store.Close()
	contextA := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	contextB := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-b", TurnID: "turn-b", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-b", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 1,
	})
	const firstExact = "6222021234567890"
	const collidingExact = "6217009876543210"
	_, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		contextA,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		firstExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		contextA,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		collidingExact,
	)); !errors.Is(err, caseentityapp.ErrPrivateStateConflict) ||
		strings.Contains(err.Error(), firstExact) ||
		strings.Contains(err.Error(), collidingExact) {
		t.Fatalf("same-scope keyed collision was not safely rejected: %v", err)
	}

	crossStore, crossService := openPersistentCaseEntityServiceV1(
		t,
		t.TempDir(),
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
	)
	defer crossStore.Close()
	referenceA, err := crossService.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		contextA,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		firstExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	referenceB, err := crossService.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		contextB,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		firstExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	if referenceA == referenceB {
		t.Fatal("default cross-case token isolation failed")
	}
	crossCaseCalls := 0
	if err := crossService.UseBoundReferenceV1(ctx, caseentityapp.ResolveBoundReferenceInputV1{
		SecurityContext:      contextB,
		FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		Reference:            referenceA,
	}, func(string) error {
		crossCaseCalls++
		return nil
	}); !errors.Is(err, caseentityapp.ErrReferenceNotFound) || crossCaseCalls != 0 {
		t.Fatalf("case B resolved case A private mapping: calls=%d err=%v", crossCaseCalls, err)
	}
	if _, err := crossService.PersistThreadCaseContextV1(
		ctx,
		domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext:  contextB,
			Generation:       1,
			EntityReferences: []domaincaseentity.ReferenceV1{referenceA},
			Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
				DatasetSnapshotID: contextB.DatasetSnapshotID,
				ContextEpoch:      contextB.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}},
			Claims:                 []domaincaseentity.CaseClaimStateV1{},
			Evidence:               []domaincaseentity.CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{},
			DataGapReferences:      []string{},
		},
	); !errors.Is(err, caseentityapp.ErrReferenceNotFound) {
		t.Fatalf("case B persisted case A reference: %v", err)
	}

	forgedReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		strings.Repeat("f", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	forgedRecord, err := crossStore.EnsureBinding(
		ctx,
		domaincaseentity.NewCaseEntityBindingRecordInputV1(
			contextA,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			forgedReference,
			firstExact,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if forgedRecord.StableOrdinal != 2 {
		t.Fatalf("forged test binding received ordinal %d", forgedRecord.StableOrdinal)
	}
	forgedCalls := 0
	if err := crossService.UseBoundReferenceV1(
		ctx,
		caseentityapp.ResolveBoundReferenceInputV1{
			SecurityContext:      contextA,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			Reference:            forgedReference,
		},
		func(string) error {
			forgedCalls++
			return nil
		},
	); !errors.Is(err, caseentityapp.ErrPrivateStateIntegrity) || forgedCalls != 0 {
		t.Fatalf("structurally valid but non-keyed binding was trusted: calls=%d err=%v", forgedCalls, err)
	}
	if _, err := crossService.PersistThreadCaseContextV1(
		ctx,
		domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext:  contextA,
			Generation:       1,
			EntityReferences: []domaincaseentity.ReferenceV1{forgedReference},
			Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
				DatasetSnapshotID: contextA.DatasetSnapshotID,
				ContextEpoch:      contextA.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}},
			Claims:                 []domaincaseentity.CaseClaimStateV1{},
			Evidence:               []domaincaseentity.CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{},
			DataGapReferences:      []string{},
		},
	); !errors.Is(err, caseentityapp.ErrPrivateStateIntegrity) {
		t.Fatalf("thread context trusted a structurally valid but non-keyed binding: %v", err)
	}
}

func TestPrivateIngressAndThreadGenerationAreNoReplace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, service := openPersistentCaseEntityServiceV1(
		t,
		root,
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
	)
	defer store.Close()
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	reference, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistAccountIngressTestV1(ctx, service, securityContext, reference, "6222-0212 3456-7890"); err != nil {
		t.Fatal(err)
	}
	if _, err := persistAccountIngressTestV1(ctx, service, securityContext, reference, "6222021234567890"); !errors.Is(err, caseentityapp.ErrPrivateStateConflict) {
		t.Fatalf("same ingress slot was overwritten: %v", err)
	}

	baseInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext:  securityContext,
		Generation:       1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      securityContext.ContextEpoch,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Claims:            []domaincaseentity.CaseClaimStateV1{},
		Evidence:          []domaincaseentity.CaseEvidenceStateV1{},
		DataGapReferences: []string{},
	}
	baseInput.OpenQuestionReferences = []string{"question_a"}
	if _, err := service.PersistThreadCaseContextV1(ctx, baseInput); err != nil {
		t.Fatal(err)
	}
	baseInput.OpenQuestionReferences = []string{"question_b"}
	if _, err := service.PersistThreadCaseContextV1(ctx, baseInput); !errors.Is(err, caseentityapp.ErrPrivateStateConflict) {
		t.Fatalf("same thread context generation was overwritten: %v", err)
	}
}

func TestPersistentThreadContextRequiresHistoricalDowngradeOnSnapshotChange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, service := openPersistentCaseEntityServiceV1(
		t,
		root,
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
	)
	defer store.Close()
	firstContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 4,
	})
	secondContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-b", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 5,
	})
	reference, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(
		firstContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	))
	if err != nil {
		t.Fatal(err)
	}
	firstInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext:  firstContext,
		Generation:       1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			ContextEpoch:      firstContext.ContextEpoch,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Evidence: []domaincaseentity.CaseEvidenceStateV1{{
			EvidenceReference: "evidence_a",
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Claims: []domaincaseentity.CaseClaimStateV1{{
			ClaimReference:            "claim_a",
			DatasetSnapshotID:         firstContext.DatasetSnapshotID,
			Currentness:               domaincaseentity.SnapshotCurrentV1,
			InvestigationState:        domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences:        []string{"evidence_a"},
			CounterEvidenceReferences: []string{},
		}},
		OpenQuestionReferences: []string{},
		DataGapReferences:      []string{},
	}
	firstRecord, err := domaincaseentity.NewThreadCaseContextRecordV1(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PersistThreadCaseContextV1(ctx, firstInput); err != nil {
		t.Fatal(err)
	}

	secondInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext:      secondContext,
		Generation:           2,
		PreviousRecordDigest: firstRecord.RecordDigest,
		EntityReferences:     []domaincaseentity.ReferenceV1{reference},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{
			{
				DatasetSnapshotID: firstContext.DatasetSnapshotID,
				ContextEpoch:      firstContext.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotHistoricalV1,
			},
			{
				DatasetSnapshotID: secondContext.DatasetSnapshotID,
				ContextEpoch:      secondContext.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			},
		},
		Evidence: []domaincaseentity.CaseEvidenceStateV1{{
			EvidenceReference: "evidence_a",
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			Currentness:       domaincaseentity.SnapshotHistoricalV1,
		}},
		Claims: []domaincaseentity.CaseClaimStateV1{{
			ClaimReference:            "claim_a",
			DatasetSnapshotID:         firstContext.DatasetSnapshotID,
			Currentness:               domaincaseentity.SnapshotHistoricalV1,
			InvestigationState:        domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences:        []string{"evidence_a"},
			CounterEvidenceReferences: []string{},
		}},
		OpenQuestionReferences: []string{},
		DataGapReferences:      []string{},
	}
	if _, err := service.PersistThreadCaseContextV1(ctx, secondInput); err != nil {
		t.Fatalf("valid historical snapshot transition failed: %v", err)
	}

	secondInput.Generation = 3
	secondInput.PreviousRecordDigest = strings.Repeat("f", 64)
	secondInput.Claims[0].Currentness = domaincaseentity.SnapshotCurrentV1
	if _, err := service.PersistThreadCaseContextV1(ctx, secondInput); !errors.Is(err, caseentityapp.ErrPrivateStateIntegrity) {
		t.Fatalf("stale claim promotion was not rejected: %v", err)
	}
}

func openPersistentCaseEntityServiceV1(
	t *testing.T,
	root string,
	digester caseentityapp.KeyedPayloadDigester,
) (*caseentityadapter.Store, *caseentityapp.Service) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	harness := currentdatasettest.NewHarness()
	return store, caseentityapp.NewPersistentService(
		digester,
		store,
		harness,
		harness,
		harness.ValidateCurrent,
	)
}

func persistAccountIngressTestV1(
	ctx context.Context,
	service *caseentityapp.Service,
	securityContext domainsecurity.TurnSecurityContext,
	reference domaincaseentity.ReferenceV1,
	rawValue string,
) (caseentityapp.PrivateRecordReferenceV1, error) {
	raw := "分析账号 " + rawValue + " 的流水"
	canonical, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(rawValue)
	if err != nil {
		return caseentityapp.PrivateRecordReferenceV1{}, err
	}
	compiled, err := service.CompileAccountIngressV1(ctx, caseentityapp.NewCompileAccountIngressInputV1(
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		raw,
		func(
			_ context.Context,
			currentContext domainsecurity.TurnSecurityContext,
			candidates caseentityapp.AccountIngressCandidateBatchV1,
		) (caseentityapp.AccountIngressResolutionBatchV1, error) {
			var values []string
			if err := candidates.UseExactV1(func(current []string) error {
				values = append([]string(nil), current...)
				return nil
			}); err != nil || len(values) != 1 || values[0] != canonical {
				return caseentityapp.AccountIngressResolutionBatchV1{}, errors.New("unexpected ingress candidate")
			}
			descriptor, err := currentdatasettest.AccountIngressDescriptorV1(currentContext)
			if err != nil {
				return caseentityapp.AccountIngressResolutionBatchV1{}, err
			}
			return caseentityapp.NewAccountIngressResolutionBatchV1(
				currentContext,
				descriptor,
				domainnative.ResolveAccountIngressResultV1{
					SchemaVersion: 1,
					Contract:      domainnative.AccountIngressResolutionResultContractV1,
					Resolutions: []domainnative.AccountIngressResolutionV1{{
						Ordinal:     0,
						Disposition: domainnative.AccountIngressResolutionDispositionResolvedV1,
						EntityType:  domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					}},
					Provenance: domainnative.AccountIngressResolutionProvenanceV1{
						DatasetSnapshotID:              currentContext.DatasetSnapshotID,
						ContextEpoch:                   currentContext.ContextEpoch,
						ContextDigest:                  currentContext.ContextDigest,
						CaseBindingHash:                currentContext.CaseBindingHash,
						ExpectedProducerContentID:      descriptor.FundsProducerContentID,
						ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
						DuckDBContentSnapshotDigest:    descriptor.DuckDBContentSnapshotDigest,
						DuckDBSnapshotManifestSHA256:   descriptor.DuckDBSnapshotManifestSHA256,
						MaterializationIdentity:        descriptor.MaterializationIdentity,
						SourceSignature: strings.TrimPrefix(
							descriptor.MaterializationIdentity,
							domainsecurity.FundsMaterializationIdentityPrefixV1,
						),
						ResultSignature: domainsecurity.SHA256Hex(
							[]byte("test-account-ingress-result:\x00" + descriptor.DescriptorDigest),
						),
						ProducerContentID:      descriptor.FundsProducerContentID,
						ProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
						QueryContract:          domainnative.AccountIngressResolutionQueryContractV1,
						QuerySQLHash:           domainnative.AccountIngressResolutionQuerySQLHashV1,
					},
				},
			)
		},
	))
	if err != nil {
		return caseentityapp.PrivateRecordReferenceV1{}, err
	}
	providerAliasObserved := false
	if err := service.UseIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		func(projection caseentityapp.ProviderIngressProjectionV1) error {
			return projection.UseExactV1(func(providerText string) error {
				providerAliasObserved = strings.Contains(providerText, "acct:1") &&
					!strings.Contains(providerText, string(reference))
				return nil
			})
		},
	); err != nil {
		return caseentityapp.PrivateRecordReferenceV1{}, err
	}
	if !providerAliasObserved {
		return caseentityapp.PrivateRecordReferenceV1{}, errors.New("stable ingress alias changed")
	}
	var privateReference caseentityapp.PrivateRecordReferenceV1
	if err := compiled.UsePrivateRecordReferenceV1(func(current caseentityapp.PrivateRecordReferenceV1) error {
		privateReference = current
		return nil
	}); err != nil {
		return caseentityapp.PrivateRecordReferenceV1{}, err
	}
	return privateReference, nil
}

type caseEntityTestContextInputV1 struct {
	ThreadID        string
	TurnID          string
	TenantID        string
	UserID          string
	CaseID          string
	CaseBindingHash string
	SnapshotSeed    string
	ContextEpoch    uint64
}

func caseEntityTestContextV1(
	t *testing.T,
	input caseEntityTestContextInputV1,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := currentdatasettest.NewHarness().NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, WorkspaceRealPath: "/cases/current",
		TenantID: input.TenantID, UserID: input.UserID, CaseID: input.CaseID, CaseBindingHash: input.CaseBindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(input.SnapshotSeed),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + input.SnapshotSeed)),
		ContextEpoch:       input.ContextEpoch, IssuedAt: time.Unix(1_700_000_000+int64(input.ContextEpoch), 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

type recordingKeyedDigesterV1 struct {
	key          []byte
	forcedDigest string
}

func (digester *recordingKeyedDigesterV1) KeyedPayloadHash(
	_ context.Context,
	purpose string,
	payload []byte,
) (string, error) {
	if digester.forcedDigest != "" {
		return digester.forcedDigest, nil
	}
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
