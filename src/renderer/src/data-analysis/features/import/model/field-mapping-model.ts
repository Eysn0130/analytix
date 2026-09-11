import {
  normalizeKind,
  sanitizeManualFieldMapping,
  type ImportDomainCategory,
  type ImportWizardFile,
  type MappingFieldBlueprint,
  type MappingFieldOrigin,
  type MappingFieldState,
  type MappingInsight,
  type MappingSourceFieldPreview,
  type MappingTargetFieldPreview,
  type MappingWorkbenchColumn,
  type MappingWorkbenchFilter,
  type MappingWorkbenchReasonTone,
  type WizardFileState
} from "./import-page-model";

export const FIELD_BLUEPRINTS: Record<string, MappingFieldBlueprint[]> = {
  fc_transaction: [
    { key: "card_no", label: "交易卡号", aliases: ["交易卡号", "查询卡号", "本方卡号", "卡号", "账卡号"], importHeader: "交易卡号" },
    { key: "acct_no", label: "交易账号", aliases: ["交易账号", "查询账号", "查询帐号", "本方账号", "账号", "账户", "卡号", "账号/卡号"], required: true, importHeader: "交易账号" },
    { key: "account_open_name", label: "账户开户名称", aliases: ["账户开户名称", "姓名", "姓名(查询条件)", "企业名称", "企业名称(查询条件)", "开户名", "户名", "账户名称"], importHeader: "账户开户名称" },
    { key: "opener_id_no", label: "开户人证件号码", aliases: ["开户人证件号码", "证件号码", "证件号码(查询条件)", "证件号", "身份证号"], importHeader: "开户人证件号码" },
    { key: "txn_time", label: "交易时间", aliases: ["交易时间", "交易日期", "日期", "记账日期", "入账时间", "交易发生时间"], required: true, importHeader: "交易时间" },
    { key: "amount", label: "交易金额", aliases: ["交易金额", "发生额", "金额", "借方金额", "贷方金额", "交易金额原币", "交易金额原币（元）", "交易金额折人民币", "交易金额折人民币（元）"], required: true, importHeader: "交易金额" },
    { key: "balance", label: "交易余额", aliases: ["交易余额", "余额", "账户余额", "可用余额"], importHeader: "交易余额" },
    { key: "dc_flag", label: "收付标志", aliases: ["收付标志", "借贷标志", "借贷方向", "借方贷方", "进出标志", "借贷标记", "资金收付标识", "资金收付标志", "收付标识"], importHeader: "收付标志" },
    { key: "counterparty_acct", label: "交易对手账卡号", aliases: ["交易对手账卡号", "交易对方账卡号", "交易对方帐卡号", "交易对方账号", "交易对方卡号", "对方账号", "对方卡号", "对手账号", "对手卡号"], importHeader: "交易对手账卡号" },
    { key: "cash_flag", label: "现金标志", aliases: ["现金标志", "现金标识", "现金类型", "现金、转账标识", "现金转账标识"], importHeader: "现金标志" },
    { key: "counterparty_name", label: "对手户名", aliases: ["对手户名", "交易对方名称", "交易对方户名", "对方名称", "对手名称"], importHeader: "对手户名" },
    { key: "counterparty_id_no", label: "对手身份证号", aliases: ["对手身份证号", "交易对方证件号码", "交易对方证件号", "对方证件号码", "对方证件号", "对手证件号"], importHeader: "对手身份证号" },
    { key: "counterparty_bank", label: "对手开户银行", aliases: ["对手开户银行", "交易对方账号开户行", "交易对方开户行", "交易对方行名称", "对方开户行", "对手开户行", "对方开户银行"], importHeader: "对手开户银行" },
    { key: "summary", label: "摘要说明", aliases: ["摘要说明", "交易摘要", "摘要", "附言"], importHeader: "摘要说明" },
    { key: "currency", label: "交易币种", aliases: ["交易币种", "币种", "币别"], importHeader: "交易币种" },
    { key: "branch_name", label: "交易网点名称", aliases: ["交易网点名称", "网点名称", "交易网点", "交易行名称", "交易机构名称"], importHeader: "交易网点名称" },
    { key: "branch_code", label: "交易网点代码", aliases: ["交易网点代码", "网点代码", "交易机构代码", "机构代码", "交易机构号", "网点号"], importHeader: "交易网点代码" },
    { key: "location", label: "交易发生地", aliases: ["交易发生地", "交易地点", "发生地"], importHeader: "交易发生地" },
    { key: "is_success", label: "交易是否成功", aliases: ["交易是否成功", "交易成功标识", "是否成功"], importHeader: "交易是否成功" },
    { key: "voucher_no", label: "传票号", aliases: ["传票号"], importHeader: "传票号" },
    { key: "terminal_no", label: "终端号", aliases: ["终端号", "终端编号", "交易终端号", "设备号", "自助设备编号", "终端代码", "ATM机具编号"], importHeader: "终端号" },
    { key: "ip_addr", label: "IP地址", aliases: ["IP地址", "IP"], importHeader: "IP地址" },
    { key: "mac_addr", label: "MAC地址", aliases: ["MAC地址", "MAC", "MAC或IMEI地址", "MAC/IMEI地址", "IMEI地址"], importHeader: "MAC地址" },
    { key: "counterparty_balance", label: "对手交易余额", aliases: ["对手交易余额", "交易对手余额", "对方余额", "对手余额"], importHeader: "对手交易余额" },
    { key: "txn_id", label: "交易流水号", aliases: ["交易流水号", "流水号", "业务流水号"], importHeader: "交易流水号" },
    { key: "log_id", label: "日志号", aliases: ["日志号"], importHeader: "日志号" },
    { key: "voucher_type", label: "凭证种类", aliases: ["凭证种类", "凭证类型"], importHeader: "凭证种类" },
    { key: "voucher_id", label: "凭证号", aliases: ["凭证号", "凭证编号"], importHeader: "凭证号" },
    { key: "teller_no", label: "交易柜员号", aliases: ["交易柜员号", "柜员号"], importHeader: "交易柜员号" },
    { key: "merchant_name", label: "商户名称", aliases: ["商户名称", "商户名", "特约商户名称", "商户户名"], importHeader: "商户名称" },
    { key: "merchant_no", label: "商户号", aliases: ["商户号", "商户编号", "商户代码", "特约商户号", "特约商户编号"], importHeader: "商户号" },
    { key: "remark", label: "备注", aliases: ["备注", "交易备注", "备注说明"], importHeader: "备注" },
    { key: "txn_type", label: "交易类型", aliases: ["交易类型", "交易种类", "业务类型", "业务种类", "业务名称", "交易类别", "业务类别"], importHeader: "交易类型" },
    { key: "query_feedback_reason", label: "查询反馈结果原因", aliases: ["查询反馈结果原因", "查询反馈结果", "查询结果", "反馈原因"], importHeader: "查询反馈结果原因" }
  ],
  fc_account: [
    { key: "account_open_name", label: "账户开户名称", aliases: ["账户开户名称", "姓名", "姓名(查询条件)", "开户名", "户名", "客户名称", "账户名称"], required: true, importHeader: "账户开户名称" },
    { key: "opener_id_no", label: "开户人证件号码", aliases: ["开户人证件号码", "证件号码", "证件号码(查询条件)", "证件号", "身份证号"], importHeader: "开户人证件号码" },
    { key: "card_no", label: "交易卡号", aliases: ["交易卡号", "卡号", "本方卡号", "账卡号", "查询卡号"], importHeader: "交易卡号" },
    { key: "acct_no", label: "交易账号", aliases: ["交易账号", "账号", "账户账号", "账户号", "本方账号", "本方账户", "查询账号", "查询帐号"], required: true, importHeader: "交易账号" },
    { key: "open_time", label: "账号开户时间", aliases: ["账号开户时间", "开户日期", "开户时间", "账号开户日期", "开户日期时间"], importHeader: "账号开户时间" },
    { key: "balance", label: "账户余额", aliases: ["账户余额", "余额", "账面余额"], importHeader: "账户余额" },
    { key: "available_balance", label: "可用余额", aliases: ["可用余额", "可用金额", "可用资金"], importHeader: "可用余额" },
    { key: "currency", label: "币种", aliases: ["币种", "币别"], importHeader: "币种" },
    { key: "branch_code", label: "开户网点代码", aliases: ["开户网点代码", "交易网点代码", "网点代码"], importHeader: "开户网点代码" },
    { key: "branch_name", label: "开户网点", aliases: ["开户网点", "网点名称", "开户机构"], importHeader: "开户网点" },
    { key: "acct_status", label: "账户状态", aliases: ["账户状态", "账号状态", "账户状态名称"], importHeader: "账户状态" },
    { key: "cashfx_flag_name", label: "钞汇标志名称", aliases: ["钞汇标志", "钞汇标志名称", "钞汇"], importHeader: "钞汇标志名称" },
    { key: "close_date", label: "销户日期", aliases: ["销户日期", "销户时间"], importHeader: "销户日期" },
    { key: "acct_type", label: "账户类型", aliases: ["账户类型", "账户类别"], importHeader: "账户类型" },
    { key: "remark", label: "备注", aliases: ["备注", "附言", "说明"], importHeader: "备注" },
    { key: "open_bank", label: "账号开户银行", aliases: ["账号开户银行", "开户银行", "开户行", "开户行名称", "账号开户行"], required: true, importHeader: "账号开户银行" },
    { key: "close_branch", label: "销户网点", aliases: ["销户网点", "销户机构"], importHeader: "销户网点" },
    { key: "last_txn_time", label: "最后交易时间", aliases: ["最后交易时间", "最后交易日期"], importHeader: "最后交易时间" }
  ],
  fc_sub_account: [
    { key: "bank_name", label: "银行名称", aliases: ["银行名称", "开户银行", "开户行", "银行"], required: true, importHeader: "银行名称" },
    { key: "parent_acct", label: "开户账号", aliases: ["开户账号", "账卡号", "账户账号", "账号", "主账户账号", "本方账号", "卡号"], required: true, importHeader: "开户账号" },
    { key: "sub_acct", label: "子账户账号", aliases: ["子账户账号", "子账号", "子账户号"], required: true, importHeader: "子账户账号" },
    { key: "balance", label: "余额", aliases: ["余额", "账户余额", "子账户余额"], importHeader: "余额" },
    { key: "available_balance", label: "可用余额", aliases: ["可用余额", "可用金额"], importHeader: "可用余额" },
    { key: "sub_type", label: "子账户类别", aliases: ["子账户类别", "子账户类型"], importHeader: "子账户类别" },
    { key: "sub_seq_no", label: "子账户序号", aliases: ["子账户序号", "子账户编号", "子账户顺序号"], importHeader: "子账户序号" },
    { key: "currency", label: "币种", aliases: ["币种", "币别"], importHeader: "币种" },
    { key: "cashfx_flag", label: "钞汇标识", aliases: ["钞汇标识", "钞汇标志", "钞汇标志名称", "钞汇"], importHeader: "钞汇标识" },
    { key: "acct_status", label: "账户状态", aliases: ["账户状态", "账号状态"], importHeader: "账户状态" },
    { key: "acct_seq_no", label: "账户序号", aliases: ["账户序号", "总账户序号", "主账户序号"], importHeader: "账户序号" }
  ],
  fc_person: [
    { key: "customer_name", label: "客户名称", aliases: ["客户名称", "姓名", "人员姓名", "姓名/名称"], required: true, importHeader: "客户名称" },
    { key: "id_type", label: "证照类型", aliases: ["证照类型"], importHeader: "证照类型" },
    { key: "id_no", label: "证照号码", aliases: ["证照号码", "证件号码", "证件号", "身份证号", "证件号码/统一社会信用代码"], required: true, importHeader: "证照号码" },
    { key: "org_addr", label: "单位地址", aliases: ["单位地址", "地址", "联系地址"], importHeader: "单位地址" },
    { key: "org_phone", label: "单位电话", aliases: ["单位电话", "联系电话", "手机号码", "手机号"], importHeader: "单位电话" },
    { key: "employer", label: "工作单位", aliases: ["工作单位"], importHeader: "工作单位" },
    { key: "email", label: "邮箱地址", aliases: ["邮箱地址", "邮箱"], importHeader: "邮箱地址" },
    { key: "agent_name", label: "代办人姓名", aliases: ["代办人姓名"], importHeader: "代办人姓名" },
    { key: "agent_id_type", label: "代办人证件类型", aliases: ["代办人证件类型"], importHeader: "代办人证件类型" },
    { key: "agent_id_no", label: "代办人证件号码", aliases: ["代办人证件号码"], importHeader: "代办人证件号码" },
    { key: "nat_tax_no", label: "国税纳税号", aliases: ["国税纳税号"], importHeader: "国税纳税号" },
    { key: "local_tax_no", label: "地税纳税号", aliases: ["地税纳税号"], importHeader: "地税纳税号" },
    { key: "legal_rep", label: "法人代表", aliases: ["法人代表"], importHeader: "法人代表" },
    { key: "license_no", label: "客户工商执照号码", aliases: ["客户工商执照号码"], importHeader: "客户工商执照号码" }
  ],
  fc_person_address: [
    { key: "open_name", label: "开户名称", aliases: ["开户名称", "姓名", "人员姓名"], required: true, importHeader: "开户名称" },
    { key: "id_type", label: "证照类型", aliases: ["证照类型"], importHeader: "证照类型" },
    { key: "id_no", label: "证照号码", aliases: ["证照号码", "证件号码", "证件号", "身份证号"], required: true, importHeader: "证照号码" },
    { key: "home_addr", label: "住宅地址", aliases: ["住宅地址", "住址", "地址", "居住地址"], required: true, importHeader: "住宅地址" },
    { key: "home_phone", label: "住宅电话", aliases: ["住宅电话", "联系电话", "家庭电话"], importHeader: "住宅电话" }
  ],
  fc_person_contact: [
    { key: "open_name", label: "开户名称", aliases: ["开户名称", "姓名", "人员姓名"], required: true, importHeader: "开户名称" },
    { key: "id_type", label: "证照类型", aliases: ["证照类型"], importHeader: "证照类型" },
    { key: "id_no", label: "证照号码", aliases: ["证照号码", "证件号码", "证件号", "身份证号"], importHeader: "证照号码" },
    { key: "contact_phone", label: "联系电话", aliases: ["联系电话", "联系方式", "手机号", "手机号码"], required: true, importHeader: "联系电话" }
  ],
  fc_coercive_measure: [
    { key: "bank_name", label: "银行名称", aliases: ["银行名称", "开户银行", "开户行", "银行"], required: true, importHeader: "银行名称" },
    { key: "acct_no", label: "账号", aliases: ["账号", "银行账号", "冻结账号"], required: true, importHeader: "账号" },
    { key: "measure_type", label: "冻结措施类型", aliases: ["冻结措施类型", "措施类型", "强制措施", "措施名称"], required: true, importHeader: "冻结措施类型" },
    { key: "amount", label: "冻结金额", aliases: ["冻结金额", "金额"], importHeader: "冻结金额" },
    { key: "agency", label: "冻结机关", aliases: ["冻结机关", "执行机关", "办案机关", "机构名称"], importHeader: "冻结机关" },
    { key: "start_date", label: "冻结开始日期", aliases: ["冻结开始日期", "开始时间", "执行时间", "生效时间"], importHeader: "冻结开始日期" },
    { key: "end_date", label: "冻结截止日期", aliases: ["冻结截止日期", "截止时间", "结束时间"], importHeader: "冻结截止日期" },
    { key: "measure_seq_no", label: "措施序号", aliases: ["措施序号"], importHeader: "措施序号" },
    { key: "remark", label: "备注", aliases: ["备注"], importHeader: "备注" }
  ],
  fc_task_success: [
    { key: "task_serial_no", label: "任务流水号", aliases: ["任务流水号", "任务编号", "任务ID", "任务号"], required: true, importHeader: "任务流水号" },
    { key: "bank_name", label: "银行名称", aliases: ["银行名称"], required: true, importHeader: "银行名称" },
    { key: "subject_type", label: "主体类别", aliases: ["主体类别"], importHeader: "主体类别" },
    { key: "id_or_acct_no", label: "证账号码", aliases: ["证账号码"], required: true, importHeader: "证账号码" },
    { key: "acct_card_no", label: "账卡号", aliases: ["账卡号"], required: true, importHeader: "账卡号" },
    { key: "send_time", label: "发送时间", aliases: ["发送时间"], importHeader: "发送时间" },
    { key: "feedback_time", label: "反馈时间", aliases: ["反馈时间", "完成时间", "结束时间"], importHeader: "反馈时间" },
    { key: "feedback_result", label: "反馈结果", aliases: ["反馈结果"], importHeader: "反馈结果" },
    { key: "feedback_non_detail", label: "反馈非明细结果", aliases: ["反馈非明细结果"], importHeader: "反馈非明细结果" },
    { key: "feedback_detail", label: "反馈明细结果", aliases: ["反馈明细结果"], importHeader: "反馈明细结果" },
    { key: "store_time", label: "入库时间", aliases: ["入库时间"], importHeader: "入库时间" },
    { key: "store_status", label: "入库状态", aliases: ["入库状态"], importHeader: "入库状态" },
    { key: "request_no", label: "请求单号", aliases: ["请求单号", "请求编号"], required: true, importHeader: "请求单号" },
    { key: "query_result", label: "查询结果", aliases: ["查询结果"], importHeader: "查询结果" }
  ],
  fc_task_fail: [
    { key: "task_serial_no", label: "任务流水号", aliases: ["任务流水号", "任务编号", "任务ID", "任务号"], required: true, importHeader: "任务流水号" },
    { key: "bank_name", label: "银行名称", aliases: ["银行名称"], required: true, importHeader: "银行名称" },
    { key: "subject_type", label: "主体类别", aliases: ["主体类别"], importHeader: "主体类别" },
    { key: "id_or_acct_no", label: "证账号码", aliases: ["证账号码"], required: true, importHeader: "证账号码" },
    { key: "acct_card_no", label: "账卡号", aliases: ["账卡号"], required: true, importHeader: "账卡号" },
    { key: "send_time", label: "发送时间", aliases: ["发送时间"], importHeader: "发送时间" },
    { key: "feedback_time", label: "反馈时间", aliases: ["反馈时间"], importHeader: "反馈时间" },
    { key: "feedback_result", label: "反馈结果", aliases: ["反馈结果", "失败原因", "错误原因", "异常描述"], importHeader: "反馈结果" },
    { key: "feedback_non_detail", label: "反馈非明细结果", aliases: ["反馈非明细结果"], importHeader: "反馈非明细结果" },
    { key: "feedback_detail", label: "反馈明细结果", aliases: ["反馈明细结果"], importHeader: "反馈明细结果" },
    { key: "store_time", label: "入库时间", aliases: ["入库时间"], importHeader: "入库时间" },
    { key: "store_status", label: "入库状态", aliases: ["入库状态"], importHeader: "入库状态" },
    { key: "request_no", label: "请求单号", aliases: ["请求单号", "请求编号"], required: true, importHeader: "请求单号" },
    { key: "query_result", label: "查询结果", aliases: ["查询结果"], importHeader: "查询结果" }
  ]
};

function normalizeMappingToken(value: string): string {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[\s_:：/\\|（）()【】[\]<>《》,.，。;；"'`*＊-]/g, "");
}

function fieldAliasMatchScore(header: string, alias: string, mode: "exact" | "fuzzy"): number {
  const headerToken = normalizeMappingToken(header);
  const aliasToken = normalizeMappingToken(alias);
  if (!headerToken || !aliasToken) {
    return 0;
  }
  if (headerToken === aliasToken) {
    return 1000 + aliasToken.length;
  }
  if (mode === "exact") {
    return 0;
  }
  const overlap = Math.min(headerToken.length, aliasToken.length);
  if (overlap < 4) {
    return 0;
  }
  if (headerToken.includes(aliasToken) || aliasToken.includes(headerToken)) {
    return 100 + overlap;
  }
  return 0;
}

function fieldBlueprintAliases(field: MappingFieldBlueprint): string[] {
  return Array.from(
    new Set(
      [field.importHeader, field.label, ...field.aliases]
        .map((item) => String(item || "").trim())
        .filter(Boolean)
    )
  );
}

export function buildMappingInsight(args: {
  selectedKind: string;
  suggestedKind: string;
  suggestedKindLabel: string;
  selectedCategory: Exclude<ImportDomainCategory, "all">;
  columnsTotal: number;
  issue: string;
  headerPreview: string[];
  fieldMappings?: Record<string, string>;
  manualMappings?: Record<string, string>;
  mappingOrigins?: Record<string, MappingFieldOrigin>;
  mappingStatus?: string;
  mappingMethod?: string;
  mappingMessage?: string;
  mappingRequiredMissing?: string[];
}): MappingInsight {
  const effectiveKind = normalizeKind(args.selectedKind || args.suggestedKind || "");
  const blueprint = FIELD_BLUEPRINTS[effectiveKind] ?? [];
  const headers = Array.from(
    new Set(
      (args.headerPreview || [])
        .map((item) => String(item || "").trim())
        .filter(Boolean)
    )
  );
  const fieldMappings = sanitizeManualFieldMapping(args.fieldMappings ?? args.manualMappings, headers);
  const mappingOrigins = args.mappingOrigins || {};
  const mappingMethod = String(args.mappingMethod || "").trim().toLowerCase();
  const backendReadyNoRequiredMissing =
    String(args.mappingStatus || "").trim().toLowerCase() === "ready" &&
    Array.isArray(args.mappingRequiredMissing) &&
    args.mappingRequiredMissing.length === 0;
  const fallbackMappedOrigin: MappingFieldOrigin =
    mappingMethod === "ai_validated"
      ? "ai"
      : backendReadyNoRequiredMissing || mappingMethod === "exact_header" || mappingMethod === "rule"
        ? "auto"
        : "manual";

  if (args.selectedCategory === "support" || effectiveKind === "support_file") {
    return {
      summary: "该文件会按研判文件登记，不进入结构化字段映射；后续可在模型与检索流程中继续使用。",
      headerCount: 0,
      matchedRequired: 0,
      requiredTotal: 0,
      matchedTotal: 0,
      unresolvedCount: 0,
      riskCount: 0,
      confidenceLabel: "文档接入",
      sourceFields: [],
      targetFields: [
        {
          key: "support_file",
          label: "文件登记",
          importHeader: "文件登记",
          state: "info",
          required: false,
          sourceLabel: args.issue || "保留文件元信息与后续知识抽取入口",
          origin: "info"
        }
      ]
    };
  }

  if (blueprint.length === 0) {
    return {
      summary: args.issue || "当前类型尚未配置字段蓝图，系统会继续按类型入库并在校验阶段补充检查。",
      headerCount: headers.length || Math.max(0, args.columnsTotal),
      matchedRequired: 0,
      requiredTotal: 0,
      matchedTotal: 0,
      unresolvedCount: headers.length,
      riskCount: headers.length > 0 ? 1 : 0,
      confidenceLabel: "待建模",
      sourceFields: headers.map((header, index) => ({
        key: `${header}:${index}`,
        label: header,
        state: "info",
        targetLabel: "等待规则补全"
      })),
      targetFields: []
    };
  }

  const matchedByTarget = new Map<string, string>();
  const originByTarget = new Map<string, MappingFieldOrigin>();
  const matchedFieldByHeader = new Map<string, { field: MappingFieldBlueprint; origin: MappingFieldOrigin }>();

  const assignMappedField = (field: MappingFieldBlueprint, header: string, origin: MappingFieldOrigin): void => {
    matchedByTarget.set(field.key, header);
    originByTarget.set(field.key, origin);
    matchedFieldByHeader.set(header, { field, origin });
  };

  for (const field of blueprint) {
    const sourceHeader = String(fieldMappings[field.key] || "").trim();
    if (!sourceHeader) {
      continue;
    }
    const origin = mappingOrigins[field.key] || fallbackMappedOrigin;
    assignMappedField(field, sourceHeader, origin === "empty" ? fallbackMappedOrigin : origin);
  }

  const pickBestAliasField = (header: string, allowMatchedTargets: boolean): MappingFieldBlueprint | null =>
    pickBestAliasFieldForHeader(blueprint, header, (field) => allowMatchedTargets || !matchedByTarget.has(field.key));

  for (const header of headers) {
    if (matchedFieldByHeader.has(header)) {
      continue;
    }
    const autoField = pickBestAliasField(header, false);
    if (autoField) {
      assignMappedField(autoField, header, "auto");
    }
  }

  const backendReadyLabel = String(args.mappingMessage || "").trim() || "导入规则已确认";

  const sourceFields: MappingSourceFieldPreview[] = headers.map((header, index) => {
    const matched = matchedFieldByHeader.get(header);
    if (matched?.origin === "manual") {
      return {
        key: `${header}:${index}`,
        label: header,
        state: matched.field.required ? "matched" : "suggested",
        targetLabel: `${matched.field.label} · 手动指定`
      };
    }
    if (matched) {
      return {
        key: `${header}:${index}`,
        label: header,
        state: matched.field.required ? "matched" : "suggested",
        targetLabel: matched.field.label
      };
    }
    if (backendReadyNoRequiredMissing) {
      const displayField = pickBestAliasField(header, true);
      const duplicateSource = displayField ? matchedByTarget.get(displayField.key) : "";
      return {
        key: `${header}:${index}`,
        label: header,
        state: "info",
        targetLabel: displayField
          ? duplicateSource
            ? `${displayField.label} · 同类字段已由「${duplicateSource}」命中`
            : `${displayField.label} · 可选候选字段`
          : "扩展字段 · 入库保留原值"
      };
    }
    return {
      key: `${header}:${index}`,
      label: header,
      state: "unresolved",
      targetLabel: "待人工确认"
    };
  });

  const requiredTotal = blueprint.filter((field) => field.required).length;
  const localMatchedRequired = blueprint.filter((field) => field.required && matchedByTarget.has(field.key)).length;
  const matchedRequired = backendReadyNoRequiredMissing ? requiredTotal : localMatchedRequired;
  const matchedTotal = matchedByTarget.size;
  const inspectable = headers.length > 0;
  const unresolvedCount = inspectable ? sourceFields.filter((field) => field.state === "unresolved").length : 0;
  const missingRequired = inspectable ? Math.max(0, requiredTotal - matchedRequired) : 0;
  const riskCount = inspectable ? unresolvedCount + missingRequired : args.columnsTotal > 0 ? Math.max(1, requiredTotal || 1) : 0;
  const targetFields: MappingTargetFieldPreview[] = blueprint.map((field) => {
    const matched = matchedByTarget.has(field.key);
    const backendConfirmedRequired = backendReadyNoRequiredMissing && Boolean(field.required);
    return {
      key: field.key,
      label: field.label,
      importHeader: field.importHeader || field.label,
      state: matched ? "matched" : backendConfirmedRequired ? "suggested" : inspectable ? (field.required ? "required" : "suggested") : "suggested",
      required: Boolean(field.required),
      sourceLabel: matchedByTarget.get(field.key) || (backendConfirmedRequired ? backendReadyLabel : inspectable ? "待映射" : "待校验阶段解析"),
      origin: matched ? originByTarget.get(field.key) || "auto" : backendConfirmedRequired ? "info" : "empty"
    };
  });

  const summary = inspectable
    ? riskCount === 0
      ? `已识别 ${headers.length} 个源字段，${matchedRequired}/${requiredTotal || matchedTotal || 1} 个关键字段命中，可直接继续。`
      : `已识别 ${headers.length} 个源字段，仍有 ${riskCount} 项需要关注。`
    : args.columnsTotal > 0
      ? `已识别 ${args.columnsTotal} 列结构，但当前未返回表头预览，建议按类型确认后继续。`
      : args.issue || "等待进一步解析字段结构。";

  let confidenceLabel = "待确认";
  if (!inspectable) {
    confidenceLabel = args.selectedKind ? "待校验" : "待确认";
  } else if (riskCount === 0) {
    confidenceLabel = "高匹配";
  } else if (matchedRequired > 0) {
    confidenceLabel = "需微调";
  } else {
    confidenceLabel = "低匹配";
  }

  return {
    summary,
    headerCount: headers.length || Math.max(0, args.columnsTotal),
    matchedRequired,
    requiredTotal,
    matchedTotal,
    unresolvedCount,
    riskCount,
    confidenceLabel,
    sourceFields,
    targetFields
  };
}

export function mappingInsightTone(insight: MappingInsight): "success" | "warn" | "error" | "muted" {
  if (insight.confidenceLabel === "高匹配" || insight.confidenceLabel === "文档接入") {
    return "success";
  }
  if (insight.confidenceLabel === "需微调" || insight.confidenceLabel === "待校验") {
    return "warn";
  }
  if (insight.confidenceLabel === "低匹配" || insight.confidenceLabel === "待建模") {
    return "error";
  }
  return "muted";
}

export function preferredActiveMappingFieldKey(targetFields: MappingTargetFieldPreview[]): string {
  return (
    targetFields.find((field) => field.required && field.origin === "empty")?.key ||
    targetFields.find((field) => field.origin === "empty")?.key ||
    targetFields[0]?.key ||
    ""
  );
}

export function mappingBlueprintForKind(kind: string): MappingFieldBlueprint[] {
  return FIELD_BLUEPRINTS[normalizeKind(kind)] ?? [];
}

export function suggestFieldMappingFromHeaders(
  kind: string,
  headerPreview: string[],
  seedMapping: Record<string, string> = {}
): Record<string, string> {
  const blueprint = mappingBlueprintForKind(kind);
  const headers = Array.from(
    new Set(
      (headerPreview || [])
        .map((item) => String(item || "").trim())
        .filter(Boolean)
    )
  );
  const nextMapping = sanitizeManualFieldMapping(seedMapping, headers);
  const usedHeaders = new Set(Object.values(nextMapping).map((item) => String(item || "").trim()).filter(Boolean));
  for (const header of headers) {
    if (usedHeaders.has(header)) {
      continue;
    }
    const field = pickBestAliasFieldForHeader(blueprint, header, (candidate) => !String(nextMapping[candidate.key] || "").trim());
    if (!field) {
      continue;
    }
    nextMapping[field.key] = header;
    usedHeaders.add(header);
  }
  return nextMapping;
}

function pickBestAliasFieldForHeader(
  blueprint: MappingFieldBlueprint[],
  header: string,
  isCandidate: (field: MappingFieldBlueprint) => boolean
): MappingFieldBlueprint | null {
  let bestField: MappingFieldBlueprint | null = null;
  let bestScore = 0;
  let bestMode: "exact" | "fuzzy" = "fuzzy";
  for (const field of blueprint) {
    if (!isCandidate(field)) {
      continue;
    }
    const match = bestAliasMatchForField(header, field);
    if (!match) {
      continue;
    }
    const modeRank = match.mode === "exact" ? 1 : 0;
    const bestModeRank = bestMode === "exact" ? 1 : 0;
    const requiredRank = field.required ? 1 : 0;
    const bestRequiredRank = bestField?.required ? 1 : 0;
    if (
      match.score > bestScore ||
      (match.score === bestScore && modeRank > bestModeRank) ||
      (match.score === bestScore && modeRank === bestModeRank && requiredRank > bestRequiredRank)
    ) {
      bestField = field;
      bestScore = match.score;
      bestMode = match.mode;
    }
  }
  return bestField;
}

function bestAliasMatchForField(header: string, field: MappingFieldBlueprint): { alias: string; mode: "exact" | "fuzzy"; score: number } | null {
  let bestAlias = "";
  let bestScore = 0;
  let bestMode: "exact" | "fuzzy" = "fuzzy";
  for (const alias of fieldBlueprintAliases(field)) {
    const score = fieldAliasMatchScore(header, alias, "fuzzy");
    if (score <= 0 || score < bestScore) {
      continue;
    }
    bestScore = score;
    bestAlias = alias;
    bestMode = fieldAliasMatchScore(header, alias, "exact") > 0 ? "exact" : "fuzzy";
  }
  return bestScore > 0 && bestAlias ? { alias: bestAlias, mode: bestMode, score: bestScore } : null;
}

function inferMappingSampleSignature(sampleValues: string[]): string {
  const values = sampleValues
    .map((value) => String(value || "").trim())
    .filter(Boolean)
    .slice(0, 5);
  if (values.length === 0) {
    return "";
  }
  const threshold = Math.min(2, values.length);
  const datetimeCount = values.filter((value) => /^\d{4}[-/.年]\d{1,2}[-/.月]\d{1,2}(?:日)?(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?$/.test(value)).length;
  if (datetimeCount >= threshold) {
    return "时间格式";
  }
  const idNoCount = values.filter((value) => /^(?:\d{15}|\d{17}[\dXx])$/.test(value.replace(/\s+/g, ""))).length;
  if (idNoCount >= threshold) {
    return "证件号码";
  }
  const accountCount = values.filter((value) => /^\d{10,22}$/.test(value.replace(/\s+/g, ""))).length;
  if (accountCount >= threshold) {
    return "账号/卡号";
  }
  const nameCount = values.filter((value) => /^[\u4e00-\u9fa5]{2,8}$/.test(value)).length;
  if (nameCount >= threshold) {
    return "姓名";
  }
  const amountCount = values.filter((value) => /^-?(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$/.test(value.replace(/\s+/g, ""))).length;
  if (amountCount >= threshold) {
    return "金额数值";
  }
  return "";
}

function resolveMappingColumnBasis(args: {
  header: string;
  origin: MappingFieldOrigin | "empty";
  state: Exclude<MappingFieldState, "required">;
  sampleValues: string[];
  matchedField?: MappingFieldBlueprint;
}): { text: string; tone: MappingWorkbenchReasonTone } {
  const sampleSignature = inferMappingSampleSignature(args.sampleValues);
  const aliasMatch = args.matchedField ? bestAliasMatchForField(args.header, args.matchedField) : null;

  if (args.origin === "manual") {
    return {
      text: aliasMatch ? `人工调整，参考别名：${aliasMatch.alias}` : "人工调整结果，可继续微调",
      tone: "manual"
    };
  }
  if (args.origin === "ai") {
    if (aliasMatch) {
      return {
        text: `AI 建议，参考${aliasMatch.mode === "exact" ? "一致表头" : "相似别名"}：${aliasMatch.alias}`,
        tone: "ai"
      };
    }
    if (sampleSignature) {
      return { text: `AI 建议，样本像${sampleSignature}`, tone: "ai" };
    }
    return { text: "AI 语义建议，建议复核", tone: "ai" };
  }
  if (args.origin === "auto") {
    if (aliasMatch) {
      if (String(args.header || "").trim() === String(args.matchedField?.importHeader || args.matchedField?.label || "").trim()) {
        return {
          text: "表头与模板完全一致",
          tone: "success"
        };
      }
      return {
        text: `${aliasMatch.mode === "exact" ? "别名命中" : "相似命中"}：${aliasMatch.alias}`,
        tone: "success"
      };
    }
    if (sampleSignature) {
      return { text: `样本像${sampleSignature}，系统已自动建议`, tone: "success" };
    }
    return { text: "系统按字段语义自动匹配", tone: "success" };
  }
  if (args.state === "info") {
    return { text: "未映射到标准字段，入库保留为扩展字段", tone: "muted" };
  }
  if (args.state === "unresolved") {
    if (sampleSignature) {
      return { text: `未命中别名，样本像${sampleSignature}`, tone: "warn" };
    }
    return { text: "未命中字段别名，需人工确认", tone: "warn" };
  }
  return {
    text: sampleSignature ? `样本像${sampleSignature}，建议复核` : "当前列可继续人工微调",
    tone: "muted"
  };
}

export function defaultMappingWorkbenchFilter(insight: MappingInsight): MappingWorkbenchFilter {
  return insight.unresolvedCount > 0 ? "pending" : "all";
}

export function mappingWorkbenchFilterMatches(
  column: MappingWorkbenchColumn,
  filter: MappingWorkbenchFilter,
  requiredPendingCount: number
): boolean {
  if (filter === "all") {
    return true;
  }
  if (filter === "pending") {
    return column.state === "unresolved" || (requiredPendingCount > 0 && !column.targetKey);
  }
  if (filter === "required") {
    return column.targetRequired || (requiredPendingCount > 0 && column.state === "unresolved");
  }
  return column.origin === "auto" || column.origin === "ai";
}

export function mappingMatchedColumnCount(insight: MappingInsight): number {
  return insight.sourceFields.filter((field) => field.state === "matched" || field.state === "suggested").length;
}

export function mappingProgressLabel(insight: MappingInsight): string {
  const total = Math.max(0, insight.headerCount);
  if (total <= 0) {
    return "待解析表头";
  }
  const matched = mappingMatchedColumnCount(insight);
  const pending = Math.max(0, insight.unresolvedCount);
  const extended = Math.max(0, total - matched - pending);
  return `${total} 列表头 / 已匹配 ${matched}${extended > 0 ? ` / 扩展 ${extended}` : ""}${pending > 0 ? ` / 待确认 ${pending}` : ""}`;
}

export function mappingScopeResult(args: {
  status: WizardFileState;
  selectedCategory: Exclude<ImportDomainCategory, "all">;
  insight: MappingInsight;
}): {
  tone: "success" | "warn" | "error";
  label: string;
  message: string;
} {
  const { status, selectedCategory, insight } = args;
  const requiredPendingCount = Math.max(0, insight.requiredTotal - insight.matchedRequired);
  const hasBlockingRisk = status === "unsupported" || status === "failed" || requiredPendingCount > 0;
  const hasPendingReview = selectedCategory !== "support" && insight.unresolvedCount > 0;
  const tone: "success" | "warn" | "error" = hasBlockingRisk ? "error" : hasPendingReview ? "warn" : "success";
  return {
    tone,
    label: tone === "error" ? "异常" : tone === "warn" ? "待处理" : "已匹配",
    message:
      tone === "success"
        ? selectedCategory === "support"
          ? "当前子项会按研判文件登记，不需要结构化字段映射。"
          : "字段确认已完成，可执行入库。"
        : "当前子项仍需处理后才能进入下一步。",
  };
}

export function mappingPendingTemplateFields(insight: MappingInsight): MappingTargetFieldPreview[] {
  return insight.targetFields
    .filter((field) => field.origin === "empty")
    .slice()
    .sort((left, right) => {
      const requiredDelta = Number(right.required) - Number(left.required);
      if (requiredDelta !== 0) {
        return requiredDelta;
      }
      return String(left.importHeader || left.label).localeCompare(String(right.importHeader || right.label), "zh-CN");
    });
}

export function buildMappingWorkbenchColumns(
  insight: MappingInsight,
  headers: string[],
  sampleRows: string[][],
  kind: string
): MappingWorkbenchColumn[] {
  const fieldByKey = new Map(mappingBlueprintForKind(kind).map((field) => [field.key, field]));
  const boundTargetByHeader = new Map<
    string,
    { key: string; label: string; importHeader: string; required: boolean; origin: MappingFieldOrigin | "empty" }
  >();
  for (const field of insight.targetFields) {
    if ((field.origin === "manual" || field.origin === "auto" || field.origin === "ai") && headers.includes(field.sourceLabel)) {
      const existing = boundTargetByHeader.get(field.sourceLabel);
      const labels = existing ? Array.from(new Set([existing.label, field.label].filter(Boolean))) : [field.label];
      boundTargetByHeader.set(field.sourceLabel, {
        key: existing?.key || field.key,
        label: labels.join(" + "),
        importHeader: existing?.importHeader || field.importHeader,
        required: Boolean(existing?.required || field.required),
        origin:
          existing?.origin === "manual" || field.origin === "manual"
            ? "manual"
            : existing?.origin === "ai" || field.origin === "ai"
              ? "ai"
              : existing?.origin || field.origin
      });
    }
  }
  return headers.map((header, index) => {
    const sourceField = insight.sourceFields[index];
    const matched = boundTargetByHeader.get(header);
    const sampleValues = sampleRows.map((row) => String(row[index] ?? ""));
    const matchBasis = resolveMappingColumnBasis({
      header,
      origin: matched?.origin || "empty",
      state: sourceField?.state || "unresolved",
      sampleValues,
      matchedField: matched ? fieldByKey.get(matched.key) : undefined
    });
    return {
      header,
      targetKey: matched?.key || "",
      targetHeader: matched?.importHeader || "",
      targetFieldLabel: matched?.label || "",
      targetLabel: matched?.label || sourceField?.targetLabel || "拖动字段到此列",
      targetRequired: Boolean(matched?.required),
      origin: matched?.origin || "empty",
      state: sourceField?.state || "unresolved",
      sampleValues,
      matchBasis: matchBasis.text,
      matchBasisTone: matchBasis.tone
    };
  });
}

export function serializeFieldMapping(file: ImportWizardFile): Record<string, string> | undefined {
  if (file.selectedCategory === "support" || file.archiveChildren.length > 0) {
    return undefined;
  }
  return serializeKindFieldMapping(file.selectedKind || file.suggestedKind || "", file.headerPreview, file.fieldMapping);
}

export function serializeFieldMappingOrigins(file: ImportWizardFile): Record<string, string> | undefined {
  if (file.selectedCategory === "support" || file.archiveChildren.length > 0) {
    return undefined;
  }
  return serializeKindFieldMappingOrigins(
    file.selectedKind || file.suggestedKind || "",
    file.headerPreview,
    file.fieldMapping,
    file.fieldMappingOrigins
  );
}

export function serializeKindFieldMapping(
  kind: string,
  headerPreview: string[] = [],
  seedMapping: Record<string, string> = {}
): Record<string, string> | undefined {
  const effectiveKind = normalizeKind(kind || "");
  const blueprint = FIELD_BLUEPRINTS[effectiveKind] ?? [];
  const headers = Array.from(
    new Set(
      (headerPreview || [])
        .map((item) => String(item || "").trim())
        .filter(Boolean)
    )
  );
  const fieldMapping = headers.length > 0 ? suggestFieldMappingFromHeaders(effectiveKind, headers, seedMapping) : seedMapping;
  const mappings: Record<string, string> = {};
  for (const field of blueprint) {
    const targetHeader = String(field.importHeader || "").trim();
    const sourceHeader = String(fieldMapping[field.key] || "").trim();
    if (!targetHeader || !sourceHeader) {
      continue;
    }
    mappings[targetHeader] = sourceHeader;
  }
  return Object.keys(mappings).length > 0 ? mappings : undefined;
}

export function serializeKindFieldMappingOrigins(
  kind: string,
  headerPreview: string[] = [],
  seedMapping: Record<string, string> = {},
  seedOrigins: Record<string, MappingFieldOrigin> = {}
): Record<string, string> | undefined {
  const effectiveKind = normalizeKind(kind || "");
  const blueprint = FIELD_BLUEPRINTS[effectiveKind] ?? [];
  const headers = Array.from(
    new Set(
      (headerPreview || [])
        .map((item) => String(item || "").trim())
        .filter(Boolean)
    )
  );
  const fieldMapping = headers.length > 0 ? suggestFieldMappingFromHeaders(effectiveKind, headers, seedMapping) : seedMapping;
  const origins: Record<string, string> = {};
  for (const field of blueprint) {
    const targetHeader = String(field.importHeader || "").trim();
    const sourceHeader = String(fieldMapping[field.key] || "").trim();
    if (!targetHeader || !sourceHeader) {
      continue;
    }
    origins[targetHeader] = String(seedOrigins[field.key] || "auto").trim() || "auto";
  }
  return Object.keys(origins).length > 0 ? origins : undefined;
}
