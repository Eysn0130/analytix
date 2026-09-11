package caseentity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestStoreHasRecordsIsBoundedAcrossFixedPartitions(t *testing.T) {
	records := caseEntityRecoveryRecordsV1(t)
	for _, fixture := range []struct {
		name string
		put  func(context.Context, *Store) error
	}{
		{name: "binding", put: func(ctx context.Context, store *Store) error {
			_, err := store.EnsureBinding(ctx, caseEntityRecoveryBindingInputV1(t))
			return err
		}},
		{name: "ingress", put: func(ctx context.Context, store *Store) error {
			return store.PutIngressIfAbsent(ctx, records.ingress)
		}},
		{name: "thread-context", put: func(ctx context.Context, store *Store) error {
			return store.PutThreadContextIfAbsent(ctx, records.threadContext)
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "case-entity")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewStore(root, access)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if has, err := store.HasRecords(ctx); err != nil || has {
				t.Fatalf("empty case entity inventory = %v, %v", has, err)
			}
			if err := fixture.put(ctx, store); err != nil {
				t.Fatal(err)
			}
			if has, err := store.HasRecords(ctx); err != nil || !has {
				t.Fatalf("case entity inventory missed %s: has=%v err=%v", fixture.name, has, err)
			}
		})
	}
}

func TestPreparedRecoveryValidatesEveryCanonicalCaseEntityPartition(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "case-entity")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	records := caseEntityRecoveryRecordsV1(t)
	if _, err := store.EnsureBinding(ctx, caseEntityRecoveryBindingInputV1(t)); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIngressIfAbsent(ctx, records.ingress); err != nil {
		t.Fatal(err)
	}
	if err := store.PutThreadContextIfAbsent(ctx, records.threadContext); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(ctx); err == nil {
		t.Fatal("unvalidated case entity recovery plan was revalidated")
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	if plans := prepared.SecurePrivateCASRecoveryPlansV2(); len(plans) != 3 {
		t.Fatalf("case entity recovery plan count = %d", len(plans))
	}
	if topologies := prepared.PrivateCASRecoveryTopologiesV3(); len(topologies) != 1 {
		t.Fatalf("case entity recovery topology count = %d", len(topologies))
	}
}

func TestPreparedRecoveryRejectsNonCanonicalAndKeyMismatchedCaseEntityRecords(t *testing.T) {
	records := caseEntityRecoveryRecordsV1(t)
	fixtures := []struct {
		name     string
		leaf     string
		maxBytes int
		key      string
		body     []byte
	}{
		{name: "binding", leaf: bindingPartitionV1, maxBytes: domaincaseentity.MaxCaseEntityBindingRecordBytesV1, key: records.binding.BindingKey, body: mustCaseEntityBindingBytesV1(t, records.binding)},
		{name: "ingress", leaf: ingressPartitionV1, maxBytes: domaincaseentity.MaxCaseIngressRecordBytesV1, key: records.ingress.IngressID, body: mustCaseIngressBytesV1(t, records.ingress)},
		{name: "thread-context", leaf: threadContextPartitionV1, maxBytes: domaincaseentity.MaxThreadCaseContextRecordBytesV1, key: records.threadContext.StorageKey, body: mustThreadCaseContextBytesV1(t, records.threadContext)},
	}
	for _, fixture := range fixtures {
		for _, mutation := range []string{"non-canonical", "key-mismatch"} {
			t.Run(fixture.name+"/"+mutation, func(t *testing.T) {
				ctx := context.Background()
				root := filepath.Join(t.TempDir(), "case-entity")
				access, err := privatecastest.NewAccessAuthority(root)
				if err != nil {
					t.Fatal(err)
				}
				empty, err := NewStore(root, access)
				if err != nil {
					t.Fatal(err)
				}
				if err := empty.Close(); err != nil {
					t.Fatal(err)
				}
				partition, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
					filepath.Join(root, fixture.leaf), fixture.maxBytes, access,
				)
				if err != nil {
					t.Fatal(err)
				}
				key := fixture.key
				body := append([]byte(nil), fixture.body...)
				if mutation == "non-canonical" {
					body = append(body, '\n')
				} else {
					key = domainsecurity.SHA256Hex([]byte("wrong-case-entity-key:" + fixture.leaf))
				}
				if err := partition.PutIfAbsent(ctx, key, body); err != nil {
					t.Fatal(err)
				}
				if err := partition.Close(); err != nil {
					t.Fatal(err)
				}

				prepared, err := PrepareRecoveryV1(ctx, root, access)
				if err != nil {
					t.Fatal(err)
				}
				if err := prepared.ValidateSemantics(ctx); err == nil {
					t.Fatal("case entity recovery accepted corrupt private state")
				} else if strings.Contains(err.Error(), "6222021234567890") {
					t.Fatal("case entity recovery error exposed a private account value")
				}
			})
		}
	}
}

func TestPreparedRecoveryRejectsDuplicateCaseScopedStableOrdinals(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "case-entity")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	records := caseEntityRecoveryRecordsV1(t)
	secondReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		domainsecurity.SHA256Hex([]byte("case-entity-recovery-second-reference")),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(
			domaincaseentity.NewCaseEntityBindingRecordInputV1(
				caseEntityRecoverySecurityContextV1(t),
				domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
				secondReference,
				"6217009876543210",
			),
			1,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	partition, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, bindingPartitionV1),
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []domaincaseentity.CaseEntityBindingRecord{records.binding, second} {
		if err := partition.PutIfAbsent(ctx, record.BindingKey, mustCaseEntityBindingBytesV1(t, record)); err != nil {
			t.Fatal(err)
		}
	}
	if err := partition.Close(); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err == nil {
		t.Fatal("case entity recovery accepted duplicate case-scoped stable ordinals")
	} else if strings.Contains(err.Error(), "6222021234567890") || strings.Contains(err.Error(), "6217009876543210") {
		t.Fatal("case entity recovery error exposed a private account value")
	}
}

func TestLegacyBindingsUpgradeRestartAndConcurrentWritesKeepStableOrdinals(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "case-entity")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	securityContext := caseEntityRecoverySecurityContextV1(t)
	legacy := []legacyCaseEntityBindingFixtureV1{
		legacyCaseEntityBindingFixture(t, securityContext, "legacy-a", domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, "6222021234567890"),
		legacyCaseEntityBindingFixture(t, securityContext, "legacy-b", domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1, "6217009876543210"),
	}
	partition, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, bindingPartitionV1),
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range legacy {
		if err := partition.PutIfAbsent(ctx, fixture.bindingKey, fixture.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := partition.Close(); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatalf("legacy V1 inventory did not pass semantic upgrade recovery: %v", err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatalf("legacy V1 inventory did not remain immutable through recovery: %v", err)
	}

	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := NewStore(root, access)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	sortedKeys := []string{legacy[0].bindingKey, legacy[1].bindingKey}
	sort.Strings(sortedKeys)
	wantOrdinal := map[string]uint32{sortedKeys[0]: 1, sortedKeys[1]: 2}
	resolvedBeforeRestart := make(map[string]domaincaseentity.CaseEntityBindingRecord, len(legacy))
	for _, fixture := range legacy {
		record, err := store.ResolveBinding(ctx, fixture.bindingKey)
		if err != nil || record.StableOrdinal != wantOrdinal[fixture.bindingKey] ||
			record.RecordDigest != fixture.recordDigest {
			t.Fatalf("legacy V1 binding did not receive its deterministic ordinal: record=%#v err=%v", record, err)
		}
		replayed, err := sibling.EnsureBinding(ctx, domaincaseentity.NewCaseEntityBindingRecordInputV1(
			securityContext, fixture.entityType, fixture.reference, fixture.exactValue,
		))
		if err != nil || replayed.StableOrdinal != record.StableOrdinal || replayed.RecordDigest != record.RecordDigest {
			t.Fatalf("legacy V1 replay did not reuse its immutable binding: record=%#v replayed=%#v err=%v", record, replayed, err)
		}
		resolvedBeforeRestart[fixture.bindingKey] = record
	}

	const concurrentBindings = 32
	type ensureResultV1 struct {
		record domaincaseentity.CaseEntityBindingRecord
		err    error
	}
	results := make(chan ensureResultV1, concurrentBindings)
	var wait sync.WaitGroup
	for index := 0; index < concurrentBindings; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			reference, referenceErr := domaincaseentity.NewReferenceV1FromKeyedDigest(
				domainsecurity.SHA256Hex([]byte(fmt.Sprintf("legacy-upgrade-concurrent-%03d", index))),
			)
			if referenceErr != nil {
				results <- ensureResultV1{err: referenceErr}
				return
			}
			writer := store
			if index%2 == 1 {
				writer = sibling
			}
			record, ensureErr := writer.EnsureBinding(ctx, domaincaseentity.NewCaseEntityBindingRecordInputV1(
				securityContext,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				reference,
				fmt.Sprintf("622202%010d", index),
			))
			results <- ensureResultV1{record: record, err: ensureErr}
		}()
	}
	wait.Wait()
	close(results)
	seen := make(map[uint32]struct{}, concurrentBindings)
	for result := range results {
		if result.err != nil || result.record.StableOrdinal < 3 ||
			result.record.StableOrdinal > concurrentBindings+2 {
			t.Fatalf("post-upgrade concurrent binding failed: record=%#v err=%v", result.record, result.err)
		}
		if _, duplicate := seen[result.record.StableOrdinal]; duplicate {
			t.Fatalf("post-upgrade concurrent ordinal %d was reused", result.record.StableOrdinal)
		}
		seen[result.record.StableOrdinal] = struct{}{}
	}
	if len(seen) != concurrentBindings {
		t.Fatalf("post-upgrade concurrent ordinal coverage=%d", len(seen))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sibling.Close(); err != nil {
		t.Fatal(err)
	}

	prepared, err = PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatalf("mixed legacy/current inventory failed restart recovery: %v", err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatalf("mixed legacy/current inventory changed during restart recovery: %v", err)
	}
	restarted, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range legacy {
		record, err := restarted.ResolveBinding(ctx, fixture.bindingKey)
		before := resolvedBeforeRestart[fixture.bindingKey]
		if err != nil || record.StableOrdinal != before.StableOrdinal || record.RecordDigest != before.RecordDigest {
			t.Fatalf("restart changed a recovered legacy V1 ordinal: before=%#v after=%#v err=%v", before, record, err)
		}
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}

	partition, err = finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, bindingPartitionV1),
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range legacy {
		persisted, err := partition.Read(ctx, fixture.bindingKey)
		if err != nil || !bytes.Equal(persisted, fixture.body) {
			t.Fatalf("legacy V1 immutable record was rewritten during upgrade: err=%v", err)
		}
	}
	if err := partition.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyOrdinalRecoveryFailsClosedOnCurrentRecordCollision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "case-entity")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	securityContext := caseEntityRecoverySecurityContextV1(t)
	legacy := legacyCaseEntityBindingFixture(
		t,
		securityContext,
		"legacy-collision",
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	)
	currentReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		domainsecurity.SHA256Hex([]byte("current-collision")),
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(
			domaincaseentity.NewCaseEntityBindingRecordInputV1(
				securityContext,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
				currentReference,
				"6217009876543210",
			),
			1,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	partition, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, bindingPartitionV1),
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	for key, body := range map[string][]byte{
		legacy.bindingKey:  legacy.body,
		current.BindingKey: mustCaseEntityBindingBytesV1(t, current),
	} {
		if err := partition.PutIfAbsent(ctx, key, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := partition.Close(); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err == nil {
		t.Fatal("recovery accepted a legacy/current stable ordinal collision")
	} else if strings.Contains(err.Error(), legacy.exactValue) {
		t.Fatal("legacy collision error exposed a private account value")
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if record, err := store.ResolveBinding(ctx, legacy.bindingKey); err == nil || record.RecordDigest != "" {
		t.Fatalf("runtime exposed a binding from a colliding legacy inventory: record=%#v err=%v", record, err)
	}
}

type legacyCaseEntityBindingFixtureV1 struct {
	bindingKey   string
	recordDigest string
	reference    domaincaseentity.ReferenceV1
	entityType   string
	exactValue   string
	body         []byte
}

type legacyCaseEntityBindingWireV1 struct {
	SchemaVersion   int                          `json:"schemaVersion"`
	Purpose         string                       `json:"purpose"`
	TenantID        string                       `json:"tenantId"`
	UserID          string                       `json:"userId"`
	CaseID          string                       `json:"caseId"`
	CaseBindingHash string                       `json:"caseBindingHash"`
	EntityType      string                       `json:"entityType"`
	Reference       domaincaseentity.ReferenceV1 `json:"reference"`
	Canonicalizer   string                       `json:"canonicalizer"`
	CanonicalValue  string                       `json:"canonicalValue"`
	BindingKey      string                       `json:"bindingKey"`
	RecordDigest    string                       `json:"recordDigest"`
}

func legacyCaseEntityBindingFixture(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	referenceSeed string,
	entityType string,
	exactValue string,
) legacyCaseEntityBindingFixtureV1 {
	t.Helper()
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		domainsecurity.SHA256Hex([]byte(referenceSeed)),
	)
	if err != nil {
		t.Fatal(err)
	}
	bindingKey, err := domaincaseentity.CaseEntityBindingLookupKeyV1(securityContext, entityType, reference)
	if err != nil {
		t.Fatal(err)
	}
	wire := legacyCaseEntityBindingWireV1{
		SchemaVersion:   domaincaseentity.PrivateStateSchemaVersionV1,
		Purpose:         domaincaseentity.CaseEntityBindingPurposeV1,
		TenantID:        securityContext.TenantID,
		UserID:          securityContext.UserID,
		CaseID:          securityContext.CaseID,
		CaseBindingHash: securityContext.CaseBindingHash,
		EntityType:      entityType,
		Reference:       reference,
		Canonicalizer:   domaincontrolledaccount.ControlledAccountFinancialCanonicalizationV1,
		CanonicalValue:  exactValue,
		BindingKey:      bindingKey,
	}
	unsigned, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	wire.RecordDigest = domainsecurity.SHA256Hex(unsigned)
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return legacyCaseEntityBindingFixtureV1{
		bindingKey: bindingKey, recordDigest: wire.RecordDigest,
		reference: reference, entityType: entityType, exactValue: exactValue, body: body,
	}
}

type caseEntityRecoveryRecords struct {
	binding       domaincaseentity.CaseEntityBindingRecord
	ingress       domaincaseentity.CaseIngressRecord
	threadContext domaincaseentity.ThreadCaseContextRecord
}

func caseEntityRecoveryRecordsV1(t *testing.T) caseEntityRecoveryRecords {
	t.Helper()
	securityContext := caseEntityRecoverySecurityContextV1(t)
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		domainsecurity.SHA256Hex([]byte("case-entity-recovery-reference")),
	)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(domaincaseentity.NewCaseEntityBindingRecordInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			reference,
			"6222021234567890",
		), 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	raw := "分析账号 6222021234567890 的流水"
	projected := "分析账号 " + string(reference) + " 的流水"
	rawStart := strings.Index(raw, "6222021234567890")
	projectedStart := strings.Index(projected, string(reference))
	ingress, err := domaincaseentity.NewCaseIngressRecordV1(
		domaincaseentity.NewCaseIngressRecordInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			raw,
			projected,
			[]domaincaseentity.CaseIngressSpanV1{{
				EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				Reference:  reference, RawStartByte: rawStart, RawEndByte: rawStart + len("6222021234567890"),
				ProjectedStartByte: projectedStart, ProjectedEndByte: projectedStart + len(reference),
			}},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	threadContext, err := domaincaseentity.NewThreadCaseContextRecordV1(
		domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext: securityContext, Generation: 1,
			EntityReferences: []domaincaseentity.ReferenceV1{reference},
			Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
				DatasetSnapshotID: securityContext.DatasetSnapshotID,
				ContextEpoch:      securityContext.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}},
			Claims: []domaincaseentity.CaseClaimStateV1{}, Evidence: []domaincaseentity.CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{"question_next"}, DataGapReferences: []string{"gap_name"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return caseEntityRecoveryRecords{binding: binding, ingress: ingress, threadContext: threadContext}
}

func caseEntityRecoveryBindingInputV1(t *testing.T) domaincaseentity.CaseEntityBindingRecordInputV1 {
	t.Helper()
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
		domainsecurity.SHA256Hex([]byte("case-entity-recovery-reference")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return domaincaseentity.NewCaseEntityBindingRecordInputV1(
		caseEntityRecoverySecurityContextV1(t),
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		"6222021234567890",
	)
}

func caseEntityRecoverySecurityContextV1(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-entity-recovery", TurnID: "turn-case-entity-recovery",
		WorkspaceRealPath: "/cases/case-entity-recovery", CaseID: "case-entity-recovery",
		ContextEpoch: 4, IssuedAt: time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func mustCaseEntityBindingBytesV1(t *testing.T, record domaincaseentity.CaseEntityBindingRecord) []byte {
	t.Helper()
	body, err := domaincaseentity.CaseEntityBindingRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustCaseIngressBytesV1(t *testing.T, record domaincaseentity.CaseIngressRecord) []byte {
	t.Helper()
	body, err := domaincaseentity.CaseIngressRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustThreadCaseContextBytesV1(t *testing.T, record domaincaseentity.ThreadCaseContextRecord) []byte {
	t.Helper()
	body, err := domaincaseentity.ThreadCaseContextRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
