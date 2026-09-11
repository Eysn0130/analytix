import type { StatsTreeGroupDTO, StatsTreeItemDTO, StatsTreeTab } from "./stats-shared";

const TREE_BANK_PLACEHOLDERS = new Set(["", "未录入", "未录入归属信息", "未录入归属行"]);
const TREE_BANK_DISPLAY_ALIASES: Array<[string, string]> = [
  ["中国邮政储蓄银行", "邮政储蓄银行"],
  ["中国工商银行", "工商银行"],
  ["中国建设银行", "建设银行"],
  ["中国农业银行", "农业银行"],
  ["中国民生银行", "民生银行"],
  ["中国光大银行", "光大银行"],
  ["中国银行", "中国银行"],
  ["上海浦东发展银行", "上海浦东发展银行"],
  ["招商银行", "招商银行"],
  ["交通银行", "交通银行"],
  ["兴业银行", "兴业银行"],
  ["华夏银行", "华夏银行"],
  ["广发银行", "广发银行"],
  ["中信银行", "中信银行"],
  ["平安银行", "平安银行"],
  ["重庆银行", "重庆银行"],
  ["贵州银行", "贵州银行"],
  ["贵阳银行", "贵阳银行"]
];
const TREE_BANK_NAME_SUFFIXES = ["股份有限公司", "有限责任公司", "股份公司", "股份", "有限公司"];

const TREE_ACCOUNT_TYPE_PLACEHOLDERS = new Set(["", "-", "—", "－", "未知", "未录入", "未录入账户类型"]);
const TREE_ACCOUNT_TYPE_EXACT_ALIASES = new Map<string, string>([
  ["借记卡", "借记卡"],
  ["个人储蓄账户", "储蓄"],
  ["零售活期结算账户", "活期结算"],
  ["个人人民币活期普通结算账户", "活期结算"],
  ["活期", "活期"],
  ["活期存款", "活期"],
  ["活期无折户", "活期"],
  ["活期多币种", "活期"],
  ["活期储蓄存款", "活期"],
  ["活期一本通", "活期"],
  ["定期", "定期"],
  ["定期一本通", "定期"],
  ["存单", "定期"],
  ["信用卡", "信用卡"],
  ["信用卡主卡", "信用卡"],
  ["贷记卡", "信用卡"],
  ["实体账户", "实体账户"],
  ["卡", "卡账户"],
  ["薪金煲", "薪金煲"]
]);

function splitMetaParts(text: string): string[] {
  return String(text || "")
    .split("·")
    .map((part) => part.trim())
    .filter(Boolean);
}

function shortenTreeBankRegion(text: string): string {
  let value = String(text || "").trim();
  for (const suffix of ["自治区", "自治州", "地区", "省", "市", "县", "区", "州", "盟"]) {
    if (value.endsWith(suffix) && value.length > suffix.length) {
      value = value.slice(0, -suffix.length).trim();
      break;
    }
  }
  return value;
}

export function normalizeStatsTreeBankName(text: string): string {
  let value = String(text || "").trim();
  if (!value) {
    return "";
  }

  value = value.replace(/（[^）]*）/g, "").replace(/\([^)]*\)/g, "").replace(/\s+/g, "").trim();
  if (TREE_BANK_PLACEHOLDERS.has(value)) {
    return "";
  }

  const unionMatch = value.match(/^(.+?)农村信用社联合社$/);
  if (unionMatch) {
    const region = shortenTreeBankRegion(unionMatch[1]);
    return region ? `${region}农信` : "农信";
  }

  const coopMatch = value.match(/^(.+?)农村信用合作联社$/);
  if (coopMatch) {
    const region = shortenTreeBankRegion(coopMatch[1]);
    return region ? `${region}农信联社` : "农信联社";
  }

  for (const [needle, normalized] of TREE_BANK_DISPLAY_ALIASES) {
    if (needle && value.includes(needle)) {
      return normalized;
    }
  }

  let normalizedValue = value;
  let changed = true;
  while (changed && normalizedValue) {
    changed = false;
    for (const suffix of TREE_BANK_NAME_SUFFIXES) {
      if (normalizedValue.endsWith(suffix)) {
        normalizedValue = normalizedValue.slice(0, -suffix.length).trim();
        changed = true;
      }
    }
  }

  return TREE_BANK_PLACEHOLDERS.has(normalizedValue) ? "" : normalizedValue;
}

function looksLikeStatsTreeBankName(text: string): boolean {
  const value = String(text || "").trim();
  if (!value) {
    return false;
  }
  if (TREE_BANK_PLACEHOLDERS.has(value)) {
    return true;
  }
  return /银行|信用社|联社/.test(value);
}

function normalizeStatsTreeAccountLevel(text: string): string {
  const value = String(text || "").trim();
  if (!value) {
    return "";
  }
  if (/(III|Ⅲ|三)类/i.test(value)) {
    return "III类";
  }
  if (/(II|Ⅱ|二)类/i.test(value)) {
    return "II类";
  }
  if (/(^|[^A-Za-z])(I|Ⅰ|一)类/i.test(value)) {
    return "I类";
  }
  return "";
}

export function normalizeStatsTreeAccountType(text: string): string {
  const raw = String(text || "").trim();
  if (!raw) {
    return "";
  }

  const value = raw.replace(/\s+/g, "").trim();
  if (TREE_ACCOUNT_TYPE_PLACEHOLDERS.has(value)) {
    return "";
  }

  const aliased = TREE_ACCOUNT_TYPE_EXACT_ALIASES.get(value);
  if (aliased != null) {
    return aliased;
  }

  const level = normalizeStatsTreeAccountLevel(value);
  if (level && /^(I|II|III|Ⅰ|Ⅱ|Ⅲ|一|二|三)类账户$/i.test(value)) {
    return level;
  }
  if (/^[0-9A-Za-z]{1,6}$/.test(value)) {
    return "";
  }

  let base = "";
  if (/贷记卡|信用卡/.test(value)) {
    base = "信用卡";
  } else if (value.includes("结算账户")) {
    base = "活期结算";
  } else if (/储蓄账户|储蓄存款/.test(value)) {
    base = "储蓄";
  } else if (/活期|无折户|一本通/.test(value)) {
    base = "活期";
  } else if (/定期|存单/.test(value)) {
    base = "定期";
  } else if (value.includes("借记卡")) {
    base = "借记卡";
  } else if (value.endsWith("卡")) {
    base = "卡账户";
  } else if (value.includes("实体账户")) {
    base = "实体账户";
  } else if (value.includes("薪金煲")) {
    base = "薪金煲";
  } else {
    base = value;
  }

  if (level) {
    return base && base !== level ? `${base} · ${level}` : level;
  }
  return base;
}

function looksLikeStatsTreeAccountType(text: string): boolean {
  const value = String(text || "").trim();
  if (!value) {
    return false;
  }
  if (TREE_ACCOUNT_TYPE_PLACEHOLDERS.has(value)) {
    return true;
  }
  if (TREE_ACCOUNT_TYPE_EXACT_ALIASES.has(value)) {
    return true;
  }
  return /账户|信用卡|贷记卡|借记卡|活期|定期|存单|储蓄|结算|一本通|薪金煲|^[0-9A-Za-z]{1,6}$/.test(value);
}

export function normalizeStatsTreeMetaPart(text: string): string {
  if (looksLikeStatsTreeBankName(text)) {
    return normalizeStatsTreeBankName(text);
  }
  if (looksLikeStatsTreeAccountType(text)) {
    return normalizeStatsTreeAccountType(text);
  }
  return String(text || "").trim();
}

export function normalizeStatsTreeGroups(tab: StatsTreeTab, groups: StatsTreeGroupDTO[]): StatsTreeGroupDTO[] {
  return (Array.isArray(groups) ? groups : []).map((group) => {
    const rawTitle = String(group.title || "");
    return {
      id: String(group.id || ""),
      title: tab === "byCard" ? normalizeStatsTreeBankName(rawTitle) || rawTitle : rawTitle,
      meta: String(group.meta || ""),
      extra: String(group.extra || ""),
      items: Array.isArray(group.items)
        ? group.items.map((item) => ({
            id: String(item.id || ""),
            title: String(item.title || ""),
            sub: String(item.sub || "")
          }))
        : []
    };
  });
}

export function getStatsTreeGroupSecondaryText(
  tab: StatsTreeTab,
  group: Pick<StatsTreeGroupDTO, "meta" | "extra">
): string {
  if (tab === "byName") {
    return String(group.meta || "").trim() || "未录入证件号";
  }
  return String(group.extra || group.meta || "").trim() || "未录入开户信息";
}

export function isStatsTreeAccountLikeGroup(
  tab: StatsTreeTab,
  group: Pick<StatsTreeGroupDTO, "title" | "meta">
): boolean {
  if (tab === "byCard") {
    return true;
  }

  const meta = String(group.meta || "").trim();
  if (meta === "未登记户名") {
    return true;
  }

  const title = String(group.title || "").trim();
  if (!title) {
    return false;
  }
  const hasHumanNameChars = /[\p{Script=Han}A-Za-z]/u.test(title);
  const digitCount = title.replace(/\D/g, "").length;
  return !hasHumanNameChars && digitCount >= 8;
}

export function getStatsTreeItemPrimaryText(
  item: Pick<StatsTreeItemDTO, "id" | "title">,
  tab: StatsTreeTab
): string {
  const primary = String(item.title || item.id || "").trim();
  if (primary) {
    return primary;
  }
  return tab === "byCard" ? "未知银行卡号" : "未命名对象";
}

export function getStatsTreeItemSecondaryText(item: Pick<StatsTreeItemDTO, "id" | "sub">): string {
  const parts = splitMetaParts(String(item.sub || ""));
  const displayParts = parts.map((part) => normalizeStatsTreeMetaPart(part)).filter(Boolean);
  return displayParts.join(" / ");
}
