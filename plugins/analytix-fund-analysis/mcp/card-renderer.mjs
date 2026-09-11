import { translateUserVisibleText } from "./user-facing-language.mjs";
import { redactRestrictedPiiText } from "./agent-context-hygiene.mjs";
import { intOrUndefined as intValue, numberOrUndefined as numberValue } from "./runtime-normalizers.mjs";

export const PHASE4_CARD_TYPES = new Set([
  "top_rankings_card",
  "claim_review_card",
  "investigation_lab_card",
  "destination_outflow_card",
  "source_to_counterparty_amount_card",
	  "name_association_quick_fact_card",
	  "financial_product_topic_card",
	  "company_to_person_recompute_card",
	  "fund_flow_graph_delivery_card",
	  "subject_dossier_card",
	  "account_dossier_card"
	]);

const OPAQUE_REF_PATTERN = /\b(?:q_[0-9a-f]{6,}|casegraph:[\w:.-]+|audit_ref|detail_ref|artifact_id|source_refs?|evidence_refs?|evidence_ledger|query_ids?|report_ids?|path_ids?|edge_id)\b(?:\s*[:：]\s*[\w:.,;/-]+)?/giu;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function moneyText(value) {
  const number = numberValue(value);
  if (number === undefined) return "未返回";
  return `${number.toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })} 元`;
}

function uniqueTexts(values, limit = 4) {
  return [...new Set(arrayOf(values).map(text).filter(Boolean))].slice(0, limit);
}

function topRankingMustWriteFacts(source, facts) {
  if (text(source.card_type) !== "top_rankings_card") return [];
  const wantedFactIds = new Set([
    "coverage.directed_transactions",
    "source_audit.import_lineage",
    "top_account.metric.turnover.rank1",
    "top_account.metric.inflow.rank1",
    "top_account.metric.outflow.rank1",
    "top_account.metric.txn_count.rank1",
    "top_account.metric.max_single_amount.rank1",
    "top_account.turnover.rank1",
    "top_holder.turnover.rank1",
    "top_counterparty.turnover.rank1",
    "blank_counterparty.both_blank.rank2"
  ]);
  const lines = facts
    .filter((fact) => wantedFactIds.has(text(objectOf(fact).fact_id)))
    .map((fact) => text(objectOf(fact).answer_text || objectOf(fact).label))
    .filter(Boolean)
    .map((line) => `Top 排名事实：${line}`);
  if (!lines.length) return [];
  lines.push("Top 排名硬边界：上述事实已由确定性排行、覆盖范围和数据质量核验返回；不得把已返回的 Top 户名主体、Top 对手方、金额或笔数写成未知、未登记、0.00 元或未返回。");
  lines.push("Top 排名排序口径：往来总额、入账、出账、笔数、最大单笔是不同口径，不得只写往来总额 Top。");
  lines.push("Top 排名下一步复核：需覆盖时间、金额、账户、对手、txn_id/交易号和口径边界；不得把线索写成最终资金归属。");
  lines.push("Top 排名输出边界：保留 Top 户名主体的账户数、交易笔数、资金流量，以及双空对手方金额，作为报告级主体口径和数据质量边界。");
  return lines;
}

function renderSourceToCounterpartyAmountCard(source) {
  const fullPeriod = objectOf(source.source_to_via_summary);
  const dateWindow = objectOf(source.source_to_via_date_window_summary);
  const coreAccount = objectOf(source.source_to_via_core_account_summary);
  const fullCluster = objectOf(fullPeriod.largest_date_cluster);
  const windowCluster = objectOf(dateWindow.largest_date_cluster);
  const coreCluster = objectOf(coreAccount.largest_date_cluster);
  const holderName = text(source.holder_name) || "付款主体";
  const viaName = text(source.via_holder_name) || "收款主体";
  const dateStart = text(source.date_start || fullPeriod.first_txn_at || fullPeriod.first_txn_time || fullPeriod.min_txn_time || dateWindow.first_txn_at || coreAccount.first_txn_at);
  const dateEnd = text(source.date_end || fullPeriod.last_txn_at || fullPeriod.last_txn_time || fullPeriod.max_txn_time || dateWindow.last_txn_at || coreAccount.last_txn_at);
  const rangeText = dateStart || dateEnd ? `${dateStart || "未知起"} 至 ${dateEnd || "未知止"}` : "本轮可核验数据范围";
  const fullAmount = numberValue(fullPeriod.amount);
  const windowAmount = numberValue(dateWindow.amount);
  const coreAmount = numberValue(coreAccount.amount);
  const fullCount = intValue(fullPeriod.txn_count);
  const windowCount = intValue(dateWindow.txn_count);
  const coreCount = intValue(coreAccount.txn_count);
  const coreAccounts = uniqueTexts(coreAccount.counterparty_accounts);
  const fullSourceAccounts = uniqueTexts(fullPeriod.source_accounts);
  const coreSourceAccounts = uniqueTexts(coreAccount.source_accounts);
  const duplicateSourceAccounts = uniqueTexts(coreAccount.duplicate_source_accounts);
  const fullFactReady = fullAmount !== undefined && fullCount !== undefined && fullAmount > 0 && fullCount > 0;
  const windowFactReady = windowAmount !== undefined && windowCount !== undefined && windowAmount > 0 && windowCount > 0;
  const coreFactReady = coreAmount !== undefined && coreCount !== undefined && coreAmount > 0 && coreCount > 0;
  if (!fullFactReady && !windowFactReady && !coreFactReady) {
    return redactRestrictedPiiText(translateUserVisibleText(`${holderName}向${viaName}的一跳转账核验未返回可验证的金额与笔数完整字段。缺失值、空结果或部分字段不得写成 0、无往来或无异常；请补齐同案、同轮、同快照的交易范围和宿主 EvidenceReceipt 后再形成案件事实。`));
  }
  const amounts = [fullAmount, windowAmount, coreAmount].filter((amount) => amount !== undefined && amount > 0);
  const amountsDiffer = new Set(amounts.map((amount) => amount.toFixed(2))).size > 1;
  const summaryFacts = [];
  if (fullFactReady) summaryFacts.push(`收款人姓名匹配结果 ${fullCount} 笔，合计 ${moneyText(fullAmount)}`);
  if (coreFactReady) summaryFacts.push(`重点收款账户${coreAccounts.length ? ` ${coreAccounts.join("、")}` : " 待核"}，核验 ${coreCount} 笔，合计 ${moneyText(coreAmount)}`);
  const lines = [
    `经梳理，${holderName}向${viaName}一跳转账核验如下`,
    "本段仅列示已核金额事实、差异来源和风险提示；正式经侦材料需补齐结论、重点交易表、异常特征、案件意义、不能认定事项和取证动作。",
    `核验期间：${rangeText}。${summaryFacts.join("；")}。`
  ];
  lines.push("流水统计:");
  if (fullFactReady) {
    const clusterText = fullCluster.date && intValue(fullCluster.txn_count) !== undefined && numberValue(fullCluster.amount) !== undefined
      ? `；主要集中日 ${text(fullCluster.date)} ${intValue(fullCluster.txn_count)} 笔，${moneyText(fullCluster.amount)}`
      : "";
    lines.push(`- 收款人姓名匹配结果：${fullCount} 笔，${moneyText(fullAmount)}${clusterText}${fullSourceAccounts.length ? `；付款账号 ${fullSourceAccounts.join("、")}` : ""}。`);
  }
  if (windowFactReady) {
    const clusterText = windowCluster.date && intValue(windowCluster.txn_count) !== undefined && numberValue(windowCluster.amount) !== undefined
      ? `；集中日 ${text(windowCluster.date)} ${intValue(windowCluster.txn_count)} 笔，${moneyText(windowCluster.amount)}`
      : "";
    lines.push(`- 指定时间范围统计：${windowCount} 笔，${moneyText(windowAmount)}${clusterText}。`);
  } else {
    lines.push("- 指定时间范围统计：本轮未取得稳定结果，需结合逐笔流水和回单继续核验。");
  }
  if (coreFactReady) {
    const clusterText = coreCluster.date && intValue(coreCluster.txn_count) !== undefined && numberValue(coreCluster.amount) !== undefined
      ? `；主要集中日 ${text(coreCluster.date)} ${intValue(coreCluster.txn_count)} 笔，${moneyText(coreCluster.amount)}`
      : "";
    lines.push(`- 重点收款账户流水：${coreCount} 笔，${moneyText(coreAmount)}${coreSourceAccounts.length ? `；付款账号 ${coreSourceAccounts.join("、")}` : ""}${coreAccounts.length ? `；收款账号 ${coreAccounts.join("、")}` : ""}${clusterText}。`);
  }
  lines.push("竞争金额业务解释:");
  if (amountsDiffer) {
    lines.push(`- ${moneyText(fullAmount)} 覆盖收款人姓名匹配结果，${moneyText(coreAmount)} 只覆盖重点收款账户；差异来自姓名匹配范围、账户范围和时间集中度不同，需分列为材料依据，不得把较大值写成唯一事实金额。`);
  } else if (amounts.length >= 2) {
    lines.push("- 当前返回的多个金额字段数值相同；这不等于已通过逐字段证据核验，仍需保留交易号、账户、时间和摘要的逐笔依据。");
  } else {
    lines.push("- 当前返回范围不足以比较竞争金额，不能写成无差异或唯一事实金额。");
  }
  lines.push("异常特征:");
  if (duplicateSourceAccounts.length) {
    lines.push(`- 付款账号 ${duplicateSourceAccounts.join("、")} 只作为同事实/换卡重复放大风险线索，不能计为新增确定性交易边。`);
  } else {
    lines.push("- 如后续发现同事实、换卡或银行重复反馈，只能列为重复放大风险，不能直接累计或扣减。");
  }
  lines.push("暂不能认定事项:");
  lines.push("- 当前仅有一跳统计候选；未经宿主 EvidenceReceipt 逐字段核验，不得发布为案件事实，更不能证明后续资金闭环、最终去向或资金性质。");
  lines.push("- 对手方排行仅作定位线索，不作为最终金额结论。");
  lines.push("下一步核查:");
  lines.push("- 补调重点收款账户流水、银行回单、付款账号同事实组回单、余额承接和交易用途材料；继续追查时再进入下游资金流向核验。");
  return redactRestrictedPiiText(translateUserVisibleText(stripOpaqueRefs(lines.join("\n"))));
}

export function stripOpaqueRefs(value) {
  if (typeof value === "string") {
    return value
      .replace(OPAQUE_REF_PATTERN, "")
      .replace(/\s{2,}/gu, " ")
      .replace(/\s+([，。；、,.])/gu, "$1")
      .trim();
  }
  if (Array.isArray(value)) {
    return value.map(stripOpaqueRefs);
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    if (/^(audit_ref|detail_ref|artifact_id|source_refs?|evidence_refs?|evidence_ledger|query_ids?|query_id)$/iu.test(key)) continue;
    output[key] = stripOpaqueRefs(item);
  }
  return output;
}

function sanitizeAgentLine(value) {
  const line = translateUserVisibleText(stripOpaqueRefs(text(value))
    .replace(/\banswer_card_complete\s*=\s*true[；;，,]?\s*/giu, "")
    .replace(/\brequired_facts_present\s*=\s*false[；;，,]?\s*/giu, "当前仍有事实缺口；")
    .replace(/write_blocked\s*=\s*true[；;，,]?\s*/giu, "")
    .replace(/转入报告门禁/gu, "转入报告级流程")
    .replace(/报告门禁[:：]?\s*/gu, "报告级边界")
    .replace(/\bmissing_source_boundary\s*[:：]\s*/giu, "")
    .replace(/\bmissing_supported_edge_boundary\s*[:：]\s*/giu, "")
    .replace(/\bmissing_claim_source_anchor\s*[:：]\s*/giu, "")
    .replace(/\bclaim_without_source_anchor\s*[:：]\s*/giu, "")
    .replace(/\bamount_without_fact_anchor\s*[:：]\s*/giu, "")
    .replace(/\bunsupported_mermaid_or_arrow_flow\s*[:：]\s*/giu, "")
    .replace(/\bcandidate_account_as_owned_account\s*[:：]\s*/giu, "")
    .replace(/\bcash_or_asset_destination_without_supported_flow\s*[:：]\s*/giu, "")
    .replace(/\bmissing_counterparty_or_cash_break_as_verified\s*[:：]\s*/giu, "")
    .replace(/\bforbidden_legal_phrasing\s*[:：]\s*/giu, "")
    .replace(/\bclaim[_.]\d+\s*[:：]\s*/giu, "")
    .replace(/最终回答必须|最终答案必须|最终回答|最终答案/gu, "")
    .replace(/逐字保留|逐字包含|逐字写|逐字/gu, "")
    .replace(/Context Compiler[:：]?/giu, "")
    .replace(/Context raw result policy[:：]?.*/giu, "")
    .replace(/Raw MCP\/backend payloads (?:stay in local audit artifacts and MCP _meta; model-visible text receives only the compact Answer Card|are not model-visible; retention may be claimed only when the host evidence registry confirms it)\.?/giu, "原始明细未向本轮模型展示；是否已留存须以宿主证据登记为准。")
    .replace(/diagnostic label/giu, "")
    .replace(/\bvalidate_report_claims\b/giu, "研判结论复核")
    .replace(/\bclaim review\b/giu, "研判结论复核")
    .replace(/\bverified\s*\/\s*corrected\s*\/\s*unsupported\s*\/\s*downgraded\b/giu, "已复核 / 需纠正 / 证据不足 / 降级")
    .replace(/\bunsupported\s+flow\b/giu, "未支持资金流")
    .replace(/This edge is a deterministic transaction seed;?\s*/giu, "该笔为当前流水已支持的一跳交易；")
    .replace(/do not infer further hops without another returned transaction edge\.?/giu, "未取得下一跳明细前，不延伸推断后续去向。")
    .replace(/\bsupported transaction edge\b|\bsupported edge\b/giu, "确定性交易边")
    .replace(/\bproduction\s+lab\s*\/\s*probe\b/giu, "当前研判卡")
    .replace(/facts\s*缺口/giu, "事实缺口")
    .replace(/\bfacts\b/giu, "事实材料")
    .replace(/研判结论复核\s+和/giu, "研判结论复核和")
    .replace(/不是\s+确定性交易边/gu, "不是确定性交易边")
    .replace(/当前研判卡\s+中/gu, "当前研判卡中")
    .replace(/\btrace_fund_next_hop\b|\btrace_fund\b/giu, "资金追踪")
    .replace(/资金追踪\s*\/\s*资金追踪/gu, "资金追踪")
    .replace(/\bsource_audit\b/giu, "来源审计")
    .replace(/\bsource audit\b/giu, "来源审计")
    .replace(/\bdata_quality\b/giu, "数据质量")
    .replace(/\bdata quality\b/giu, "数据质量")
    .replace(/\bmandatory_review_cards\b/giu, "必需核验事项")
    .replace(/\bMCP result\b|\bMCP\/backend\b|\bMCP\b/giu, "可回放结果")
    .replace(/\bartifact\b/giu, "交付附件")
    .replace(/\bfact_refs\s*\/\s*source_refs\s*\/\s*evidence_refs\b/giu, "事实来源")
    .replace(/\bfact_refs\b|\bsource_refs\b|\bevidence_refs\b/giu, "事实来源")
    .replace(/\bdeterministic fact\b/giu, "确定性事实")
    .replace(/\bSQL\b/giu, "可回放计算")
    .replace(/\bedge_status\s*=\s*"supported"\b/giu, "确定性交易边")
    .replace(/\bdoctor\b/giu, "")
    .replace(/\bscore\b/giu, "")
    .replace(/\/goal/gu, "")
    .replace(/蓝皮书|执行方案/gu, "")
    .replace(/必须/gu, "需")
    .replace(/\s{2,}/gu, " ")
    .replace(/^[：:；;，,\s]+/gu, "")
    .trim());
  if (!line) return "";
  if (/golden answer|\boracle\b|\brubric\b/iu.test(line)) return "";
  return line;
}

function visibleAnswerConstraint(value) {
  let line = sanitizeAgentLine(value);
  if (!line) return "";
  if (/(answer_card|状态机|字段名|不要继续调用|重复调用|funds_investigate|validate_report_claims|Context Compiler)/iu.test(line)) return "";
  if (/^(输出|本题不是|普通问答|补调清单形成正式附件前|正式报告级文本形成后)/u.test(line)) return "";
  line = line
    .replace(/^说明/u, "")
    .replace(/^先写/u, "")
    .replace(/^只点名/u, "仅点名")
    .replace(/^每个可疑簇都要标明\s*support_status；?/iu, "每个可疑线索集合标明支持程度；")
    .replace(/可疑簇/gu, "可疑线索集合")
    .replace(/金额簇/gu, "金额集中")
    .replace(/时间簇/gu, "时间集中")
    .replace(/\bsupport_status\b/giu, "支持程度")
    .trim();
  return line ? `研判要点：${line}` : "";
}

function diagnosticSupportEvidence(value, depth = 0) {
  if (depth > 6) return undefined;
  if (Array.isArray(value)) {
    const rows = value.slice(0, 12).map((item) => diagnosticSupportEvidence(item, depth + 1)).filter((item) => item !== undefined);
    return rows.length ? rows : undefined;
  }
  if (!value || typeof value !== "object") {
    const item = typeof value === "string" ? text(value) : value;
    return item === "" || item === undefined || item === null ? undefined : item;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    if (/^(?:answer_draft|answer_sections|answer_constraints|answer_text|title|summary|description|interpretation|prompt|question)$/iu.test(key)) continue;
    if (/^(?:warnings|unsupported_claims|unsupported_flows|forbidden_as_facts|missing_source_boundaries|quality_guards|next_queries|next_review_actions|context_compiler)$/iu.test(key)) continue;
    const next = diagnosticSupportEvidence(item, depth + 1);
    if (next === undefined) continue;
    output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

function diagnosticSupportTexts(source, keys) {
  return uniqueTexts(keys.flatMap((key) => arrayOf(source[key])).map((item) => {
    if (item && typeof item === "object") {
      const row = objectOf(item);
      return text(row.message || row.code || row.text || row.claim || row.reason || row.why_this_query || row.boundary || row.label || row.summary);
    }
    return text(item);
  }).filter(Boolean), 16);
}

export function renderDiagnosticCardForAgent(card) {
  const source = stripOpaqueRefs(objectOf(card));
  if (PHASE4_CARD_TYPES.has(text(source.card_type)) && source.fact_answer_allowed !== true) {
    return "案件事实未发布：当前卡片没有通过宿主 Final Evidence Gate。只能说明来源缺口、已检查范围和补证建议；缺失、空结果或部分字段不得写成 0、无关联或无异常。";
  }
  const facts = diagnosticSupportEvidence(source) || {};
  const lines = ["案件事实摘录: 诊断核验"];
  const factLines = [];
  const objectSummary = (item) => Object.values(objectOf(item))
    .map((part) => typeof part === "string" || typeof part === "number" || typeof part === "boolean" ? text(part) : "")
    .filter(Boolean)
    .slice(0, 5)
    .join("，");
  const pushFact = (label, value) => {
    const item = translateUserVisibleText(stripOpaqueRefs(text(value)));
    if (item) factLines.push(`${label}: ${item}`);
  };
  for (const [key, value] of Object.entries(facts)) {
    if (/^(?:card_type|case_id|answer_card_complete|write_blocked|report_grade_blocked|required_facts_present)$/iu.test(key)) continue;
    if (Array.isArray(value)) {
      const rows = value.slice(0, 8).map((row) => {
        if (row && typeof row === "object") {
          const item = objectOf(row);
          return text(item.claim || item.text || item.statement || item.summary || item.reason || item.label || item.title || objectSummary(item));
        }
        return text(row);
      }).filter(Boolean);
      if (rows.length) pushFact(translateUserVisibleText(key), rows.join("；"));
      continue;
    }
    if (value && typeof value === "object") {
      const item = objectOf(value);
      pushFact(translateUserVisibleText(key), text(item.claim || item.text || item.statement || item.summary || item.reason || item.label || item.title || objectSummary(item)));
      continue;
    }
    pushFact(translateUserVisibleText(key), value);
  }
  lines.push(...factLines.slice(0, 16));
  const risks = diagnosticSupportTexts(source, [
    "warnings",
    "unsupported_claims",
    "unsupported_flows",
    "forbidden_as_facts",
    "missing_source_boundaries",
    "quality_guards"
  ]).map((item) => translateUserVisibleText(stripOpaqueRefs(item))).filter(Boolean);
  if (risks.length) lines.push(`边界与风险: ${risks.slice(0, 8).join("；")}`);
  const guidance = diagnosticSupportTexts(source, ["next_queries", "next_review_actions"])
    .map((item) => translateUserVisibleText(stripOpaqueRefs(item))).filter(Boolean);
  if (guidance.length) lines.push(`后续核验: ${guidance.slice(0, 6).join("；")}`);
  if (lines.length === 1) lines.push("当前诊断卡未返回可直接成稿的事实字段；请按来源边界继续核验。");
  return lines.join("\n");
}

export function renderDiagnosticCardForAudit(card, options = {}) {
  const source = stripOpaqueRefs(objectOf(card));
  if (text(source.card_type) === "source_to_counterparty_amount_card") {
    return renderSourceToCounterpartyAmountCard(source);
  }
  const facts = arrayOf(source.facts);
  const contextCompiler = objectOf(source.context_compiler);
  const contextLine = (item) => typeof item === "string"
    ? text(item)
    : [
        text(objectOf(item).why_this_query || objectOf(item).reason || objectOf(item).text || objectOf(item).summary || objectOf(item).label)
      ].filter(Boolean).join("：");
  const factLines = [
    text(source.card_type) === "claim_review_card" && source.required_facts_present === true && source.write_blocked !== true
      ? "必需事实已齐：当前研判结论已绑定可复核事实，可按复核结论作答。"
      : "",
    text(contextCompiler.raw_result_policy) ? "原始明细未向本轮模型展示；是否已留存须以宿主证据登记为准。" : "",
    ...arrayOf(contextCompiler.must_write_facts).map((item) => `研判依据：${contextLine(item)}`),
    ...facts
      .map((fact) => text(objectOf(fact).answer_text || objectOf(fact).label)),
    ...arrayOf(source.verified_facts).map((item) => `已证实：${text(item)}`),
    ...arrayOf(source.confirmed_facts).map((item) => `已证实：${text(item)}`),
    ...arrayOf(source.hypothesis_queue).map((item) => `待检假设：${text(item)}`),
    ...[...arrayOf(source.suspicious_groups), ...arrayOf(source.suspicious_clusters)].map((item) => `可疑线索集合：${text(item)}`),
    ...[...arrayOf(source.amount_concentrations), ...arrayOf(source.amount_clusters)].map((item) => `金额集中：${text(item)}`),
    ...[...arrayOf(source.time_concentrations), ...arrayOf(source.temporal_clusters)].map((item) => `时间集中：${text(item)}`),
    ...arrayOf(source.anomalous_amounts).map((item) => `反常金额：${text(item)}`),
    ...arrayOf(source.duplicate_or_same_fact_risks).map((item) => `重复/同事实风险：${text(item)}`),
    ...arrayOf(source.related_account_or_card_switch_risks).map((item) => `关联账户/换卡风险：${text(item)}`),
    ...arrayOf(source.duplicate_or_card_replacement_risks).map((item) => `重复/换卡风险：${text(item)}`),
    ...arrayOf(source.missing_counterparty_or_cash_breaks).map((item) => `缺失对手方/现金断点：${text(item)}`),
    ...arrayOf(source.missing_counterparties).map((item) => `缺失对手方：${text(item)}`),
    ...arrayOf(source.wealth_management_or_asset_clues).map((item) => `理财/资产线索：${text(item)}`),
    ...arrayOf(source.continuation_paths).map((item) => `追查路径：${typeof item === "string" ? text(item) : text(objectOf(item).summary || objectOf(item).path || "结构化追查路径未返回可读摘要；是否留存须以宿主证据登记为准")}`),
    ...arrayOf(source.excluded_or_downgraded_hypotheses).map((item) => `已排除/降级假设：${text(item)}`),
    ...arrayOf(source.terminal_classification).map((item) => {
      const row = objectOf(item);
      return `终点分类：${text(row.name)}=${text(row.category)}；${text(row.boundary)}`;
    }).filter(Boolean),
	    ...arrayOf(source.continuation_targets).map((item) => {
	      const row = objectOf(item);
	      return `补证建议（补调对象）：${text(row.target)}；${text(row.reason)}`;
	    }).filter(Boolean),
    ...arrayOf(source.verified_claims).map((item) => `已复核研判结论：${text(item)}`),
    ...arrayOf(source.corrected_claims).map((item) => `需纠正研判结论：${text(item)}`)
  ].filter(Boolean);
  const boundaryLines = [
    source.write_blocked === true ? "正式报告暂不出具：复核或来源边界仍有缺口。" : "",
    ...arrayOf(source.warnings).map(text),
    ...arrayOf(source.stop_conditions).map(text),
    ...arrayOf(contextCompiler.must_state_boundaries).map(contextLine),
    ...arrayOf(source.claim_review?.corrected).map((item) => `需纠正：${text(item)}`),
    ...arrayOf(source.claim_review?.downgraded).map((item) => `需降级：${text(item)}`),
    ...arrayOf(source.unsupported_claims).map((item) => `未获支持的研判结论：${text(item)}；需核实。`),
    ...arrayOf(source.unsupported_flows).map((item) => `证据不足/不能确认资金流：${text(item)}；需核实；没有确定性资金边，证据不足时不得绘制资金流向图。`),
    ...arrayOf(source.missing_source_boundaries).map((item) => `来源边界：${text(item)}`),
    ...arrayOf(source.forbidden_phrasings).map((item) => `禁用表述：${text(item)}`),
    ...arrayOf(source.required_subpoena_fields).map((item) => `补调字段：${text(item)}`),
    ...arrayOf(source.quality_guards).map((item) => `质量边界：${text(item)}`),
    text(source.investigation_intent) ? `研判目标：${text(source.investigation_intent)}` : "",
    ...arrayOf(source.hypothesis_status).map((item) => {
      const status = objectOf(item);
      return `线索支持程度：${text(status.hypothesis)}=${text(status.status || status.support_status)}`;
    }).filter(Boolean),
    ...arrayOf(source.next_queries).map((item) => {
      const query = objectOf(item);
      return `后续核验：${text(query.why_this_query || query.reason || query.text || query.label)}`;
    }).filter(Boolean)
  ].filter(Boolean);
  const answerConstraintLines = arrayOf(source.answer_constraints)
    .map(visibleAnswerConstraint)
    .filter(Boolean);
  const unsupportedLines = [
    ...arrayOf(source.claim_review?.unsupported).map(text),
    ...arrayOf(source.forbidden_as_facts).map((item) => `禁止写成事实：${text(item)}`)
      .concat(arrayOf(contextCompiler.forbidden_as_facts).map((item) => `禁止写成事实：${contextLine(item)}`))
  ].filter(Boolean);
  const nextQueryLines = [
    ...arrayOf(source.next_review_actions).map((item) => `复核动作：${text(item)}`),
    ...arrayOf(contextCompiler.next_queries).map((item) => `后续核验：${contextLine(item)}`),
    ...arrayOf(contextCompiler.stop_conditions).map((item) => `收口边界：${contextLine(item)}`)
  ].filter(Boolean);
  const sections = [
    ...(text(source.card_type) === "claim_review_card" ? [["复核范围", [
      "本卡只复核用户提交或报告自述的金额、链路、资金流向图、归属和法律敏感表述；不新增全案结论。",
      "逐项研判结论使用核验通过、当前证据不足、降级为线索、需补证；证据不足时给纠正口径、降级表述和下一步补证。"
    ]]] : []),
    ["研判事实", factLines],
    ...(answerConstraintLines.length ? [["研判要点", answerConstraintLines]] : []),
    ["核验意见", boundaryLines],
    ["禁止写成事实的内容", unsupportedLines],
    ...(nextQueryLines.length ? [["下一步复核", nextQueryLines]] : [])
  ];
  const lines = [];
  if (options.includeTitle !== false) {
    lines.push(text(source.title) || "Analytix 资金研判材料");
  }
  for (const [title, rows] of sections) {
    lines.push(title);
    const visibleRows = rows.length ? rows : [emptyAuditSectionBoundary(title)];
    for (const row of visibleRows) {
      const safeRow = sanitizeAgentLine(row);
      if (safeRow) lines.push(`- ${safeRow}`);
    }
  }
  return translateUserVisibleText(stripOpaqueRefs(lines.join("\n")));
}

function emptyAuditSectionBoundary(title) {
  switch (text(title)) {
    case "研判事实":
      return "本轮未取得可验证事实；未返回不等于不存在、为零或无异常。";
    case "核验意见":
      return "本轮未取得可验证核验结果；应说明已检查范围并继续补证。";
    case "禁止写成事实的内容":
      return "本轮未返回具体待排除表述；不得据此推断不存在其他未支持表述。";
    case "下一步复核":
      return "本轮未返回具体复核动作；需按来源缺口补充查询范围。";
    default:
      return "本轮未返回该部分的可验证内容。";
  }
}
