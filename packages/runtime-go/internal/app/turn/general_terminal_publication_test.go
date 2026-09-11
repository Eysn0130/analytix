package turn

import (
	"testing"

	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func TestGeneralTerminalPublicationCommitRequiresExactCanonicalWinner(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	if err := ValidateGeneralTerminalPublicationCommitForThreadV1(thread, commit.TurnID, commit); err != nil {
		t.Fatalf("exact canonical outbox was rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "turn is not terminal",
			mutate: func(thread map[string]any) {
				publicationFixtureTurn(thread)["status"] = "running"
			},
		},
		{
			name: "finished timestamp",
			mutate: func(thread map[string]any) {
				publicationFixtureTurn(thread)["finishedAt"] = "2026-07-15T00:00:01Z"
			},
		},
		{
			name: "accepted final",
			mutate: func(thread map[string]any) {
				publicationFixtureTurn(thread)["acceptedFinal"] = map[string]any{"recordDigest": "foreign"}
			},
		},
		{
			name: "stored commit",
			mutate: func(thread map[string]any) {
				stored := publicationFixtureTurn(thread)["generalTerminalPublication"].(map[string]any)
				stored["commitDigest"] = domainsecurity.SHA256Hex([]byte("other"))
			},
		},
		{
			name: "assistant text",
			mutate: func(thread map[string]any) {
				publicationFixtureAssistant(thread)["text"] = "other"
			},
		},
		{
			name: "assistant timestamp",
			mutate: func(thread map[string]any) {
				publicationFixtureAssistant(thread)["finishedAt"] = "2026-07-15T00:00:01Z"
			},
		},
		{
			name: "duplicate assistant",
			mutate: func(thread map[string]any) {
				turn := publicationFixtureTurn(thread)
				turn["items"] = append(turn["items"].([]any), contracts.CloneMap(publicationFixtureAssistant(thread)))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := contracts.CloneMap(thread)
			test.mutate(candidate)
			if err := ValidateGeneralTerminalPublicationCommitForThreadV1(candidate, commit.TurnID, commit); err == nil {
				t.Fatal("tampered or stale canonical winner was accepted")
			}
		})
	}
}

func TestGeneralTerminalPublicationRejectsOpenTelemetryBeforeCommitAndOnReplay(t *testing.T) {
	thread, canonicalCommit, item, binding := pendingGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	context, err := domainsecurity.ParseTurnSecurityContext(publicationFixtureTurn(thread)["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	itemEvent := AssistantItemCompletedEvent(item)
	itemEvent["timestamp"] = canonicalCommit.CommittedAt
	usageEvent := contracts.CloneMap(canonicalCommit.UsageEvent)
	usageEvent["cacheDiagnostics"].(map[string]any)["outputPreview"] = "PRIVATE_PROVIDER_SENTINEL"
	terminalEvent := contracts.CloneMap(canonicalCommit.TerminalEvent)

	if _, err := BuildGeneralTerminalPublicationCommitForEventsV1(
		context, binding, itemEvent, usageEvent, terminalEvent, canonicalCommit.CommittedAt, true,
	); err == nil {
		t.Fatal("general terminal builder accepted open cache diagnostics")
	}

	legacyCommit, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(
		context,
		binding,
		canonicalCommit.CommittedAt,
		[]domainturnterminal.GeneralTerminalPublicationDraftV1{
			{Slot: "terminal-item", Draft: itemEvent},
			{Slot: "usage", Draft: usageEvent},
			{Slot: "terminal", Draft: terminalEvent},
		},
	)
	if err != nil {
		t.Fatalf("domain fixture should preserve the legacy open-map drift: %v", err)
	}
	turn := publicationFixtureTurn(thread)
	turn["status"] = "completed"
	turn["finishedAt"] = legacyCommit.CommittedAt
	turn["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	turn["generalTerminalPublication"] = domainturnterminal.GeneralTerminalPublicationCommitV1Map(legacyCommit)
	if err := ValidateGeneralTerminalPublicationCommitForThreadV1(thread, legacyCommit.TurnID, legacyCommit); err == nil {
		t.Fatal("general terminal replay accepted an open telemetry commit")
	}
}

func TestPrepareGeneralTerminalEventBundleIsContiguousAndAllOrNothing(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 41)
	if err != nil || len(events) != 3 {
		t.Fatalf("prepare exact event bundle: events=%#v err=%v", events, err)
	}
	for index, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq != 41+index || stringField(event, "generalTerminalSlot") != commit.Events[index].Slot ||
			domainturnterminal.GeneralTerminalPublicationPayloadDigestV1(event) != commit.Events[index].PayloadDigest {
			t.Fatalf("bundle event %d is not exact and contiguous: %#v", index, event)
		}
	}
	tampered := contracts.CloneMap(thread)
	publicationFixtureAssistant(tampered)["text"] = "different"
	if events, err := PrepareGeneralTerminalEventBundleV1(tampered, commit.TurnID, commit, 41); err == nil || len(events) != 0 {
		t.Fatalf("tampered canonical item produced a partial bundle: %#v err=%v", events, err)
	}
}

func TestGeneralTerminalPublicationUpdateRequiresBindingAndOutboxTogether(t *testing.T) {
	thread, commit, item, binding := pendingGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	fields := map[string]any{
		"generalTerminalCASBinding":  domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	}
	if err := ValidateGeneralTerminalPublicationUpdateV1(thread, commit.TurnID, "completed", []map[string]any{item}, fields); err != nil {
		t.Fatalf("exact prospective update was rejected: %v", err)
	}
	for _, missing := range []string{"generalTerminalCASBinding", "generalTerminalPublication"} {
		candidate := contracts.CloneMap(fields)
		delete(candidate, missing)
		if err := ValidateGeneralTerminalPublicationUpdateV1(thread, commit.TurnID, "completed", []map[string]any{item}, candidate); err == nil {
			t.Fatalf("prospective update without %s was accepted", missing)
		}
	}
	wrongItem := contracts.CloneMap(item)
	wrongItem["id"] = "item-other"
	if err := ValidateGeneralTerminalPublicationUpdateV1(thread, commit.TurnID, "completed", []map[string]any{wrongItem}, fields); err == nil {
		t.Fatal("outbox accepted a different assistant item")
	}
	tamperedFields := contracts.CloneMap(fields)
	tamperedCommit := tamperedFields["generalTerminalPublication"].(map[string]any)
	tamperedCommit["unexpected"] = true
	if err := ValidateGeneralTerminalPublicationUpdateV1(thread, commit.TurnID, "completed", []map[string]any{item}, tamperedFields); err == nil {
		t.Fatal("outbox with an unknown field was accepted")
	}
	stale := contracts.CloneMap(thread)
	current, _ := domainsecurity.ParseTurnSecurityContext(stale["securityState"])
	stale["securityState"] = turnSecurityContextRecord(generalTerminalTestContext(t, current.ThreadID, current.TurnID, current.ContextEpoch+1))
	if err := ValidateGeneralTerminalPublicationUpdateV1(stale, commit.TurnID, "completed", []map[string]any{item}, fields); err == nil {
		t.Fatal("prospective outbox accepted a stale current context")
	}
}

func TestNextGeneralTerminalPublicationArchiveIsStrictAndIdempotent(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	delete(thread, GeneralTerminalPublicationArchiveFieldV1)
	first, err := NextGeneralTerminalPublicationArchiveV1(
		thread,
		domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = first
	second, err := NextGeneralTerminalPublicationArchiveV1(
		thread,
		domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	)
	if err != nil || !canonicalJSONEqual(first, second) {
		t.Fatalf("archive append was not idempotent: first=%#v second=%#v err=%v", first, second, err)
	}
	bad := contracts.CloneMap(thread)
	bad[GeneralTerminalPublicationArchiveFieldV1].(map[string]any)["unexpected"] = true
	if _, err := NextGeneralTerminalPublicationArchiveV1(
		bad,
		domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	); err == nil {
		t.Fatal("archive with an unknown field was accepted")
	}
	foreign := contracts.CloneMap(thread)
	foreign["id"] = "thread-other"
	if _, err := NextGeneralTerminalPublicationArchiveV1(
		foreign,
		domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	); err == nil {
		t.Fatal("foreign-thread archive commit was accepted")
	}
	_, foreignCommit, _, _ := pendingGeneralTerminalPublicationFixtureForIdentity(
		t,
		GeneralProviderFinalQuarantinedText,
		"thread-foreign-archive",
		"turn-foreign-archive",
	)
	foreignArchive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{foreignCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(foreignArchive)
	if _, err := NextGeneralTerminalPublicationArchiveV1(
		thread,
		domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	); err == nil {
		t.Fatal("existing foreign-thread archive commit passed the terminal CAS preflight")
	}
}

func TestGeneralTerminalInventoryValidatesCompactedArchiveWithoutReconstructingProse(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{commit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 7)
	if err != nil {
		t.Fatal(err)
	}
	compacted := contracts.CloneMap(thread)
	compacted["turns"] = []any{}
	entries, err := PreflightGeneralTerminalPublicationInventoryV1(compacted, events)
	if err != nil || len(entries) != 1 || !entries[0].ArchivedOnly || entries[0].State != GeneralTerminalPublicationCompleteV1 {
		t.Fatalf("exact compacted bundle was rejected: entries=%#v err=%v", entries, err)
	}
	if entries, err := PreflightGeneralTerminalPublicationInventoryV1(compacted, nil); err == nil || entries != nil {
		t.Fatalf("missing compacted bundle was treated as reconstructable: entries=%#v err=%v", entries, err)
	}
	tampered := make([]map[string]any, len(events))
	for index := range events {
		tampered[index] = contracts.CloneMap(events[index])
	}
	tampered[0]["itemId"] = "item-other"
	if entries, err := PreflightGeneralTerminalPublicationInventoryV1(compacted, tampered); err == nil || entries != nil {
		t.Fatalf("tampered compacted payload passed its archived manifest: entries=%#v err=%v", entries, err)
	}
}

func TestGeneralTerminalInventoryRequiresAtomicThreadArchiveEntry(t *testing.T) {
	thread, _ := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	if entries, err := PreflightGeneralTerminalPublicationInventoryV1(thread, nil); err == nil || entries != nil {
		t.Fatalf("turn-only outbox passed without its atomic archive: entries=%#v err=%v", entries, err)
	}
}

func TestGeneralTerminalInventoryRejectsGlobalSequenceCollisionAndForeignUnmarkedEvent(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{commit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 11)
	if err != nil {
		t.Fatal(err)
	}
	collision := append([]map[string]any{}, events...)
	collision = append(collision, map[string]any{
		"kind": "progress", "threadId": commit.ThreadID, "turnId": commit.TurnID, "seq": float64(11),
	})
	if entries, err := PreflightGeneralTerminalPublicationInventoryV1(thread, collision); err == nil || entries != nil {
		t.Fatalf("unmarked global seq collision passed inventory: entries=%#v err=%v", entries, err)
	}
	foreign := append([]map[string]any{}, events...)
	foreign = append(foreign, map[string]any{
		"kind": "progress", "threadId": "thread-foreign", "turnId": commit.TurnID, "seq": float64(20),
	})
	if entries, err := PreflightGeneralTerminalPublicationInventoryV1(thread, foreign); err == nil || entries != nil {
		t.Fatalf("foreign unmarked event passed inventory: entries=%#v err=%v", entries, err)
	}
}

func TestCompactionTailRestoresOnlyValidatedGeneralTerminalAuthority(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{commit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
	authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		t.Fatal(err)
	}
	turn := publicationFixtureTurn(thread)
	turn["executionGrant"] = map[string]any{"grantId": "PRIVATE_GRANT_SENTINEL"}
	turn["generalTerminalForged"] = map[string]any{"sentinel": "FORGED_GENERAL_SENTINEL"}
	turn["items"] = append(turn["items"].([]any), map[string]any{
		"id": "user-tail", "turnId": commit.TurnID, "threadId": commit.ThreadID,
		"kind": "user_message", "role": "user", "text": "retained user text", "status": "completed",
		"executionGrant": map[string]any{"grantId": "ITEM_PRIVATE_GRANT_SENTINEL"},
	})
	projectedValue, ok := sanitizeCompactionTailTurn(turn, authorities)
	projected, _ := projectedValue.(map[string]any)
	if !ok || projected == nil || projected["generalTerminalPublication"] == nil || projected["generalTerminalCASBinding"] == nil {
		t.Fatalf("validated terminal authority was not retained: %#v", projected)
	}
	if projected["executionGrant"] != nil || projected["generalTerminalForged"] != nil {
		t.Fatalf("unrelated private or forged authority survived compaction: %#v", projected)
	}
	assistant := projected["items"].([]any)[0].(map[string]any)
	if assistant["generalTerminalCASBinding"] == nil {
		t.Fatalf("assistant binding/private projection mismatch: %#v", assistant)
	}
	for _, rawItem := range projected["items"].([]any) {
		if item, _ := rawItem.(map[string]any); item["executionGrant"] != nil {
			t.Fatalf("non-target item execution grant survived compaction: %#v", item)
		}
	}
	minimal := map[string]any{"id": commit.ThreadID, "turns": []any{projected}}
	if err := ValidateGeneralTerminalPublicationCommitForThreadV1(minimal, commit.TurnID, commit); err != nil {
		t.Fatalf("retained terminal authority is not reconstructable: %v", err)
	}
	tampered := contracts.CloneMap(turn)
	tampered["generalTerminalPublication"].(map[string]any)["unexpected"] = true
	if value, ok := sanitizeCompactionTailTurn(tampered, authorities); ok || value != nil {
		t.Fatalf("tampered terminal authority survived compaction: %#v", value)
	}
}

func committedGeneralTerminalPublicationFixture(
	t *testing.T,
	text string,
) (map[string]any, domainturnterminal.GeneralTerminalPublicationCommitV1) {
	t.Helper()
	thread, commit, item, binding := pendingGeneralTerminalPublicationFixture(t, text)
	turn := publicationFixtureTurn(thread)
	turn["status"] = "completed"
	turn["finishedAt"] = commit.CommittedAt
	turn["items"] = []any{item}
	turn["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	turn["generalTerminalPublication"] = domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)
	return thread, commit
}

func pendingGeneralTerminalPublicationFixture(
	t *testing.T,
	text string,
) (map[string]any, domainturnterminal.GeneralTerminalPublicationCommitV1, map[string]any, domainturnterminal.GeneralTerminalCASBindingV1) {
	t.Helper()
	return pendingGeneralTerminalPublicationFixtureForIdentity(t, text, "thread-general-outbox", "turn-general-outbox")
}

func pendingGeneralTerminalPublicationFixtureForIdentity(
	t *testing.T,
	text string,
	threadID string,
	turnID string,
) (map[string]any, domainturnterminal.GeneralTerminalPublicationCommitV1, map[string]any, domainturnterminal.GeneralTerminalCASBindingV1) {
	t.Helper()
	context := generalTerminalTestContext(t, threadID, turnID, 2)
	committedAt := "2026-07-15T00:00:00Z"
	record := BuildCompletionRecord(CompletionRecordInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, Model: "gpt-5", AssistantText: text,
		CreatedAt: committedAt, FinishedAt: committedAt,
		Usage: appusage.NewTerminalTelemetryV1(
			domainmodel.Usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3}, nil,
		).PublicUsageMap(),
		CacheDiagnostics: map[string]any{},
		UsageSource:      "subagent", ChildRunID: "child-1",
	})
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingV1(context, text)
	if err != nil {
		t.Fatal(err)
	}
	bindingMap := domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	record.AssistantItem["generalTerminalCASBinding"] = bindingMap
	record.ItemCompletedEvent = AssistantItemCompletedEvent(record.AssistantItem)
	record.ItemCompletedEvent["timestamp"] = committedAt
	record.UsageEvent["timestamp"] = committedAt
	record.TurnCompletedEvent["timestamp"] = committedAt
	record.TurnCompletedEvent["terminalReason"] = "success"
	record.TurnCompletedEvent["generalTerminalCASBindingDigest"] = binding.BindingDigest
	commit, err := BuildGeneralTerminalPublicationCommitV1(context, binding, record, committedAt, text != "")
	if err != nil {
		t.Fatal(err)
	}
	items := []any{}
	if text != "" {
		items = append(items, record.AssistantItem)
	}
	turn := map[string]any{
		"id": context.TurnID, "threadId": context.ThreadID, "status": "running",
		"securityContext": turnSecurityContextRecord(context), "items": items,
	}
	thread := map[string]any{
		"id": context.ThreadID, "status": "running", "securityState": turnSecurityContextRecord(context), "turns": []any{turn},
	}
	return thread, commit, record.AssistantItem, binding
}

func publicationFixtureTurn(thread map[string]any) map[string]any {
	return thread["turns"].([]any)[0].(map[string]any)
}

func publicationFixtureAssistant(thread map[string]any) map[string]any {
	return publicationFixtureTurn(thread)["items"].([]any)[0].(map[string]any)
}
