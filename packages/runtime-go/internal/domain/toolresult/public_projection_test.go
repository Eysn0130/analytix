package toolresult

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanTargetBoundsMatchPublicResult(t *testing.T) {
	for _, tc := range []struct {
		id, path string
		valid    bool
	}{
		{strings.Repeat("i", 256), strings.Repeat("p", 1024), true},
		{strings.Repeat("i", 257), "plan.md", false},
		{"plan", strings.Repeat("p", 1025), false},
		{"plan", "../plan.md", false},
		{"plan", "/plan.md", false},
	} {
		if valid := ValidatePlanTargetV1(tc.id, tc.path) == nil; valid != tc.valid {
			t.Fatalf("plan target boundary mismatch: idBytes=%d pathBytes=%d", len(tc.id), len(tc.path))
		}
	}
}

func TestPublicToolResultProjectionV1StrictRoundTrip(t *testing.T) {
	projection := WithheldProjectionV1("completed", "tool_output_private")
	record := PublicToolResultProjectionRecordV1(projection)
	parsed, err := ParsePublicToolResultProjectionV1(record)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != projection {
		t.Fatalf("projection round trip mismatch: %#v", parsed)
	}

	hostile := map[string]any{}
	for key, value := range record {
		hostile[key] = value
	}
	hostile["rawPayload"] = map[string]any{"account": "6222020202020202020"}
	if _, err := ParsePublicToolResultProjectionV1(hostile); err == nil {
		t.Fatal("strict public projection accepted an unknown raw payload field")
	}
}

func TestOutcomeUnknownProjectionRequiresCanonicalHostTriple(t *testing.T) {
	projection := OutcomeUnknownAfterRestartProjectionV1()
	if err := ValidatePublicToolResultProjectionV1(projection); err != nil {
		t.Fatalf("canonical outcome-unknown projection rejected: %v", err)
	}
	for name, mutate := range map[string]func(*PublicToolResultProjectionV1){
		"status":  func(value *PublicToolResultProjectionV1) { value.Status = "cancelled" },
		"message": func(value *PublicToolResultProjectionV1) { value.MessageKey = "tool_cancelled" },
		"code":    func(value *PublicToolResultProjectionV1) { value.Code = "tool_cancelled" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := projection
			mutate(&changed)
			if err := ValidatePublicToolResultProjectionV1(changed); err == nil {
				t.Fatalf("mismatched outcome-unknown %s was accepted: %#v", name, changed)
			}
		})
	}
}

func TestInvalidPublicToolResultProjectionDowngradesWithoutPayload(t *testing.T) {
	invalid := PublicToolResultProjectionV1{
		SchemaVersion:          1,
		ProjectionKind:         ProjectionWithheld,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             "provider supplied account 6222020202020202020",
		Status:                 "completed",
		PrivatePayloadWithheld: true,
	}
	record := PublicToolResultProjectionRecordV1(invalid)
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "6222020202020202020") || record["projectionKind"] != "withheld" || record["status"] != "failed" {
		t.Fatalf("invalid projection did not fail closed: %s", encoded)
	}
}

func TestPublicToolResultItemProjectionDropsUnknownRootPayload(t *testing.T) {
	const sentinel = "PRIVATE_TOOL_RESULT_ROOT_SENTINEL"
	const account = "6222020202020202020"
	projected := PublicToolResultItemRecordV1(map[string]any{
		"id":               "item_result_turn-1_provider_call_" + account,
		"turnId":           "turn-1",
		"threadId":         "thread-1",
		"role":             "tool",
		"status":           "completed",
		"createdAt":        "2026-07-14T00:00:00Z",
		"kind":             "tool_result",
		"toolName":         "read",
		"callId":           "provider_call_" + account,
		"toolKind":         "tool_call",
		"isError":          false,
		"contextDigest":    "private-digest",
		"contextEpoch":     float64(9),
		"executionGrantId": "private-grant",
		"summary":          sentinel,
		"details":          map[string]any{"account": sentinel},
		"dataUrl":          "data:image/png;base64," + sentinel,
		"attachments": []any{map[string]any{
			"localFilePath": "/tmp/" + sentinel,
		}},
		"output": map[string]any{"content": sentinel},
	})
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), sentinel) || strings.Contains(string(encoded), account) || projected["summary"] != nil || projected["details"] != nil || projected["dataUrl"] != nil ||
		projected["contextDigest"] != nil || projected["contextEpoch"] != nil || projected["executionGrantId"] != nil {
		t.Fatalf("unknown root payload escaped public item projection: %s", encoded)
	}
	output, _ := projected["output"].(map[string]any)
	if output["messageKey"] != "legacy_output_withheld" || output["evidenceAuthority"] != false {
		t.Fatalf("legacy root output was not withheld: %#v", output)
	}
}

func TestPublicToolResultItemNormalizesSkillKindToClosedContract(t *testing.T) {
	callID := "call_host_" + strings.Repeat("a", 64)
	item := map[string]any{
		"turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"kind": "tool_result", "toolName": "run_skill", "callId": callID, "toolKind": "skill",
		"isError": false,
		"output":  PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	private, ok := PrivateDurableToolResultItemRecordV1(item)
	if !ok || private["toolKind"] != "skill" {
		t.Fatalf("private durable skill kind was not retained: ok=%v item=%#v", ok, private)
	}
	projected := PublicToolResultItemRecordV1(item)

	if projected["toolKind"] != "tool_call" || projected["callId"] != callID ||
		projected["status"] != "completed" || projected["isError"] != false {
		t.Fatalf("skill tool result did not use the closed public tool kind: %#v", projected)
	}
}

func TestPublicToolResultItemDoesNotReflectLifecycleControlText(t *testing.T) {
	const hostile = "PRIVATE_PROVIDER_LIFECYCLE_SENTINEL"
	projected := PublicToolResultItemRecordV1(map[string]any{
		"turnId": "turn-1", "threadId": "thread-1", "role": hostile, "status": hostile,
		"kind": hostile, "toolName": "read_file", "callId": "call_host_" + strings.Repeat("a", 64), "toolKind": hostile,
		"isError": false,
		"output":  PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	})
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if projected["kind"] != "tool_result" || projected["role"] != "tool" || projected["status"] != "completed" ||
		projected["isError"] != false || projected["toolKind"] != nil || strings.Contains(string(body), hostile) {
		t.Fatalf("tool-result lifecycle control text was reflected: %s", body)
	}

	missingError := PublicToolResultItemRecordV1(map[string]any{
		"turnId": "turn-1", "threadId": "thread-1", "toolName": "read_file", "callId": "call_host_" + strings.Repeat("b", 64),
	})
	if missingError["status"] != "failed" || missingError["isError"] != true || missingError["lifecycleStatusWithheld"] != true {
		t.Fatalf("missing tool-result lifecycle did not fail closed: %#v", missingError)
	}
}

func TestPrivateDurableToolResultRetainsOnlySettlementAuthority(t *testing.T) {
	contextDigest := strings.Repeat("a", 64)
	grantID := strings.Repeat("b", 64)
	const sentinel = "PRIVATE_TOOL_RESULT_SENTINEL_6222020202020202020"
	projected, ok := PrivateDurableToolResultItemRecordV1(map[string]any{
		"id": "item-1", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z", "kind": "tool_result",
		"toolName": "read_file", "callId": "call-1", "toolKind": "tool_call", "isError": false,
		"contextDigest": contextDigest, "contextEpoch": uint64(7), "executionGrantId": grantID,
		"hostEvidenceSettlement": map[string]any{"settlementId": "opaque-marker"},
		"output":                 map[string]any{"content": sentinel},
		"details":                map[string]any{"account": sentinel},
	})
	if !ok || projected["contextDigest"] != contextDigest || projected["contextEpoch"] != uint64(7) || projected["executionGrantId"] != grantID ||
		projected["callId"] != "call-1" || projected["hostEvidenceSettlement"] == nil {
		t.Fatalf("private durable result lost settlement authority: %#v", projected)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sentinel) || projected["details"] != nil {
		t.Fatalf("private durable result retained raw payload: %s", body)
	}
	output, _ := projected["output"].(map[string]any)
	if output["messageKey"] != "legacy_output_withheld" || output["privatePayloadWithheld"] != true {
		t.Fatalf("private durable result did not close raw output: %#v", output)
	}
}

func TestPrivateReportAdmissionMarkerIsTypedAndNeverPublic(t *testing.T) {
	decisionID := strings.Repeat("c", 64)
	decisionDigest := strings.Repeat("d", 64)
	item := map[string]any{
		"id": "item-report", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z", "kind": "tool_result",
		"toolName": "stage_case_report", "callId": "call-report", "toolKind": "tool_call", "isError": false,
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": uint64(7), "executionGrantId": strings.Repeat("b", 64),
		"hostReportAdmission": HostReportAdmissionRecordV1(NewHostReportAdmissionV1(decisionID, decisionDigest)),
		"output":              PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	private, ok := PrivateDurableToolResultItemRecordV1(item)
	if !ok {
		t.Fatal("valid private report admission marker was rejected")
	}
	admission, err := ParseHostReportAdmissionV1(private["hostReportAdmission"])
	if err != nil || admission.DecisionID != decisionID || admission.DecisionRecordDigest != decisionDigest {
		t.Fatalf("private report admission binding changed: %#v err=%v", admission, err)
	}
	public := PublicToolResultItemRecordV1(private)
	if public["hostReportAdmission"] != nil {
		t.Fatalf("private report admission entered public projection: %#v", public)
	}
	forged := HostReportAdmissionRecordV1(admission)
	forged["rawAccount"] = "00123456789012345678"
	item["hostReportAdmission"] = forged
	if _, ok := PrivateDurableToolResultItemRecordV1(item); ok {
		t.Fatal("open-ended report admission marker was accepted")
	}
}

func TestPrivateProtocolObservationMarkerIsTypedBoundAndNeverPublic(t *testing.T) {
	item := map[string]any{
		"id": "item-private-protocol", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z", "kind": "tool_result",
		"toolName": "read_file", "callId": "call-private-protocol", "toolKind": "tool_call", "isError": false,
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": uint64(7), "executionGrantId": strings.Repeat("b", 64),
		"privateProtocolObserved": true,
		"output":                  PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	private, ok := PrivateDurableToolResultItemRecordV1(item)
	if !ok || private["privateProtocolObserved"] != true {
		t.Fatalf("valid private-protocol observation marker was rejected: %#v", private)
	}
	public := PublicToolResultItemRecordV1(private)
	if _, present := public["privateProtocolObserved"]; present {
		t.Fatalf("private-protocol observation marker entered public projection: %#v", public)
	}
	for name, mutate := range map[string]func(map[string]any){
		"false":          func(record map[string]any) { record["privateProtocolObserved"] = false },
		"wrong type":     func(record map[string]any) { record["privateProtocolObserved"] = "true" },
		"missing digest": func(record map[string]any) { delete(record, "contextDigest") },
		"missing grant":  func(record map[string]any) { delete(record, "executionGrantId") },
		"missing epoch":  func(record map[string]any) { delete(record, "contextEpoch") },
		"authority free": func(record map[string]any) {
			delete(record, "contextDigest")
			delete(record, "executionGrantId")
			delete(record, "contextEpoch")
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := cloneToolResultJSONValue(item).(map[string]any)
			mutate(record)
			if _, valid := PrivateDurableToolResultItemRecordV1(record); valid {
				t.Fatal("malformed or authority-free private-protocol marker was accepted")
			}
		})
	}
}

func TestPrivateAdmittedReportResultRequiresExactDecisionAndClosedSuccess(t *testing.T) {
	decisionID := strings.Repeat("c", 64)
	decisionDigest := strings.Repeat("d", 64)
	base := map[string]any{
		"id": "item-report", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z", "kind": "tool_result",
		"toolName": "stage_case_report", "callId": "provider_call_report", "toolKind": "tool_call", "isError": false,
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": uint64(7), "executionGrantId": strings.Repeat("b", 64),
		"hostReportAdmission": HostReportAdmissionRecordV1(NewHostReportAdmissionV1(decisionID, decisionDigest)),
		"output":              PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	if err := ValidatePrivateAdmittedReportResultItemV1(base, decisionID, decisionDigest); err != nil {
		t.Fatalf("exact private admitted report result was rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong decision": func(item map[string]any) {
			item["hostReportAdmission"] = HostReportAdmissionRecordV1(NewHostReportAdmissionV1(strings.Repeat("e", 64), decisionDigest))
		},
		"missing admission": func(item map[string]any) { delete(item, "hostReportAdmission") },
		"generic tool":      func(item map[string]any) { item["toolName"] = "write_file" },
		"failed result":     func(item map[string]any) { item["isError"] = true },
		"open payload":      func(item map[string]any) { item["rawAccount"] = "0006222020202020202020" },
		"fact authority": func(item map[string]any) {
			projection := WithheldProjectionV1("completed", "tool_output_private")
			projection.FactAnswerAllowed = true
			item["output"] = PublicToolResultProjectionRecordV1(projection)
		},
	} {
		t.Run(name, func(t *testing.T) {
			item := cloneToolResultJSONValue(base).(map[string]any)
			mutate(item)
			if err := ValidatePrivateAdmittedReportResultItemV1(item, decisionID, decisionDigest); err == nil {
				t.Fatal("non-authoritative report result acquired settlement eligibility")
			}
		})
	}
}

func TestPrivateDurableToolResultRejectsPartialSettlementAuthority(t *testing.T) {
	base := map[string]any{
		"id": "item-1", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z", "kind": "tool_result",
		"toolName": "read_file", "callId": "call-1", "toolKind": "tool_call", "isError": false,
		"output": PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	for name, mutate := range map[string]func(map[string]any){
		"digest only": func(item map[string]any) { item["contextDigest"] = strings.Repeat("a", 64) },
		"marker only": func(item map[string]any) { item["hostEvidenceSettlement"] = map[string]any{"settlementId": "forged"} },
		"zero epoch": func(item map[string]any) {
			item["contextDigest"] = strings.Repeat("a", 64)
			item["executionGrantId"] = strings.Repeat("b", 64)
			item["contextEpoch"] = uint64(0)
		},
	} {
		t.Run(name, func(t *testing.T) {
			item := map[string]any{}
			for key, value := range base {
				item[key] = value
			}
			mutate(item)
			if _, ok := PrivateDurableToolResultItemRecordV1(item); ok {
				t.Fatal("partial tool-result settlement authority was accepted")
			}
		})
	}
}

func TestCaseSourceProjectionCannotClaimAnswerOrEvidenceAuthority(t *testing.T) {
	projection := PublicToolResultProjectionV1{
		SchemaVersion:          1,
		ProjectionKind:         ProjectionCaseSourceStatus,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             "case_source_private",
		Status:                 "completed",
		Code:                   "case_source_result_private",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      true,
		EvidenceAuthority:      false,
	}
	if err := ValidatePublicToolResultProjectionV1(projection); err == nil {
		t.Fatal("public case projection claimed fact-answer authority")
	}
	projection.FactAnswerAllowed = false
	if err := ValidatePublicToolResultProjectionV1(projection); err != nil {
		t.Fatalf("closed metadata-only case projection rejected: %v", err)
	}
	for name, hostile := range map[string]map[string]any{
		"case outcome":  map[string]any{"caseOutcome": map[string]any{"executionGrantId": "grant-private"}},
		"authority ref": map[string]any{"AuthorityRef": "authority-private"},
	} {
		record := PublicToolResultProjectionRecordV1(projection)
		for key, value := range hostile {
			record[key] = value
		}
		if _, err := ParsePublicToolResultProjectionV1(record); err == nil {
			t.Fatalf("%s was accepted by strict public case projection", name)
		}
	}
}

func TestCaseSourceBindingProofIsPrivateExactAndFailsClosedOnRebind(t *testing.T) {
	projection := PublicToolResultProjectionV1{
		SchemaVersion: PublicProjectionSchemaVersion, ProjectionKind: ProjectionCaseSourceStatus,
		Disclosure: MetadataOnlyDisclosure, MessageKey: "case_source_private", Status: "completed",
		Code: "case_source_result_private", PrivatePayloadWithheld: true,
	}
	callID := "call_host_" + strings.Repeat("c", 64)
	contextDigest := strings.Repeat("a", 64)
	grantID := strings.Repeat("b", 64)
	proof, err := NewCaseSourceBindingProofV1(CaseSourceBindingInputV1{
		ToolName: "mcp__analytix-fund-analysis__query_transactions", ToolCallID: callID,
		ContextDigest: contextDigest, ContextEpoch: 7, ExecutionGrantID: grantID,
		Projection: projection, IsError: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"id": ToolResultItemIDV1("turn-case-source", callID), "turnId": "turn-case-source", "threadId": "thread-case-source",
		"role": "tool", "status": "completed", "kind": "tool_result",
		"toolName": "mcp__analytix-fund-analysis__query_transactions", "callId": callID, "toolKind": "tool_call", "isError": false,
		"contextDigest": contextDigest, "contextEpoch": uint64(7), "executionGrantId": grantID,
		CaseSourceBindingProofFieldV1: CaseSourceBindingProofRecordV1(proof),
		"output":                      PublicToolResultProjectionRecordV1(projection),
	}
	private, ok := PrivateDurableToolResultItemRecordV1(base)
	if !ok || private[CaseSourceBindingProofFieldV1] == nil {
		t.Fatalf("canonical private case-source result was rejected: %#v", private)
	}
	public := PublicToolResultItemRecordV1(private)
	publicProjection, parseErr := ParsePublicToolResultProjectionV1(public["output"])
	if parseErr != nil || publicProjection.ProjectionKind != ProjectionCaseSourceStatus ||
		public[CaseSourceBindingProofFieldV1] != nil || public["contextDigest"] != nil ||
		public["contextEpoch"] != nil || public["executionGrantId"] != nil {
		t.Fatalf("public case-source status did not strip its private binding: %#v err=%v", public, parseErr)
	}

	mutations := map[string]func(map[string]any){
		"missing proof":  func(item map[string]any) { delete(item, CaseSourceBindingProofFieldV1) },
		"tool rebound":   func(item map[string]any) { item["toolName"] = "mcp__analytix-fund-analysis__other" },
		"call rebound":   func(item map[string]any) { item["callId"] = "call_host_" + strings.Repeat("d", 64) },
		"digest rebound": func(item map[string]any) { item["contextDigest"] = strings.Repeat("d", 64) },
		"epoch rebound":  func(item map[string]any) { item["contextEpoch"] = uint64(8) },
		"grant rebound":  func(item map[string]any) { item["executionGrantId"] = strings.Repeat("d", 64) },
		"status rebound": func(item map[string]any) { item["status"] = "failed" },
		"error rebound":  func(item map[string]any) { item["isError"] = true },
		"projection rebound": func(item map[string]any) {
			output := item["output"].(map[string]any)
			output["status"] = "failed"
			output["code"] = "case_source_failed"
			output["messageKey"] = "case_source_failed"
		},
		"projection kind rebound": func(item map[string]any) {
			item["output"] = PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private"))
		},
		"wrong proof": func(item map[string]any) {
			item[CaseSourceBindingProofFieldV1].(map[string]any)["digest"] = strings.Repeat("d", 64)
		},
		"open proof": func(item map[string]any) {
			item[CaseSourceBindingProofFieldV1].(map[string]any)["rawPath"] = "/private/case.sqlite"
		},
		"unknown proof": func(item map[string]any) {
			item[CaseSourceBindingProofFieldV1].(map[string]any)["purpose"] = "analytix.case-source-result-binding/v2"
		},
		"duplicate proof digest": func(item map[string]any) {
			item[CaseSourceBindingProofFieldV1] = json.RawMessage(`{"version":1,"purpose":"` + CaseSourceBindingProofPurposeV1 + `","digest":"` + proof.Digest + `","digest":"` + proof.Digest + `"}`)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			item := cloneToolResultJSONValue(base).(map[string]any)
			mutate(item)
			if _, valid := PrivateDurableToolResultItemRecordV1(item); valid {
				t.Fatal("rebound or malformed case-source result remained private-durable valid")
			}
			projected := PublicToolResultItemRecordV1(item)
			closed, err := ParsePublicToolResultProjectionV1(projected["output"])
			if err != nil || closed.ProjectionKind != ProjectionWithheld || closed.MessageKey != "legacy_output_withheld" ||
				closed.Status != projected["status"] {
				t.Fatalf("rebound case-source result did not fail closed: %#v err=%v", closed, err)
			}
			body, _ := json.Marshal(projected)
			for _, forbidden := range []string{CaseSourceBindingProofFieldV1, contextDigest, grantID, "/private/case.sqlite"} {
				if strings.Contains(string(body), forbidden) {
					t.Fatalf("failed-closed public result retained private binding %q: %s", forbidden, body)
				}
			}
		})
	}

	nonCase := cloneToolResultJSONValue(base).(map[string]any)
	nonCase["output"] = PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private"))
	if _, valid := PrivateDurableToolResultItemRecordV1(nonCase); valid {
		t.Fatal("non-case projection retained a case-source binding proof")
	}
	closed, err := ParsePublicToolResultProjectionV1(PublicToolResultItemRecordV1(nonCase)["output"])
	if err != nil || closed.ProjectionKind != ProjectionWithheld || closed.MessageKey != "legacy_output_withheld" {
		t.Fatalf("non-case proof was not downgraded: %#v err=%v", closed, err)
	}
}

func TestMCPDiagnosticRequiresCanonicalCodeForClass(t *testing.T) {
	projection := PublicToolResultProjectionV1{
		SchemaVersion:          1,
		ProjectionKind:         ProjectionMCPDiagnostic,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             "mcp_request_rejected",
		Status:                 "failed",
		Code:                   "mcp_request_rejected",
		PrivatePayloadWithheld: true,
		RPCError: &MCPRPCErrorDiagnostic{
			Code: 6222020202020, Class: "invalid_params", DataPresent: true,
		},
	}
	if err := ValidatePublicToolResultProjectionV1(projection); err == nil {
		t.Fatal("public MCP diagnostic accepted a noncanonical remote code")
	}
	projection.RPCError.Code = -32602
	if err := ValidatePublicToolResultProjectionV1(projection); err != nil {
		t.Fatalf("canonical MCP diagnostic rejected: %v", err)
	}
}
