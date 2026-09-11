package steering

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type promotionStoreStub struct {
	pending        []map[string]any
	promoted       []map[string]any
	items          []map[string]any
	pendingErr     error
	promoteErr     error
	recordErr      error
	pendingCalls   int
	promotionCalls int
	recordCalls    int
	prefix         []domainsteering.PendingEntryExpectationV1
}

func (store *promotionStoreStub) PendingSteeringEntriesForContext(string, string, string) ([]map[string]any, error) {
	store.pendingCalls++
	return store.pending, store.pendingErr
}

func (store *promotionStoreStub) PromotePendingSteeringEntriesForContext(string, string, string) ([]map[string]any, []map[string]any, error) {
	store.promotionCalls++
	return store.promoted, store.items, store.promoteErr
}

func (store *promotionStoreStub) PromotePendingSteeringEntryPrefixForContext(
	_, _, _ string,
	expected []domainsteering.PendingEntryExpectationV1,
) ([]map[string]any, []map[string]any, error) {
	store.promotionCalls++
	store.prefix = append([]domainsteering.PendingEntryExpectationV1(nil), expected...)
	return store.promoted, store.items, store.promoteErr
}

func (store *promotionStoreStub) RecordEvent(map[string]any) (map[string]any, []string, error) {
	store.recordCalls++
	return nil, nil, store.recordErr
}

func TestPromoteCurrentForProviderRequiresCompleteEffectLease(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	for _, test := range []struct {
		name       string
		effectCtx  context.Context
		release    func()
		wantClosed bool
	}{
		{name: "missing effect context", release: func() {}, wantClosed: true},
		{name: "missing release", effectCtx: context.Background()},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &promotionStoreStub{}
			closed := false
			release := test.release
			if release != nil {
				release = func() { closed = true }
			}
			_, err := PromoteCurrentForProvider(context.Background(), securityContext, Dependencies{
				Store: store,
				AcquireContextEffect: func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
					return test.effectCtx, release, nil
				},
				ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error {
					t.Fatal("current authority must not run without a complete lease")
					return nil
				},
			})
			if !errors.Is(err, ErrAuthorityUnavailable) || store.pendingCalls != 0 || store.promotionCalls != 0 || closed != test.wantClosed {
				t.Fatalf("incomplete lease did not fail closed: err=%v store=%#v closed=%v", err, store, closed)
			}
		})
	}
}

func TestPromoteCurrentForProviderEffectBatchAlwaysAcquiresOrdinaryLease(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	requestedCaseData := true
	_, err := PromoteCurrentForProviderEffectBatch(context.Background(), securityContext, EffectAwareDependencies{
		Store: &promotionStoreStub{},
		AcquireContextEffect: func(
			ctx context.Context,
			_ domainsecurity.TurnSecurityContext,
			caseDataEffect bool,
		) (context.Context, func(), error) {
			requestedCaseData = caseDataEffect
			return ctx, func() {}, nil
		},
	})
	if requestedCaseData || err == nil {
		t.Fatalf("steering queue work did not remain on the ordinary lease: caseData=%t err=%v", requestedCaseData, err)
	}
}

func TestPromoteCurrentForProviderRejectsChangedAuthorityBeforeReadingPending(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	store := &promotionStoreStub{}
	changed := errors.New("turn_security_dataset_snapshot_mismatch")
	released := false
	_, err := PromoteCurrentForProvider(context.Background(), securityContext, Dependencies{
		Store: store,
		AcquireContextEffect: func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return context.Background(), func() { released = true }, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return changed },
	})
	var currentErr CurrentAuthorityError
	if !errors.As(err, &currentErr) || !errors.Is(currentErr, changed) || !released || store.pendingCalls != 0 || store.promotionCalls != 0 {
		t.Fatalf("changed authority did not block before durable reads: err=%v store=%#v released=%v", err, store, released)
	}
}

func TestPromoteCurrentForProviderRequiresTaskJobLedgerBeforeDurablePromotion(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	store := &promotionStoreStub{pending: []map[string]any{{
		"id": "steer-1", "status": "pending", "contentDigest": strings.Repeat("a", 64),
		"text": "continue", "jobId": "job-1",
	}}}
	_, err := PromoteCurrentForProvider(context.Background(), securityContext, Dependencies{
		Store: store,
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
	})
	if err == nil || store.pendingCalls != 1 || store.promotionCalls != 0 {
		t.Fatalf("missing task-job ledger reached durable promotion: err=%v store=%#v", err, store)
	}
}

func TestPromoteCurrentForProviderDeliversOnlyAfterPromotionEventPersists(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	store := &promotionStoreStub{
		pending: []map[string]any{{
			"id": "steer-1", "status": "pending", "contentDigest": strings.Repeat("a", 64),
			"text": "continue safely", "logicalEffect": "ordinary", "ordinaryWork": true,
		}},
		promoted: []map[string]any{{"id": "steer-1", "text": "continue safely"}},
		items:    []map[string]any{{"id": "steer-1", "text": "continue safely"}},
	}
	messages, err := PromoteCurrentForProvider(context.Background(), securityContext, Dependencies{
		Store: store,
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
	})
	if err != nil || len(messages) != 1 || !strings.HasSuffix(messages[0].Content, "\n\ncontinue safely") || store.recordCalls != 1 {
		t.Fatalf("valid ordinary steer was not promoted atomically enough for delivery: messages=%#v err=%v store=%#v", messages, err, store)
	}
}

func TestPromoteCurrentForProviderBatchSelectsMaximalContiguousEffectPrefix(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	pending := []map[string]any{
		steeringBatchPendingForTest("steer-1", "ordinary one", domainsecurity.LogicalEffectOrdinary, false),
		steeringBatchPendingForTest("steer-2", "ordinary two", domainsecurity.LogicalEffectOrdinary, true),
		steeringBatchPendingForTest("steer-3", "case next", domainsecurity.LogicalEffectCaseData, false),
	}
	store := &promotionStoreStub{
		pending: pending,
		promoted: []map[string]any{
			{"id": "steer-1", "text": "ordinary one"},
			{"id": "steer-2", "text": "ordinary two"},
		},
		items: []map[string]any{
			{"id": "steer-1", "text": "ordinary one"},
			{"id": "steer-2", "text": "ordinary two"},
		},
	}
	batch, err := PromoteCurrentForProviderBatch(context.Background(), securityContext, Dependencies{
		Store: store,
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
	})
	if err != nil || batch.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !batch.OrdinaryWork ||
		batch.Prompt != "ordinary one\n\nordinary two" || len(batch.Messages) != 2 || len(store.prefix) != 2 {
		t.Fatalf("maximal same-effect prefix mismatch: batch=%#v prefix=%#v err=%v", batch, store.prefix, err)
	}
	if store.prefix[0].ID != "steer-1" || store.prefix[1].ID != "steer-2" {
		t.Fatalf("prefix order changed: %#v", store.prefix)
	}
}

func TestLegacySteeringEffectFallbackNeverGuessesFunds(t *testing.T) {
	general := steeringExecutionContext(t)
	caseContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-steering", TurnID: "turn-steering", WorkspaceRealPath: "/workspace/steering",
		CaseID: "case-steering", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")),
		DatasetSnapshotID: "dsv2_" + strings.Repeat("a", 64), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")),
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"id": "steer-legacy", "status": "pending", "contentDigest": strings.Repeat("b", 64), "text": "legacy request",
	}
	for name, test := range map[string]struct {
		context      domainsecurity.TurnSecurityContext
		wantEffect   domainsecurity.LogicalEffect
		wantOrdinary bool
	}{
		"general": {context: general, wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true},
		"case":    {context: caseContext, wantEffect: domainsecurity.LogicalEffectCaseData, wantOrdinary: false},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, batch, err := providerSteeringPrefixV1([]map[string]any{legacy}, test.context)
			if err != nil || batch.LogicalEffect != test.wantEffect || batch.OrdinaryWork != test.wantOrdinary ||
				batch.LogicalEffect == domainsecurity.LogicalEffectFundsData {
				t.Fatalf("legacy fallback widened authority: batch=%#v err=%v", batch, err)
			}
		})
	}
	_, _, explicitFunds, err := providerSteeringPrefixV1([]map[string]any{
		steeringBatchPendingForTest(
			"steer-funds", "explicit funds request", domainsecurity.LogicalEffectFundsData, false,
		),
	}, general)
	if err != nil || explicitFunds.LogicalEffect != domainsecurity.LogicalEffectFundsData {
		t.Fatalf("explicit signed funds effect was not preserved: batch=%#v err=%v", explicitFunds, err)
	}
}

func TestProviderSteeringBatchValidatesAndSettlesOnlySelectedTaskJobPrefix(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(entry map[string]any, contextDigest string) error {
		material, materialErr := domainsteering.EntryAuthorityMaterialV1(entry, contextDigest)
		if materialErr != nil || !bytes.Equal(material.PublicKey, publicKey) ||
			!ed25519.Verify(publicKey, material.SigningBytes, material.Signature) {
			return domainsteering.ErrProjectionInvalid
		}
		return nil
	}
	promote := func(entry map[string]any, contextDigest string) (map[string]any, error) {
		signingBytes, signingErr := domainsteering.PromotedEntrySigningBytesV1(entry, contextDigest)
		if signingErr != nil {
			return nil, signingErr
		}
		return domainsteering.SealPromotedEntryAuthorityV1(
			entry, contextDigest, domainsecurity.SHA256Hex(publicKey), publicKey,
			ed25519.Sign(privateKey, signingBytes),
		)
	}

	firstRecord, firstMessage, firstPending := steeringTaskJobPendingForBatchTest(
		t, securityContext, publicKey, privateKey,
		"job-selected", "11111111-1111-4111-8111-111111111111", "ordinary child steer",
		domainsecurity.LogicalEffectOrdinary, true,
	)
	secondRecord, _, secondPending := steeringTaskJobPendingForBatchTest(
		t, securityContext, publicKey, privateKey,
		"job-tail", "22222222-2222-4222-8222-222222222222", "case child steer",
		domainsecurity.LogicalEffectCaseData, false,
	)
	expected, err := domainsteering.NewPendingEntryExpectationV1(firstPending)
	if err != nil {
		t.Fatal(err)
	}
	_, promoted, items, err := domainsteering.PromoteTurnEntryPrefixV1(
		securityContext.ThreadID, securityContext.TurnID,
		map[string]any{"steering": []any{firstPending, secondPending}, "items": []any{}},
		[]domainsteering.PendingEntryExpectationV1{expected}, securityContext.ContextDigest,
		"2026-07-18T01:02:05Z", verify, promote,
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &promotionStoreStub{
		pending: []map[string]any{firstPending, secondPending}, promoted: promoted, items: items,
	}
	jobs := &batchTaskJobStoreStub{records: map[string]domainjob.Record{
		firstRecord.ID:  firstRecord,
		secondRecord.ID: secondRecord,
	}}
	batch, err := PromoteCurrentForProviderBatch(context.Background(), securityContext, Dependencies{
		Store: store, Jobs: jobs,
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		VerifyAuthority: func(_ context.Context, entry map[string]any, contextDigest string) error {
			return verify(entry, contextDigest)
		},
	})
	if err != nil || batch.LogicalEffect != domainsecurity.LogicalEffectOrdinary || len(batch.Messages) != 1 ||
		len(jobs.loadIDs) != 1 || jobs.loadIDs[0] != firstRecord.ID ||
		len(jobs.settleIDs) != 1 || jobs.settleIDs[0] != firstRecord.ID ||
		jobs.settledMessages[0] != firstMessage.ID || len(store.prefix) != 1 {
		t.Fatalf("task-job tail crossed the selected effect prefix: batch=%#v loads=%#v settles=%#v messages=%#v prefix=%#v err=%v", batch, jobs.loadIDs, jobs.settleIDs, jobs.settledMessages, store.prefix, err)
	}
}

func TestProviderSteeringBatchUsesFrozenGeneralContextForLegacyTaskJobEffect(t *testing.T) {
	securityContext := steeringExecutionContext(t)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(entry map[string]any, contextDigest string) error {
		material, materialErr := domainsteering.EntryAuthorityMaterialV1(entry, contextDigest)
		if materialErr != nil || !bytes.Equal(material.PublicKey, publicKey) ||
			!ed25519.Verify(publicKey, material.SigningBytes, material.Signature) {
			return domainsteering.ErrProjectionInvalid
		}
		return nil
	}
	record, message, pending := steeringTaskJobPendingForBatchTest(
		t, securityContext, publicKey, privateKey,
		"job-legacy", "33333333-3333-4333-8333-333333333333", "legacy child steer", "", false,
	)
	expected, err := domainsteering.NewPendingEntryExpectationV1(pending)
	if err != nil {
		t.Fatal(err)
	}
	_, promoted, items, err := domainsteering.PromoteTurnEntryPrefixV1(
		securityContext.ThreadID, securityContext.TurnID,
		map[string]any{"steering": []any{pending}, "items": []any{}},
		[]domainsteering.PendingEntryExpectationV1{expected}, securityContext.ContextDigest,
		"2026-07-18T01:02:05Z", verify,
		func(entry map[string]any, contextDigest string) (map[string]any, error) {
			signingBytes, signingErr := domainsteering.PromotedEntrySigningBytesV1(entry, contextDigest)
			if signingErr != nil {
				return nil, signingErr
			}
			return domainsteering.SealPromotedEntryAuthorityV1(
				entry, contextDigest, domainsecurity.SHA256Hex(publicKey), publicKey,
				ed25519.Sign(privateKey, signingBytes),
			)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &promotionStoreStub{pending: []map[string]any{pending}, promoted: promoted, items: items}
	jobs := &batchTaskJobStoreStub{records: map[string]domainjob.Record{record.ID: record}}
	batch, err := PromoteCurrentForProviderBatch(context.Background(), securityContext, Dependencies{
		Store: store, Jobs: jobs,
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		VerifyAuthority: func(_ context.Context, entry map[string]any, contextDigest string) error {
			return verify(entry, contextDigest)
		},
	})
	if err != nil || batch.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !batch.OrdinaryWork ||
		len(jobs.settleIDs) != 1 || jobs.settleIDs[0] != record.ID || jobs.settledMessages[0] != message.ID {
		t.Fatalf("legacy task-job effect was not conservatively resumed: batch=%#v settles=%#v messages=%#v err=%v", batch, jobs.settleIDs, jobs.settledMessages, err)
	}
}

func steeringBatchPendingForTest(
	id, text string,
	effect domainsecurity.LogicalEffect,
	ordinaryWork bool,
) map[string]any {
	return map[string]any{
		"id": id, "status": "pending", "contentDigest": domainsecurity.SHA256Hex([]byte(id + ":" + text)),
		"text": text, "logicalEffect": string(effect), "ordinaryWork": ordinaryWork,
	}
}

type batchTaskJobStoreStub struct {
	records         map[string]domainjob.Record
	loadIDs         []string
	settleIDs       []string
	settledMessages []string
}

func (store *batchTaskJobStoreStub) LoadChildRun(id string) (domainjob.Record, error) {
	store.loadIDs = append(store.loadIDs, id)
	record, found := store.records[id]
	if !found {
		return domainjob.Record{}, errors.New("task job not found")
	}
	return record, nil
}

func (store *batchTaskJobStoreStub) UpdateChildRun(id string, _ domainjob.UpdateRequest) (domainjob.Record, error) {
	return store.records[id], nil
}

func (store *batchTaskJobStoreStub) QueueSteerMessage(
	id string,
	_ domainjob.SteerQueueAuthorityV1,
	message domainjob.SteerMessage,
) (domainjob.Record, domainjob.SteerMessage, error) {
	return store.records[id], message, nil
}

func (store *batchTaskJobStoreStub) SettleSteerPromotionExact(
	id string,
	expected domainjob.SteerMessage,
	settlement domainjob.SteerPromotionSettlementV1,
) (domainjob.Record, domainjob.SteerMessage, error) {
	store.settleIDs = append(store.settleIDs, id)
	store.settledMessages = append(store.settledMessages, expected.ID)
	expected.Status = "admitted"
	expected.AdmittedAt = settlement.PromotedAt
	expected.PromotionCommitID = settlement.PromotionCommitID
	expected.PromotionEntryID = settlement.PromotionEntryID
	return store.records[id], expected, nil
}

func (store *batchTaskJobStoreStub) RejectSteerMessage(
	id string,
	message domainjob.SteerMessage,
	_ string,
) (domainjob.Record, domainjob.SteerMessage, error) {
	return store.records[id], message, nil
}

func steeringTaskJobPendingForBatchTest(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
	jobID, messageID, text string,
	effect domainsecurity.LogicalEffect,
	ordinaryWork bool,
) (domainjob.Record, domainjob.SteerMessage, map[string]any) {
	t.Helper()
	message := domainjob.SteerMessage{
		ID: messageID, ParentThreadID: "thread-parent", ChildRunID: jobID, JobID: jobID,
		Text: text, ProjectionVersion: domainjob.SteerMessageProjectionVersionV1,
		ContextDigest:        securityContext.ContextDigest,
		AuthorityDigest:      domainsecurity.SHA256Hex([]byte("authority:" + jobID)),
		QueueAuthorityDigest: domainsecurity.SHA256Hex([]byte("queue:" + jobID)),
		Status:               "queued", CreatedAt: "2026-07-18T01:02:03Z",
		LogicalEffect: effect, OrdinaryWork: ordinaryWork,
	}
	message.ContentDigest = domainjob.SteerMessageContentDigestV1(message)
	record := domainjob.Record{
		ID: jobID, ParentThreadID: message.ParentThreadID,
		ChildThreadID: securityContext.ThreadID, ChildTurnID: securityContext.TurnID,
		Steers: []domainjob.SteerMessage{message},
	}
	raw := subagentapp.TaskJobSteerTurnEntry(record, message)
	if effect != "" {
		raw["logicalEffect"] = string(effect)
		raw["ordinaryWork"] = ordinaryWork
	}
	pending, err := domainsteering.BindPendingEntryV1(raw, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signingBytes, err := domainsteering.PendingEntrySigningBytesV1(pending, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = domainsteering.SealPendingEntryAuthorityV1(
		pending, securityContext.ContextDigest, domainsecurity.SHA256Hex(publicKey), publicKey,
		ed25519.Sign(privateKey, signingBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	return record, message, pending
}

func steeringExecutionContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-steering", TurnID: "turn-steering", WorkspaceRealPath: "/workspace/steering",
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
