package subagent

import (
	"bytes"
	"errors"
	"testing"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestRecoveredJobToolResultIsExplicitlyAuthorityFree(t *testing.T) {
	callID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x51}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	records, err := RecoveredJobToolResult(RecoveredJobToolResultInput{
		ThreadID: "thread-parent", TurnID: "turn-parent",
		Record:  domainjob.Record{ID: "job-1", Status: string(domainjob.StatusInterrupted), RecoveryUpdatedAt: "2026-07-14T06:00:00Z"},
		Message: "runtime restarted", CallID: callID, ToolName: "task",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []map[string]any{records.ResultItem, records.Event} {
		for _, key := range []string{"contextDigest", "contextEpoch", "executionGrantId", "executionGrant", "hostEvidenceSettlement"} {
			if _, ok := record[key]; ok {
				t.Fatalf("recovered authority-free result emitted %s: %#v", key, record)
			}
		}
	}
	if records.ResultItem["createdAt"] != "2026-07-14T06:00:00Z" || records.ResultItem["finishedAt"] != "2026-07-14T06:00:00Z" {
		t.Fatalf("recovered result did not use durable recovery time: %#v", records.ResultItem)
	}
	eventItem, _ := records.Event["item"].(map[string]any)
	for _, key := range []string{"contextDigest", "contextEpoch", "executionGrantId", "executionGrant", "hostEvidenceSettlement"} {
		if _, ok := eventItem[key]; ok {
			t.Fatalf("recovered event item emitted %s: %#v", key, eventItem)
		}
	}
}

func TestRecoveredJobToolResultRejectsProviderIdentityWithoutRecords(t *testing.T) {
	records, err := RecoveredJobToolResult(RecoveredJobToolResultInput{
		ThreadID: "thread-parent", TurnID: "turn-parent",
		Record:  domainjob.Record{ID: "job-1", Status: string(domainjob.StatusInterrupted), RecoveryUpdatedAt: "2026-07-14T06:00:00Z"},
		Message: "runtime restarted", CallID: "provider_call_6222020202020202020", ToolName: "task",
	})
	if !errors.Is(err, toolcatalogapp.ErrToolResultSettlementInvalid) || records.ResultItem != nil || records.Event != nil || records.ResultItemID != "" {
		t.Fatalf("invalid recovered result produced records=%#v err=%v", records, err)
	}
}
