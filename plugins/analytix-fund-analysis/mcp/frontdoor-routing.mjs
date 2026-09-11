export const FRONTDOOR_ROUTING_VERSION = "0.14.4";

const VIA_NAME_STOPWORDS = new Set([
  "案件",
  "文件",
  "三文件",
  "户名",
  "聚合",
  "复核",
  "单进程",
  "资金",
  "主要",
  "流向",
  "流向谁",
  "去向",
  "下游",
  "后续",
  "收到",
  "收款",
  "付款",
  "自然人",
  "企业",
  "理财",
  "证券",
  "银行",
  "产品",
  "认购",
  "缺失",
  "对手",
  "字段",
  "证据",
  "边界",
  "事实",
  "统计",
  "特征",
  "可疑",
  "线索",
  "口径",
  "金额",
  "关键",
  "核心",
  "窗口",
  "任务",
  "目标",
  "问题",
  "主体",
  "分类",
  "对象",
  "不足",
  "不能",
  "确认",
  "需要",
  "补调",
  "要求",
  "输出",
  "实质",
  "研判",
  "结论"
]);

const PERSON_NAME_BOUNDARY_PATTERN = String.raw`(?=\s*(?:多少钱|多少|金额|合计|总额|笔数|流水|往来|资金|后续|后|又|再|去向|流向|核心日期|核心集中日期|核心卡|核心|口径|情况|明细|是否|有无|有没有|的|[0-9０-９￥¥]|[？?，,。；;：:、]|$))`;

function text(value) {
  return String(value || "").trim();
}

function escapeRegExp(value) {
  return text(value).replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

export function inferViaNameFromQuestion(question, { holderName = "", focusKeywords = [] } = {}) {
  const q = text(question);
  const holder = text(holderName);
  const explicitTransferPatterns = holder
    ? [
        new RegExp(`${escapeRegExp(holder)}.{0,8}(?:转给|转至|转入|转|汇给|打给|给(?!出|我|出具))\\s*([\\u4e00-\\u9fa5]{2,4}?)${PERSON_NAME_BOUNDARY_PATTERN}`, "u")
      ]
    : [];
  explicitTransferPatterns.push(new RegExp(`[\\u4e00-\\u9fa5]{2,4}.{0,6}(?:转给|转至|转入|转|汇给|打给|给(?!出|我|出具))\\s*([\\u4e00-\\u9fa5]{2,4}?)${PERSON_NAME_BOUNDARY_PATTERN}`, "u"));
  for (const pattern of explicitTransferPatterns) {
    const match = q.match(pattern);
    const candidate = text(match?.[1]).replace(/[款账钱万千百十元]+$/u, "");
    if (
      candidate &&
      candidate !== holder &&
      candidate.length >= 2 &&
      candidate.length <= 4 &&
      !VIA_NAME_STOPWORDS.has(candidate) &&
      ![...VIA_NAME_STOPWORDS].some((word) => word.includes(candidate) || candidate.includes(word))
    ) {
      return candidate;
    }
  }
  for (const keyword of arrayOf(focusKeywords).map(text).filter(Boolean)) {
    if (keyword !== holder && /[\u4e00-\u9fa5]{2,}/u.test(keyword) && q.includes(keyword)) {
      return keyword;
    }
  }
  const candidates = [...q.matchAll(/[\u4e00-\u9fa5]{2,8}/gu)]
    .map((match) => match[0])
    .flatMap((chunk) => {
      const pieces = [chunk];
      if (chunk.length > 4) {
        for (let index = 0; index <= chunk.length - 2; index += 1) {
          pieces.push(chunk.slice(index, Math.min(chunk.length, index + 3)));
          pieces.push(chunk.slice(index, Math.min(chunk.length, index + 4)));
        }
      }
      return pieces;
    })
    .map(text)
    .filter((name) => name.length >= 2 && name.length <= 4)
    .filter((name) => name !== holder)
    .filter((name) => !holder || (!holder.includes(name) && !name.includes(holder)))
    .filter((name) => !VIA_NAME_STOPWORDS.has(name))
    .filter((name) => ![...VIA_NAME_STOPWORDS].some((word) => word.includes(name) || name.includes(word)));
  const scored = candidates.map((name) => {
    let score = 0;
    if (/[张王李刘陈杨赵黄周吴徐孙马朱胡郭何高林罗郑梁谢宋唐许邓冯韩曹曾彭萧蔡潘田董袁于余叶蒋杜苏魏程吕丁沈任姚卢姜崔钟谭陆汪范金石廖贾夏韦付侯熊戴白阳]/u.test(name[0])) score += 2;
    if (q.includes(`复核${name}`) || q.includes(`重点${name}`) || q.includes(`${name}收到`) || q.includes(`${name}又`)) score += 4;
    if (q.includes(`->${name}`) || q.includes(`到${name}`) || q.includes(`给${name}`)) score += 3;
    score += Math.min(name.length, 3);
    return { name, score, index: q.indexOf(name) };
  }).filter((item) => item.score >= 4);
  scored.sort((left, right) => right.score - left.score || right.index - left.index);
  return text(scored[0]?.name);
}

export function inferSourceHolderFromQuestion(question, { viaName = "" } = {}) {
  const q = text(question);
  const via = text(viaName);
  const beforeTransfer = q.match(new RegExp(`([\\u4e00-\\u9fa5]{2,4})\\s*(?:转给|转至|转入|转|汇给|打给|给(?!出|我|出具))\\s*([\\u4e00-\\u9fa5]{2,4}?)${PERSON_NAME_BOUNDARY_PATTERN}`, "u"));
  if (beforeTransfer) {
    const candidate = text(beforeTransfer[1]).replace(/[转汇打给入]+$/u, "");
    if (candidate && candidate !== via && !VIA_NAME_STOPWORDS.has(candidate)) return candidate;
  }
  return "";
}

export function inferDateStartFromQuestion(question) {
  const q = text(question);
  const fullDate = q.match(/(20\d{2})[-年./](\d{1,2})[-月./](\d{1,2})\s*(?:日)?\s*(?:以来|起|之后|后|至今)/u);
  const isNegatedWindow = (match) => {
    if (!match || typeof match.index !== "number") return false;
    const before = q.slice(Math.max(0, match.index - 28), match.index);
    const after = q.slice(match.index + match[0].length, match.index + match[0].length + 12);
    return /(不得|不要|不能|不可|不应|禁止|未限定|题目未限定|使用全期间|全期间)[^，。；\n]{0,16}(自行)?(缩成|限定|改成|当作|作为)?$/u.test(before)
      || /(窗口|口径)?[^，。；\n]{0,8}(不得|不要|不能|不可|不应|禁止)/u.test(after);
  };
  if (fullDate) {
    if (isNegatedWindow(fullDate)) return "";
    const [, year, month, day] = fullDate;
    return `${year}-${month.padStart(2, "0")}-${day.padStart(2, "0")}`;
  }
  const yearOnly = q.match(/(20\d{2})\s*年?\s*(?:以来|起|之后|后|至今)/u);
  if (yearOnly) {
    if (isNegatedWindow(yearOnly)) return "";
    return `${yearOnly[1]}-01-01`;
  }
  return "";
}

export function isViaContinuationQuestion(question) {
  const q = text(question);
  return /转给.{0,12}后|收到.{0,12}后|(?:又|后续|下游|去向|流向|穿透|再转|转给谁|给谁)/u.test(q);
}

export function isTopOutflowClassificationQuestion(question) {
  const q = text(question);
  return /(?:Top|top|前\s*\d+).{0,16}出账/u.test(q) && /终点|补调|分类|去向|流向/u.test(q);
}

export function isExplicitTransferAmountQuestion(question, { holderName = "", viaName = "" } = {}) {
  const q = text(question);
  const holder = text(holderName);
  const via = text(viaName);
  if (!q || !holder || !via) return false;
  const holderPattern = escapeRegExp(holder);
  const viaPattern = escapeRegExp(via);
  const explicitPair = new RegExp(
    `${holderPattern}.{0,24}(?:转给|给|转入|汇给|打给).{0,24}${viaPattern}|${viaPattern}.{0,24}(?:收到|收了|收款|入账).{0,24}${holderPattern}`,
    "u"
  ).test(q);
  const transferSignal = /(?:转给|给|转入|汇给|打给|收到|收款|入账)/u.test(q);
  const amountOrScopeSignal = /(?:多少|多少钱|金额|合计|总额|笔数|核心日期|核心集中日期|核心卡|同名对手|全期间|口径|\d+(?:\.\d+)?\s*(?:万|元))/u.test(q);
  return explicitPair && transferSignal && amountOrScopeSignal;
}

export function inferRankingTarget(args = {}) {
  const q = `${text(args.question)} ${text(args.intent)}`;
  if (/对手|对方|交易对手|收款|付款|给谁|转给谁|汇给谁|打给谁|流向谁|去向|资金去向|counterpart/u.test(q)) return "counterparties";
  if (/户名|主体|人员|人名|持有人|holder/u.test(q) && !/账户|账号|卡号|银行卡/u.test(q)) return "holders";
  return "accounts";
}

export function inferRankingMetric(args = {}) {
  const explicitMetric = text(args.metric || args.metric_mode || args.metricMode);
  if (["inflow", "outflow", "turnover", "txn_count", "max_single_amount"].includes(explicitMetric)) return explicitMetric;
  const q = text(args.question);
  if (/最大单笔|单笔最大/u.test(q)) return "max_single_amount";
  if (/入账|收入|流入|进账/u.test(q)) return "inflow";
  if (/出账|支出|流出|转出/u.test(q)) return "outflow";
  if (/笔数|次数|交易次数/u.test(q)) return "txn_count";
  return "turnover";
}

export function inferRankingMetrics(args = {}) {
  const q = text(args.question);
  const explicitMetric = text(args.metric || args.metric_mode || args.metricMode);
  if (["inflow", "outflow", "turnover", "txn_count", "max_single_amount"].includes(explicitMetric)) return [explicitMetric];
  if (!q && text(args.intent) === "ranking") {
    return ["turnover", "outflow", "inflow", "txn_count", "max_single_amount"];
  }
  const metrics = [];
  const add = (metric) => {
    if (!metrics.includes(metric)) metrics.push(metric);
  };
  if (/往来|总额|交易量|资金最大|最大账户|资金规模|流水规模/u.test(q)) add("turnover");
  if (/出账|支出|流出|转出/u.test(q)) add("outflow");
  if (/入账|收入|流入|进账/u.test(q)) add("inflow");
  if (/笔数|次数|交易次数/u.test(q)) add("txn_count");
  if (/最大单笔|单笔最大/u.test(q)) add("max_single_amount");
  if (!metrics.length) add(inferRankingMetric(args));
  return metrics.slice(0, 5);
}

export function directionModeForMetric(metric) {
  if (metric === "inflow") return "in";
  if (metric === "outflow") return "out";
  return "both";
}

export function rankingSkillForTarget(target) {
  if (target === "holders") return "rank_holders";
  if (target === "counterparties") return "rank_counterparties";
  return "rank_accounts";
}
