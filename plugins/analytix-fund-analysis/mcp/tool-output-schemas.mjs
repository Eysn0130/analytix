import {
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE
} from "./source-unavailable-boundary.mjs";

export const TOOL_OUTPUT_SCHEMAS_VERSION = "0.16.16";
export const QUARANTINED_FUNDS_OUTPUT_CONTRACT = "funds-source-quarantine-v1";

// This is the exact structuredContent currently returned by the production
// handler. It is deliberately not a schema for the retired analytical payloads.
export const QUARANTINED_FUNDS_TOOL_OUTCOME_SCHEMA = {
  type: "object",
  properties: {
    transportStatus: { type: "string", const: "success" },
    semanticStatus: { type: "string", const: "unavailable" },
    reportedSemanticStatus: { type: "string", const: "unavailable" },
    safeToAnswer: { type: "boolean", const: false },
    isError: { type: "boolean", const: true },
    blocker: { type: "string", const: FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE },
    partialCoverage: {
      type: "object",
      properties: {},
      additionalProperties: false
    },
    data: {
      type: "object",
      properties: {},
      additionalProperties: false
    },
    candidateEvidenceReceipts: {
      type: "array",
      maxItems: 0,
      items: {
        type: "object",
        properties: {},
        additionalProperties: false
      }
    }
  },
  required: [
    "transportStatus",
    "semanticStatus",
    "reportedSemanticStatus",
    "safeToAnswer",
    "isError",
    "blocker",
    "partialCoverage",
    "data",
    "candidateEvidenceReceipts"
  ],
  additionalProperties: false
};

const quarantinedToolNames = [
  "investigate_pair_amount",
  "funds_investigate",
  "get_casegraph",
  "build_fund_flow_graph",
  "get_current_case",
  "get_case_status",
  "get_import_overview",
  "get_cleaning_overview",
  "get_case_data_pipeline_overview",
  "get_case_scope_map",
  "get_stats_meta",
  "get_stats_tree",
  "query_stats_rows",
  "query_stats_txn_rows",
  "query_account_txn_rows",
  "get_analysis_dashboard",
  "query_txn_slice",
  "export_cleaned_case_data",
  "run_case_sql",
  "explain_case_sql",
  "diagnose_case_sql",
  "count_case_rows",
  "profile_case_schema",
  "preview_case_rows",
  "inspect_workbench_history",
  "case_sql_recipes",
  "create_case_notebook",
  "get_account_stats",
  "inspect_case_schema",
  "audit_unindexed_sources",
  "rank_accounts",
  "rank_holders",
  "rank_counterparties",
  "resolve_owner_scope",
  "compare_analysis_scopes",
  "hypothesis_probe",
  "run_discovery_scan",
  "trace_holder_destinations",
  "detect_cash_breakpoints",
  "detect_financial_product_flows",
  "detect_project_litigation_asset_leads",
  "generate_followup_investigation_list",
  "trace_subject_top_outflows",
  "classify_missing_counterparty_business",
  "validate_continuation_list",
  "get_rule_hits",
  "trace_fund_next_hop",
  "trace_fund",
  "get_case_reconciliation",
  "audit_case_data_quality",
  "resolve_duplicate_families",
  "get_scope_coverage",
  "resolve_account_scope",
  "resolve_holder_scope",
  "analyze_account_full",
  "analyze_holder_full",
  "validate_report_claims",
  "get_evidence_pack",
  "scan_case_risks",
  "plan_case_analysis",
  "run_investigation_lab",
  "run_full_case_analysis"
];

// The pre-quarantine analytical handlers return many unrelated payload
// families and are not connected to the production MCP entry. They must stay
// unadvertised until each family receives its own host-authorized contract.
export const DORMANT_ANALYTICAL_OUTPUT_UNCLOSED_TOOL_NAMES = Object.freeze([
  ...quarantinedToolNames
]);
export const DORMANT_ANALYTICAL_OUTPUT_SCHEMA_COUNT = 0;

export const TOOL_OUTPUT_CONTRACT_BY_NAME = Object.freeze(Object.fromEntries(
  quarantinedToolNames.map((name) => [name, QUARANTINED_FUNDS_OUTPUT_CONTRACT])
));

export function outputSchemaForTool(name) {
  if (TOOL_OUTPUT_CONTRACT_BY_NAME[name] !== QUARANTINED_FUNDS_OUTPUT_CONTRACT) {
    throw new Error(`No quarantined output contract is registered for tool ${name || "(missing)"}`);
  }
  return QUARANTINED_FUNDS_TOOL_OUTCOME_SCHEMA;
}
