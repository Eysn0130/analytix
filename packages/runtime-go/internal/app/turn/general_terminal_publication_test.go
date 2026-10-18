package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
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

func generalTerminalTelemetryCASInputV1(t *testing.T) PrepareTerminalCASInput {
	t.Helper()
	thread, previous, _, binding := pendingGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	frozen, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	telemetry := appusage.NewTerminalTelemetryV1(domainmodel.Usage{
		PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7,
	}, map[string]any{
		"providerAttemptTelemetrySchema":   appusage.ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":    false,
		"providerAttemptTelemetryError":    "invalid_or_incomplete",
		"providerAttemptInventoryComplete": false,
	})
	record := BuildCompletionRecord(CompletionRecordInput{
		ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Model: "test-model",
		AssistantText: GeneralProviderFinalQuarantinedText, CreatedAt: previous.CommittedAt, FinishedAt: previous.CommittedAt,
		Usage: telemetry.PublicUsageMap(), CacheDiagnostics: telemetry.PublicCacheDiagnosticsMap(),
	})
	record.AssistantItem["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	record.ItemCompletedEvent = AssistantItemCompletedEvent(record.AssistantItem)
	record.ItemCompletedEvent["timestamp"] = previous.CommittedAt
	record.UsageEvent["timestamp"] = previous.CommittedAt
	record.TurnCompletedEvent["timestamp"] = previous.CommittedAt
	record.TurnCompletedEvent["terminalReason"] = "success"
	record.TurnCompletedEvent["generalTerminalCASBindingDigest"] = binding.BindingDigest
	commit, err := BuildGeneralTerminalPublicationCommitV1(frozen, binding, record, previous.CommittedAt, true)
	if err != nil {
		t.Fatal(err)
	}
	return PrepareTerminalCASInput{
		Thread: thread, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Status: "completed",
		AppendItems: []map[string]any{record.AssistantItem},
		Fields: map[string]any{
			"generalTerminalCASBinding":  domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
			"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
		},
		Now:                       time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		LoadGeneralTerminalEvents: func() ([]map[string]any, error) { return nil, nil },
	}
}

func TestGeneralTerminalCASTelemetryScanViewPreservesActiveAndReplayPayloadV1(t *testing.T) {
	input := generalTerminalTelemetryCASInputV1(t)
	if err := domainevent.ValidatePublicRecord(input.Fields); !errors.Is(err, domainevent.ErrRawFailurePersistence) {
		t.Fatalf("generic nested outbox scanner no longer reproduces the boundary: %v", err)
	}
	before, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareTerminalCASMutation(input)
	if err != nil || !prepared.Applied || prepared.HasAcceptedFinal {
		t.Fatalf("validated active outbox was rejected: applied=%t err=%v", prepared.Applied, err)
	}
	after, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("terminal CAS preparation changed its original payload")
	}
	winner := publicationFixtureTurn(prepared.Thread)
	winnerPublication, winnerOK := winner["generalTerminalPublication"].(map[string]any)
	inputPublication, inputOK := input.Fields["generalTerminalPublication"].(map[string]any)
	if !winnerOK || !inputOK || !canonicalJSONEqual(winnerPublication, inputPublication) {
		t.Fatal("prepared CAS changed the private outbox")
	}
	input.Thread = prepared.Thread
	replayBefore, _ := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
	replay, err := PrepareTerminalCASMutation(input)
	if err != nil || replay.Applied || replay.CurrentStatus != "completed" {
		t.Fatalf("exact terminal replay did not remain a no-op: applied=%t err=%v", replay.Applied, err)
	}
	replayAfter, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
	if err != nil || !bytes.Equal(replayBefore, replayAfter) {
		t.Fatal("terminal replay changed its original payload")
	}
}

func TestGeneralTerminalCASTelemetryScanViewRejectsDetachedOrUnsafeFieldsV1(t *testing.T) {
	for name, mutate := range map[string]func(*PrepareTerminalCASInput){
		"missing binding": func(input *PrepareTerminalCASInput) { delete(input.Fields, "generalTerminalCASBinding") },
		"unknown outbox field": func(input *PrepareTerminalCASInput) {
			input.Fields["generalTerminalPublication"].(map[string]any)["unexpected"] = true
		},
		"pseudo envelope": func(input *PrepareTerminalCASInput) {
			input.Fields["generalTerminalPublication"] = map[string]any{"usageEvent": map[string]any{
				"kind": "usage", "cacheDiagnostics": map[string]any{"providerAttemptTelemetryError": "invalid_or_incomplete"},
			}}
		},
		"wrong thread": func(input *PrepareTerminalCASInput) { input.ThreadID = "other-thread" },
		"wrong status": func(input *PrepareTerminalCASInput) { input.Status = "failed" },
		"changed frozen context": func(input *PrepareTerminalCASInput) {
			changed := turnSecurityContextRecord(generalTerminalTestContext(t, input.ThreadID, input.TurnID, 3))
			input.Thread["securityState"] = changed
			publicationFixtureTurn(input.Thread)["securityContext"] = changed
		},
		"raw sibling":        func(input *PrepareTerminalCASInput) { input.Fields["lastError"] = "PRIVATE_PROVIDER_FAILURE" },
		"credential sibling": func(input *PrepareTerminalCASInput) { input.Fields["extra"] = "api_key=sk-test-private-value" },
		"nested outbox": func(input *PrepareTerminalCASInput) {
			input.Fields["wrapper"] = contracts.CloneValue(input.Fields["generalTerminalPublication"])
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, replay := range []bool{false, true} {
				input := generalTerminalTelemetryCASInputV1(t)
				if replay {
					prepared, err := PrepareTerminalCASMutation(input)
					if err != nil || !prepared.Applied {
						t.Fatalf("exact active fixture could not prepare replay: %v", err)
					}
					input.Thread = prepared.Thread
				}
				mutate(&input)
				before, _ := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
				prepared, err := PrepareTerminalCASMutation(input)
				if err == nil || prepared.Applied {
					t.Fatalf("unsafe outbox received scan-view qualification: replay=%t applied=%t err=%v", replay, prepared.Applied, err)
				}
				after, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("outbox rejection changed its original payload")
				}
			}
		})
	}
}

func TestGeneralTerminalPublicationValidationViewPreservesClosedTelemetryV1(t *testing.T) {
	input := generalTerminalTelemetryCASInputV1(t)
	original := input.Fields["generalTerminalPublication"].(map[string]any)
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainevent.ValidatePublicRecord(original); !errors.Is(err, domainevent.ErrRawFailurePersistence) {
		t.Fatalf("generic nested commit scanner no longer reproduces the boundary: %v", err)
	}
	view, err := GeneralTerminalPublicationCommitValidationViewV1(original)
	if err != nil {
		t.Fatalf("canonical fixed-error telemetry commit was rejected: %v", err)
	}
	if _, present := view["usageEvent"]; present {
		t.Fatal("separately validated root usage remained nested in the scan view")
	}
	want := contracts.CloneMap(original)
	delete(want, "usageEvent")
	if !canonicalJSONEqual(view, want) {
		t.Fatal("scan view changed fields outside its separately validated usage event")
	}
	if err := domainevent.ValidatePublicRecord(view); err != nil {
		t.Fatalf("remaining complete commit fields failed ordinary scanning: %v", err)
	}
	view["terminalEvent"].(map[string]any)["status"] = "failed"
	after, err := json.Marshal(original)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("validation view changed or shared mutable content with the original commit")
	}
	if _, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(original); err != nil {
		t.Fatalf("original persisted digest no longer validates: %v", err)
	}
}

func TestGeneralTerminalPublicationValidationViewRejectsUnsafeRootUsageV1(t *testing.T) {
	for name, test := range map[string]struct {
		mutate func(map[string]any)
		want   error
	}{
		"unknown diagnostic with fixed error": {
			mutate: func(usage map[string]any) {
				usage["cacheDiagnostics"].(map[string]any)["outputPreview"] = "PRIVATE_PROVIDER_SENTINEL"
			},
			want: domainevent.ErrRawFailurePersistence,
		},
		"noncanonical telemetry pair": {
			mutate: func(usage map[string]any) { usage["usage"].(map[string]any)["totalTokens"] = 1 },
			want:   domainevent.ErrRawFailurePersistence,
		},
		"raw diagnostic error": {
			mutate: func(usage map[string]any) {
				usage["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			},
			want: domainevent.ErrRawFailurePersistence,
		},
		"credential in usage": {
			mutate: func(usage map[string]any) { usage["model"] = "api_key=sk-test-private-value" },
			want:   domainevent.ErrCredentialProjection,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// This uses the domain constructor, bypassing the app builder's root
			// telemetry check. Envelope and digest validity alone are insufficient.
			original := generalTerminalValidationViewDomainCommitV1(t, test.mutate)
			before, _ := json.Marshal(original)
			view, err := GeneralTerminalPublicationCommitValidationViewV1(original)
			if view != nil || !errors.Is(err, test.want) {
				t.Fatalf("unsafe root usage received a scan view: view=%t err=%v", view != nil, err)
			}
			after, err := json.Marshal(original)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("root usage rejection changed the original commit")
			}
		})
	}
}

func TestGeneralTerminalPublicationValidationViewRejectsInvalidEnvelopeAndBudgetsV1(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"unknown field":       func(commit map[string]any) { commit["unexpected"] = true },
		"incomplete envelope": func(commit map[string]any) { delete(commit, "terminalEvent") },
		"digest mismatch":     func(commit map[string]any) { commit["commitDigest"] = strings.Repeat("0", 64) },
		"non JSON type":       func(commit map[string]any) { commit["extra"] = acceptedFinalMarshalSentinelV1{} },
	} {
		t.Run(name, func(t *testing.T) {
			original := generalTerminalTelemetryCASInputV1(t).Fields["generalTerminalPublication"].(map[string]any)
			mutate(original)
			if view, err := GeneralTerminalPublicationCommitValidationViewV1(original); err == nil || view != nil {
				t.Fatal("invalid original envelope received a scan view")
			}
		})
	}
	for _, aggregate := range []bool{false, true} {
		original := generalTerminalTelemetryCASInputV1(t).Fields["generalTerminalPublication"].(map[string]any)
		// The marshaler must not run even when another field exceeds the budget.
		original["untrusted"] = acceptedFinalMarshalSentinelV1{}
		if aggregate {
			original["oversized"] = []any{strings.Repeat("x", 1<<20), strings.Repeat("x", 1<<20),
				strings.Repeat("x", 1<<20), strings.Repeat("x", 1<<20), strings.Repeat("x", 1<<20),
				strings.Repeat("x", 1<<20), strings.Repeat("x", 1<<20), strings.Repeat("x", 1<<20)}
		} else {
			original["oversized"] = strings.Repeat("x", (1<<20)+1)
		}
		if view, err := GeneralTerminalPublicationCommitValidationViewV1(original); view != nil || !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
			t.Fatalf("original complete budget did not precede parsing: aggregate=%t err=%v", aggregate, err)
		}
	}
}

func generalTerminalValidationViewDomainCommitV1(t *testing.T, mutate func(map[string]any)) map[string]any {
	t.Helper()
	input := generalTerminalTelemetryCASInputV1(t)
	previous, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(input.Fields["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	context, err := domainsecurity.ParseTurnSecurityContext(input.Thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(input.Fields["generalTerminalCASBinding"])
	if err != nil {
		t.Fatal(err)
	}
	usage := contracts.CloneMap(previous.UsageEvent)
	mutate(usage)
	itemEvent := AssistantItemCompletedEvent(input.AppendItems[0])
	itemEvent["timestamp"] = previous.CommittedAt
	commit, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(context, binding, previous.CommittedAt,
		[]domainturnterminal.GeneralTerminalPublicationDraftV1{
			{Slot: "terminal-item", Draft: itemEvent},
			{Slot: "usage", Draft: usage},
			{Slot: "terminal", Draft: contracts.CloneMap(previous.TerminalEvent)},
		})
	if err != nil {
		t.Fatalf("domain fixture must retain valid envelope/digest with open usage diagnostics: %v", err)
	}
	value := domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)
	if _, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(value); err != nil {
		t.Fatalf("domain fixture did not pass strict commit parsing: %v", err)
	}
	return value
}

func TestGeneralTerminalCASRejectsNonJSONOutboxBeforeAuthorityParsingV1(t *testing.T) {
	for name, replay := range map[string]bool{"active": false, "replay": true} {
		t.Run(name, func(t *testing.T) {
			input := generalTerminalTelemetryCASInputV1(t)
			if replay {
				prepared, err := PrepareTerminalCASMutation(input)
				if err != nil || !prepared.Applied {
					t.Fatalf("exact active fixture could not prepare replay: %v", err)
				}
				input.Thread = prepared.Thread
			}
			commit := input.Fields["generalTerminalPublication"].(map[string]any)
			// Do not marshal this input: the existing sentinel panics if any
			// authority parser reaches it before the original JSON type guard.
			commit["unexpected"] = acceptedFinalMarshalSentinelV1{}
			prepared, err := PrepareTerminalCASMutation(input)
			if err == nil || prepared.Applied {
				t.Fatal("non-JSON outbox reached canonical authority qualification")
			}
			if _, present := commit["unexpected"].(acceptedFinalMarshalSentinelV1); !present {
				t.Fatal("non-JSON rejection changed the original outbox")
			}
		})
	}
}

func TestGeneralTerminalCASLateLoserPreservesWinnerV1(t *testing.T) {
	for _, outcome := range []struct {
		name, winnerReason, loserReason string
	}{
		{name: "same status different accepted time", winnerReason: "success", loserReason: "success"},
		{name: "completed winner failed loser", winnerReason: "success", loserReason: "timeout"},
		{name: "failed winner completed loser", winnerReason: "timeout", loserReason: "success"},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			for _, advanced := range []bool{false, true} {
				name := "same current context"
				if advanced {
					name = "newer current turn"
				}
				t.Run(name, func(t *testing.T) {
					base := generalTerminalTelemetryCASInputV1(t)
					frozen, err := domainsecurity.ParseTurnSecurityContext(base.Thread["securityState"])
					if err != nil {
						t.Fatal(err)
					}
					winnerInput := generalTerminalLateLoserInputV1(t, frozen, outcome.winnerReason, base.Now)
					winner, err := PrepareTerminalCASMutation(winnerInput)
					if err != nil || !winner.Applied || winner.Thread == nil {
						t.Fatalf("could not commit the ordinary winner fixture: %v", err)
					}
					if advanced {
						current := generalTerminalTestContext(t, frozen.ThreadID, "turn-newer-general-current", frozen.ContextEpoch+1)
						winner.Thread["securityState"] = turnSecurityContextRecord(current)
						winner.Thread["turns"] = append(winner.Thread["turns"].([]any), map[string]any{
							"id": current.TurnID, "threadId": current.ThreadID, "status": "running",
							"securityContext": turnSecurityContextRecord(current), "items": []any{},
						})
					}
					loser := generalTerminalLateLoserInputV1(t, frozen, outcome.loserReason, base.Now.Add(time.Second))
					loser.Thread = winner.Thread
					winnerStatus := stringField(publicationFixtureTurn(winner.Thread), "status")
					winnerCommit := publicationFixtureTurn(winner.Thread)["generalTerminalPublication"].(map[string]any)
					loserCommit := loser.Fields["generalTerminalPublication"].(map[string]any)
					if winnerCommit["committedAt"] == loserCommit["committedAt"] || winnerCommit["commitDigest"] == loserCommit["commitDigest"] {
						t.Fatal("late loser fixture accidentally reused the exact winner")
					}
					loads := 0
					loser.LoadGeneralTerminalEvents = func() ([]map[string]any, error) {
						loads++
						return nil, errors.New("late loser must not load publication inventory")
					}
					before, err := json.Marshal([]any{loser.Thread, loser.AppendItems, loser.Fields})
					if err != nil {
						t.Fatal(err)
					}
					result, err := PrepareTerminalCASMutation(loser)
					if err != nil || result.Applied || result.CurrentStatus != winnerStatus || result.Thread != nil || result.HasAcceptedFinal || loads != 0 {
						t.Fatalf("valid ordinary late loser did not preserve the winner: applied=%t status=%s returned_thread=%t accepted_final=%t inventory_loads=%d err=%v", result.Applied, result.CurrentStatus, result.Thread != nil, result.HasAcceptedFinal, loads, err)
					}
					after, err := json.Marshal([]any{loser.Thread, loser.AppendItems, loser.Fields})
					if err != nil || !bytes.Equal(before, after) {
						t.Fatal("late loser changed the original winner or contender payload")
					}
				})
			}
		})
	}
}

func TestGeneralTerminalCASLateLoserRejectsInvalidCandidateV1(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, *PrepareTerminalCASInput){
		"missing outbox": func(_ *testing.T, input *PrepareTerminalCASInput) {
			delete(input.Fields, "generalTerminalPublication")
		},
		"missing binding": func(_ *testing.T, input *PrepareTerminalCASInput) {
			delete(input.Fields, "generalTerminalCASBinding")
		},
		"status differs from own commit": func(_ *testing.T, input *PrepareTerminalCASInput) {
			input.Status = "failed"
		},
		"wrong thread": func(_ *testing.T, input *PrepareTerminalCASInput) {
			input.ThreadID = "other-thread"
		},
		"wrong target turn": func(_ *testing.T, input *PrepareTerminalCASInput) {
			input.TurnID = "other-turn"
		},
		"different frozen context": func(t *testing.T, input *PrepareTerminalCASInput) {
			other := generalTerminalTestContext(t, input.ThreadID, input.TurnID, 3)
			foreign := generalTerminalLateLoserInputV1(t, other, "success", input.Now)
			input.AppendItems, input.Fields = foreign.AppendItems, foreign.Fields
		},
		"different frozen context with authority override": func(t *testing.T, input *PrepareTerminalCASInput) {
			other := generalTerminalTestContext(t, input.ThreadID, input.TurnID, 3)
			foreign := generalTerminalLateLoserInputV1(t, other, "success", input.Now)
			input.AppendItems, input.Fields = foreign.AppendItems, foreign.Fields
			input.Fields["securityContext"] = turnSecurityContextRecord(other)
		},
		"digest bound unsafe telemetry": func(t *testing.T, input *PrepareTerminalCASInput) {
			input.Fields["generalTerminalPublication"] = generalTerminalValidationViewDomainCommitV1(t, func(usage map[string]any) {
				usage["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			})
		},
		"raw sibling": func(_ *testing.T, input *PrepareTerminalCASInput) {
			input.Fields["lastError"] = "PRIVATE_PROVIDER_FAILURE"
		},
		"original resource budget": func(_ *testing.T, input *PrepareTerminalCASInput) {
			input.Fields["oversized"] = strings.Repeat("x", (1<<20)+1)
		},
		"corrupt existing winner": func(_ *testing.T, input *PrepareTerminalCASInput) {
			publicationFixtureTurn(input.Thread)["generalTerminalPublication"].(map[string]any)["commitDigest"] = strings.Repeat("b", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := generalTerminalTelemetryCASInputV1(t)
			winner, err := PrepareTerminalCASMutation(input)
			if err != nil || !winner.Applied {
				t.Fatalf("could not commit the ordinary winner fixture: %v", err)
			}
			input.Thread = winner.Thread
			mutate(t, &input)
			loads := 0
			input.LoadGeneralTerminalEvents = func() ([]map[string]any, error) {
				loads++
				return nil, errors.New("invalid late loser must not load publication inventory")
			}
			before, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
			if err != nil {
				t.Fatal(err)
			}
			result, err := PrepareTerminalCASMutation(input)
			if err == nil || result.Applied || result.Thread != nil || loads != 0 {
				t.Fatalf("invalid contender received ordinary terminal no-op success: applied=%t returned_thread=%t inventory_loads=%d err=%v", result.Applied, result.Thread != nil, loads, err)
			}
			after, err := json.Marshal([]any{input.Thread, input.AppendItems, input.Fields})
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("late loser rejection changed the original winner or contender payload")
			}
		})
	}
}

func generalTerminalLateLoserInputV1(t *testing.T, frozen domainsecurity.TurnSecurityContext, reason string, acceptedAt time.Time) PrepareTerminalCASInput {
	t.Helper()
	base := generalTerminalTelemetryCASInputV1(t)
	previous, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(base.Fields["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	status, ok := domainturnterminal.GeneralTerminalStatusForReasonV1(reason)
	if !ok {
		t.Fatal("late loser fixture requires a known terminal reason")
	}
	text := ""
	if reason == "success" {
		text = GeneralProviderFinalQuarantinedText
	}
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(frozen, reason, status, text)
	if err != nil {
		t.Fatal(err)
	}
	stamp := acceptedAt.UTC().Format(time.RFC3339Nano)
	usage := contracts.CloneMap(previous.UsageEvent)
	usage["threadId"], usage["turnId"], usage["timestamp"] = frozen.ThreadID, frozen.TurnID, stamp
	terminal := map[string]any{
		"kind": "turn_" + status, "threadId": frozen.ThreadID, "turnId": frozen.TurnID,
		"status": status, "timestamp": stamp, "terminalReason": reason, "generalTerminalCASBindingDigest": binding.BindingDigest,
	}
	items := []map[string]any{}
	var itemEvent map[string]any
	if reason == "success" {
		item := BuildAssistantTextItem(AssistantTextItemInput{
			ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Text: text, Status: status, CreatedAt: stamp, FinishedAt: stamp,
		})
		item["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
		items = append(items, item)
		itemEvent = AssistantItemCompletedEvent(item)
		itemEvent["timestamp"] = stamp
	} else {
		projection, ok := domainterminal.GeneralFailureProjectionV1(reason)
		if !ok {
			t.Fatal("late loser fixture requires a closed host failure projection")
		}
		usage["usageFinalStatus"] = status
		terminal["code"], terminal["message"], terminal["error"], terminal["severity"] = projection.Code, projection.Message, projection.Message, projection.Severity
	}
	commit, err := BuildGeneralTerminalPublicationCommitForEventsV1(frozen, binding, itemEvent, usage, terminal, stamp, len(items) != 0)
	if err != nil {
		t.Fatalf("could not build a legal ordinary late loser fixture: %v", err)
	}
	return PrepareTerminalCASInput{
		Thread: map[string]any{
			"id": frozen.ThreadID, "status": "running", "securityState": turnSecurityContextRecord(frozen),
			"turns": []any{map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "securityContext": turnSecurityContextRecord(frozen), "items": []any{}}},
		},
		ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Status: status, AppendItems: items,
		Fields: map[string]any{"generalTerminalCASBinding": domainturnterminal.GeneralTerminalCASBindingV1Map(binding), "generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)},
		Now:    acceptedAt, LoadGeneralTerminalEvents: func() ([]map[string]any, error) { return nil, nil },
	}
}
