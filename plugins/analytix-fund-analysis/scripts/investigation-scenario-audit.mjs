#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  resolveCaseDuckdbPath,
  resolveCaseProjectContext
} from "../mcp/case-project-context.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");

const PYTHON_SCRIPT = String.raw`
import datetime
import decimal
import json
import sys
import duckdb

def normalize(value):
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        try:
            return value.isoformat(sep=" ")
        except TypeError:
            return value.isoformat()
    if isinstance(value, decimal.Decimal):
        return float(value)
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, dict):
        return {str(key): normalize(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [normalize(item) for item in value]
    return value

payload = json.loads(sys.stdin.read())
case_db = payload["case_db"]
con = duckdb.connect(case_db, read_only=True)

SCENARIOS = [
    {
        "id": "duplicate_same_fact_card_replacement",
        "title": "duplicate data, same-fact transactions, and card-replacement repeats",
        "skills": ["data-quality", "case-workbench"],
        "tools": ["audit_case_data_quality", "resolve_duplicate_families", "profile_case_schema", "run_case_sql"],
        "required": {
            "fc_transaction_norm": [
                "row_hash", "card_no_norm", "acct_no_norm", "counterparty_acct_norm",
                "txn_ts", "amount_val", "dc_final", "clean_duplicate", "clean_suffix_fixed"
            ]
        },
        "support_metrics": ["transaction_rows"],
        "metrics_sql": {
            "transaction_rows": "SELECT COUNT(*) FROM fc_transaction_norm",
            "duplicate_flag_rows": "SELECT COUNT(*) FROM fc_transaction_norm WHERE COALESCE(CAST(clean_duplicate AS BIGINT), 0) <> 0",
            "suffix_fixed_rows": "SELECT COUNT(*) FROM fc_transaction_norm WHERE COALESCE(CAST(clean_suffix_fixed AS BIGINT), 0) <> 0",
            "same_fact_groups": """
                SELECT COUNT(*) FROM (
                  SELECT COALESCE(acct_no_norm, card_no_norm, '') AS account_key,
                         COALESCE(counterparty_acct_norm, '') AS counterparty_key,
                         amount_val, txn_ts, COALESCE(dc_final, '') AS direction, COUNT(*) AS row_count
                  FROM fc_transaction_norm
                  WHERE amount_val IS NOT NULL AND txn_ts IS NOT NULL
                  GROUP BY 1, 2, 3, 4, 5
                  HAVING COUNT(*) > 1
                )
            """
        }
    },
    {
        "id": "subject_account_overview_opening_balance",
        "title": "holder account overview, account range, opening information, status, and balances",
        "skills": ["subject-dossier", "account-dossier", "case-context"],
        "tools": ["resolve_owner_scope", "resolve_account_scope", "analyze_holder_full", "analyze_account_full", "get_scope_coverage"],
        "required": {
            "fc_account_norm": [
                "account_open_name", "opener_id_no", "card_no_norm", "acct_no_norm",
                "open_time_ts", "balance_val", "available_balance_val", "branch_name", "acct_status"
            ],
            "analysis_account_dim": ["account_key", "open_name", "id_no", "bank_name", "branch_name", "acct_type"]
        },
        "support_metrics": ["account_rows", "account_dim_rows"],
        "metrics_sql": {
            "account_rows": "SELECT COUNT(*) FROM fc_account_norm",
            "account_dim_rows": "SELECT COUNT(*) FROM analysis_account_dim",
            "named_holder_count": "SELECT COUNT(DISTINCT account_open_name) FROM fc_account_norm WHERE account_open_name IS NOT NULL AND account_open_name <> ''",
            "status_rows": "SELECT COUNT(*) FROM fc_account_norm WHERE acct_status IS NOT NULL AND acct_status <> ''",
            "balance_rows": "SELECT COUNT(*) FROM fc_account_norm WHERE balance_val IS NOT NULL OR available_balance_val IS NOT NULL"
        }
    },
    {
        "id": "single_account_transaction_profile",
        "title": "single account transaction profile, income-expense structure, counterparties, and abnormality leads",
        "skills": ["account-dossier", "counterparty-analysis", "case-workbench"],
        "tools": ["analyze_account_full", "rank_counterparties", "profile_case_schema", "run_case_sql"],
        "required": {
            "analysis_txn_detail_idx": [
                "acct_key", "cp_key", "counterparty_acct", "counterparty_name",
                "amount", "dc_val", "txn_ts", "summary", "remark", "txn_type"
            ],
            "analysis_txn_daily_agg": ["txn_day", "acct_key", "cp_key", "txn_count", "amt_sum", "first_ts", "last_ts"]
        },
        "support_metrics": ["txn_rows", "daily_agg_rows"],
        "metrics_sql": {
            "txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx",
            "daily_agg_rows": "SELECT COUNT(*) FROM analysis_txn_daily_agg",
            "account_count": "SELECT COUNT(DISTINCT acct_key) FROM analysis_txn_detail_idx WHERE acct_key IS NOT NULL AND acct_key <> ''",
            "counterparty_count": "SELECT COUNT(DISTINCT cp_key) FROM analysis_txn_detail_idx WHERE cp_key IS NOT NULL AND cp_key <> ''",
            "direction_value_count": "SELECT COUNT(DISTINCT dc_val) FROM analysis_txn_detail_idx WHERE dc_val IS NOT NULL AND dc_val <> ''"
        }
    },
    {
        "id": "text_remark_purpose_classification",
        "title": "transaction remarks, summaries, purpose, and transaction-type suspicious feature grouping",
        "skills": ["investigation-lab", "case-workbench", "data-quality"],
        "tools": ["hypothesis_probe", "run_investigation_lab", "profile_case_schema", "run_case_sql"],
        "required": {
            "analysis_txn_detail_idx": ["summary", "remark", "txn_type", "merchant_name", "query_feedback_reason"],
            "analysis_txn_keyword_idx": ["txn_row_id", "stable_txn_id", "kind", "token", "token_order"]
        },
        "support_metrics": ["keyword_rows", "text_field_rows"],
        "metrics_sql": {
            "keyword_rows": "SELECT COUNT(*) FROM analysis_txn_keyword_idx",
            "keyword_kind_count": "SELECT COUNT(DISTINCT kind) FROM analysis_txn_keyword_idx WHERE kind IS NOT NULL AND kind <> ''",
            "text_field_rows": """
                SELECT COUNT(*) FROM analysis_txn_detail_idx
                WHERE COALESCE(summary, '') <> ''
                   OR COALESCE(remark, '') <> ''
                   OR COALESCE(txn_type, '') <> ''
                   OR COALESCE(merchant_name, '') <> ''
            """,
            "query_feedback_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE query_feedback_reason IS NOT NULL AND query_feedback_reason <> ''"
        }
    },
    {
        "id": "large_cash_consumption_destination",
        "title": "large-value flows, cash in/out, consumption, and destination clues",
        "skills": ["quick-fact", "account-dossier", "fund-tracing", "investigation-lab"],
        "tools": ["rank_accounts", "trace_subject_top_outflows", "trace_fund", "run_case_sql"],
        "required": {
            "analysis_txn_detail_idx": [
                "amount", "cash_flag", "merchant_name", "summary", "remark", "txn_type",
                "counterparty_acct", "counterparty_name", "counterparty_bank", "dc_val"
            ]
        },
        "support_metrics": ["txn_rows", "large_txn_rows"],
        "metrics_sql": {
            "txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx",
            "large_txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE amount >= 50000",
            "very_large_txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE amount >= 1000000",
            "cash_flag_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE cash_flag IS NOT NULL AND cash_flag <> ''",
            "merchant_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE merchant_name IS NOT NULL AND merchant_name <> ''",
            "max_amount": "SELECT MAX(amount) FROM analysis_txn_detail_idx"
        }
    },
    {
        "id": "rapid_in_out_collection_distribution",
        "title": "rapid in-out, concentrated collection, distributed outflow, and aggregation-transfer patterns",
        "skills": ["fund-tracing", "investigation-lab", "full-case-analysis"],
        "tools": ["trace_fund_next_hop", "trace_fund", "run_investigation_lab", "plan_case_analysis"],
        "required": {
            "analysis_txn_detail_idx": [
                "acct_key", "cp_key", "dc_val", "txn_ts", "amount", "txn_id"
            ],
            "analysis_txn_daily_agg": [
                "txn_day", "acct_key", "cp_key", "dc_val", "txn_count", "amt_sum", "first_ts", "last_ts"
            ],
            "analysis_rule_hit": ["rule_code", "risk_type", "severity", "entity_ids_json", "txn_ids_json", "evidence_ids_json"]
        },
        "support_metrics": ["rapid_in_out_account_days", "collection_distribution_groups"],
        "metrics_sql": {
            "txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx",
            "daily_agg_rows": "SELECT COUNT(*) FROM analysis_txn_daily_agg",
            "rapid_in_out_account_days": """
                SELECT COUNT(*) FROM (
                  SELECT acct_key, txn_day,
                         SUM(CASE WHEN dc_val LIKE '%进%' OR dc_val LIKE '%入%' OR dc_val IN ('C', 'CR', '贷', '收入') THEN 1 ELSE 0 END) AS in_rows,
                         SUM(CASE WHEN dc_val LIKE '%出%' OR dc_val LIKE '%支%' OR dc_val IN ('D', 'DR', '借', '支出') THEN 1 ELSE 0 END) AS out_rows
                  FROM analysis_txn_daily_agg
                  WHERE acct_key IS NOT NULL AND acct_key <> '' AND txn_day IS NOT NULL
                  GROUP BY 1, 2
                  HAVING in_rows > 0 AND out_rows > 0
                )
            """,
            "collection_distribution_groups": """
                SELECT COUNT(*) FROM (
                  SELECT acct_key, txn_day, COALESCE(dc_val, '') AS direction,
                         COUNT(DISTINCT cp_key) AS counterparty_count,
                         SUM(ABS(amt_sum)) AS total_amount
                  FROM analysis_txn_daily_agg
                  WHERE acct_key IS NOT NULL AND acct_key <> '' AND txn_day IS NOT NULL
                  GROUP BY 1, 2, 3
                  HAVING counterparty_count >= 5 AND total_amount >= 50000
                )
            """,
            "rule_hit_rows": "SELECT COUNT(*) FROM analysis_rule_hit",
            "rule_code_count": "SELECT COUNT(DISTINCT rule_code) FROM analysis_rule_hit WHERE rule_code IS NOT NULL AND rule_code <> ''"
        }
    },
    {
        "id": "shared_ip_mac_device_contact_address",
        "title": "same IP, MAC, terminal, contact, address, and reserved-phone association leads",
        "skills": ["case-context", "data-quality", "investigation-lab"],
        "tools": ["get_scope_coverage", "audit_case_data_quality", "hypothesis_probe", "run_case_sql"],
        "required": {
            "analysis_txn_detail_idx": ["acct_key", "ip_addr", "mac_addr", "terminal_no"],
            "fc_person_contact_norm": ["open_name", "id_no", "contact_phone"],
            "fc_person_address_norm": ["open_name", "id_no", "home_addr", "home_phone"]
        },
        "support_metrics": ["device_rows", "contact_rows", "address_rows"],
        "metrics_sql": {
            "ip_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE ip_addr IS NOT NULL AND ip_addr <> ''",
            "mac_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE mac_addr IS NOT NULL AND mac_addr <> ''",
            "terminal_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE terminal_no IS NOT NULL AND terminal_no <> ''",
            "device_rows": """
                SELECT COUNT(*) FROM analysis_txn_detail_idx
                WHERE COALESCE(ip_addr, '') <> '' OR COALESCE(mac_addr, '') <> '' OR COALESCE(terminal_no, '') <> ''
            """,
            "shared_ip_groups": """
                SELECT COUNT(*) FROM (
                  SELECT ip_addr FROM analysis_txn_detail_idx
                  WHERE ip_addr IS NOT NULL AND ip_addr <> ''
                  GROUP BY ip_addr
                  HAVING COUNT(DISTINCT acct_key) > 1
                )
            """,
            "shared_mac_groups": """
                SELECT COUNT(*) FROM (
                  SELECT mac_addr FROM analysis_txn_detail_idx
                  WHERE mac_addr IS NOT NULL AND mac_addr <> ''
                  GROUP BY mac_addr
                  HAVING COUNT(DISTINCT acct_key) > 1
                )
            """,
            "contact_rows": "SELECT COUNT(*) FROM fc_person_contact_norm",
            "address_rows": "SELECT COUNT(*) FROM fc_person_address_norm"
        }
    },
    {
        "id": "opening_bank_subject_relationship",
        "title": "opening bank, opening address, reserved information, account holder, and subject relationship",
        "skills": ["case-context", "subject-dossier", "account-dossier"],
        "tools": ["get_case_scope_map", "resolve_owner_scope", "resolve_account_scope", "get_scope_coverage"],
        "required": {
            "fc_account_norm": ["open_bank", "branch_name", "branch_code", "open_time_ts", "account_open_name", "opener_id_no"],
            "fc_sub_account_norm": ["parent_acct", "sub_acct", "sub_type", "acct_status"],
            "fc_person_norm": ["customer_name", "id_no", "org_addr", "org_phone", "employer", "legal_rep"]
        },
        "support_metrics": ["account_open_rows", "person_rows"],
        "metrics_sql": {
            "account_open_rows": "SELECT COUNT(*) FROM fc_account_norm",
            "open_bank_rows": "SELECT COUNT(*) FROM fc_account_norm WHERE COALESCE(open_bank, '') <> '' OR COALESCE(branch_name, '') <> ''",
            "sub_account_rows": "SELECT COUNT(*) FROM fc_sub_account_norm",
            "person_rows": "SELECT COUNT(*) FROM fc_person_norm"
        }
    },
    {
        "id": "abnormal_time_holiday_high_frequency",
        "title": "abnormal time, weekend/night transactions, and short-window high-frequency activity",
        "skills": ["data-quality", "investigation-lab", "full-case-analysis"],
        "tools": ["audit_case_data_quality", "hypothesis_probe", "run_investigation_lab", "run_case_sql"],
        "required": {
            "analysis_txn_detail_idx": ["acct_key", "txn_ts", "txn_day", "amount", "dc_val"]
        },
        "support_metrics": ["txn_rows", "night_rows", "weekend_rows", "high_frequency_minute_groups"],
        "metrics_sql": {
            "txn_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE txn_ts IS NOT NULL",
            "night_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE txn_ts IS NOT NULL AND EXTRACT(hour FROM txn_ts) BETWEEN 0 AND 5",
            "weekend_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE txn_ts IS NOT NULL AND strftime(txn_ts, '%w') IN ('0', '6')",
            "high_frequency_minute_groups": """
                SELECT COUNT(*) FROM (
                  SELECT acct_key, date_trunc('minute', txn_ts) AS minute_bucket, COUNT(*) AS txn_count
                  FROM analysis_txn_detail_idx
                  WHERE txn_ts IS NOT NULL AND acct_key IS NOT NULL AND acct_key <> ''
                  GROUP BY 1, 2
                  HAVING COUNT(*) >= 5
                )
            """
        }
    },
    {
        "id": "asset_purchase_financing_cash_break",
        "title": "vehicle, property, investment, insurance, loan repayment, project, and cash-break destination leads",
        "skills": ["fund-tracing", "investigation-lab", "evidence-request", "claim-review"],
        "tools": ["classify_missing_counterparty_business", "trace_fund", "run_investigation_lab", "validate_report_claims"],
        "required": {
            "analysis_txn_detail_idx": ["summary", "remark", "txn_type", "merchant_name", "cash_flag", "counterparty_name"],
            "analysis_txn_keyword_idx": ["token", "kind", "stable_txn_id"]
        },
        "support_metrics": ["keyword_rows", "destination_keyword_hits", "cash_flag_rows"],
        "metrics_sql": {
            "keyword_rows": "SELECT COUNT(*) FROM analysis_txn_keyword_idx",
            "destination_keyword_hits": """
                SELECT COUNT(*) FROM analysis_txn_keyword_idx
                WHERE regexp_matches(COALESCE(token, ''), '车|房|基金|证券|保险|理财|贷款|还款|工程|项目|现金|消费|POS')
            """,
            "destination_text_hits": """
                SELECT COUNT(*) FROM analysis_txn_detail_idx
                WHERE regexp_matches(
                  COALESCE(summary, '') || COALESCE(remark, '') || COALESCE(txn_type, '') || COALESCE(merchant_name, ''),
                  '车|房|基金|证券|保险|理财|贷款|还款|工程|项目|现金|消费|POS'
                )
            """,
            "cash_flag_rows": "SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE cash_flag IS NOT NULL AND cash_flag <> ''"
        }
    },
    {
        "id": "account_role_key_node_classification",
        "title": "account feature classification, key account, transfer account, sink account, terminal account, and suspected-control leads",
        "skills": ["full-case-analysis", "investigation-lab", "account-dossier"],
        "tools": ["rank_accounts", "plan_case_analysis", "run_investigation_lab", "analyze_account_full"],
        "required": {
            "analysis_key_node_features": [
                "node_key", "display_name", "txn_count", "total_amount", "in_amount",
                "out_amount", "key_score", "reasons_json", "quality_label"
            ],
            "analysis_signal_features": ["signal_type", "title", "severity", "support_count", "summary"],
            "analysis_rule_hit": ["rule_code", "risk_type", "severity", "score", "summary"]
        },
        "support_metrics": ["key_node_rows", "signal_rows", "rule_hit_rows"],
        "metrics_sql": {
            "key_node_rows": "SELECT COUNT(*) FROM analysis_key_node_features",
            "signal_rows": "SELECT COUNT(*) FROM analysis_signal_features",
            "rule_hit_rows": "SELECT COUNT(*) FROM analysis_rule_hit",
            "ranked_node_rows": "SELECT COUNT(*) FROM analysis_key_node_features WHERE key_score IS NOT NULL"
        }
    },
    {
        "id": "fund_graph_path_evidence_artifact",
        "title": "fund-flow graph, fund path, evidence table, statistics table, and chart support",
        "skills": ["fund-tracing", "graph-visualization", "visual-evidence", "delivery-qc"],
        "tools": ["build_fund_flow_graph", "trace_fund", "get_evidence_pack", "validate_report_claims"],
        "required": {
            "analysis_relation_edge": ["edge_id", "src_entity_id", "dst_entity_id", "txn_count", "amount_sum", "first_seen_at", "last_seen_at"],
            "analysis_entity_node": ["entity_id", "entity_type", "entity_key", "display_name"],
            "analysis_trace_path": ["path_id", "trace_id", "hop_count", "evidence_ids_json", "detail_json"],
            "analysis_trace_path_hop": ["path_id", "hop_index", "txn_id", "src_entity_id", "dst_entity_id", "amount"],
            "analysis_evidence_ref": ["evidence_id", "ref_table", "ref_pk", "title", "payload_json"]
        },
        "support_metrics": ["edge_rows", "trace_path_rows", "evidence_ref_rows"],
        "metrics_sql": {
            "edge_rows": "SELECT COUNT(*) FROM analysis_relation_edge",
            "missing_endpoint_edges": """
                SELECT COUNT(*) FROM analysis_relation_edge edge
                LEFT JOIN analysis_entity_node src ON src.entity_id = edge.src_entity_id
                LEFT JOIN analysis_entity_node dst ON dst.entity_id = edge.dst_entity_id
                WHERE src.entity_id IS NULL OR dst.entity_id IS NULL
            """,
            "trace_path_rows": "SELECT COUNT(*) FROM analysis_trace_path",
            "trace_hop_rows": "SELECT COUNT(*) FROM analysis_trace_path_hop",
            "trace_hop_missing_txn_support": """
                SELECT COUNT(*) FROM analysis_trace_path_hop hop
                LEFT JOIN analysis_txn_detail_idx txn
                  ON CAST(txn.id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)
                  OR CAST(txn.txn_id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)
                WHERE hop.txn_id IS NOT NULL AND txn.id IS NULL
            """,
            "evidence_ref_rows": "SELECT COUNT(*) FROM analysis_evidence_ref"
        }
    }
]

def table_columns():
    rows = con.execute("""
        SELECT table_name, column_name
        FROM information_schema.columns
        WHERE table_schema='main'
          AND (lower(table_name) LIKE 'analysis_%' OR regexp_matches(lower(table_name), '^fc_[a-z0-9_]*_norm$'))
    """).fetchall()
    result = {}
    for table, column in rows:
        result.setdefault(table, set()).add(column)
    return result

def scalar(sql):
    row = con.execute(sql).fetchone()
    if not row:
        return None
    return normalize(row[0])

def analyze_scenario(defn, columns_by_table):
    missing_tables = []
    missing_columns = []
    for table, columns in defn["required"].items():
        if table not in columns_by_table:
            missing_tables.append(table)
            continue
        for column in columns:
            if column not in columns_by_table[table]:
                missing_columns.append({"table": table, "column": column})
    metrics = {}
    metric_errors = {}
    if not missing_tables and not missing_columns:
        for name, sql in defn["metrics_sql"].items():
            try:
                metrics[name] = scalar(sql)
            except Exception as exc:
                metric_errors[name] = str(exc)
    structural_ok = not missing_tables and not missing_columns and not metric_errors
    support_values = [metrics.get(name) for name in defn.get("support_metrics", [])]
    support_count = sum(1 for value in support_values if isinstance(value, (int, float)) and value > 0)
    status = "supported" if structural_ok and support_count else "data_gap" if structural_ok else "structural_gap"
    findings = []
    for table in missing_tables:
        findings.append({"severity": "p1", "message": f"missing required table {table}"})
    for item in missing_columns:
        findings.append({"severity": "p1", "message": f"missing required column {item['table']}.{item['column']}"})
    for name, message in metric_errors.items():
        findings.append({"severity": "p1", "message": f"metric {name} failed: {message}"})
    if status == "data_gap":
        findings.append({"severity": "case_data_gap", "message": "required structures exist but this case has no supporting rows for the scenario support metrics"})
    if defn["id"] == "fund_graph_path_evidence_artifact":
        missing_edges = metrics.get("missing_endpoint_edges")
        missing_hops = metrics.get("trace_hop_missing_txn_support")
        if isinstance(missing_edges, (int, float)) and missing_edges > 0:
            findings.append({"severity": "p1", "message": f"graph has {missing_edges} relation edges with missing endpoint nodes"})
        if isinstance(missing_hops, (int, float)) and missing_hops > 0:
            findings.append({"severity": "p1", "message": f"trace hops have {missing_hops} transaction ids not supported by analysis_txn_detail_idx.id/txn_id"})
    return {
        "id": defn["id"],
        "title": defn["title"],
        "status": status,
        "structural_ok": structural_ok,
        "skills": defn["skills"],
        "tools": defn["tools"],
        "required_tables": list(defn["required"].keys()),
        "required_column_count": sum(len(columns) for columns in defn["required"].values()),
        "missing_tables": missing_tables,
        "missing_columns": missing_columns,
        "metrics": metrics,
        "metric_errors": metric_errors,
        "findings": findings
    }

columns_by_table = table_columns()
scenarios = [analyze_scenario(defn, columns_by_table) for defn in SCENARIOS]
con.close()
print(json.dumps({"scenarios": scenarios}, ensure_ascii=False))
`;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function defaultOutputDir() {
  return path.join(REPO_ROOT, "output", "analytix-fund-analysis", "scenario-coverage", timestamp());
}

function defaultCasesRoot(env = process.env) {
  const explicit = text(env.ANALYTIX_DATA_ANALYSIS_CASES_ROOT || env.ANALYTIX_CASES_ROOT);
  if (explicit) return explicit;
  if (process.platform === "darwin") {
    return path.join(os.homedir(), "Library", "Application Support", "analytix", "data-analysis", "cases");
  }
  const appData = text(env.APPDATA || env.LOCALAPPDATA);
  if (appData) return path.join(appData, "analytix", "data-analysis", "cases");
  return path.join(os.homedir(), ".analytix", "data-analysis", "cases");
}

function defaultCaseProjectRoot(env = process.env) {
  const explicit = text(env.ANALYTIX_CASE_PROJECT_ROOT || env.ANALYTIX_WORKSPACE_ROOT);
  if (explicit) return explicit;
  const localOracleProject = path.join(REPO_ROOT, "output", "analytix-fund-analysis", "oracle-case-project");
  if (fs.existsSync(path.join(localOracleProject, ".analytix", "case-project.json"))) {
    return localOracleProject;
  }
  return "";
}

function defaultCaseDb(env = process.env) {
  const explicit = text(env.ANALYTIX_FUNDS_ORACLE_CASE_DB || env.ANALYTIX_FUNDS_CASE_DB || env.ANALYTIX_CASE_DB);
  if (explicit) return explicit;
  const caseProjectRoot = defaultCaseProjectRoot(env);
  if (caseProjectRoot) {
    const context = resolveCaseProjectContext({ _analytix: { workspaceRealPath: caseProjectRoot } }, env);
    if (context?.case_id) return resolveCaseDuckdbPath(context.case_id, env);
  }
  const caseId = text(env.ANALYTIX_FUNDS_ORACLE_CASE_ID || env.ANALYTIX_FUNDS_ACTIVE_CASE_ID || env.ANALYTIX_ACTIVE_CASE_ID);
  if (!caseId) return "";
  const casesRoot = text(env.ANALYTIX_FUNDS_CASES_ROOT) || defaultCasesRoot(env);
  return path.join(casesRoot, caseId, "case.duckdb");
}

function parseArgs(argv) {
  const options = {
    json: false,
    caseDb: defaultCaseDb(),
    outputDir: "",
    failOnStructuralGaps: false,
    requireCaseDb: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--json") options.json = true;
    else if (arg === "--case-db") options.caseDb = next();
    else if (arg.startsWith("--case-db=")) options.caseDb = arg.slice("--case-db=".length);
    else if (arg === "--output-dir") options.outputDir = next();
    else if (arg.startsWith("--output-dir=")) options.outputDir = arg.slice("--output-dir=".length);
    else if (arg === "--fail-on-structural-gaps") options.failOnStructuralGaps = true;
    else if (arg === "--require-case-db") options.requireCaseDb = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node plugins/analytix-fund-analysis/scripts/investigation-scenario-audit.mjs [options]

Audits whether a real read-only DuckDB case has cleaned/analysis-table support
for common public-security economic-investigation fund-analysis scenarios.
It does not launch Analytix and does not call or score an LLM.

Options:
  --case-db <path>               Real case DuckDB path.
  --output-dir <dir>             Write JSON/Markdown evidence to this directory.
  --fail-on-structural-gaps      Exit non-zero when any required table/column/query is missing.
  --require-case-db              Treat a missing case DB as failure instead of skipped.
  --json                         Print JSON to stdout.
`);
}

function pythonCandidates(env = process.env) {
  return [
    env.ANALYTIX_FUNDS_DUCKDB_PYTHON,
    env.ANALYTIX_BACKEND_PYTHON,
    env.PYTHON,
    path.join(REPO_ROOT, ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python"),
    "python3",
    "python"
  ].map(text).filter(Boolean);
}

function runPython(caseDb) {
  const input = JSON.stringify({ case_db: caseDb });
  const errors = [];
  for (const python of pythonCandidates()) {
    if (python.includes(path.sep) && !fs.existsSync(python)) continue;
    const result = spawnSync(python, ["-c", PYTHON_SCRIPT], {
      input,
      encoding: "utf8",
      timeout: 60_000,
      maxBuffer: 20 * 1024 * 1024
    });
    if (result.status === 0) return JSON.parse(result.stdout || "{}");
    errors.push(text(result.stderr || result.stdout || `${python} exited ${result.status}`));
  }
  throw new Error(`DuckDB scenario audit failed: ${errors.slice(-2).join(" | ")}`);
}

function countStatuses(scenarios) {
  return scenarios.reduce((acc, scenario) => {
    const status = text(scenario.status) || "unknown";
    acc[status] = (acc[status] || 0) + 1;
    return acc;
  }, {});
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Analytix Funds Investigation Scenario Coverage Audit");
  lines.push("");
  lines.push(`Generated at: ${report.generated_at}`);
  lines.push("");
  lines.push("This audit checks real read-only DuckDB cleaned/analysis tables for scenario support. It does not launch Analytix, call an LLM, score model answers, or write case facts into plugin fixtures.");
  lines.push("");
  lines.push("## Summary");
  lines.push("");
  lines.push(`- Case DB: ${report.case_db}`);
  lines.push(`- Scenario count: ${report.scenario_count}`);
  lines.push(`- Status counts: ${JSON.stringify(report.status_counts)}`);
  lines.push(`- Structural gaps: ${report.structural_gap_count}`);
  lines.push(`- Case-data gaps: ${report.data_gap_count}`);
  lines.push("");
  lines.push("## Scenario Coverage");
  lines.push("");
  lines.push("| Scenario | Status | Skills | Tools | Key Metrics | Findings |");
  lines.push("| --- | --- | --- | --- | --- | --- |");
  for (const scenario of report.scenarios) {
    const metrics = Object.entries(objectOf(scenario.metrics))
      .slice(0, 8)
      .map(([key, value]) => `${key}=${value}`)
      .join("; ");
    const findings = arrayOf(scenario.findings).map((item) => `${item.severity}: ${item.message}`).join("; ") || "none";
    lines.push(`| ${scenario.title} | ${scenario.status} | ${arrayOf(scenario.skills).join(", ")} | ${arrayOf(scenario.tools).join(", ")} | ${metrics} | ${findings} |`);
  }
  lines.push("");
  lines.push("## Boundary");
  lines.push("");
  lines.push("- Use this as a plugin QA artifact: it proves whether Skills/MCP can inspect the data scene and route to evidence-backed analysis.");
  lines.push("- It is not a user-facing investigation report and does not replace MCP/DuckDB source-backed fact verification for a specific question.");
  lines.push("- Data gaps are case-data boundaries, not permission for the model to invent facts.");
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function writeOutput(report, outputDir) {
  const dir = outputDir ? path.resolve(REPO_ROOT, outputDir) : defaultOutputDir();
  fs.mkdirSync(dir, { recursive: true });
  const jsonPath = path.join(dir, "investigation-scenario-audit.json");
  const markdownPath = path.join(dir, "investigation-scenario-audit.md");
  fs.writeFileSync(jsonPath, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(markdownPath, renderMarkdown(report));
  return { output_dir: dir, json_path: jsonPath, markdown_path: markdownPath };
}

function skippedPayload(reason) {
  return {
    status: "skipped",
    audit: "analytix-investigation-scenario-coverage",
    generated_at: new Date().toISOString(),
    reason,
    note: "Provide --case-db or ANALYTIX_FUNDS_ORACLE_CASE_DB to run the read-only DuckDB scenario audit."
  };
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (!text(options.caseDb)) {
    const payload = skippedPayload("case DB not provided");
    if (options.json) console.log(JSON.stringify(payload, null, 2));
    else console.log(`Skipped: ${payload.reason}`);
    process.exit(options.requireCaseDb ? 1 : 0);
  }
  const caseDb = path.resolve(options.caseDb);
  if (!fs.existsSync(caseDb)) {
    const payload = skippedPayload(`case DB not found: ${caseDb}`);
    if (options.json) console.log(JSON.stringify(payload, null, 2));
    else console.log(`Skipped: ${payload.reason}`);
    process.exit(options.requireCaseDb ? 1 : 0);
  }

  const raw = runPython(caseDb);
  const scenarios = arrayOf(raw.scenarios);
  const statusCounts = countStatuses(scenarios);
  const structuralGapCount = scenarios.filter((scenario) => text(scenario.status) === "structural_gap").length;
  const dataGapCount = scenarios.filter((scenario) => text(scenario.status) === "data_gap").length;
  const report = {
    status: structuralGapCount ? "failed" : "ok",
    audit: "analytix-investigation-scenario-coverage",
    generated_at: new Date().toISOString(),
    case_db: caseDb,
    scenario_count: scenarios.length,
    status_counts: statusCounts,
    supported_count: Number(statusCounts.supported || 0),
    structural_gap_count: structuralGapCount,
    data_gap_count: dataGapCount,
    scenarios
  };
  report.output = writeOutput(report, options.outputDir);
  if (options.json) console.log(JSON.stringify(report, null, 2));
  else {
    console.log(`Scenario audit: ${report.status}`);
    console.log(`Supported: ${report.supported_count}/${report.scenario_count}`);
    console.log(`Structural gaps: ${report.structural_gap_count}`);
    console.log(`Output: ${report.output.markdown_path}`);
  }
  if (options.failOnStructuralGaps && structuralGapCount) process.exit(1);
}

main();
