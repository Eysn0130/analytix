package turnterminal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestGeneralTerminalPublicationCommitIsDeterministicWithoutAssistantTextCopy(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	first, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Events) != 3 ||
		first.Events[0].Slot != "terminal-item" || first.Events[1].Slot != "usage" || first.Events[2].Slot != "terminal" {
		t.Fatalf("general publication is not deterministic: first=%#v second=%#v", first, second)
	}
	body, _ := json.Marshal(first)
	if strings.Contains(string(body), domainevent.GeneralTerminalCompletedBoundaryTextV1) || first.TerminalItemID == "" {
		t.Fatalf("outbox duplicated assistant prose or lost its item identity: %s", body)
	}
	if _, err := ParseGeneralTerminalPublicationCommitV1(GeneralTerminalPublicationCommitV1Map(first)); err != nil {
		t.Fatalf("strict round trip failed: %v", err)
	}
}

func TestGeneralTerminalPublicationRejectsArbitraryCompletedAssistantText(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, "该案涉案金额为 2,645,472 元。")
	if commit, err := NewGeneralTerminalPublicationCommitV1(
		context, binding, "2026-07-15T00:00:00Z", drafts,
	); err == nil {
		t.Fatalf("arbitrary completed assistant prose entered ordinary terminal publication: %#v", commit)
	}
}

func TestGeneralTerminalPublicationEmptyAssistantUsesTwoSlots(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, "")
	drafts = drafts[1:]
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	if commit.TerminalItemID != "" || len(commit.Events) != 2 ||
		commit.Events[0].Slot != "usage" || commit.Events[1].Slot != "terminal" {
		t.Fatalf("empty assistant produced the wrong manifest: %#v", commit)
	}
}

func TestGeneralTerminalPublicationRejectsUnknownStoredUsageFields(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	drafts[1].Draft["text"] = "usage prose must never enter the archive"
	if commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts); err == nil {
		t.Fatalf("unknown usage field entered a general terminal commit: %#v", commit)
	}
}

func TestGeneralTerminalPublicationRejectsUnknownFieldsAndRecomputedTampering(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	unknown := GeneralTerminalPublicationCommitV1Map(commit)
	unknown["unknown"] = true
	if _, err := ParseGeneralTerminalPublicationCommitV1(unknown); err == nil {
		t.Fatal("top-level unknown field was accepted")
	}
	unknownEvent := GeneralTerminalPublicationCommitV1Map(commit)
	events := unknownEvent["events"].([]any)
	events[0].(map[string]any)["unknown"] = true
	if _, err := ParseGeneralTerminalPublicationCommitV1(unknownEvent); err == nil {
		t.Fatal("event unknown field was accepted")
	}

	tampered := cloneGeneralTerminalPublicationCommitV1(commit)
	tampered.UsageEvent["model"] = "tampered"
	tampered.CommitDigest = generalTerminalPublicationCommitDigestV1(tampered)
	if ValidateGeneralTerminalPublicationCommitV1(tampered) == nil {
		t.Fatal("usage tampering with a recomputed outer digest bypassed commitId binding")
	}

	tampered = cloneGeneralTerminalPublicationCommitV1(commit)
	tampered.Events[0].Slot = "usage"
	tampered.ManifestDigest = manifestDigestForGeneralTerminalTest(tampered.Events)
	tampered.CommitDigest = generalTerminalPublicationCommitDigestV1(tampered)
	if ValidateGeneralTerminalPublicationCommitV1(tampered) == nil {
		t.Fatal("slot tampering with recomputed outer digests was accepted")
	}
}

func TestGeneralTerminalPublicationEventReconstructionRequiresExactBasePayload(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	reconstructed, err := GeneralTerminalPublicationEventDraftV1(commit, "terminal-item", drafts[0].Draft)
	if err != nil || GeneralTerminalPublicationPayloadDigestV1(reconstructed) != commit.Events[0].PayloadDigest {
		t.Fatalf("exact base payload did not reconstruct: event=%#v err=%v", reconstructed, err)
	}
	reconstructed["seq"] = float64(17)
	if GeneralTerminalPublicationPayloadDigestV1(reconstructed) != commit.Events[0].PayloadDigest {
		t.Fatal("durable sequence changed the committed payload digest")
	}
	wrong := cloneGeneralTerminalMap(drafts[0].Draft)
	wrong["item"].(map[string]any)["text"] = "other"
	if _, err := GeneralTerminalPublicationEventDraftV1(commit, "terminal-item", wrong); err == nil {
		t.Fatal("different assistant payload reconstructed from the manifest")
	}
	withMarker := cloneGeneralTerminalMap(drafts[1].Draft)
	withMarker["generalTerminalCommitId"] = commit.CommitID
	if _, err := GeneralTerminalPublicationEventDraftV1(commit, "usage", withMarker); err == nil {
		t.Fatal("caller-supplied publication marker was accepted")
	}
}

func TestGeneralTerminalPublicationRejectsAcceptedFinalAuthorityMarkers(t *testing.T) {
	markers := []string{
		"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId",
		"publicationEventId", "publicationSlot", "publicationPayloadDigest",
	}
	for _, marker := range markers {
		t.Run(marker+"/draft-terminal", func(t *testing.T) {
			context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
			drafts[2].Draft[marker] = "forged"
			if commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts); err == nil {
				t.Fatalf("accepted-final marker entered ordinary terminal draft: %#v", commit)
			}
		})
		t.Run(marker+"/draft-item", func(t *testing.T) {
			context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
			drafts[0].Draft["item"].(map[string]any)[marker] = "forged"
			if commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts); err == nil {
				t.Fatalf("nested accepted-final marker entered ordinary terminal item: %#v", commit)
			}
		})
		t.Run(marker+"/reconstruction", func(t *testing.T) {
			context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
			commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
			if err != nil {
				t.Fatal(err)
			}
			base := cloneGeneralTerminalMap(drafts[1].Draft)
			base["details"] = map[string]any{marker: "forged"}
			if event, err := GeneralTerminalPublicationEventDraftV1(commit, "usage", base); err == nil {
				t.Fatalf("accepted-final marker reconstructed through ordinary outbox: %#v", event)
			}
		})
		t.Run(marker+"/strict-parse", func(t *testing.T) {
			context, binding, drafts := generalTerminalPublicationFixture(t, "")
			commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts[1:])
			if err != nil {
				t.Fatal(err)
			}
			tampered := cloneGeneralTerminalPublicationCommitV1(commit)
			tampered.TerminalEvent[marker] = "forged"
			rebindGeneralTerminalCommitForTest(t, &tampered)
			if ValidateGeneralTerminalPublicationCommitV1(tampered) == nil {
				t.Fatal("fully rehashed ordinary terminal commit retained accepted-final authority")
			}
			if parsed, err := ParseGeneralTerminalPublicationCommitV1(GeneralTerminalPublicationCommitV1Map(tampered)); err == nil {
				t.Fatalf("strict parser accepted rehashed authority marker: %#v", parsed)
			}
		})
	}
}

func generalTerminalPublicationFixture(
	t *testing.T,
	text string,
) (domainsecurity.TurnSecurityContext, GeneralTerminalCASBindingV1, []GeneralTerminalPublicationDraftV1) {
	t.Helper()
	context, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-publication", TurnID: "turn-general-publication",
		WorkspaceRealPath: "/workspace", ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewGeneralTerminalCASBindingV1(context, text)
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	item := map[string]any{
		"id": "item-general-publication", "turnId": context.TurnID, "threadId": context.ThreadID,
		"role": "assistant", "status": "completed", "createdAt": committedAt, "finishedAt": committedAt,
		"kind": "assistant_text", "text": text, "generalTerminalCASBinding": GeneralTerminalCASBindingV1Map(binding),
	}
	return context, binding, []GeneralTerminalPublicationDraftV1{
		{Slot: "terminal-item", Draft: map[string]any{
			"kind": "item_completed", "threadId": context.ThreadID, "turnId": context.TurnID,
			"itemId": item["id"], "item": item, "timestamp": committedAt,
		}},
		{Slot: "usage", Draft: map[string]any{
			"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID,
			"model": "gpt-5", "usage": map[string]any{"totalTokens": float64(3)}, "timestamp": committedAt,
		}},
		{Slot: "terminal", Draft: map[string]any{
			"kind": "turn_completed", "threadId": context.ThreadID, "turnId": context.TurnID,
			"status": "completed", "timestamp": committedAt, "terminalReason": "success",
			"generalTerminalCASBindingDigest": binding.BindingDigest,
		}},
	}
}

func TestGeneralTerminalPublicationSupportsHostFailureOutcome(t *testing.T) {
	context, _, _ := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	message := "The provider request timed out before a verified response was available."
	binding, err := NewGeneralTerminalCASBindingForOutcomeV1(context, "timeout", "failed", message)
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	item := map[string]any{
		"id": "item-general-timeout", "turnId": context.TurnID, "threadId": context.ThreadID,
		"role": "system", "status": "failed", "createdAt": committedAt, "finishedAt": committedAt,
		"kind": "error", "code": "provider_timeout", "message": message, "severity": "error",
		"generalTerminalCASBinding": GeneralTerminalCASBindingV1Map(binding),
	}
	drafts := []GeneralTerminalPublicationDraftV1{
		{Slot: "terminal-item", Draft: map[string]any{
			"kind": "item_completed", "threadId": context.ThreadID, "turnId": context.TurnID,
			"itemId": item["id"], "item": item, "timestamp": committedAt,
		}},
		{Slot: "usage", Draft: map[string]any{
			"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID,
			"model": "gpt-5", "usage": map[string]any{"totalTokens": float64(3)},
			"usageFinalStatus": "failed", "timestamp": committedAt,
		}},
		{Slot: "terminal", Draft: map[string]any{
			"kind": "turn_failed", "threadId": context.ThreadID, "turnId": context.TurnID,
			"status": "failed", "timestamp": committedAt, "terminalReason": "timeout",
			"code": "provider_timeout", "message": message,
			"generalTerminalCASBindingDigest": binding.BindingDigest,
		}},
	}
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, committedAt, drafts)
	if err != nil {
		t.Fatal(err)
	}
	if commit.TerminalStatus != "failed" || commit.TerminalReason != "timeout" ||
		commit.TerminalItemID != "item-general-timeout" || commit.Events[0].Slot != "terminal-item" {
		t.Fatalf("host failure outcome was not committed exactly: %#v", commit)
	}
}

func TestGeneralTerminalPublicationRequiresExactCancelMetadata(t *testing.T) {
	context, binding, drafts := generalTerminalTwoSlotFailureFixture(t, "cancel")
	terminal := drafts[1].Draft
	terminal["discard"] = false
	terminal["cancelled"] = true
	terminal["cancelledPendingGates"] = 0
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatalf("exact cancel metadata was rejected: %v", err)
	}

	missing := cloneGeneralTerminalPublicationCommitV1(commit)
	delete(missing.TerminalEvent, "cancelledPendingGates")
	rebindGeneralTerminalCommitForTest(t, &missing)
	if ValidateGeneralTerminalPublicationCommitV1(missing) == nil {
		t.Fatal("fully rehashed cancel commit without all interrupt fields was accepted")
	}

	context, binding, drafts = generalTerminalTwoSlotFailureFixture(t, "cancel")
	drafts[1].Draft["discard"] = false
	drafts[1].Draft["cancelled"] = false
	drafts[1].Draft["cancelledPendingGates"] = 1
	if invalid, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts); err == nil {
		t.Fatalf("pending cancel gates without cancellation were accepted: %#v", invalid)
	}

	context, binding, drafts = generalTerminalTwoSlotFailureFixture(t, "timeout")
	drafts[1].Draft["discard"] = false
	drafts[1].Draft["cancelled"] = true
	drafts[1].Draft["cancelledPendingGates"] = 0
	if invalid, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts); err == nil {
		t.Fatalf("non-cancel terminal retained interrupt fields: %#v", invalid)
	}
}

func generalTerminalTwoSlotFailureFixture(
	t *testing.T,
	reason string,
) (domainsecurity.TurnSecurityContext, GeneralTerminalCASBindingV1, []GeneralTerminalPublicationDraftV1) {
	t.Helper()
	context, _, _ := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	status, ok := domainterminal.StatusForReasonV1(reason)
	if !ok {
		t.Fatalf("unknown terminal reason %q", reason)
	}
	projection, ok := domainterminal.GeneralFailureProjectionV1(reason)
	if !ok {
		t.Fatalf("terminal reason %q has no closed projection", reason)
	}
	binding, err := NewGeneralTerminalCASBindingForOutcomeV1(context, reason, status, "")
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	return context, binding, []GeneralTerminalPublicationDraftV1{
		{Slot: "usage", Draft: map[string]any{
			"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID,
			"model": "gpt-5", "usage": map[string]any{"totalTokens": float64(3)},
			"usageFinalStatus": status, "timestamp": committedAt,
		}},
		{Slot: "terminal", Draft: map[string]any{
			"kind": "turn_" + status, "threadId": context.ThreadID, "turnId": context.TurnID,
			"status": status, "timestamp": committedAt, "terminalReason": reason,
			"code": projection.Code, "message": projection.Message, "error": projection.Message,
			"severity": projection.Severity, "generalTerminalCASBindingDigest": binding.BindingDigest,
		}},
	}
}

func manifestDigestForGeneralTerminalTest(events []GeneralTerminalPublicationEventV1) string {
	manifest := make([]map[string]any, 0, len(events))
	for _, event := range events {
		manifest = append(manifest, map[string]any{
			"slot": event.Slot, "eventId": event.EventID, "payloadDigest": event.PayloadDigest,
		})
	}
	body, _ := json.Marshal(manifest)
	return domainsecurity.SHA256Hex(body)
}

func rebindGeneralTerminalCommitForTest(t *testing.T, commit *GeneralTerminalPublicationCommitV1) {
	t.Helper()
	commit.CommitID = generalTerminalPublicationCommitIDV1(*commit)
	commit.Events = nil
	for _, candidate := range []struct {
		slot  string
		draft map[string]any
	}{
		{slot: "usage", draft: commit.UsageEvent},
		{slot: "terminal", draft: commit.TerminalEvent},
	} {
		eventID := generalTerminalPublicationEventIDV1(commit.CommitID, candidate.slot)
		draft := cloneGeneralTerminalMap(candidate.draft)
		addGeneralTerminalPublicationMarkers(draft, *commit, candidate.slot, eventID)
		payloadDigest := GeneralTerminalPublicationPayloadDigestV1(draft)
		if payloadDigest == "" {
			t.Fatal("test publication payload digest is empty")
		}
		commit.Events = append(commit.Events, GeneralTerminalPublicationEventV1{
			Slot: candidate.slot, EventID: eventID, PayloadDigest: payloadDigest,
		})
	}
	commit.ManifestDigest = manifestDigestForGeneralTerminalTest(commit.Events)
	commit.CommitDigest = generalTerminalPublicationCommitDigestV1(*commit)
}
