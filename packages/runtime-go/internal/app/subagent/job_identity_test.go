package subagent

import (
	"bytes"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestJobToolIdentityRequiresExactThreadAuthority(t *testing.T) {
	callID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x41}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	const threadID = "thread-parent"
	const turnID = "turn-parent"
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	record := domainjob.Record{
		ID: "job-parent", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: itemID, ParentToolCallID: callID, Kind: "subagent",
	}
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID,
			"items": []any{map[string]any{
				"id": itemID, "threadId": threadID, "turnId": turnID,
				"kind": "tool_call", "callId": callID, "toolName": "task",
			}},
		}},
	}
	gotItemID, gotCallID, gotToolName, ok := JobToolIdentity(thread, record)
	if !ok || gotItemID != itemID || gotCallID != callID || gotToolName != "task" {
		t.Fatalf("exact parent identity was not resolved: item=%q call=%q tool=%q ok=%t", gotItemID, gotCallID, gotToolName, ok)
	}

	tests := map[string]func(map[string]any, *domainjob.Record){
		"missing thread authority": func(candidate map[string]any, record *domainjob.Record) {
			delete(candidate, "turns")
		},
		"provider call id": func(candidate map[string]any, record *domainjob.Record) {
			record.ParentToolCallID = "provider_call_6222020202020202020"
		},
		"record item mismatch": func(candidate map[string]any, record *domainjob.Record) {
			record.ParentToolItemID = "item-forged"
		},
		"thread item mismatch": func(candidate map[string]any, record *domainjob.Record) {
			candidate["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["id"] = "item-forged"
		},
		"thread mismatch": func(candidate map[string]any, record *domainjob.Record) {
			candidate["id"] = "thread-other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := contracts.CloneMap(thread)
			candidateRecord := record
			mutate(candidate, &candidateRecord)
			if item, call, tool, valid := JobToolIdentity(candidate, candidateRecord); valid || item != "" || call != "" || tool != "" {
				t.Fatalf("forged parent identity was resolved: item=%q call=%q tool=%q valid=%t", item, call, tool, valid)
			}
		})
	}

	if item, call, tool, valid := JobToolIdentity(nil, record); valid || item != "" || call != "" || tool != "" {
		t.Fatalf("record fields became authority without the parent thread: item=%q call=%q tool=%q valid=%t", item, call, tool, valid)
	}
}
