package mcp

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestGoFundsExactHostCatalogKeepsCountCanaryAndCapturesAccountFlow(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required for the Go-to-funds V2 contract: %v", err)
	}
	manifestDigest := domainsecurity.SHA256Hex([]byte("go-wire-funds-manifest"))
	projection, err := domainsecurity.NewFundsCountProjectionV2(domainsecurity.FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          domainsecurity.SHA256Hex([]byte("go-wire-security-context")),
		DatasetSnapshotID:                  domainsecurity.DatasetSnapshotIDPrefixV2 + manifestDigest,
		DatasetSelectionDigest:             domainsecurity.SHA256Hex([]byte("go-wire-selection")),
		DatasetRecordDigest:                domainsecurity.SHA256Hex([]byte("go-wire-record")),
		DatasetManifestDigest:              manifestDigest,
		FundsProducerContentID:             domainsecurity.FundsProducerContentIDPrefixV2 + domainsecurity.SHA256Hex([]byte("go-wire-producer")),
		FundsProducerContentManifestSHA256: domainsecurity.SHA256Hex([]byte("go-wire-producer-manifest")),
		DetailContentSHA256:                domainsecurity.SHA256Hex([]byte("go-wire-detail")),
		RowCount:                           2645472,
	})
	if err != nil {
		t.Fatal(err)
	}
	probeParams, err := sourceProbeNativeParamsV2(projection)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(probeParams)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"workspace", "caseId", "tenantId", "userId", "account", "card",
		"database", "duckdb", "pluginRoot", "workspaceRealPath",
	} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("Go V2 projection carrier leaked %q: %s", forbidden, body)
		}
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve funds plugin contract path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	handlerPath := filepath.Join(repositoryRoot, "plugins", "analytix-fund-analysis", "mcp", "mcp-request-handler-runtime.mjs")
	script := `
import fs from "node:fs";
import { pathToFileURL } from "node:url";
const { createMcpRequestHandlerRuntime } = await import(pathToFileURL(process.argv[1]).href);
const params = JSON.parse(fs.readFileSync(0, "utf8"));
let dataEffects = 0;
const runtime = createMcpRequestHandlerRuntime({
  serverName:"analytix_funds",serverVersion:"0.16.16",
  callTool:async()=>{dataEffects+=1;throw new Error("plugin data execution is forbidden");}
});
await runtime.handleRequest({method:"initialize",params:{protocolVersion:"2025-11-25",capabilities:{},clientInfo:{name:"contract-test",version:"1"}}});
await runtime.handleNotification({method:"notifications/initialized",params:{}});
const source = await runtime.handleRequest({method:"analytix/sourceProbe",params});
const listed = await runtime.handleRequest({method:"tools/list",params:{}});
const called = await runtime.handleRequest({method:"tools/call",params:{
  name:"count_case_rows",arguments:{table_name:"analysis_txn_detail_idx"},_meta:params._meta
}});
const evidence = await runtime.handleRequest({method:"analytix/evidenceRead",params:{
  tool:"count_case_rows",tableName:"analysis_txn_detail_idx",noFilter:true,_meta:params._meta
}});
const accountFlowArgs = {
  subject_alias:"acct:1",
  start_inclusive:"2026-01-01T00:00:00.000000Z",
  end_inclusive:"2026-01-31T23:59:59.999000Z",
  evidence_row_limit:100
};
const accountFlow = await runtime.handleRequest({method:"tools/call",params:{
  name:"analyze_account_flows",arguments:accountFlowArgs,
  _meta:{analytixRuntimeContext:{caseId:"forged-case",datasetSnapshotId:"dsv2_"+"b".repeat(64)}}
}});
const accountFlowErrors = [];
for (const argumentsValue of [
  {...accountFlowArgs,subject_alias:"6217000012345678901"},
  {...accountFlowArgs,evidence_row_limit:513},
  {...accountFlowArgs,case_id:"case-a"},
  {...accountFlowArgs,datasetSnapshotId:"dsv2_"+"c".repeat(64)},
  {...accountFlowArgs,db_path:"/private/case.duckdb"},
  {...accountFlowArgs,sql:"SELECT * FROM secret"}
]) {
  try {
    await runtime.handleRequest({method:"tools/call",params:{
      name:"analyze_account_flows",arguments:argumentsValue
    }});
    accountFlowErrors.push(0);
  } catch (error) {
    accountFlowErrors.push(error.code);
  }
}
const errors = [];
for (const bad of [
  {_meta:{...params._meta,analytixRuntimeContext:{version:1}}},
  {_meta:{analytixFundsCountProjectionV2:{...params._meta.analytixFundsCountProjectionV2,rowCount:"02645472"}}},
  {_meta:{analytixFundsCountProjectionV2:{...params._meta.analytixFundsCountProjectionV2,accountNumber:"6217000012345678901"}}},
  {_meta:{analytixFundsCountProjectionV2:{...params._meta.analytixFundsCountProjectionV2,fundsProducerContentId:"fpc3_"+params._meta.analytixFundsCountProjectionV2.fundsProducerContentId.slice(5)}}}
]) {
  try {
    await runtime.handleRequest({method:"analytix/sourceProbe",params:bad});
    errors.push(0);
  } catch (error) {
    errors.push(error.code);
  }
}
process.stdout.write(JSON.stringify({
  source,listed,called,evidence,errors,accountFlow,accountFlowErrors,dataEffects
}));
`
	command := exec.Command(nodePath, "--input-type=module", "--eval", script, handlerPath)
	command.Stdin = bytes.NewReader(body)
	stdout, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("funds V2 plugin contract failed: %v\n%s", err, stdout)
	}
	var result struct {
		Source map[string]any `json:"source"`
		Listed struct {
			Tools []map[string]any `json:"tools"`
		} `json:"listed"`
		Called            map[string]any `json:"called"`
		Evidence          map[string]any `json:"evidence"`
		Errors            []int          `json:"errors"`
		AccountFlow       map[string]any `json:"accountFlow"`
		AccountFlowErrors []int          `json:"accountFlowErrors"`
		DataEffects       int            `json:"dataEffects"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode funds V2 contract: %v\n%s", err, stdout)
	}
	if result.Source["ready"] != true || result.Source["projectionDigest"] != projection.ProjectionDigest ||
		len(result.Listed.Tools) != 2 || result.Listed.Tools[0]["name"] != hostFundsCountToolNameV1 ||
		result.Listed.Tools[1]["name"] != hostFundsAccountFlowToolNameV1 ||
		result.Evidence["purpose"] != "analytix.funds.count-case-rows-evidence-candidate/v2" ||
		result.Evidence["paginationComplete"] != true ||
		len(result.Errors) != 4 || result.Errors[0] != -32602 || result.Errors[1] != -32602 ||
		result.Errors[2] != -32602 || result.Errors[3] != -32602 ||
		len(result.AccountFlowErrors) != 6 || result.DataEffects != 0 {
		t.Fatalf("funds V2 plugin contract drifted: %#v", result)
	}
	for _, code := range result.AccountFlowErrors {
		if code != -32602 {
			t.Fatalf("unsafe account-flow argument was not rejected before dispatch: %#v", result.AccountFlowErrors)
		}
	}
	listedBody, err := json.Marshal(result.Listed)
	if err != nil {
		t.Fatal(err)
	}
	parsedTools, err := parseMCPTools(listedBody)
	if err != nil || !exactHostFundsToolCatalogV1(parsedTools, 0) {
		t.Fatalf("live plugin catalog does not match the exact host-installed contract: err=%v tools=%#v", err, parsedTools)
	}
	structured, ok := result.Called["structuredContent"].(map[string]any)
	if !ok || structured["semanticStatus"] != "success" ||
		structured["purpose"] != "analytix.funds-count-tool-outcome/v2" {
		t.Fatalf("funds count tool did not return strict semantic success: %#v", result.Called)
	}
	callProjection, ok := structured["data"].(map[string]any)
	evidenceProjection, evidenceOK := result.Evidence["projection"].(map[string]any)
	if !ok || !evidenceOK ||
		callProjection["projectionDigest"] != projection.ProjectionDigest ||
		evidenceProjection["projectionDigest"] != projection.ProjectionDigest ||
		callProjection["rowCount"] != projection.RowCount ||
		evidenceProjection["rowCount"] != projection.RowCount {
		t.Fatalf("funds projection drifted across source/tool/evidence: %#v", result)
	}
	accountFlowStructured, ok := result.AccountFlow["structuredContent"].(map[string]any)
	accountFlowContent, contentOK := result.AccountFlow["content"].([]any)
	if !ok || !contentOK || len(accountFlowContent) != 0 || result.AccountFlow["isError"] != true ||
		accountFlowStructured["schemaVersion"] != float64(1) ||
		accountFlowStructured["purpose"] != "analytix.funds-account-flow-host-capture/v1" ||
		accountFlowStructured["semanticStatus"] != "host_authority_required" ||
		accountFlowStructured["hostCaptureRequired"] != true ||
		accountFlowStructured["factAnswerAllowed"] != false || len(accountFlowStructured) != 5 {
		t.Fatalf("direct Node account-flow dispatch did not stop at the closed host boundary: %#v", result.AccountFlow)
	}
	accountFlowBody, err := json.Marshal(result.AccountFlow)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"cer1_", "6217000012345678901", "2026-01-01", "forged-case", "dsv2_",
		"duckdb", "db_path", "sql", "/private/",
	} {
		if bytes.Contains(bytes.ToLower(accountFlowBody), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("direct Node account-flow boundary reflected private/factual input %q: %s", forbidden, accountFlowBody)
		}
	}
	outputBody, _ := json.Marshal(result)
	if bytes.Contains(outputBody, []byte("safeToAnswer")) ||
		bytes.Contains(outputBody, []byte("analytixRuntimeContext")) {
		t.Fatalf("plugin self-granted publication or echoed legacy authority: %s", outputBody)
	}
}
