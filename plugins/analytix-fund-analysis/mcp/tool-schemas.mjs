import {
  analysisPlanBudgetSchema,
  chartFilterTokenSchema,
  chartPanelViewSchema,
  continuationListInputProperties,
  frontDoorInputProperties,
  investigationPlanContextSchema,
  missingBusinessInputProperties,
  ownerScopeInputProperties,
  probeInputProperties,
  subjectTopOutflowInputProperties,
  transactionCursorSchema,
  transactionSliceFilterSchema,
  transactionSliceSortSchema
} from "./tool-input-schemas.mjs";
import { INVESTIGATION_ANSWER_SCHEMA } from "./investigation-answer-contract.mjs";
import {
  outputSchemaForTool,
  TOOL_OUTPUT_CONTRACT_BY_NAME
} from "./tool-output-schemas.mjs";
import { assertToolCatalogSchemas } from "./tool-schema-validation.mjs";

export const TOOL_SCHEMAS_VERSION = "0.16.16";

const REPORT_CLAIM_SOURCE_SCHEMA = {
  type: "object",
  properties: {
    source_id: { type: "string" },
    source_type: { type: "string" },
    tool_name: { type: "string" },
    query_id: { type: "string" },
    query_ids: { type: "array", items: { type: "string" } },
    evidence_id: { type: "string" },
    evidence_ids: { type: "array", items: { type: "string" } },
    evidence_receipt_ids: { type: "array", items: { type: "string" } },
    source_hash: { type: "string" },
    dataset_snapshot_id: { type: "string" },
    as_of: { type: "string" }
  },
  additionalProperties: false
};

const REPORT_CLAIM_INPUT_SCHEMA = {
  type: "object",
  properties: {
    claim_id: { type: "string" },
    claimId: { type: "string" },
    id: { type: "string" },
    key: { type: "string" },
    text: { type: "string" },
    claim: { type: "string" },
    statement: { type: "string" },
    summary: { type: "string" },
    reason: { type: "string" },
    status: { type: "string" },
    support_status: { type: "string" },
    asserted_support_status: { type: "string" },
    correction: { type: "string" },
    finding: { type: "string" },
    category: { type: "string" },
    risk_marker: { type: "string" },
    fact_refs: { type: "array", items: { type: "string" } },
    missing_fact_refs: { type: "array", items: { type: "string" } },
    evidence_ids: { type: "array", items: { type: "string" } },
    evidence_receipt_ids: { type: "array", items: { type: "string" } },
    source_refs: { type: "array", items: REPORT_CLAIM_SOURCE_SCHEMA },
    source: REPORT_CLAIM_SOURCE_SCHEMA
  },
  additionalProperties: false
};

function visibleToolMeta(title, invoking, invoked, options = {}) {
  return {
    title,
    annotations: {
      title,
      readOnlyHint: options.readOnlyHint !== false,
      destructiveHint: false,
      idempotentHint: options.idempotentHint !== false,
      openWorldHint: false
    },
    _meta: {
      "openai/toolInvocation/invoking": invoking || title,
      "openai/toolInvocation/invoked": invoked || title,
      "analytix/displayName": title
    }
  };
}

const TOOL_VISIBLE_TITLES = {
  query_account_txn_rows: "账户交易明细核验",
  get_analysis_dashboard: "账户分析看板核验",
  query_txn_slice: "交易切片核验",
  export_cleaned_case_data: "清洗明细导出",
  run_case_sql: "专项资金核算",
  explain_case_sql: "查询计划核验",
  diagnose_case_sql: "查询问题诊断",
  count_case_rows: "记录数核验",
  profile_case_schema: "字段覆盖勘查",
  preview_case_rows: "样本形态核验",
  inspect_workbench_history: "核算历史摘要",
  case_sql_recipes: "专项模板目录",
  create_case_notebook: "研判附件生成",
  get_account_stats: "账户统计核验",
  inspect_case_schema: "数据表结构核验",
  audit_unindexed_sources: "未纳入数据核验",
  rank_accounts: "账户排行核验",
  rank_holders: "主体排行核验",
  rank_counterparties: "对手方排行核验",
  resolve_owner_scope: "主体账户范围核验",
  compare_analysis_scopes: "分析范围对比",
  hypothesis_probe: "专项线索核验",
  run_discovery_scan: "线索扫描",
  trace_holder_destinations: "主体去向核验",
  detect_cash_breakpoints: "现金断点核验",
  detect_financial_product_flows: "理财证券资金核验",
  detect_project_litigation_asset_leads: "项目资产线索核验",
  generate_followup_investigation_list: "补充取证清单核验",
  trace_subject_top_outflows: "主体出账去向核验",
  classify_missing_counterparty_business: "空对手业务核验",
  validate_continuation_list: "续查清单复核",
  get_rule_hits: "异常规则核验",
  trace_fund_next_hop: "资金下一跳核验",
  trace_fund: "资金穿透核验",
  get_case_reconciliation: "案件金额勾稽核验",
  audit_case_data_quality: "数据质量核验",
  resolve_duplicate_families: "重复交易复核",
  get_scope_coverage: "范围覆盖核验",
  resolve_account_scope: "账户范围核验",
  resolve_holder_scope: "户名范围核验",
  analyze_account_full: "账户研判画像",
  analyze_holder_full: "主体研判画像",
  validate_report_claims: "报告事实结论复核",
  get_evidence_pack: "证据包核验",
  scan_case_risks: "案件风险扫描",
  plan_case_analysis: "研判任务规划",
  run_investigation_lab: "专题研判实验室",
  run_full_case_analysis: "全案资金研判"
};

export const tools = [
  {
    name: "investigate_pair_amount",
    ...visibleToolMeta("特定双方资金往来核验", "正在核验双方资金往来", "已完成特定双方资金往来核验"),
    description: "当前案件特定双方资金往来核验一等事实入口（双方转账金额事实入口）。用于自然问句“A 转给 B 多少钱”、金额争议、重复/换卡/同事实风险。普通双方资金往来金额首答优先调用本入口，且必须先于资金研判 navigator、排行、自由查询、旧报告、旧图脚本、全量主体画像或本地文件检索。调用时先核验事实，不生成计划、承诺、能力公告或流程说明；如运行环境必须显示短提示，只能逐字写“正在核验当前案件事实。” 同一会话同一付款/收款事实只调用一次；不得为补账号、补逐笔字段、扩大 top_n 或改写问题再次调用，缺失字段写入核验意见和补证动作。本入口返回支撑事实而非最终成稿：未限定时间/账号/交易集合时以当前案件双方完整明细聚合作为控制事实；若一次选择内同一持有人姓名+证件号、方向、时间、金额、余额、对手和流水/摘要等交易事实完全一致且仅卡号/账号不同，则标为已确定重复并计一次；同时返回主要集中转账作为异常和续查线索、统计期间、付款/收款账户集合与账号、原明细金额/笔数、有效金额/笔数、已确定重复或未确认差异、逐笔交易候选、排行交叉差异和下一步补证/核查动作；最终可见答复由 focused owner 组织成公安经侦材料，并展开逐笔交易表。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional explicit analytix case id; when provided it must match the current case project binding." },
        question: { type: "string", description: "User's original two-party amount question." },
        payer_name: { type: "string", description: "付款主体/人员/户名，例如 A 转给 B 中的 A；首选字段。" },
        receiver_name: { type: "string", description: "收款主体/人员/户名，例如 A 转给 B 中的 B；首选字段。" },
        holder_name: { type: "string", description: "Alias of payer_name." },
        via_holder_name: { type: "string", description: "Alias of receiver_name." },
        counterparty_name: { type: "string", description: "Declared alias of receiver_name for front-door natural-language pair amount calls; normalized before execution." },
        payee_name: { type: "string", description: "Declared alias of receiver_name; normalized before execution." },
        receiver_holder_name: { type: "string", description: "Declared alias of receiver_name; normalized before execution." },
        from_name: { type: "string", description: "Declared alias of payer_name; normalized before execution." },
        source_name: { type: "string", description: "Declared alias of payer_name; normalized before execution." },
        to_name: { type: "string", description: "Declared alias of receiver_name; normalized before execution." },
        target_name: { type: "string", description: "Declared alias of receiver_name; normalized before execution." },
        date_start: { type: "string" },
        date_end: { type: "string" },
        start_date: { type: "string", description: "Declared alias of date_start; normalized before execution." },
        end_date: { type: "string", description: "Declared alias of date_end; normalized before execution." },
        account_keys: { type: "array", items: { type: "string" } },
        top_n: { type: "integer", minimum: 1, maximum: 100 }
      },
      required: ["question"],
      additionalProperties: false
    }
  },
  {
    name: "funds_investigate",
    ...visibleToolMeta("资金事实核验", "正在核验当前案件事实", "已完成资金事实核验"),
    description: "当前案件资金问题的自然语言事实核验入口，但定位只是自然语言 shortcut / navigator 和轻量核验支持来源。不要用于“A 转给 B 多少钱”、金额争议、重复/换卡/同事实风险的首个事实调用或最终成稿；这类问题必须使用特定双方资金往来核验一等事实入口并由 focused owner 成稿。仅在问题模糊、跨能力或需要快速落到案件事实时识别任务画像、建议语义事实工具、返回最小事实和缺口；不得成为普通问法的唯一首跳或最终成稿 owner。姓名/主体是否和案件资金有关联可返回严格命中/模糊命中/排除对象/不能确认。明确 Top/排行、画像、穿透、假设、质量、续查、证据图表或交付复核任务可直接选择当前已广告的对应语义事实工具或 focused owner。P0 隔离期不得调用或模拟隐藏的全案/报告入口，也不得用本入口补扫拼接全案材料。不得为了当前案件事实先搜索工作区文件、技能文件、本地目录、Shell 或历史会话；不要向普通用户展示内部编号或大块 JSON。",
    inputSchema: {
      type: "object",
      properties: frontDoorInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "get_casegraph",
    ...visibleToolMeta("案件资金流向图谱核验", "正在核验案件图谱", "已完成案件图谱核验"),
    description: "Return Analytix casegraph v1: compact case scope graph, quality gates, top entities, duplicate/data-quality boundaries, evidence-status labels, and next semantic tool choices. Use this before build_fund_flow_graph for broad current-case graph, relationship graph, casegraph, or visual graph QA requests so the answer is grounded in case context and quality boundaries. It provides context, not proof of unsupported candidate flows.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        include_rankings: { type: "boolean" },
        include_debug: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "build_fund_flow_graph",
    ...visibleToolMeta("资金流向图核验", "正在生成资金流向证据", "已完成资金流向图核验"),
    description: "Build a deterministic fund-flow graph fact pack for fund-flow diagrams and graph/table path questions. Use when the user explicitly asks for 资金流向图/资金穿透图/graph, or when a semantic trace result explicitly lacks a graph/table artifact the user requested. For broad current-case graph, relationship graph, casegraph, or visual graph QA requests, call get_casegraph first in the same turn, then use this tool for transactions already supported by case data. Ordinary one-layer downstream continuation can usually answer from trace_subject_top_outflows and should not call this tool unless a graph is requested. If the backend semantic trace service is unavailable, this runtime can still fall back to current-case DuckDB trace_subject_top_outflows for one-hop facts and graph boundaries; do not turn backend downtime into an empty graph. Ordinary graph/downstream answers must not use this as a trigger to inspect schema, run SQL, search local files, pull evidence packs, or verify every downstream counterparty with separate two-party amount calls; use the graph/downstream fact pack once, draw supported transaction links, and mark unproven continuity/product redemption/final beneficiary matters as 需复核/需补证. User-visible graph text must use economic-investigation wording such as 已有流水支持、需复核、需补证、线索、未调取对象、资金断点; incomplete or candidate endpoints are evidence boundaries, not final flows.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        holder_name: { type: "string", description: "Source holder/person named by the user or resolved from the current case-project context." },
        id_no: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        via_holder_name: { type: "string", description: "Optional intermediate receiver/person when the user asks for a one-hop continuation." },
        counterparty_name: { type: "string", description: "Alias for via_holder_name when the user asks '转给谁/又给谁'." },
        date_start: { type: "string" },
        date_end: { type: "string" },
        top_n: { type: "integer", minimum: 1, maximum: 100 },
        trace_depth: { type: "integer", minimum: 1, maximum: 2 },
        include_debug: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_current_case",
    ...visibleToolMeta("当前案件同步", "正在同步当前案件", "已完成当前案件同步"),
    description: "解析当前案件或指定案件编号，默认返回简洁案件卡。若当前案件事实来源不可用，返回案件来源缺口和恢复动作；不得搜索本机目录、*.duckdb、历史记录、样例或猜测案件编号。看板计数只作元信息，分析覆盖范围请使用案件范围图或覆盖口径核验。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional explicit Analytix case id." },
        include_imports: { type: "boolean", description: "Include compact import-file rows. Defaults to false to keep the first tool call lightweight." }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_case_status",
    ...visibleToolMeta("案件状态核验", "正在核验案件状态", "已完成案件状态核验"),
    description: "Read compact current-case metadata and available Analytix skill surfaces before analysis. Dashboard counters are metadata only; use get_case_scope_map/get_scope_coverage for analysis coverage.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional explicit Analytix case id." },
        include_imports: { type: "boolean", description: "Include compact import-file rows. Defaults to false." }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_import_overview",
    ...visibleToolMeta("导入数据核验", "正在核验导入数据", "已完成导入数据核验"),
    description: "Summarize analytix data-import files, file types, kinds, row counts, status, and recent import jobs for the current case project.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        view: { type: "string", enum: ["active", "recycle"], description: "Import file ledger view." },
        include_jobs: { type: "boolean" },
        page_size: { type: "integer", minimum: 1, maximum: 500 }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_cleaning_overview",
    ...visibleToolMeta("清洗数据核验", "正在核验清洗数据", "已完成清洗数据核验"),
    description: "Summarize analytix data-cleaning jobs, cleaned row counts, step impacts, and recent cleaning history for the current case project.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        include_logs: { type: "boolean" },
        page_size: { type: "integer", minimum: 1, maximum: 500 },
        log_limit: { type: "integer", minimum: 1, maximum: 1000 }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_case_data_pipeline_overview",
    ...visibleToolMeta("数据流程核验", "正在核验数据流程", "已完成数据流程核验"),
    description: "Inspect data import, cleaning, stats metadata, and the Analytix by-name analysis tree as a publication-blocked diagnostic overview; this tool cannot establish case facts or report readiness.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        include_tree: { type: "boolean" },
        tree_limit_groups: { type: "integer", minimum: 1, maximum: 500 },
        tree_limit_items: { type: "integer", minimum: 0, maximum: 500 }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_case_scope_map",
    ...visibleToolMeta("案件数据范围核验", "正在核验案件数据范围", "已完成案件数据范围核验"),
    description: "Build a CodeGraph-style low-context map of case data layers, scope semantics, quality risks, and required gates before a bounded investigation. This composes backend candidates and does not expose source detail rows, local paths, or SQL. It is not a substitute for the P0-hidden full-case/report entry and must not be swept with other tools to synthesize a whole-case conclusion.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        table_limit: { type: "integer", minimum: 1, maximum: 500 },
        column_limit: { type: "integer", minimum: 0, maximum: 200 },
        include_schema_columns: { type: "boolean" },
        include_tree_items: { type: "boolean" },
        tree_limit_groups: { type: "integer", minimum: 1, maximum: 500 },
        tree_limit_items: { type: "integer", minimum: 0, maximum: 200 },
        include_child_details: { type: "boolean" },
        child_timeout_ms: { type: "integer", minimum: 1000, maximum: 90000 }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_stats_meta",
    ...visibleToolMeta("统计范围核验", "正在核验统计范围", "已完成统计范围核验"),
    description: "Read Analytix stats metadata, including funds status and transaction date range.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_stats_tree",
    ...visibleToolMeta("分析索引核验", "正在核验分析索引", "已完成分析索引核验"),
    description: "Read the Analytix data-analysis left tree grouped by holder/name or card, matching the frontend stats tree.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        tab: { type: "string", enum: ["byName", "byCard"] },
        limit_groups: { type: "integer", minimum: 1, maximum: 1000 },
        limit_items: { type: "integer", minimum: 0, maximum: 1000 },
        include_items: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "query_stats_rows",
    ...visibleToolMeta("排行明细核验", "正在核验排行明细", "已完成排行明细核验"),
    description: "Query the Analytix stats table rows for counterparty rankings by inflow/outflow account or name.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        mode: { type: "string", enum: ["inAccount", "outAccount", "inName", "outName"] },
        selected: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        search_text: { type: "string" },
        row_sort_col: { type: "string" },
        row_sort_dir: { type: "string", enum: ["asc", "desc"] },
        row_offset: { type: "integer", minimum: 0 },
        row_limit: { type: "integer", minimum: 1, maximum: 5000 }
      },
      additionalProperties: false
    }
  },
  {
    name: "query_stats_txn_rows",
    ...visibleToolMeta("交易明细核验", "正在核验交易明细", "已完成交易明细核验"),
    description: "Query bounded transaction rows from the Analytix stats surface for a selected account/name scope.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        selected: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        key_type: { type: "string", enum: ["account", "name"] },
        key_value: { type: "string" },
        key_values: { type: "array", items: { type: "string" } },
        filter: { type: "string", enum: ["all", "in", "out"] },
        sort_col: { type: "string", enum: ["txn_time", "amount"] },
        sort_dir: { type: "string", enum: ["asc", "desc"] },
        limit: { type: "integer", minimum: 1, maximum: 5000 },
        cursor: transactionCursorSchema,
        fields: { type: "array", items: { type: "string" } }
      },
      additionalProperties: false
    }
  },
  {
    name: "query_account_txn_rows",
    description: "Query transaction rows for one account key with optional date and clock-time windows, matching Analytix flow graph account drilldown.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_key: { type: "string" },
        date_start: { type: "string" },
        date_end: { type: "string" },
        start_time: { type: "string", description: "Optional clock or timestamp lower bound, for example 22:00:00 or YYYY-MM-DD HH:mm:ss." },
        end_time: { type: "string", description: "Optional clock or timestamp upper bound, for example 06:00:00 or YYYY-MM-DD HH:mm:ss." },
        sort_dir: { type: "string", enum: ["asc", "desc"] },
        limit: { type: "integer", minimum: 1, maximum: 5000 },
        cursor: transactionCursorSchema
      },
      required: ["account_key"],
      additionalProperties: false
    }
  },
  {
    name: "get_analysis_dashboard",
    description: "Read Analytix chart-analysis dashboard data for selected accounts, including trend, counterparties, structure, heatmap, flow, and anomaly panels.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        selected: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        metric_mode: { type: "string", enum: ["amount", "count", "counterparty"] },
        direction_mode: { type: "string", enum: ["all", "in", "out", "net"] },
        granularity: { type: "string", enum: ["day", "week", "month", "hour"] },
        success_filter: { type: "string", enum: ["all", "success", "failed"] },
        cash_filter: { type: "string", enum: ["all", "cash", "non-cash"] },
        chart_filters: { type: "array", items: chartFilterTokenSchema },
        panel_views: { type: "array", items: chartPanelViewSchema }
      },
      additionalProperties: false
    }
  },
  {
    name: "query_txn_slice",
    description: "Query a bounded transaction slice through Analytix skill runtime.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        filters: transactionSliceFilterSchema,
        visible_columns: { type: "array", items: { type: "string" } },
        sort: transactionSliceSortSchema,
        cursor: { type: "string" },
        limit: { type: "integer", minimum: 1, maximum: 500 }
      },
      additionalProperties: false
    }
  },
  {
    name: "export_cleaned_case_data",
    ...visibleToolMeta("清洗明细导出", "正在创建清洗明细导出", "已完成清洗明细导出", { readOnlyHint: false }),
    description: "为用户明确要求的当前案件清洗明细或证据底表创建真实导出任务，并在交付前读取检查生成的 CSV/XLSX 文件。默认仅导出清洗后的交易明细；保留 Analytix 中文表头、字段顺序和行顺序。该工具会写入 Analytix 当前案件 exports 目录，必须由用户明确要求并设置 confirm_export=true；不得用于普通事实问答、原始数据导出、任意目录写入或把未完成任务宣称为已交付。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional explicit analytix case id; when provided it must match the current case project binding." },
        purpose: { type: "string", minLength: 1, description: "说明用户要求交付哪类清洗明细或证据底表。" },
        confirm_export: { type: "boolean", description: "创建新导出任务必须为 true；仅继续检查已有 job_id 时可省略。" },
        job_id: { type: "string", description: "已有清洗导出任务编号；用于多轮继续检查，不创建新任务。" },
        export_format: { type: "string", enum: ["xlsx", "csv"], description: "Default: xlsx." },
        tables: {
          type: "array",
          items: {
            type: "string",
            enum: [
              "fc_transaction",
              "fc_account",
              "fc_sub_account",
              "fc_person",
              "fc_person_contact",
              "fc_person_address",
              "fc_coercive_measure",
              "fc_task_fail",
              "fc_task_success"
            ]
          },
          description: "Default: fc_transaction. Review comments or investigative columns must not be mixed into these cleaned exports."
        },
        output_name: { type: "string", maxLength: 120 },
        wait_for_completion: { type: "boolean", description: "Default: true. false returns a pending delivery state." },
        timeout_ms: { type: "integer", minimum: 1000, maximum: 110000 }
      },
      required: ["purpose"],
      additionalProperties: false
    }
  },
  {
    name: "run_case_sql",
    description: "专项资金核算 SQL 执行器，用于明确的当前案件自定义范围缺口、语义事实不足、结果冲突或用户要求可复算 SQL/notebook 的金额/范围挑战。普通资金流向图、主体画像、下游追踪、全案研判或报告初稿已有语义事实可回答时，不得为了补图、找字段、丰富材料或重复取值调用本工具；双方资金往来金额题已返回原始明细金额、有效金额和重复/同事实风险金额时，后续只是改成三档展示或解释差异，不得调用本工具。只有用户明确要求自定义范围/可复算 SQL/notebook，或已说明语义事实冲突且需要受控升级时才使用。仅在完成当前案件范围、表结构和必要数据质量预检后使用。适用于既有语义核验能力无法覆盖的有界自定义范围；这不是对 DuckDB/SQL 的禁令，而是 Data Analytics 式 source + validation 边界。SQL 字段必须来自 inspect_case_schema 返回的 table.sql_name、column.sql_name 或 sql_identifier；中文 display_name 只用于展示，不能写进可执行 SQL。执行前必须先通过静态只读/清洗范围 guard 和 DuckDB EXPLAIN parser/binder 预检；预检通过后才执行用户 SQL，并读取返回的 sql_policy、evidence_card、source_hash、metric_scope、validation_state、专项资金核算预览和验证摘要。对目标指标执行一次完整聚合 SQL 后，不要为了重复取值反复执行 SQL。后端必须强制当前案件、只读 SQL、清洗表/分析索引范围、行数上限、用途说明、核验状态、核验意见和审计记录。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional explicit analytix case id; when provided it must match the current case project binding." },
        purpose: { type: "string", minLength: 1, description: "说明既有语义核验能力为何不足、本查询回答哪个侦查问题，以及范围/表结构/数据质量预检已完成或受阻原因。" },
        query_request: { type: "string", description: "Planning note only. This natural-language口径 is not executed; provide sql for an executed result." },
        sql: { type: "string", description: "Executable complete single read-only SELECT/WITH over cleaned fc_*_norm and approved analysis_* views only. No ellipsis/placeholders, unbounded SELECT *, DDL/DML, unapproved source tables, external files, or cross-case access." },
        parameters: { type: "object", description: "Reserved for a future parameter binder. Leave empty in the current runtime.", additionalProperties: false },
        date_start: { type: "string" },
        date_end: { type: "string" },
        row_limit: { type: "integer", minimum: 1, maximum: 500 },
        result_mode: { type: "string", enum: ["aggregate", "preview", "evidence_table", "chart_ready"] },
        allowed_view_policy: { type: "string", enum: ["cleaned_and_analysis_only"] },
        include_notebook_cell: { type: "boolean" },
        include_debug: { type: "boolean" }
      },
      required: ["purpose", "sql"],
      additionalProperties: false
    }
  },
  {
    name: "explain_case_sql",
    ...visibleToolMeta("查询计划核验", "正在核验查询计划", "已完成查询计划核验"),
    description: "数据库现场勘查支持工具。仅在明确自定义 SQL/notebook、金额口径挑战、语义事实冲突、字段/表不确定或性能风险需要 live verification 时使用。先返回 DuckDB EXPLAIN parser/binder 预检，再返回执行计划、涉及表、预计或实际行数、耗时、是否可能全表扫描和修正方向，不返回来源明细行，不生成最终中文研判。SQL 必须是当前案件清洗 fc_*_norm 或已批准 analysis_* 的单条 SELECT/WITH；带 LIMIT 的 SELECT * 只适合 preview 形态核验，报告级金额仍需显式列/聚合。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        purpose: { type: "string", minLength: 1 },
        sql: { type: "string", minLength: 1 },
        row_limit: { type: "integer", minimum: 1, maximum: 500 },
        include_analyze: { type: "boolean", description: "Runs EXPLAIN ANALYZE through read-only backend when true; default false." }
      },
      required: ["purpose", "sql"],
      additionalProperties: false
    }
  },
  {
    name: "diagnose_case_sql",
    ...visibleToolMeta("查询问题诊断", "正在诊断查询问题", "已完成查询问题诊断"),
    description: "数据库现场勘查支持工具。用于 SQL 失败、空结果、字段不存在、表不允许、性能过慢或金额口径冲突时，给出可行动修正方向。它会先走静态只读/清洗范围 guard 和 DuckDB parser/binder 预检；字段错误应返回 DuckDB candidate bindings 和 SQL-safe candidate columns。它不返回明细行，不替代语义事实工具，不得把诊断结果写成案件金额/笔数/资金链路事实。若诊断为字段问题，先回 inspect_case_schema/profile_case_schema；若诊断为性能风险，先 explain_case_sql 或收窄范围；若诊断为空结果，只能说明当前条件未命中。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        purpose: { type: "string", minLength: 1 },
        sql: { type: "string" },
        error_message: { type: "string" },
        diagnostic_mode: { type: "string", enum: ["validate", "empty_result", "performance", "error"] },
        row_limit: { type: "integer", minimum: 1, maximum: 500 }
      },
      required: ["purpose"],
      additionalProperties: false
    }
  },
  {
    name: "count_case_rows",
    ...visibleToolMeta("记录数核验", "正在核验记录数", "已完成记录数核验"),
    description: "当前案件安全记录数核验：用于快速核验批准清洗/分析表在可选过滤条件下的记录数。用户问“交易明细表/交易明细/明细表数据量”时，优先计数 `analysis_txn_detail_idx`；不要猜 `fc_transaction`，除非 schema 已明确存在该表。只返回 COUNT，不返回样本或明细，不替代金额、排名、资金链路或报告结论。table_name 必须是当前案件 `analysis_*` 或 `fc_*_norm` 表；where_sql 只能是简单过滤条件，不得包含 SELECT/WITH/FROM/JOIN、DDL/DML、外部文件、网络、secret 或多语句。普通用户答案只能写“清洗后的交易明细/交易明细表”等业务名称，不得写 DuckDB、MCP、query_id、case_id、Workbench 或 SQL 表名。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        table_name: { type: "string", minLength: 1 },
        where_sql: { type: "string", description: "Optional simple WHERE condition without the WHERE keyword. Omit for total row count." }
      },
      required: ["table_name"],
      additionalProperties: false
    }
  },
  {
    name: "profile_case_schema",
    ...visibleToolMeta("字段覆盖勘查", "正在勘查字段覆盖", "已完成字段覆盖勘查"),
    description: "数据库现场勘查支持工具。用于 SQL/notebook 升级前或口径冲突时，对当前案件清洗表和 analysis 索引返回字段语义标签、空值率、distinct 规模、金额/日期 min-max、方向/状态枚举、账户/对手方/设备等关键字段覆盖。不返回 raw rows，不返回账户/姓名/证件/地址/设备原值样本。profile 只能说明数据现场边界，不能单独控制报告级金额或资金链路 claim。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        tables: { type: "array", items: { type: "string" } },
        table_limit: { type: "integer", minimum: 1, maximum: 20 },
        column_limit: { type: "integer", minimum: 1, maximum: 80 },
        enum_limit: { type: "integer", minimum: 1, maximum: 20 }
      },
      additionalProperties: false
    }
  },
  {
    name: "preview_case_rows",
    ...visibleToolMeta("样本形态核验", "正在核验样本形态", "已完成样本形态核验"),
    description: "数据库现场勘查支持工具。仅在字段 profile/explain 仍不足以判断记录形态，且明确需要看少量当前案件样本时使用。必须提供 table_name、where_sql 过滤条件、row_limit<=20；后端做静态 SQL guardrail、DuckDB parser/binder 预检和隐私投影。返回样本必须标注“样本不可汇总”，不得从样本外推金额、笔数、排名、资金链路或法律判断。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        purpose: { type: "string", minLength: 1 },
        table_name: { type: "string", minLength: 1 },
        columns: { type: "array", items: { type: "string" } },
        where_sql: { type: "string", minLength: 1 },
        row_limit: { type: "integer", minimum: 1, maximum: 20 }
      },
      required: ["purpose", "table_name", "where_sql"],
      additionalProperties: false
    }
  },
  {
    name: "inspect_workbench_history",
    ...visibleToolMeta("核算历史摘要", "正在读取核算历史", "已完成核算历史摘要"),
    description: "读取当前案件最近专项资金核算、执行计划、诊断、字段 profile、样本 preview 和 recipe 调用摘要。只返回 query digest、purpose、base tables、duration、row_count、truncated、validation_state、error_class、evidence id，不返回 SQL 文本、raw rows、敏感路径或 debug payload。用于 source-of-truth selection、冲突排查和可回放定位，不是案件事实来源本身。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        limit: { type: "integer", minimum: 1, maximum: 50 }
      },
      additionalProperties: false
    }
  },
  {
    name: "case_sql_recipes",
    ...visibleToolMeta("专项模板目录", "正在读取专项模板", "已完成专项模板目录"),
    description: "返回经审核的当前案件专项资金核算模板目录，例如全案总览、一跳金额、户名名下账户概览、Top 对手方、下游去向、同事实/换卡复核、强制措施、理财/基金/证券、资产消费、IP/MAC/联系方式/住址关联。recipe 应包含 parameters、preferred_tables 和面向 `analysis_account_dim`、`analysis_txn_daily_agg`、`analysis_txn_detail_idx`、`fc_account_norm`、`fc_coercive_measure_norm` 等清洗/分析表的 SQL template。recipe 是参数化、可测试、可回放的起点，不是固定案件答案；参数渲染后的 SQL 仍必须经过 schema/profile/explain/run_case_sql 验证。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        category: { type: "string" },
        recipe_id: { type: "string" },
        limit: { type: "integer", minimum: 1, maximum: 100 }
      },
      additionalProperties: false
    }
  },
  {
    name: "create_case_notebook",
    ...visibleToolMeta("研判附件生成", "正在创建研判附件", "已完成研判附件生成", { readOnlyHint: false }),
    description: "专项资金核算 notebook/附件创建器，用于可回放的当前案件自定义分析。应在有界专项资金核算 SQL 已执行后使用，或为同一 SQL/口径创建可回放附件。需包含核验和核验意见说明；用户回答只保留读者需要的来源、核验和交付摘要，不原样复述 notebook/support 尾注或内部字段。单元格必须限定范围、查询、图表和复核，保持只读、清洗表/分析索引范围、限行且可审计。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "可选的明确案件编号；可使用当前案件时尽量省略。" },
        title: { type: "string" },
        analysis_goal: { type: "string", minLength: 1, description: "自定义分析目标、既有语义核验能力为何不足，以及范围/表结构/数据质量预检状态。" },
        cells: {
          type: "array",
          items: {
            type: "object",
            properties: {
              type: { type: "string", enum: ["scope", "query", "chart", "markdown", "qa"] },
              purpose: { type: "string" },
              sql: { type: "string", description: "Executable complete read-only SQL cell over cleaned fc_*_norm and approved analysis_* views only; no ellipsis/placeholders." },
              query_request: { type: "string", description: "Planning note only; the cell does not execute unless sql is present." },
              fields: { type: "array", items: { type: "string" } },
              notes: { type: "string" }
            },
            additionalProperties: false
          }
        },
        max_rows_per_query: { type: "integer", minimum: 1, maximum: 500 },
        allowed_view_policy: { type: "string", enum: ["cleaned_and_analysis_only"] },
        delivery_mode: { type: "string", enum: ["notebook_plan", "notebook_artifact", "workbench_artifact"] },
        include_debug: { type: "boolean" }
      },
      required: ["analysis_goal"],
      additionalProperties: false
    }
  },
  {
    name: "get_account_stats",
    description: "Build account-level statistics for selected accounts or the current scope.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        success_filter: { type: "string" },
        cash_filter: { type: "string" },
        metric_mode: { type: "string" },
        group_by: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "inspect_case_schema",
    description: "Inspect current-case-project schema through analytix backend or the current-case DuckDB fallback as preflight for explicit controlled SQL/notebook escalation. Do not call this for ordinary graph/profile/downstream/report answers that can be handled by semantic fact tools. Do not call this for a two-party amount follow-up when the previous answer already returned original-detail amount, effective amount, and duplicate/same-fact risk amount; restate those facts instead. Returns tables, columns, row counts, analysis-scope classification, semantic hints, key/metric columns, join hints, and SQL-safe table.sql_name / column.sql_name / sql_identifier mappings for run_case_sql. Chinese display_name labels are UI-only and are not executable SQL. Does not expose raw transaction rows or SQL.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        table_limit: { type: "integer", minimum: 1, maximum: 500 },
        column_limit: { type: "integer", minimum: 0, maximum: 200 },
        include_columns: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "audit_unindexed_sources",
    description: "Audit whether transaction-looking tables or new/nonstandard sources may be outside the normalized transaction and analysis-index scope.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        table_limit: { type: "integer", minimum: 1, maximum: 500 },
        column_limit: { type: "integer", minimum: 0, maximum: 200 }
      },
      additionalProperties: false
    }
  },
  {
    name: "rank_accounts",
    description: "Rank accounts over the current case project, holder scope, or account scope by inflow, outflow, turnover, transaction count, or max single amount. Use directly for Top/biggest-account questions instead of routing through the navigator. Omit case_id to use the current analytix case project from runtime workspace; do not call get_current_case first for ordinary ranking. Result includes ordinary ranking coverage counts, so do not add get_scope_coverage after a successful rank_accounts call.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional. Omit to use the current analytix case project resolved from runtime workspace." },
        account_keys: { type: "array", items: { type: "string" } },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        source_file_ids: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        success_filter: { type: "string", enum: ["success_only", "all"], description: "Use all for ordinary 全案/最大/Top rankings. Use success_only only when the user explicitly asks for successful transactions." },
        cash_filter: { type: "string", enum: ["cash_only", "non_cash_only", "all"] },
        metric: { type: "string", enum: ["inflow", "outflow", "turnover", "txn_count", "max_single_amount"] },
        limit: { type: "integer", description: "Requested row count. Runtime clamps out-of-range values to the safe 1..200 window; ordinary Top questions should use 3..10." },
        top_n: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit when the user explicitly asks Top N; preserve the requested N." },
        requested_limit: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit; must reflect the user-requested visible ranking row count." }
      },
      additionalProperties: false
    }
  },
  {
    name: "rank_holders",
    description: "按确定性聚合指标对全案、主体范围或账户范围的户名/相关账户排行。用于明确的 Top 主体、人员、户名排行问题。不要把本工具作为是否有关联、是否和案件资金有关联、是否涉案资金命中等姓名/主体关联快查入口；此类问题走资金研判入口的资金事实快查路线，以便输出严格命中、模糊命中、排除对象和不能确认四类结果。省略案件编号时使用当前案件；普通排行题不要先调用当前案件查询。结果已包含普通排行所需覆盖笔数，不要在成功排行后追加范围覆盖查询。不得用本工具补排行表、扩写 Top20 或与其他工具拼接 P0 全案结论；只回答用户明确限定的排行范围。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional. Omit to use the current analytix case project resolved from runtime workspace." },
        account_keys: { type: "array", items: { type: "string" } },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        source_file_ids: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        success_filter: { type: "string", enum: ["success_only", "all"], description: "Use all for ordinary 全案/最大/Top rankings. Use success_only only when the user explicitly asks for successful transactions." },
        cash_filter: { type: "string", enum: ["cash_only", "non_cash_only", "all"] },
        metric: { type: "string", enum: ["inflow", "outflow", "turnover", "txn_count", "max_single_amount"] },
        limit: { type: "integer", description: "Requested row count. Runtime clamps out-of-range values to the safe 1..200 window; ordinary Top questions should use 3..10." },
        top_n: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit when the user explicitly asks Top N; preserve the requested N." },
        requested_limit: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit; must reflect the user-requested visible ranking row count." }
      },
      additionalProperties: false
    }
  },
  {
    name: "rank_counterparties",
    description: "按确定性聚合指标对全案、主体范围或账户范围的对手方排行。用于明确的收款、付款、对手方、资金去向排行和重算统计范围。用户说“在某案件/某案件分析中/当前案件中/项目中资金流向前 N 位”时，这是案件范围，不是 holder_name；必须省略 holder_name 做全案对手方排行。资金流向/资金去向/去向前 N/转给谁/付款对象/收款方前 N 默认是出账去向口径，必须使用 metric=outflow、direction_mode=out、counterparty_group_mode=name；只有用户明确说交易总额、往来总额、双向、不区分方向或入账+出账时才用 metric=turnover/direction_mode=both。只有用户明确点名某个付款主体/户名/账号的去向时才传 holder_name。自然语言问“谁/户名/对手方/去向前 N 位”时默认按解析户名聚合；只有用户明确要求账号、卡号、账户粒度或核验空户名账号时才传 counterparty_group_mode=account。不要把本工具作为是否有关联、是否和案件资金有关联、是否涉案资金命中等姓名关联快查入口；此类问题走资金研判入口的资金事实快查路线。对 A 转给 B 多少钱或两个金额范围冲突等金额核验题，本工具只能作为排行/线索候选来源，不得直接用单行排行作为最终金额。金额核验必须复核事实来源、主体/账户范围、对手方颗粒度、原明细/有效交易/去重依据、时间窗口、同事实/换卡风险；既有证据不足时，使用定向专项资金核算聚合并标注核验状态。省略案件编号时使用当前案件；普通排行题不要先调用当前案件查询。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string", description: "Optional. Omit to use the current analytix case project resolved from runtime workspace." },
        account_keys: { type: "array", items: { type: "string" } },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        source_file_ids: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        question: { type: "string", description: "Optional current user request for intent normalization; when it says 资金流向/去向/转给谁/付款对象/收款方前 N, runtime enforces outflow destination ranking." },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        success_filter: { type: "string", enum: ["success_only", "all"], description: "Use all for ordinary 全案/最大/Top rankings. Use success_only only when the user explicitly asks for successful transactions." },
        cash_filter: { type: "string", enum: ["cash_only", "non_cash_only", "all"] },
        metric: { type: "string", enum: ["inflow", "outflow", "turnover", "txn_count", "max_single_amount"] },
        limit: { type: "integer", description: "Requested row count. Runtime clamps out-of-range values to the safe 1..200 window; ordinary Top questions should use 3..10." },
        top_n: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit when the user explicitly asks Top N; preserve the requested N." },
        requested_limit: { type: "integer", minimum: 1, maximum: 200, description: "Alias for limit; must reflect the user-requested visible ranking row count." },
        dedupe_same_holder_same_fact: { type: "boolean", description: "For same-holder two-party amount/destination ranking, fold duplicate rows only when the composite fact key matches: valid non-placeholder txn_id plus time, amount, direction, balance, counterparty, summary/remark/type, and success status. Empty/查无信息/unknown/null/无 style placeholders fall back to row identity. Never use as blind full-case dedupe." },
        counterparty_group_mode: { type: "string", enum: ["account", "name"], description: "Group counterparties by parsed/display name or by account. Default is name for ordinary 谁/户名/对手方/资金去向 Top-N questions; use account only when the user explicitly asks for账号/卡号/账户粒度 or missing-name account investigation." },
        include_self_counterparty: { type: "boolean", description: "Default false for name-grain ordinary destination/counterparty rankings: exclude empty counterparty names and rows where counterparty name equals source holder name, then backfill to the requested Top N. Set true only when the user explicitly asks to include同名自转/内部调拨/self transfers." }
      },
      additionalProperties: false
    }
  },
  {
    name: "resolve_owner_scope",
    description: "Resolve direct registered accounts and candidate linked accounts for a holder/person scope; candidate accounts are leads, not ownership facts.",
    inputSchema: {
      type: "object",
      properties: ownerScopeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "compare_analysis_scopes",
    description: "Compare full-case analysis scopes and row-count/amount semantics before report-grade totals.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "hypothesis_probe",
    description: "Run a deterministic fund-analysis probe for an Agent-chosen hypothesis such as destination flow, negative search, cash breakpoint, financial product, project, litigation, asset leads, or open-ended irregular pattern discovery. Use for negative-search field checks and state the searched-field boundary; if the user did not provide a precise target, return the boundary instead of claiming absolute absence. Preserve evidence_status and downgrade weak leads.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "run_discovery_scan",
    description: "Alias of hypothesis_probe for broad discovery over category summaries and lead families.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "trace_holder_destinations",
    description: "Alias of hypothesis_probe for holder/account destination aggregation and follow-up targets.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "detect_cash_breakpoints",
    description: "Alias of hypothesis_probe for cash, counterparty-missing, ATM/POS/counter and cash-bridge lead detection.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "detect_financial_product_flows",
    description: "Alias of hypothesis_probe for financial product subscription/redemption/securities/fund/wealth-management flow leads.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "detect_project_litigation_asset_leads",
    description: "Alias of hypothesis_probe for project/business, litigation/enforcement, and asset-consumption lead detection.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "generate_followup_investigation_list",
    description: "Alias of hypothesis_probe for ranked counterparties and supplementary evidence request targets.",
    inputSchema: {
      type: "object",
      properties: probeInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "trace_subject_top_outflows",
    description: "Trace a holder/account scope's requested Top outflows, classify terminals, find bounded in-case downstream candidates, and produce follow-up request targets. This is the ordinary source-of-truth fact pack for `继续追下游`, `向下追一层`, Top 出账, 多主体追踪, 补调, 终点分类 and continuation queues. The requested top_n/limit is a user-intent contract; do not collapse Top 10 to a five-row preview. If the backend semantic service is unavailable, the MCP runtime falls back to current-case DuckDB `analysis_txn_detail_idx` and returns one-hop Top outflows, terminal categories, follow-up requests, query ids, and an explicit trace-depth boundary instead of a capability-gap answer. When it returns outflow amount/count, main destinations, terminal categories, or follow-up requests, the focused skill should answer from it immediately; do not enrich the same first answer with inspect_case_schema, run_case_sql, get_evidence_pack, analyze_holder_full, analyze_account_full, rank tools, hypothesis probes, funds_investigate, shell/Python, or workspace search. When used after a high-share two-party focused inflow, query the receiver holder/account from the focused inflow date through at least 30 calendar days later, or omit date_end; do not default to next-day/24-hour only unless the user explicitly asks for that period. In a single two-party amount, graph, or downstream answer, call this support check at most once for the same holder/account/date scope; do not verify every listed downstream destination with repeated pair amount calls. If the result is incomplete, state the 需复核/需补证 evidence boundary instead of repeating the same call.",
    inputSchema: {
      type: "object",
      properties: subjectTopOutflowInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "classify_missing_counterparty_business",
    description: "Classify missing-holder or missing-counterparty transactions into financial product, cash breakpoint, channel, project, litigation, asset, or transfer buckets.",
    inputSchema: {
      type: "object",
      properties: missingBusinessInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "validate_continuation_list",
    description: "Validate follow-up or appendix rows for blank fields, duplicates, yuan/wan unit closure, and consistency with the current case detail index.",
    inputSchema: {
      type: "object",
      properties: continuationListInputProperties,
      additionalProperties: false
    }
  },
  {
    name: "get_rule_hits",
    description: "Read abnormal rule hits for a case, account, entity, or transaction scope.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        entity_id: { type: "string" },
        txn_id: { type: "string" },
        limit: { type: "integer", minimum: 1, maximum: 500 },
        force_refresh: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "trace_fund_next_hop",
    description: "Trace the next hop from a suspicious transaction or account when the user asks to continue one more layer and a seed_txn_id, seed_account_id, or account_id is known. If no seed is available, first use build_fund_flow_graph or trace_subject_top_outflows to find supported-edge context; do not call this tool with an empty seed. If the backend semantic service is unavailable, the MCP runtime falls back to the current-case DuckDB detail index, matches the receiving account/name back into case accounts, and returns bounded next-hop candidates or a visible current-data breakpoint. amount_tolerance is a ratio, for example 0.03 means 3%, not an absolute yuan amount.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        seed_txn_id: { type: "string" },
        seed_account_id: { type: "string" },
        account_id: { type: "string" },
        time_window_minutes: { type: "number" },
        amount_tolerance: { type: "number", minimum: 0, maximum: 1, description: "Relative tolerance ratio, not yuan. Use 0.03 for 3%." },
        amount_tolerance_ratio: { type: "number", minimum: 0, maximum: 1, description: "Alias for amount_tolerance. Relative tolerance ratio, not yuan." },
        include_split_merge: { type: "boolean" },
        max_depth: { type: "integer", minimum: 1, maximum: 4 },
        scope_source: { type: "string", enum: ["db", "uploaded_file", "temp_scope"] },
        source_file_ids: { type: "array", items: { type: "string" } },
        temp_scope_id: { type: "string" },
        resolved_scope_id: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "trace_fund",
    description: "Run deterministic fund tracing from a seed transaction or account for explicit path, loop, or rescope/recompute questions. If backend tracing is unavailable, the MCP runtime falls back to current-case DuckDB: seed/account inputs become next-hop candidate tracing, and holder/account Top outflow inputs become one-hop trace facts with explicit depth boundaries. tolerance_amount is currently a ratio, for example 0.03 means 3%, not an absolute yuan amount.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        seed_txn_id: { type: "string" },
        account_id: { type: "string" },
        depth: { type: "integer", minimum: 1, maximum: 6 },
        time_window_minutes: { type: "number" },
        tolerance_amount: { type: "number", minimum: 0, maximum: 1, description: "Relative tolerance ratio, not yuan. Use 0.03 for 3%." },
        include_return_flows: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_case_reconciliation",
    description: "Verify case-level DuckDB table/index reconciliation before any full-analysis claim.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "audit_case_data_quality",
    description: "Audit import/cleaning lineage, empty-holder grouping, materialized indexes, and card-replacement/same-fact duplicate candidates before relying on bounded fund statistics. It is not a substitute for the P0-hidden full-case/report entry and must not be swept with other tools to synthesize a whole-case conclusion. Translate any cleaning flags into business language such as 清洗标记不能单独证明无重复.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        example_limit: { type: "integer", minimum: 1, maximum: 50 }
      },
      additionalProperties: false
    }
  },
  {
    name: "resolve_duplicate_families",
    description: "Resolve same-fact duplicate/card-replacement candidate families with the minimum safe scope of same holder/person or explicit account set. Full-case mode is review-only and must not be used for blind dedupe.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        date_start: { type: "string" },
        date_end: { type: "string" },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        min_amount: { type: "number", minimum: 0 },
        scope_mode: { type: "string", enum: ["same_holder_accounts", "selected_scope", "full_case_review"] },
        limit: { type: "integer", minimum: 1, maximum: 200 }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_scope_coverage",
    description: "返回当前案件或选定账户的确定性覆盖计数，包括已分析交易数和字段完整性。本工具不能替代专项资金核算或报告级自定义 SQL 前的数据质量预检；相关场景应直接调用数据质量与口径复核，不要把轻量覆盖计数当成核验结论。",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        source_file_ids: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        direction_mode: { type: "string", enum: ["both", "in", "out"] },
        success_filter: { type: "string" },
        cash_filter: { type: "string" }
      },
      additionalProperties: false
    }
  },
  {
    name: "resolve_account_scope",
    description: "Resolve a card number, account number, or account key into Analytix account_keys with coverage and ambiguity warnings.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } }
      },
      additionalProperties: false
    }
  },
  {
    name: "resolve_holder_scope",
    description: "Resolve a holder/name or id number into a case account set with coverage and ambiguity warnings.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] }
      },
      additionalProperties: false
    }
  },
  {
    name: "analyze_account_full",
    description: "Run full account analysis through backend coverage, stats, counterparty rankings, behavior profile, and rule hits. Use directly for account profile questions; bounded rows are only evidence examples.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_key: { type: "string" },
        card_no: { type: "string" },
        acct_no: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        counterparty_limit: { type: "integer", minimum: 1, maximum: 50 },
        large_amount_threshold: { type: "number" }
      },
      additionalProperties: false
    }
  },
  {
    name: "analyze_holder_full",
    description: "One-stop current-case-project holder/person/company dossier fact pack. Use directly for ordinary 主体资金画像/名下账户研判 questions: it returns registered-account scope, proof-needed account leads, account statistics, Top accounts, Top counterparties, evidence gaps, and source boundaries for the focused skill to draft a public-security economic-investigation profile. If these facts are present, do not call shell, Python, backend APIs, local files, SQL/notebook, repeated rankings, or old reports to enrich the same first answer. Use rank_accounts/rank_counterparties only when a required Top table is absent or a different metric is explicitly requested. If a named two-party relationship is asked, investigate_pair_amount may be called once; ranking must not rewrite the pair amount. Candidate accounts are leads, not ownership facts.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        date_start: { type: "string" },
        date_end: { type: "string" },
        counterparty_limit: { type: "integer", minimum: 1, maximum: 50 }
      },
      additionalProperties: false
    }
  },
  {
    name: "validate_report_claims",
    description: "Review claims from an existing user-supplied report or paragraph without authorizing a new report. During P0 containment, do not use this tool to draft, continue, or publish a report and do not call the hidden full-case entry first. Provider/backend supported or verified labels remain candidates until the host evidence registry validates each claim. Unsupported claims or numbers remain 暂不能认定/需复核/需补证 with proof actions.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        claims: { type: "array", items: REPORT_CLAIM_INPUT_SCHEMA },
        facts: {
          type: "object",
          properties: {},
          additionalProperties: false,
          description: "P0 containment accepts no provider-supplied fact map. Facts must enter through host-issued evidence receipts."
        },
        final_investigation_answer: {
          ...INVESTIGATION_ANSWER_SCHEMA,
          description: "Optional schema-first FinalInvestigationAnswer candidate. Its receipt strings remain untrusted until the host registry and Final Evidence Gate validate them."
        },
        report_text: { type: "string" },
        strict_report_text: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "get_evidence_pack",
    description: "Build a bounded evidence package for visual tables, appendix inventory, or narrowed account/lead review after scope is known. Do not use it to fill or replace the P0-hidden full-case/report path. Evidence pack rows are candidate support, not host-verified claims or legal conclusions.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        txn_ids: { type: "array", items: { type: "string" } }
      },
      additionalProperties: false
    }
  },
  {
    name: "scan_case_risks",
    description: "Scan case-level risk coverage and return structured leads for follow-up.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        include_graph: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "plan_case_analysis",
    description: "Build sub-agent lanes, tool budgets, output schemas, and shared deterministic context before complex case analysis. After deferred tool discovery, call this as mcp__analytix_funds__plan_case_analysis; if a bare plan_case_analysis call is reported unsupported, retry with the analytix_funds namespace instead of skipping the planning gate.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        analysis_goal: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        include_risk_scan: { type: "boolean" },
        max_lanes: { type: "integer", minimum: 1, maximum: 8 },
        budget: analysisPlanBudgetSchema
      },
      additionalProperties: false
    }
  },
  {
    name: "run_investigation_lab",
    description: "Run an open-ended hypothesis lab that preserves Agent freedom while returning deterministic evidence cards for suspicious features, irregular patterns, rankings, Top outflows, and evidence gaps.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        analysis_goal: { type: "string" },
        holder_name: { type: "string" },
        id_no: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        match_mode: { type: "string", enum: ["exact", "contains"] },
        include_candidate_accounts: { type: "boolean" },
        candidate_min_turnover: { type: "number", minimum: 0 },
        focus_keywords: { type: "array", items: { type: "string" } },
        date_start: { type: "string" },
        date_end: { type: "string" },
        max_hypotheses: { type: "integer", minimum: 1, maximum: 20 },
        include_top_outflows: { type: "boolean" },
        include_full_case_rankings: { type: "boolean" },
        plan_context: investigationPlanContextSchema,
        skip_internal_plan: { type: "boolean" }
      },
      additionalProperties: false
    }
  },
  {
    name: "run_full_case_analysis",
    ...visibleToolMeta("全案资金研判", "正在开展全案资金研判", "已完成全案资金研判", { readOnlyHint: false }),
    description: "P0 quarantined full-case/report entry. The MCP host does not advertise or execute this tool, for either write_report value, until the host EvidenceReceipt registry, claim gate, PublicationReceipt registry, and atomic publication pipeline are available. Callers must use only currently advertised bounded evidence tools; without verified same-case coverage they must return a capability/evidence boundary, never a whole-case conclusion or report. A path, file-open/render/inspection status, or model-supplied receipt ID never proves publication.",
    inputSchema: {
      type: "object",
      properties: {
        case_id: { type: "string" },
        account_keys: { type: "array", items: { type: "string" } },
        max_accounts: { type: "integer", minimum: 1, maximum: 12 },
        include_internal_playbooks: { type: "boolean" },
        write_report: { type: "boolean", description: "Both values are P0-quarantined. false must not execute analysis or create files; true must not execute a report writer. Formal publication requires a host-issued PublicationReceipt after evidence and claim validation." },
        force_refresh: { type: "boolean" }
      },
      additionalProperties: false
    }
  }
].map((tool) => {
  const title = TOOL_VISIBLE_TITLES[tool.name];
  const visibleTool = !title || tool.title
    ? tool
    : {
        ...visibleToolMeta(title, `正在处理：${title}`, `已完成：${title}`),
        ...tool
      };
  return {
    ...visibleTool,
    outputSchema: outputSchemaForTool(tool.name)
  };
});

export const TOOL_SCHEMA_INVENTORY = assertToolCatalogSchemas(tools, TOOL_OUTPUT_CONTRACT_BY_NAME);
