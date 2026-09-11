package mcp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

const hostFundsServerTimeoutMSV1 int64 = 120_000

const (
	hostFundsCountToolNameV1          = "count_case_rows"
	hostFundsAccountFlowToolNameV1    = "analyze_account_flows"
	hostFundsCountToolDescriptionV1   = "Returns only the host-projected row count for the exact immutable DSV2/FPC selection. The result is evidence material and never grants publication authority."
	hostFundsAccountFlowDescriptionV1 = "Requests a bounded account-flow analysis for one host-resolved case-scoped account or card alias and inclusive time range. The Go host captures the call and owns all snapshot, identity, query, evidence, claim, and publication effects."
)

// HostFundsServerSpecV1 is an opaque capability minted only by the runtime
// composition layer after package and installation authority validation.
// Ordinary MCP JSON can describe a structurally similar process, but it cannot
// construct this capability or escape the reserved-namespace quarantine.
type HostFundsServerSpecV1 struct {
	spec              ServerSpec
	fingerprint       string
	admissionInput    domainpluginpackage.StaticAdmissionInputV1
	sourceReadBinding domainplugincapability.FundsSourceReadBindingV1
}

func NewHostFundsServerSpecV1(
	spec ServerSpec,
	admissionInput domainpluginpackage.StaticAdmissionInputV1,
) (*HostFundsServerSpecV1, error) {
	spec = cloneServerSpecForAuthority(spec)
	if err := validateHostFundsServerSpecV1(spec); err != nil {
		return nil, err
	}
	if err := mcpidentity.VerifyConfiguredProvenance(spec); err != nil {
		return nil, errors.Join(errors.New("host funds MCP provenance is invalid"), err)
	}
	fingerprint := SpecFingerprint(spec)
	sourceReadBinding, err := domainplugincapability.BindFundsSourceReadV1(admissionInput, fingerprint)
	if err != nil || sourceReadBinding.PackageID() != domainpluginpackage.FirstPartyFundsPackageIDV1 ||
		sourceReadBinding.PackageVersion() != spec.ExpectedServerVersion {
		return nil, errors.New("host funds static admission binding is invalid")
	}
	return &HostFundsServerSpecV1{
		spec: spec, fingerprint: fingerprint,
		admissionInput:    cloneStaticAdmissionInputV1(admissionInput),
		sourceReadBinding: sourceReadBinding,
	}, nil
}

func (binding *HostFundsServerSpecV1) valid() bool {
	if binding == nil || binding.fingerprint == "" ||
		validateHostFundsServerSpecV1(binding.spec) != nil ||
		SpecFingerprint(binding.spec) != binding.fingerprint {
		return false
	}
	revalidated, err := domainplugincapability.BindFundsSourceReadV1(
		binding.admissionInput,
		binding.fingerprint,
	)
	return err == nil && revalidated.BindingDigest() == binding.sourceReadBinding.BindingDigest() &&
		revalidated.AdmissionDigest() == binding.sourceReadBinding.AdmissionDigest() &&
		revalidated.ExactRequest() == binding.sourceReadBinding.ExactRequest()
}

func cloneHostFundsServerSpecV1(binding *HostFundsServerSpecV1) *HostFundsServerSpecV1 {
	if !binding.valid() {
		return nil
	}
	return &HostFundsServerSpecV1{
		spec:              cloneServerSpecForAuthority(binding.spec),
		fingerprint:       binding.fingerprint,
		admissionInput:    cloneStaticAdmissionInputV1(binding.admissionInput),
		sourceReadBinding: binding.sourceReadBinding,
	}
}

func cloneStaticAdmissionInputV1(
	input domainpluginpackage.StaticAdmissionInputV1,
) domainpluginpackage.StaticAdmissionInputV1 {
	input.CanonicalDeclaration = append([]byte(nil), input.CanonicalDeclaration...)
	return input
}

func (binding *HostFundsServerSpecV1) sourceReadRequestedV1() bool {
	return binding.valid() && binding.sourceReadBinding.ExactRequest()
}

func (binding *HostFundsServerSpecV1) matches(spec ServerSpec, fingerprint string) bool {
	return binding.valid() && fingerprint != "" &&
		fingerprint == binding.fingerprint && SpecFingerprint(spec) == fingerprint &&
		reflect.DeepEqual(cloneServerSpecForAuthority(spec), binding.spec)
}

func validateHostFundsServerSpecV1(spec ServerSpec) error {
	if spec.ID != "analytix_funds" || spec.Transport != "stdio" ||
		spec.Command == "" || spec.Command != strings.TrimSpace(spec.Command) ||
		!filepath.IsAbs(spec.Command) || filepath.Clean(spec.Command) != spec.Command ||
		!validHostFundsApplicationRunnerPathV1(spec.Command) ||
		len(spec.Args) != 1 || spec.Args[0] != spec.EntrypointPath ||
		!reflect.DeepEqual(spec.Env, map[string]string{"ELECTRON_RUN_AS_NODE": "1"}) ||
		spec.URL != "" || len(spec.Headers) != 0 || spec.CWD != spec.PluginRootPath ||
		spec.ExpectedServerName != "analytix_funds" ||
		!validHostFundsPackageVersionV1(spec.ExpectedServerVersion) ||
		spec.IdentitySource != mcpidentity.HostInstalledGenerationSourceV1 ||
		!canonicalHostFundsSHA256V1(spec.ManifestSHA256) ||
		!validHostFundsEntrypointPathV1(spec.PluginRootPath, spec.EntrypointPath) ||
		!canonicalHostFundsSHA256V1(spec.EntrypointSHA256) ||
		spec.PluginRootPath == "" || !filepath.IsAbs(spec.PluginRootPath) ||
		filepath.Clean(spec.PluginRootPath) != spec.PluginRootPath ||
		!canonicalHostFundsSHA256V1(spec.SourceTreeSHA256) ||
		!canonicalHostFundsSHA256V1(spec.HostInstallMarkerSHA256) ||
		spec.TrustScope != "user" || len(spec.TrustedWorkspaceRoots) != 0 ||
		spec.LowPriority || spec.BackgroundStart || spec.TimeoutMS != hostFundsServerTimeoutMSV1 ||
		!reflect.DeepEqual(spec.ReadOnlyToolNames, map[string]bool{
			hostFundsCountToolNameV1:       true,
			hostFundsAccountFlowToolNameV1: true,
		}) ||
		len(spec.Tools) != 0 {
		return errors.New("host funds MCP server specification is invalid")
	}
	return nil
}

func validHostFundsEntrypointPathV1(pluginRoot, entrypoint string) bool {
	if pluginRoot == "" || pluginRoot != strings.TrimSpace(pluginRoot) ||
		!filepath.IsAbs(pluginRoot) || filepath.Clean(pluginRoot) != pluginRoot ||
		entrypoint == "" || entrypoint != strings.TrimSpace(entrypoint) ||
		!filepath.IsAbs(entrypoint) || filepath.Clean(entrypoint) != entrypoint {
		return false
	}
	relative, err := filepath.Rel(pluginRoot, entrypoint)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func validHostFundsPackageVersionV1(version string) bool {
	return domainpluginpackage.ValidPackageIdentityV1(domainpluginpackage.PackageIdentityV1{
		PackageID: "analytix-fund-analysis", PackageVersion: version,
	})
}

func validHostFundsApplicationRunnerPathV1(command string) bool {
	switch filepath.Base(command) {
	case "analytix":
		macOSDirectory := filepath.Dir(command)
		return filepath.Base(macOSDirectory) == "MacOS" &&
			filepath.Base(filepath.Dir(macOSDirectory)) == "Contents"
	case "analytix.exe":
		return true
	default:
		return false
	}
}

func canonicalHostFundsSHA256V1(value string) bool {
	if len(value) != 64 || value != strings.ToLower(strings.TrimSpace(value)) {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func exactHostFundsToolCatalogV1(catalogTools []ToolSpec, quarantined int) bool {
	if len(catalogTools) != 2 || quarantined != 0 {
		return false
	}
	byName := make(map[string]ToolSpec, len(catalogTools))
	for _, tool := range catalogTools {
		taskSupport, ok := domainmcp.NormalizeToolTaskSupport(tool.TaskSupport)
		if !ok || taskSupport != domainmcp.ToolTaskSupportForbidden || !tool.ReadOnlyHint {
			return false
		}
		if _, duplicate := byName[tool.Name]; duplicate {
			return false
		}
		byName[tool.Name] = tool
	}
	count, countOK := byName[hostFundsCountToolNameV1]
	accountFlow, accountFlowOK := byName[hostFundsAccountFlowToolNameV1]
	return countOK && accountFlowOK && exactHostFundsCountToolV1(count) &&
		exactHostFundsAccountFlowToolV1(accountFlow, hostFundsAccountFlowOutputSchemaV1)
}

// exactHostFundsRuntimeToolCatalogV1 validates the host-owned executable
// surface. The packaged protocol contract always declares both known tools,
// while the in-process host advertises account-flow analysis only when the
// production executor has been composed.
func exactHostFundsRuntimeToolCatalogV1(
	catalogTools []ToolSpec,
	quarantined int,
	sourceReadReady bool,
	accountFlowReady bool,
) bool {
	want := 0
	if sourceReadReady {
		want++
	}
	if accountFlowReady {
		want++
	}
	if len(catalogTools) != want || quarantined != 0 {
		return false
	}
	byName := make(map[string]ToolSpec, len(catalogTools))
	for _, tool := range catalogTools {
		taskSupport, ok := domainmcp.NormalizeToolTaskSupport(tool.TaskSupport)
		if !ok || taskSupport != domainmcp.ToolTaskSupportForbidden || !tool.ReadOnlyHint {
			return false
		}
		if _, duplicate := byName[tool.Name]; duplicate {
			return false
		}
		byName[tool.Name] = tool
	}
	count, countOK := byName[hostFundsCountToolNameV1]
	if sourceReadReady != countOK || (countOK && !exactHostFundsCountToolV1(count)) {
		return false
	}
	accountFlow, accountFlowOK := byName[hostFundsAccountFlowToolNameV1]
	if !accountFlowReady {
		return !accountFlowOK
	}
	return accountFlowOK && exactHostFundsAccountFlowToolV1(
		accountFlow,
		hostFundsAccountFlowAnalysisOutputSchemaV1,
	)
}

func exactHostFundsCountToolV1(tool ToolSpec) bool {
	return tool.Name == hostFundsCountToolNameV1 &&
		tool.Description == hostFundsCountToolDescriptionV1 &&
		canonicalJSONBytes(tool.InputSchema) == canonicalJSONBytes(json.RawMessage(hostFundsInputSchemaV1)) &&
		canonicalJSONBytes(tool.OutputSchema) == canonicalJSONBytes(json.RawMessage(hostFundsOutputSchemaV1))
}

func exactHostFundsAccountFlowToolV1(tool ToolSpec, outputSchema string) bool {
	return tool.Name == hostFundsAccountFlowToolNameV1 &&
		tool.Description == hostFundsAccountFlowDescriptionV1 &&
		canonicalJSONBytes(tool.InputSchema) == canonicalJSONBytes(json.RawMessage(hostFundsAccountFlowInputSchemaV1)) &&
		canonicalJSONBytes(tool.OutputSchema) == canonicalJSONBytes(json.RawMessage(outputSchema))
}

const hostFundsInputSchemaV1 = `{
  "type":"object",
  "properties":{"table_name":{"type":"string","const":"analysis_txn_detail_idx"}},
  "required":["table_name"],
  "additionalProperties":false
}`

const hostFundsOutputSchemaV1 = `{
  "type":"object",
  "properties":{
    "schemaVersion":{"type":"integer","const":2},
    "purpose":{"type":"string","const":"analytix.funds-count-tool-outcome/v2"},
    "semanticStatus":{"type":"string","const":"success"},
    "data":{
      "type":"object",
      "properties":{
        "schemaVersion":{"type":"integer","const":2},
        "purpose":{"type":"string","const":"analytix.host.funds-count-projection/v2"},
        "turnSecurityContextDigest":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "datasetSnapshotId":{"type":"string","pattern":"^dsv2_[a-f0-9]{64}$"},
        "datasetSelectionDigest":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "datasetRecordDigest":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "datasetManifestDigest":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "fundsProducerContentId":{"type":"string","pattern":"^fpc[12]_[a-f0-9]{64}$"},
        "fundsProducerContentManifestSha256":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "detailContentSha256":{"type":"string","pattern":"^[a-f0-9]{64}$"},
        "tableName":{"type":"string","const":"analysis_txn_detail_idx"},
        "rowCount":{"type":"string","pattern":"^(0|[1-9][0-9]*)$","maxLength":20},
        "projectionDigest":{"type":"string","pattern":"^[a-f0-9]{64}$"}
      },
      "required":[
        "schemaVersion","purpose","turnSecurityContextDigest","datasetSnapshotId",
        "datasetSelectionDigest","datasetRecordDigest","datasetManifestDigest",
        "fundsProducerContentId","fundsProducerContentManifestSha256",
        "detailContentSha256","tableName","rowCount","projectionDigest"
      ],
      "additionalProperties":false
    }
  },
  "required":["schemaVersion","purpose","semanticStatus","data"],
  "additionalProperties":false
}`

const hostFundsAccountFlowInputSchemaV1 = `{
  "type":"object",
  "properties":{
    "subject_alias":{"type":"string","pattern":"^(acct|card):(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$","minLength":6,"maxLength":15},
    "start_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "end_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "evidence_row_limit":{"type":"integer","minimum":1,"maximum":512}
  },
  "required":["subject_alias","start_inclusive","end_inclusive","evidence_row_limit"],
  "additionalProperties":false
}`

const hostFundsAccountFlowOutputSchemaV1 = `{
  "type":"object",
  "properties":{
    "schemaVersion":{"type":"integer","const":1},
    "purpose":{"type":"string","const":"analytix.funds-account-flow-host-capture/v1"},
    "semanticStatus":{"type":"string","const":"host_authority_required"},
    "hostCaptureRequired":{"type":"boolean","const":true},
    "factAnswerAllowed":{"type":"boolean","const":false}
  },
  "required":[
    "schemaVersion","purpose","semanticStatus","hostCaptureRequired","factAnswerAllowed"
  ],
  "additionalProperties":false
}`

// hostFundsAccountFlowAnalysisOutputSchemaV1 is the provider-safe executable
// result. Opaque case/source-artifact-bound evidence references and closed
// case-scoped aliases are provider-visible. Currentness is the closed
// execution-time state established by the host; exact snapshot identity and
// host-private dataset, raw source locators, query and evidence authority
// bindings are carried out of band and are deliberately absent from this
// schema.
const hostFundsAccountFlowAnalysisOutputSchemaV1 = `{
  "type":"object",
  "properties":{
    "schemaVersion":{"type":"integer","const":3},
    "purpose":{"type":"string","const":"analytix.funds-account-flow-analysis/v3"},
    "semanticStatus":{"type":"string","enum":["success","partial"]},
    "data":{
      "type":"object",
      "properties":{
        "subjectAlias":{"type":"string","pattern":"^(acct|card):(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$","minLength":6,"maxLength":15},
        "startInclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
        "endInclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
        "timezone":{"type":"string","pattern":"^(Z|[+-](0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)$","minLength":1,"maxLength":6},
        "currency":{"type":"string","pattern":"^[A-Z]{3}$","minLength":3,"maxLength":3},
        "minorUnitScale":{"type":"integer","const":2},
        "inflowMinor":{"type":"string","pattern":"^(0|[1-9][0-9]*)$","maxLength":128},
        "outflowMinor":{"type":"string","pattern":"^(0|[1-9][0-9]*)$","maxLength":128},
        "netMinor":{"type":"string","pattern":"^(0|-?[1-9][0-9]*)$","maxLength":129},
        "transactionCount":{"type":"integer","minimum":0,"maximum":100000},
        "evidenceTransactionCount":{"type":"integer","minimum":0,"maximum":512},
        "evidenceRowLimit":{"type":"integer","minimum":1,"maximum":512},
        "aggregateComplete":{"type":"boolean"},
        "evidenceRowsComplete":{"type":"boolean"},
        "counterpartySemanticsComplete":{"type":"boolean"},
        "currentness":{"type":"string","const":"current"},
        "coverage":{
          "type":"object",
          "properties":{
            "state":{"type":"string","enum":["complete","partial","observed_no_hit_pending_host_binding"]},
            "gaps":{"type":"array","maxItems":5,"uniqueItems":true,"items":{"type":"string","enum":["rejected_source_rows","duplicate_source_rows","untimed_subject_rows","evidence_row_limit","counterparty_resolution"]}},
            "normalizedSnapshotRows":{"type":"integer","minimum":0,"maximum":100000},
            "acceptedSnapshotRows":{"type":"integer","minimum":0,"maximum":100000},
            "rejectedSnapshotRows":{"type":"integer","minimum":0,"maximum":100000},
            "duplicateSnapshotRows":{"type":"integer","minimum":0,"maximum":100000},
            "untimedSubjectRows":{"type":"integer","minimum":0,"maximum":100000},
            "observedMatchingRows":{"type":"integer","minimum":0,"maximum":100000}
          },
          "required":["state","gaps","normalizedSnapshotRows","acceptedSnapshotRows","rejectedSnapshotRows","duplicateSnapshotRows","untimedSubjectRows","observedMatchingRows"],
          "additionalProperties":false
        },
        "queryHash":{"type":"string","pattern":"^[a-f0-9]{64}$","minLength":64,"maxLength":64},
        "resultHash":{"type":"string","pattern":"^[a-f0-9]{64}$","minLength":64,"maxLength":64},
        "outcome":{
          "type":"object",
          "properties":{
            "aggregateCompleteness":{"type":"string","enum":["complete","incomplete"]},
            "evidenceRowsCompleteness":{"type":"string","enum":["complete","incomplete"]},
            "typedSlotEligibility":{"type":"string","const":"pending_final_gate"},
            "localDisplayAvailability":{"type":"string","const":"pending_final_gate"},
            "localDisplayCompletion":{"type":"string","const":"not_requested"},
            "factAnswerAllowed":{"type":"boolean","const":false},
            "queryScopeBindingEligibility":{"type":"string","const":"host_verified"},
            "currentnessEligibility":{"type":"string","const":"current"},
            "lineageEligibility":{"type":"string","const":"pending_evidence_receipt"},
            "sourceFieldBindingEligibility":{"type":"string","const":"pending_evidence_receipt"},
            "sourceFieldReference":{
              "type":"object",
              "properties":{
                "schemaVersion":{"type":"integer","const":1},
                "purpose":{"type":"string","const":"analytix.account-flow-typed-source-field-reference/v1"},
                "bindingRef":{"type":"string","pattern":"^afslot1_[a-f0-9]{64}$","minLength":72,"maxLength":72},
                "field":{"type":"string","enum":["account","card"]}
              },
              "required":["schemaVersion","purpose","bindingRef","field"],
              "additionalProperties":false
            }
          },
          "required":["aggregateCompleteness","evidenceRowsCompleteness","typedSlotEligibility","localDisplayAvailability","localDisplayCompletion","factAnswerAllowed","queryScopeBindingEligibility","currentnessEligibility","lineageEligibility","sourceFieldBindingEligibility","sourceFieldReference"],
          "additionalProperties":false
        },
        "transactions":{
          "type":"array",
          "maxItems":512,
          "items":{
            "type":"object",
            "properties":{
              "evidenceRef":{"type":"string","pattern":"^srow1_[a-f0-9]{64}$","minLength":70,"maxLength":70},
              "counterparty":{
                "type":"object",
                "properties":{
                  "status":{"type":"string","enum":["resolved","partial","unresolved"]},
                  "alias":{"type":"string","pattern":"^acct:(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$","minLength":6,"maxLength":15},
                  "entityType":{"type":"string","const":"bank_account_number"},
                  "accountType":{"type":"string","const":"交易对手账户"}
                },
                "required":["status"],
                "additionalProperties":false
              },
              "occurredAt":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
              "direction":{"type":"string","enum":["inflow","outflow"]},
              "amountMinor":{"type":"string","pattern":"^(0|[1-9][0-9]*)$","maxLength":128},
              "currency":{"type":"string","pattern":"^[A-Z]{3}$","minLength":3,"maxLength":3},
              "minorUnitScale":{"type":"integer","const":2}
            },
            "required":["evidenceRef","counterparty","occurredAt","direction","amountMinor","currency","minorUnitScale"],
            "additionalProperties":false
          }
        }
      },
      "required":["subjectAlias","startInclusive","endInclusive","timezone","currency","minorUnitScale","inflowMinor","outflowMinor","netMinor","transactionCount","evidenceTransactionCount","evidenceRowLimit","aggregateComplete","evidenceRowsComplete","counterpartySemanticsComplete","currentness","coverage","queryHash","resultHash","outcome","transactions"],
      "additionalProperties":false
    }
  },
  "required":["schemaVersion","purpose","semanticStatus","data"],
  "additionalProperties":false
}`
