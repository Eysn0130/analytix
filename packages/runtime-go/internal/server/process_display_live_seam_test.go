package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSharedRunningInputUsesOriginalAcceptedCallGrantAndExpansionV2(t *testing.T) {
	t.Run("escaped credential remains unavailable under the actual grant", func(t *testing.T) {
		const secret = "opaque-current-attempt-value-9251"
		h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": secret})
		pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 7, nil)
		pending.Call.Arguments = json.RawMessage(`{"path":"\u006fpaque-current-attempt-value-9251"}`)
		decoded, err := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
		if err != nil || decoded["path"] != secret {
			t.Fatal("fixture did not decode to the actual known credential")
		}
		h.recordRuntimeToolStarted(pending)
		ctx := domainprocess.WithKnownSecretsV1(context.Background(), secret)
		defer domainprocess.ClearKnownSecretsV1(ctx)
		h.registerProcessToolInputV2(ctx, pending)
		if processSeamAuthorizeV1(t, h, pending, processSeamViewerV1).Status != "authorized" {
			t.Fatal("actual escaped-input grant was denied")
		}
		meta, wire := processSeamReadV1(t, h, processSeamViewerV1)
		if len(meta.Blocks) != 1 || meta.Blocks[0].Input != nil || strings.Contains(wire, secret) {
			t.Fatal("collapsed escaped input escaped its existing boundary")
		}
		selected, selectedWire := processSeamReadV1(t, h, processSeamViewerV1, meta.Blocks[0].BlockID)
		if len(selected.Blocks) != 1 || selected.Blocks[0].Source.CallID != pending.Call.ID || selected.Blocks[0].Input == nil || selected.Blocks[0].Input.Availability != "unavailable" || selected.Blocks[0].Input.InputJSON != "" || selected.Blocks[0].Classification != "withheld" || strings.Contains(selectedWire, secret) {
			t.Fatal("granted expanded running input retained a decoded credential")
		}
		h.turnSecurity.Identity = nil
		if retired, _ := processSeamReadV1(t, h, processSeamViewerV1, meta.Blocks[0].BlockID); retired.Status != "unavailable" {
			t.Fatal("escaped-input semantic guard bypassed lease revocation")
		}
	})
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "Alice.txt"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 7, nil)
	pending.Call.Arguments = json.RawMessage(" \n" + string(pending.Call.Arguments) + " \n")
	h.recordRuntimeToolStarted(pending)
	h.registerProcessToolInputV2(context.Background(), pending)
	if processSeamAuthorizeV1(t, h, pending, processSeamViewerV1).Status != "authorized" {
		t.Fatal("actual shared input denied")
	}
	meta, wire := processSeamReadV1(t, h, processSeamViewerV1)
	if len(meta.Blocks) != 1 || meta.Blocks[0].Input != nil || strings.Contains(wire, "Alice.txt") {
		t.Fatal("collapsed private input escaped")
	}
	id := meta.Blocks[0].BlockID
	expanded, _ := processSeamReadV1(t, h, processSeamViewerV1, id)
	if len(expanded.Blocks) != 1 || expanded.Blocks[0].Input == nil || expanded.Blocks[0].Input.InputJSON != string(pending.Call.Arguments) || expanded.Blocks[0].Source.ToolResultItemID != "" {
		t.Fatal("active call lost exact bytes or became a result")
	}
	h.turnSecurity.Identity = nil
	retired, wire := processSeamReadV1(t, h, processSeamViewerV1, id)
	if retired.Status != "unavailable" || strings.Contains(wire, "Alice.txt") {
		t.Fatal("revoked source served complete input")
	}
}

func TestDispatchedCommandUsesSignedSourceLeaseWithoutOutputV1(t *testing.T) {
	const secret = "synthetic-process-command-credential"
	const command = "echo actual-command " + secret
	h, pending, _, _ := protectedSeamFixtureV1(t, "bash", map[string]any{"command": command})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 7, nil)
	h.recordRuntimeToolStarted(pending)
	ctx := domainprocess.WithKnownSecretsV1(context.Background(), secret)
	defer domainprocess.ClearKnownSecretsV1(ctx)
	h.registerProcessShellInputV1(ctx, pending)
	if response := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); response.Status != "authorized" {
		t.Fatalf("command authorization=%s", response.Status)
	}
	response, wire := processSeamReadV1(t, h, processSeamViewerV1)
	if response.Status != "snapshot" || len(response.Blocks) != 1 {
		t.Fatalf("command response=%s blocks=%d", response.Status, len(response.Blocks))
	}
	block := response.Blocks[0]
	if block.Kind != "shell" || block.State != "running" || block.Attempt != 0 || block.Source.Kind != "tool" || block.Source.CallID != pending.Call.ID || block.Source.ToolCallItemID != pending.ToolCallItemID || block.Source.StartedEventSeq == "" || block.Source.ToolResultItemID != "" {
		t.Fatal("command lacks actual dispatched coordinates")
	}
	if block.Classification != "credential_redacted" || !strings.Contains(block.Title, "actual-command") || strings.Contains(wire, secret) || block.Body != nil || block.Preview != "" || block.CaptureDigest != "" || block.ExitCode != nil {
		t.Fatal("command lost credential floor or invented output")
	}
	selected, selectedWire := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	if selected.Status != "snapshot" || len(selected.Blocks) != 1 || selected.Blocks[0].BlockID != block.BlockID || selected.Blocks[0].Title != block.Title || selected.Blocks[0].Body != nil || strings.Contains(selectedWire, secret) {
		t.Fatal("expanded command became an output capture")
	}
	h.turnSecurity.Identity = nil
	retired, retiredWire := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	if retired.Status != "unavailable" || retired.Scope != nil || len(retired.Blocks) != 0 || strings.Contains(retiredWire, "actual-command") {
		t.Fatal("revoked source delivered command")
	}
}
