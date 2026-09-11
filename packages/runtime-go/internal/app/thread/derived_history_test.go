package thread

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCaseDerivedHistoryAcceptsRealStartMetadataAndRejectsAuthority(t *testing.T) {
	maxSteps := 8
	start := appturn.BuildStartRecord(appturn.StartRecordInput{ThreadID: "source", TurnID: "turn-source", Prompt: "retained user request", Model: "model", CreatedAt: "2026-09-07T00:00:00Z", ApprovalPolicy: "on-request", SandboxMode: "workspace-write", AttachmentIDs: []string{"attachment"}, Attachments: []map[string]any{{"id": "attachment"}}, FileReferences: []any{map[string]any{"path": "file.txt"}}, WorkspaceCheckpointID: "checkpoint", GUIPlan: map[string]any{"id": "plan"}, DisableUserInput: true, MaxModelSteps: &maxSteps})
	start.Turn["steering"] = []any{map[string]any{"id": "historical-steering", "status": "cancelled"}}
	start.Turn["acceptedFinalView"] = map[string]any{"historical": "display"}
	start.Turn["securityContext"] = domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{ThreadID: "source", TurnID: "turn-source", WorkspaceRealPath: "/workspace", CaseID: "case", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0)})
	source := map[string]any{"id": "source", "turns": []any{start.Turn}}
	derived, err := BuildFork(AuthorizeCaseForkV1(ForkInput{Source: source, ForkID: "target", ParentThreadID: "source", Now: "2026-09-07T00:01:00Z"}))
	if err != nil {
		t.Fatal(err)
	}
	turn := derived["turns"].([]any)[0].(map[string]any)
	if err := ValidateCaseDerivedHistoryTurnV1("target", turn); err != nil {
		t.Fatal(err)
	}
	for _, number := range []string{"9007199254740993", "1.0", "1e3"} {
		t.Run("lossless_"+number, func(t *testing.T) {
			candidate := contracts.CloneMap(turn)
			candidate["acceptedFinalView"] = map[string]any{"historicalNumber": json.Number(number)}
			body, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			if err := decoder.Decode(&candidate); err != nil {
				t.Fatal(err)
			}
			if err := ValidateCaseDerivedHistoryTurnV1("target", candidate); err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(candidate)
			if err != nil || !bytes.Equal(body, after) {
				t.Fatal("history validation changed original numeric values")
			}
		})
	}
	for _, fault := range []string{"user_extra_authority", "wrong_user_turn", "pending_user", "unknown_root"} {
		t.Run(fault, func(t *testing.T) {
			candidate := contracts.CloneMap(turn)
			item := candidate["items"].([]any)[0].(map[string]any)
			switch fault {
			case "user_extra_authority":
				item["executionGrantId"] = "forged"
			case "wrong_user_turn":
				item["turnId"] = "wrong"
			case "pending_user":
				item["status"] = "pending"
			case "unknown_root":
				candidate["unknownAuthority"] = true
			}
			if err := ValidateCaseDerivedHistoryTurnV1("target", candidate); err == nil {
				t.Fatal("invalid original history was projected away")
			}
		})
	}
}
