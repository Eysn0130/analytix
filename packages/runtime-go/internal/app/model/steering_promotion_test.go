package model

import (
	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildPromotedSteeringBuildsEventsAndProviderMessages(t *testing.T) {
	firstItem := map[string]any{"id": "item_1", "kind": "user_message", "text": " keep going "}
	out := BuildPromotedSteering(PromotedSteeringInput{
		ThreadID: " thread_1 ",
		TurnID:   " turn_1 ",
		Entries: []map[string]any{
			{"id": "steer_1", "admittedAt": "2026-07-03T08:00:00Z"},
		},
		Items: []map[string]any{
			firstItem,
			{"id": "item_2", "kind": "user_message", "text": "   "},
		},
	})
	if len(out.ItemCreatedEvents) != 2 {
		t.Fatalf("expected item_created events for all promoted items: %#v", out.ItemCreatedEvents)
	}
	firstEvent := out.ItemCreatedEvents[0]
	if firstEvent["kind"] != "item_created" || firstEvent["threadId"] != "thread_1" || firstEvent["turnId"] != "turn_1" || firstEvent["itemId"] != "item_1" {
		t.Fatalf("unexpected item event: %#v", firstEvent)
	}
	item, _ := firstEvent["item"].(map[string]any)
	item["text"] = "mutated"
	if firstItem["text"] != " keep going " {
		t.Fatalf("event item should not alias source item: %#v", firstItem)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected one non-empty provider message: %#v", out.Messages)
	}
	message := out.Messages[0]
	if message.Role != "user" || message.Content == "keep going" {
		t.Fatalf("expected steering provider content wrapper: %#v", message)
	}
	if strings.Contains(message.Content, "Admitted at:") ||
		strings.Contains(message.Content, "2026-07-03T08:00:00Z") ||
		!strings.Contains(message.Content, midTurnSteeringPrefix) ||
		!strings.Contains(message.Content, "keep going") {
		t.Fatalf("steering provider content missing metadata/text: %q", message.Content)
	}
}

func TestBuildPromotedSteeringRejectsMissingIdentity(t *testing.T) {
	if out := BuildPromotedSteering(PromotedSteeringInput{TurnID: "turn_1", Items: []map[string]any{{"id": "item_1"}}}); len(out.ItemCreatedEvents) != 0 || len(out.Messages) != 0 {
		t.Fatalf("missing thread id should reject promotion: %#v", out)
	}
	if out := BuildPromotedSteering(PromotedSteeringInput{ThreadID: "thread_1", Items: []map[string]any{{"id": "item_1"}}}); len(out.ItemCreatedEvents) != 0 || len(out.Messages) != 0 {
		t.Fatalf("missing turn id should reject promotion: %#v", out)
	}
}

// Signed projection fixture only; production timeout/admission is exercised by
// server/pending_gate_resolution_test.go, not this constructor test.
func lateQuestionModelProjectionFixtureV1(t *testing.T) (domainsecurity.TurnSecurityContext, *providerHistorySteeringAuthority, map[string]any, map[string]any) {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "late-projection-thread", TurnID: "late-projection-turn", WorkspaceRealPath: "/workspace", TenantID: "tenant", UserID: "user", SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 4, IssuedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	authority := newProviderHistorySteeringAuthority(t)
	inputID := "input_" + strings.Repeat("a", 26) + "123456789012" + strings.Repeat("a", 26)
	questionID := inputID + "_1"
	identity := domainsteering.UserInputReplyIdentityV1{CallID: ("call_host_" + strings.Repeat("a", 26) + "123456789012" + strings.Repeat("a", 26)), ToolName: "request_user_input", Questions: []domainsteering.UserInputReplyQuestionIdentityV1{{ID: questionID, OptionIDs: []string{questionID + "_option_1"}, SelectedOptionIDs: []string{questionID + "_option_1"}}}}
	content := domainsteering.UserInputReplyContentV1{Kind: domainsteering.UserInputReplyContentKindV1, Questions: []domainsteering.UserInputReplyQuestionContentV1{{Question: "Choose direction", MultiSelect: true, Header: "Direction", Options: []domainsteering.UserInputReplyOptionContentV1{{Label: "Proceed", Description: "Continue useful work"}}}}, Answers: []domainsteering.UserInputReplyAnswerContentV1{{Selected: []string{"Proceed"}, Custom: "ordinary answer"}}}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	clientID := "00000000-0000-4000-8000-000000000081"
	entryID := domainsteering.EntryIDV1(securityContext.TurnID, clientID)
	entry, err := domainsteering.BindPendingEntryV1(map[string]any{"id": entryID, "clientUserMessageId": clientID, "text": string(encoded), "delivery": "steer", "admittedAt": "2026-07-18T01:02:03Z", "userInputId": inputID, "userInputCaptureReceiptId": strings.Repeat("b", 64), "userInputReplyIdentity": domainsteering.UserInputReplyIdentityMapV1(identity)}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signing, err := domainsteering.PendingEntrySigningBytesV1(entry, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	entry, err = domainsteering.SealPendingEntryAuthorityV1(entry, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), ed25519.Sign(authority.privateKey, signing))
	if err != nil {
		t.Fatal(err)
	}
	entry["status"], entry["promotedAt"], entry["promotedItemId"] = "promoted", "2026-07-18T01:02:04Z", entryID
	signing, err = domainsteering.PromotedEntrySigningBytesV1(entry, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	entry, err = domainsteering.SealPromotedEntryAuthorityV1(entry, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), ed25519.Sign(authority.privateKey, signing))
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": entryID, "turnId": securityContext.TurnID, "threadId": securityContext.ThreadID, "role": "user", "status": "completed", "kind": "user_message", "delivery": "steer", "text": string(encoded), "createdAt": "2026-07-18T01:02:03Z", "finishedAt": "2026-07-18T01:02:04Z", "clientUserMessageId": clientID, "contextDigest": securityContext.ContextDigest, "steeringOrigin": "ordinary", "steeringProjectionVersion": float64(domainsteering.ProjectionVersionV1), "steeringContentDigest": entry["contentDigest"]}
	return securityContext, authority, entry, item
}
func TestLateQuestionLiveBindingRequiresExactSignedProjection(t *testing.T) {
	current, _, entry, item := lateQuestionModelProjectionFixtureV1(t)
	out := BuildPromotedSteering(PromotedSteeringInput{ThreadID: current.ThreadID, TurnID: current.TurnID, Entries: []map[string]any{entry}, Items: []map[string]any{item}})
	if len(out.Messages) != 1 {
		t.Fatal("late projection did not produce one message")
	}
	identity, content, err := UserInputReplyProviderPayloadV1(out.Messages[0], current)
	if err != nil || identity.CallID != ("call_host_"+strings.Repeat("a", 26)+"123456789012"+strings.Repeat("a", 26)) || len(content.Answers) != 1 {
		t.Fatal("late projection lost exact signed identity")
	}
	if strings.Contains(out.Messages[0].Content, identity.CallID) || strings.Contains(out.Messages[0].Content, identity.Questions[0].ID) {
		t.Fatal("durable content includes Host identity")
	}
	for _, change := range []func(map[string]any, map[string]any){
		func(e, i map[string]any) { i["text"] = "changed" },
		func(e, i map[string]any) { i["threadId"] = "wrong-thread" },
		func(e, i map[string]any) { delete(e, "userInputReplyIdentity") },
		func(e, i map[string]any) {
			identity := e["userInputReplyIdentity"].(map[string]any)
			identity["callId"] = "call_host_" + strings.Repeat("b", 64)
		},
		func(e, i map[string]any) {
			identity := e["userInputReplyIdentity"].(map[string]any)
			identity["content"] = "user text cannot hide in metadata"
		},
	} {
		badEntry, badItem := contracts.CloneMap(entry), contracts.CloneMap(item)
		change(badEntry, badItem)
		bad := BuildPromotedSteering(PromotedSteeringInput{ThreadID: current.ThreadID, TurnID: current.TurnID, Entries: []map[string]any{badEntry}, Items: []map[string]any{badItem}})
		if len(bad.Messages) != 1 {
			t.Fatal("malformed late message did not remain fail-closed")
		}
		if _, _, err := UserInputReplyProviderPayloadV1(bad.Messages[0], current); err == nil {
			t.Fatal("altered projection minted late binding")
		}
	}
	var roundtrip domainmodel.Message
	encoded, _ := json.Marshal(out.Messages[0])
	if json.Unmarshal(encoded, &roundtrip) != nil || roundtrip.PrivateUserInputReplyBinding != nil || roundtrip.PrivateUserInputReplyRequired {
		t.Fatal("late binding became JSON-visible")
	}
}

func TestLateQuestionIdentityPreservesExistingToolNames(t *testing.T) {
	_, _, entry, _ := lateQuestionModelProjectionFixtureV1(t)
	for _, name := range []string{"user_input", "request_user_input"} {
		identity := contracts.CloneMap(entry["userInputReplyIdentity"].(map[string]any))
		identity["toolName"] = name
		if _, err := domainsteering.ParseUserInputReplyIdentityV1(identity, entry["userInputId"].(string)); err != nil {
			t.Fatalf("existing timed tool name rejected: %s", name)
		}
	}
}
