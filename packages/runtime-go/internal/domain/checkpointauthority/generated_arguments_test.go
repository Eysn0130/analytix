package checkpointauthority

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestGeneratedOperationArgumentsAuthorityRoundTrip(t *testing.T) {
	for name, fields := range map[string]map[string]any{
		"markdown": {"markdown": strings.Repeat("m", 600*1024)},
		"image":    {"markdown": "report", "images": []any{map[string]any{"id": "image-1", "type": "png", "dataBase64": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 525*1024))}}},
	} {
		t.Run(name, func(t *testing.T) {
			fields["path"], fields["kind"] = "report.docx", "docx"
			arguments, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1_700_000_000, 0).UTC()
			securityContext, grant := operationGroupSecurity(t, now, "generate_office_document", "large-"+name, arguments)
			intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
				SecurityContext: securityContext, ExecutionGrant: grant,
				CheckpointID: domaincheckpointref.RuntimeID("generated-large"), SourceWorkspaceCheckpointID: "generated-large",
				GenerationPrincipalDigest: domainsecurity.SHA256Hex([]byte("synthetic-principal")),
				OperationOrdinal:          1, ToolName: "generate_office_document", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
				Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
					ArgumentKey: "path", RequestedPath: "report.docx", RelativePath: "report.docx", Role: "target",
					ExpectedAfterExisted: true, ExpectedAfterHash: domainsecurity.SHA256Hex([]byte("generated")),
				}, securityContext.WorkspaceRealPath)},
			})
			if err != nil {
				t.Fatal(err)
			}
			body, err := OperationGroupIntentV2Bytes(intent)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseOperationGroupIntentV2(body)
			if err != nil || !bytes.Equal(parsed.ArgumentsJSON, arguments) || parsed.ExecutionGrant.ArgsHash != domainsecurity.CanonicalJSONHash(arguments) {
				t.Fatalf("generated authority roundtrip failed: %v", err)
			}
			parsed.ArgumentsJSON = bytes.Replace(parsed.ArgumentsJSON, []byte("report.docx"), []byte("forged.docx"), 1)
			parsed.OperationGroupID, parsed.IntentDigest = operationGroupIdentity(parsed), ""
			parsed.IntentDigest = operationGroupIntentDigest(parsed)
			if ValidateOperationGroupIntentV2(parsed) == nil {
				t.Fatal("changed arguments bypassed execution grant hash")
			}
		})
	}
}

func TestGeneratedOperationArgumentsLimitsRemainToolSpecific(t *testing.T) {
	largeString, _ := json.Marshal(map[string]any{"path": "report.docx", "markdown": strings.Repeat("m", 600*1024)})
	largeObject, _ := json.Marshal(map[string]any{"a": strings.Repeat("a", 512*1024), "b": strings.Repeat("b", 512*1024)})
	for _, arguments := range [][]byte{largeString, largeObject} {
		if _, err := canonicalOperationArguments("generate_office_document", arguments); err != nil {
			t.Fatal(err)
		}
		for _, tool := range []string{"write_file", "edit_file", "generate_office_document_other"} {
			if _, err := canonicalOperationArguments(tool, arguments); err == nil {
				t.Fatalf("ordinary argument limit widened for %s", tool)
			}
		}
	}
	tooLong, _ := json.Marshal(map[string]any{"markdown": strings.Repeat("x", MaxGeneratedOperationStringBytes+1)})
	tooLarge, _ := json.Marshal(map[string]any{"a": strings.Repeat("x", MaxGeneratedOperationStringBytes), "b": strings.Repeat("x", MaxGeneratedOperationStringBytes), "c": strings.Repeat("x", MaxGeneratedOperationStringBytes), "d": strings.Repeat("x", MaxGeneratedOperationStringBytes)})
	for _, arguments := range [][]byte{tooLong, tooLarge, []byte(`{"markdown":"a","markdown":"b"}`)} {
		if _, err := canonicalOperationArguments("generate_office_document", arguments); err == nil {
			t.Fatal("generated argument ceiling or strict JSON validation bypassed")
		}
	}
	largeRecord, _ := json.Marshal(map[string]any{"toolName": "write_file", "argumentsJson": map[string]any{"a": strings.Repeat("x", 700*1024), "b": strings.Repeat("x", 700*1024), "c": strings.Repeat("x", 700*1024)}})
	if err := decodeStrictOperationGroupRecord(largeRecord, &OperationGroupIntentV2{}); err == nil {
		t.Fatal("ordinary intent record ceiling widened")
	}
	if err := decodeStrictOperationGroupRecord(largeRecord, &OperationGroupTerminalV2{}); err == nil {
		t.Fatal("terminal record ceiling widened")
	}
}
