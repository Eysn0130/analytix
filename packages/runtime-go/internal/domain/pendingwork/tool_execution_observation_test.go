package pendingwork

import (
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSuccessfulToolExecutionObservationV1IsClosedMetadataOnly(t *testing.T) {
	for _, toolName := range []string{"bash", "read"} {
		observation := SuccessfulToolExecutionObservationV1{
			SchemaVersion: SuccessfulToolExecutionObservationSchemaVersionV1,
			Disclosure:    ToolExecutionObservationDisclosureV1, PrivatePayloadWithheld: true,
			ThreadID: "thread-1", TurnID: "turn-1", ToolName: toolName, Status: StatusCompleted,
			WorkID: domainsecurity.SHA256Hex([]byte("work")), ReceiptID: domainsecurity.SHA256Hex([]byte("receipt")),
			DispositionID: domainsecurity.SHA256Hex([]byte("disposition")), ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")),
			ResultItemID: "item_result_" + strings.Repeat("a", 64), ResultItemDigest: domainsecurity.SHA256Hex([]byte("result")),
		}
		if err := ValidateSuccessfulToolExecutionObservationV1(observation); err != nil {
			t.Fatalf("valid %s observation was rejected: %v", toolName, err)
		}
		body, err := json.Marshal(observation)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"command", "workspace", "arguments", "output", "provider", "case", "account"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("closed observation leaked forbidden field %q: %s", forbidden, body)
			}
		}
	}

	invalid := SuccessfulToolExecutionObservationV1{ToolName: "mcp__analytix_funds__account_flow"}
	if err := ValidateSuccessfulToolExecutionObservationV1(invalid); err == nil {
		t.Fatal("protected tool was accepted as ordinary execution observation")
	}
	legacy := SuccessfulToolExecutionObservationV1{
		SchemaVersion: SuccessfulToolExecutionObservationSchemaVersionV1,
		Disclosure:    ToolExecutionObservationDisclosureV1, PrivatePayloadWithheld: true,
		ThreadID: "thread-1", TurnID: "turn-1", ToolName: "bash", Status: StatusCompleted,
		WorkID: domainsecurity.SHA256Hex([]byte("work")), ReceiptID: domainsecurity.SHA256Hex([]byte("receipt")),
		DispositionID: domainsecurity.SHA256Hex([]byte("disposition")), ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")),
		ResultItemID:     "item_result_provider-call-6222021234567890123",
		ResultItemDigest: domainsecurity.SHA256Hex([]byte("result")),
	}
	if err := ValidateSuccessfulToolExecutionObservationV1(legacy); err == nil {
		t.Fatal("legacy or provider-derived result item ID was accepted")
	}
}
