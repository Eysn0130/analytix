package checkpointauthority

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestGeneratedOperationArgumentsCASReopenAndRecovery(t *testing.T) {
	ctx := context.Background()
	privateRoot := filepath.Join(t.TempDir(), "private")
	root := filepath.Join(privateRoot, "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "generated-large", "", "generated")
	image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 525*1024))
	arguments, err := json.Marshal(map[string]any{"path": "report.docx", "kind": "docx", "markdown": strings.Repeat("m", 600*1024), "images": []any{
		map[string]any{"id": "image-1", "type": "png", "dataBase64": image},
		map[string]any{"id": "image-2", "type": "png", "dataBase64": image},
		map[string]any{"id": "image-3", "type": "png", "dataBase64": image},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input.ToolName, input.ArgumentsJSON = "generate_office_document", arguments
	input.GenerationPrincipalDigest = domainsecurity.SHA256Hex([]byte("synthetic-principal"))
	input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.SecurityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: input.ToolName,
		ToolCallID: checkpointAuthorityTestHostToolCallID(t, "generated-large"), ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	input.Paths[0].BeforeExisted, input.Paths[0].BeforeAvailable = false, false
	input.Paths[0].BeforeHash, input.Paths[0].BeforeContent = "", ""
	input.Paths[0].RequestedPath, input.Paths[0].RelativePath = "report.docx", "report.docx"
	intent, _, _, err := store.BeginOperationGroup(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domaincheckpoint.OperationGroupIntentV2Bytes(intent)
	if err != nil || len(body) <= domaincheckpoint.MaxSnapshotAuthorityRecordBytes {
		t.Fatalf("fixture does not cross old CAS ceiling: bytes=%d err=%v", len(body), err)
	}
	check := func(opened *Store) {
		t.Helper()
		states, err := opened.ListOperationGroups(ctx)
		if err != nil || len(states) != 1 {
			t.Fatalf("generated intent inventory: count=%d err=%v", len(states), err)
		}
		actual := states[0].Intent
		if !bytes.Equal(actual.ArgumentsJSON, arguments) || actual.IntentDigest != intent.IntentDigest || actual.ExecutionGrant.ArgsHash != input.ExecutionGrant.ArgsHash {
			t.Fatal("generated authority changed during CAS roundtrip")
		}
	}
	reopened, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened)
	existing, present, err := OpenExistingStoreContext(ctx, root, access)
	if err != nil || !present {
		t.Fatalf("existing store reopen: present=%v err=%v", present, err)
	}
	check(existing)
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	check(existing)
	oversizedOrdinary := bytes.Repeat([]byte("x"), domaincheckpoint.MaxSnapshotAuthorityRecordBytes+1)
	if store.intentCAS.PutIfAbsent(ctx, domainsecurity.SHA256Hex(oversizedOrdinary), oversizedOrdinary) == nil {
		t.Fatal("snapshot CAS ceiling widened")
	}
	if store.operationTerminalCAS.PutIfAbsent(ctx, domainsecurity.SHA256Hex(oversizedOrdinary), oversizedOrdinary) == nil {
		t.Fatal("terminal CAS ceiling widened")
	}
}
