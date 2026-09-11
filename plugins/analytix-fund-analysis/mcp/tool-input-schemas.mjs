export const TOOL_INPUT_SCHEMAS_VERSION = "0.16.16";

const cursorFlagSchema = {
  oneOf: [
    { type: "boolean" },
    { type: "integer", enum: [0, 1] }
  ]
};

export const transactionCursorSchema = {
  oneOf: [
    {
      type: "object",
      properties: {
        id: { type: "integer" },
        abs: { type: "number", minimum: 0 }
      },
      required: ["id", "abs"],
      additionalProperties: false
    },
    {
      type: "object",
      properties: {
        id: { type: "integer" },
        absNull: { type: "boolean", const: true }
      },
      required: ["id", "absNull"],
      additionalProperties: false
    },
    {
      type: "object",
      properties: {
        id: { type: "integer" },
        ts: { type: "string", minLength: 1 },
        tsNull: cursorFlagSchema
      },
      required: ["id", "ts"],
      additionalProperties: false
    },
    {
      type: "object",
      properties: {
        id: { type: "integer" },
        ts: { type: "string", maxLength: 0 },
        tsNull: {
          oneOf: [
            { type: "boolean", const: true },
            { type: "integer", const: 1 }
          ]
        }
      },
      required: ["id", "tsNull"],
      additionalProperties: false
    }
  ]
};

export const chartFilterPayloadSchema = {
  type: "object",
  properties: {
    weekday: { type: "integer", minimum: 0, maximum: 6 },
    hour: { type: "integer", minimum: 0, maximum: 23 },
    granularity: { type: "string", enum: ["day", "week", "month", "hour"] },
    bucket: { type: "string" },
    range_start: { type: "string" },
    range_end: { type: "string" },
    side: { type: "string", enum: ["in", "out"] }
  },
  additionalProperties: false
};

export const chartFilterTokenSchema = {
  type: "object",
  properties: {
    source_panel_id: { type: "string" },
    dimension: { type: "string", minLength: 1 },
    value: { type: "string" },
    label: { type: "string" },
    payload: chartFilterPayloadSchema
  },
  required: ["dimension"],
  additionalProperties: false
};

export const chartPanelViewSchema = {
  type: "object",
  properties: {
    panel_id: { type: "string", minLength: 1 },
    view: { type: "string" },
    metric_basis: { type: "string" },
    dimension: { type: "string" }
  },
  required: ["panel_id"],
  additionalProperties: false
};

export const transactionSliceFilterSchema = {
  type: "object",
  properties: {
    entity_id: { type: "string" },
    account_key: { type: "string" },
    person_id: { type: "string" },
    direction: { type: "string", enum: ["in", "out"] },
    counterparty_acct: { type: "string" },
    counterparty_name: { type: "string" },
    ip_addr: { type: "string" },
    mac_addr: { type: "string" },
    branch_name: { type: "string" },
    voucher_no: { type: "string" },
    teller_no: { type: "string" },
    log_no: { type: "string" },
    keyword: { type: "string" },
    txn_ids: { type: "array", items: { type: "string" } },
    file_ids: { type: "array", items: { type: "string" } },
    time_range: {
      type: "object",
      properties: {
        start: { type: "string" },
        end: { type: "string" }
      },
      additionalProperties: false
    },
    start: { type: "string" },
    end: { type: "string" },
    min_amount: { type: "number" },
    max_amount: { type: "number" }
  },
  additionalProperties: false
};

export const transactionSliceSortSchema = {
  type: "object",
  properties: {
    field: { type: "string", enum: ["txn_time", "amount"] },
    order: { type: "string", enum: ["asc", "desc"] }
  },
  additionalProperties: false
};

export const continuationRowSchema = {
  type: "object",
  properties: {
    txn_id: { type: "string" },
    transaction_id: { type: "string" },
    "交易ID": { type: "string" },
    "流水号": { type: "string" },
    account_key: { type: "string" },
    acct_key: { type: "string" },
    account: { type: "string" },
    "卡号": { type: "string" },
    "账户": { type: "string" },
    txn_time: { type: "string" },
    txn_ts: { type: "string" },
    "交易时间": { type: "string" },
    amount_yuan: { type: "number" },
    amount: { type: "number" },
    amount_val: { type: "number" },
    "金额": { type: "number" },
    direction: { type: "string" },
    dc_val: { type: "string" },
    "收付标志": { type: "string" },
    counterparty_name: { type: "string" },
    counterparty_acct: { type: "string" },
    summary: { type: "string" },
    remark: { type: "string" },
    file_id: { type: "string" }
  },
  additionalProperties: false
};

export const analysisPlanBudgetSchema = {
  type: "object",
  properties: {
    max_tool_calls_per_lane: { type: "integer", minimum: 0 },
    max_evidence_rows_per_lane: { type: "integer", minimum: 0 },
    max_prompt_chars_per_lane: { type: "integer", minimum: 0 },
    max_total_tool_calls: { type: "integer", minimum: 0 },
    max_report_claims: { type: "integer", minimum: 0 },
    focus: { type: "array", items: { type: "string" } },
    focus_keywords: { type: "array", items: { type: "string" } }
  },
  additionalProperties: false
};

export const investigationPlanContextSchema = {
  type: "object",
  properties: {
    version: { type: "string" },
    plan_id: { type: "string" },
    case_id: { type: "string" },
    analysis_goal: { type: "string" },
    status: { type: "string", enum: ["pending", "ready", "blocked", "partial"] },
    lane_ids: { type: "array", items: { type: "string" } },
    selected_tools: { type: "array", items: { type: "string" } },
    context_digest: { type: "string" },
    dataset_snapshot_id: { type: "string" },
    warnings: { type: "array", items: { type: "string" } }
  },
  additionalProperties: false
};

export const ownerScopeInputProperties = {
  case_id: { type: "string" },
  holder_name: { type: "string" },
  id_no: { type: "string" },
  account_keys: { type: "array", items: { type: "string" } },
  match_mode: { type: "string", enum: ["exact", "contains"], description: "Compatibility hint accepted from Agent prompts; deterministic owner resolution stays exact unless the backend supports broader matching." },
  include_candidate_accounts: { type: "boolean" },
  candidate_min_turnover: { type: "number", minimum: 0 },
  limit: { type: "integer", minimum: 1, maximum: 200 }
};

export const probeInputProperties = {
  case_id: { type: "string" },
  probe_type: {
    type: "string",
    enum: [
      "discovery",
      "run_discovery_scan",
      "destination_flow",
      "trace_holder_destinations",
      "holder_to_counterparty",
      "cash_breakpoints",
      "cash",
      "financial_product_flows",
      "financial",
      "project_litigation_asset",
      "topic_leads",
      "keyword_leads",
      "report_claim_review",
      "investigative_patterns",
      "pattern_lab",
      "irregularity",
      "old_thread_regression"
    ]
  },
  hypothesis: { type: "string" },
  holder_name: { type: "string" },
  id_no: { type: "string" },
  account_keys: { type: "array", items: { type: "string" } },
  via_holder_name: { type: "string", description: "Optional intermediate holder/person; converted to keywords for backend probes." },
  counterparty_name: { type: "string", description: "Optional counterparty/person; converted to keywords for backend probes." },
  include_candidate_accounts: { type: "boolean" },
  candidate_min_turnover: { type: "number", minimum: 0 },
  date_start: { type: "string" },
  date_end: { type: "string" },
  keywords: { type: "array", items: { type: "string" } },
  min_amount: { type: "number", minimum: 0 },
  limit: { type: "integer", minimum: 1, maximum: 200 }
};

export const subjectTopOutflowInputProperties = {
  case_id: { type: "string" },
  holder_name: { type: "string" },
  id_no: { type: "string" },
  account_keys: { type: "array", items: { type: "string" } },
  include_candidate_accounts: { type: "boolean" },
  candidate_min_turnover: { type: "number", minimum: 0 },
  date_start: {
    type: "string",
    description: "Optional YYYY-MM-DD lower bound. For Pair Amount receiver continuation, use the focused inflow date."
  },
  date_end: {
    type: "string",
    description: "Optional YYYY-MM-DD upper bound. For high-share Pair Amount receiver continuation, cover at least 30 calendar days after date_start or omit date_end; do not set only the next day unless the user asks."
  },
  top_n: { type: "integer", minimum: 1, maximum: 100 },
  min_amount: { type: "number", minimum: 0 },
  dedupe_seed_txn_id: { type: "boolean" },
  trace_depth: { type: "integer", minimum: 1, maximum: 3 },
  time_window_hours: {
    type: "number",
    minimum: 1,
    maximum: 720,
    description: "For Pair Amount receiver continuation, omit this or use 720; do not use 24 by default because in-case product/redemption/outflow facts may occur days later."
  },
  amount_tolerance_ratio: { type: "number", minimum: 0, maximum: 1 },
  amount_tolerance_abs: { type: "number", minimum: 0 },
  downstream_limit_per_seed: { type: "integer", minimum: 0, maximum: 20 }
};

export const missingBusinessInputProperties = {
  case_id: { type: "string" },
  holder_name: { type: "string" },
  id_no: { type: "string" },
  account_keys: { type: "array", items: { type: "string" } },
  include_candidate_accounts: { type: "boolean" },
  candidate_min_turnover: { type: "number", minimum: 0 },
  date_start: { type: "string" },
  date_end: { type: "string" },
  missing_kind: { type: "string", enum: ["holder", "counterparty", "both"] },
  min_amount: { type: "number", minimum: 0 },
  limit: { type: "integer", minimum: 1, maximum: 200 }
};

export const continuationListInputProperties = {
  case_id: { type: "string" },
  rows: { type: "array", items: continuationRowSchema },
  required_fields: { type: "array", items: { type: "string" } },
  amount_unit: { type: "string", enum: ["yuan", "wan", "元", "万元"] },
  expected_total_amount: { type: "number", minimum: 0 },
  expected_txn_count: { type: "integer", minimum: 0 },
  amount_tolerance: { type: "number", minimum: 0 },
  strict_db_match: { type: "boolean" }
};

export const frontDoorInputProperties = {
  case_id: { type: "string" },
  question: { type: "string", description: "User's natural-language investigation request." },
  claim_texts: { type: "array", items: { type: "string" }, description: "可选：待复核的报告研判结论文本。" },
  report_text: { type: "string", description: "Optional report text to split into claim-review candidates." },
  intent: {
    type: "string",
    enum: [
      "auto",
      "holder_analysis",
      "account_analysis",
      "destination",
      "flow_graph",
      "casegraph",
      "claim_review",
      "full_case",
      "old_thread_patterns",
      "ranking",
      "risk_discovery",
      "qa_review"
    ]
  },
  holder_name: { type: "string" },
  id_no: { type: "string" },
  account_keys: { type: "array", items: { type: "string" } },
  via_holder_name: { type: "string", description: "Optional intermediate holder/person for one-hop continuation." },
  counterparty_name: { type: "string" },
  date_start: { type: "string" },
  date_end: { type: "string" },
  focus_keywords: { type: "array", items: { type: "string" } },
  top_n: { type: "integer", minimum: 1, maximum: 100 },
  max_cards: { type: "integer", minimum: 1, maximum: 12 },
  include_debug: { type: "boolean", description: "Developer-only debug hint. Production agent output stays compact unless ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD=true." }
};
