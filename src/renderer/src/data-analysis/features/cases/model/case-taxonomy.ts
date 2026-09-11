export type CaseTypeGroupKey =
  | "public_security_general"
  | "economic_crime_focus"
  | "supervision_compatible";

export type CaseTagGroupKey =
  | "fund_flow"
  | "subject_account"
  | "business_scene"
  | "evidence_supervision";

export type CaseAnalysisSkeletonKey =
  | "fraud"
  | "fundraising_pyramid"
  | "tax"
  | "anti_money_laundering"
  | "internal_embezzlement"
  | "securities_futures"
  | "supervision_duty_crime";

export interface CaseTypeGroupDefinition {
  key: CaseTypeGroupKey;
  label: string;
  types: string[];
}

export interface CaseTagGroupDefinition {
  key: CaseTagGroupKey;
  label: string;
  tags: string[];
}

export interface CaseAnalysisSkeletonDefinition {
  key: CaseAnalysisSkeletonKey;
  label: string;
}

const CASE_TYPE_ALIASES: Record<string, string> = {
  "贷款黑灰产": "贷款黑灰产 / 帮信通道",
  "帮信通道": "贷款黑灰产 / 帮信通道",
  "虚拟币": "虚拟币 / 链上资金",
  "链上资金": "虚拟币 / 链上资金",
  "涉税骗税": "涉税犯罪",
  "涉税": "涉税犯罪",
  "商贸领域": "商贸欺诈",
};

const CASE_TAG_ALIAS_OVERRIDES: Record<string, string> = {};

export const CASE_TYPE_OPTIONS = [
  "非法集资",
  "合同诈骗",
  "职务侵占",
  "挪用资金",
  "贷款黑灰产 / 帮信通道",
  "洗钱",
  "地下钱庄",
  "虚拟币 / 链上资金",
  "涉税犯罪",
  "保险诈骗",
  "传销",
  "证券期货",
  "商贸欺诈",
  "非法经营",
  "监察委案件",
  "贪污贿赂（兼监察）",
  "国企职务犯罪 / 工程招投标（兼监察）",
  "其他",
] as const;

export const CASE_TYPE_GROUPS: CaseTypeGroupDefinition[] = [
  {
    key: "public_security_general",
    label: "公安通用",
    types: [
      "洗钱",
      "地下钱庄",
      "虚拟币 / 链上资金",
      "非法经营",
      "其他",
    ],
  },
  {
    key: "economic_crime_focus",
    label: "经侦重点",
    types: [
      "非法集资",
      "合同诈骗",
      "职务侵占",
      "挪用资金",
      "贷款黑灰产 / 帮信通道",
      "涉税犯罪",
      "保险诈骗",
      "传销",
      "证券期货",
      "商贸欺诈",
    ],
  },
  {
    key: "supervision_compatible",
    label: "监察兼容",
    types: [
      "监察委案件",
      "贪污贿赂（兼监察）",
      "国企职务犯罪 / 工程招投标（兼监察）",
    ],
  },
];

export const CASE_TAG_GROUPS: CaseTagGroupDefinition[] = [
  {
    key: "fund_flow",
    label: "通用资金标签",
    tags: [
      "资金穿透",
      "异常流水",
      "资金池",
      "资金回流",
      "公转私",
      "私转公",
      "多级分流",
      "洗钱链路",
    ],
  },
  {
    key: "subject_account",
    label: "主体账户标签",
    tags: [
      "对公账户",
      "境外账户",
      "空壳公司",
      "关联公司",
      "实控人",
      "受益人",
      "归集账户",
      "过渡账户",
    ],
  },
  {
    key: "business_scene",
    label: "场景业务标签",
    tags: [
      "跑分卡",
      "第三方支付",
      "POS机",
      "USDT",
      "OTC",
      "币商",
      "发票",
      "票货分离",
      "四流不一致",
      "涉众",
    ],
  },
  {
    key: "evidence_supervision",
    label: "证据 / 监察兼容标签",
    tags: [
      "电子证据",
      "利益输送",
      "招投标",
    ],
  },
];

export const CASE_TAG_PRESETS = CASE_TAG_GROUPS.flatMap((group) => group.tags);

export const CASE_ANALYSIS_SKELETONS: Record<
  CaseAnalysisSkeletonKey,
  CaseAnalysisSkeletonDefinition
> = {
  fraud: { key: "fraud", label: "涉诈骨架" },
  fundraising_pyramid: {
    key: "fundraising_pyramid",
    label: "非法集资 / 传销骨架",
  },
  tax: { key: "tax", label: "涉税骨架" },
  anti_money_laundering: {
    key: "anti_money_laundering",
    label: "反洗钱骨架",
  },
  internal_embezzlement: {
    key: "internal_embezzlement",
    label: "企业内部侵财骨架",
  },
  securities_futures: {
    key: "securities_futures",
    label: "证券期货骨架",
  },
  supervision_duty_crime: {
    key: "supervision_duty_crime",
    label: "监察职务犯罪骨架",
  },
};

export const CASE_ANALYSIS_SKELETON_BY_TYPE: Partial<
  Record<string, CaseAnalysisSkeletonKey>
> = {
  "合同诈骗": "fraud",
  "贷款黑灰产 / 帮信通道": "fraud",
  "保险诈骗": "fraud",
  "商贸欺诈": "fraud",
  "非法集资": "fundraising_pyramid",
  "传销": "fundraising_pyramid",
  "涉税犯罪": "tax",
  "洗钱": "anti_money_laundering",
  "地下钱庄": "anti_money_laundering",
  "虚拟币 / 链上资金": "anti_money_laundering",
  "非法经营": "anti_money_laundering",
  "职务侵占": "internal_embezzlement",
  "挪用资金": "internal_embezzlement",
  "证券期货": "securities_futures",
  "监察委案件": "supervision_duty_crime",
  "贪污贿赂（兼监察）": "supervision_duty_crime",
  "国企职务犯罪 / 工程招投标（兼监察）": "supervision_duty_crime",
};

export const CASE_TAG_RECOMMENDATIONS_BY_TYPE: Record<string, string[]> = {
  "非法集资": [
    "涉众",
    "资金池",
    "异常流水",
    "对公账户",
    "归集账户",
    "资金回流",
  ],
  "合同诈骗": [
    "对公账户",
    "空壳公司",
    "关联公司",
    "异常流水",
    "电子证据",
    "资金回流",
  ],
  "职务侵占": [
    "资金穿透",
    "异常流水",
    "受益人",
    "关联公司",
    "对公账户",
    "电子证据",
  ],
  "挪用资金": [
    "资金穿透",
    "对公账户",
    "过渡账户",
    "资金回流",
    "异常流水",
    "受益人",
  ],
  "贷款黑灰产 / 帮信通道": [
    "跑分卡",
    "对公账户",
    "资金池",
    "第三方支付",
    "异常流水",
    "多级分流",
  ],
  "洗钱": [
    "洗钱链路",
    "异常流水",
    "对公账户",
    "境外账户",
    "公转私",
    "私转公",
    "多级分流",
  ],
  "地下钱庄": [
    "境外账户",
    "对公账户",
    "洗钱链路",
    "第三方支付",
    "公转私",
    "私转公",
    "异常流水",
  ],
  "虚拟币 / 链上资金": [
    "USDT",
    "OTC",
    "币商",
    "境外账户",
    "洗钱链路",
    "资金穿透",
    "多级分流",
  ],
  "涉税犯罪": [
    "发票",
    "空壳公司",
    "资金穿透",
    "对公账户",
    "票货分离",
    "四流不一致",
  ],
  "保险诈骗": [
    "电子证据",
    "异常流水",
    "对公账户",
    "关联公司",
    "受益人",
  ],
  "传销": [
    "涉众",
    "资金池",
    "对公账户",
    "异常流水",
    "资金回流",
    "多级分流",
  ],
  "证券期货": [
    "对公账户",
    "异常流水",
    "电子证据",
    "境外账户",
    "关联公司",
    "受益人",
  ],
  "商贸欺诈": [
    "对公账户",
    "关联公司",
    "空壳公司",
    "异常流水",
    "电子证据",
    "资金回流",
  ],
  "非法经营": [
    "对公账户",
    "第三方支付",
    "POS机",
    "境外账户",
    "异常流水",
    "公转私",
  ],
  "监察委案件": [
    "招投标",
    "利益输送",
    "资金回流",
    "对公账户",
    "关联公司",
    "受益人",
  ],
  "贪污贿赂（兼监察）": [
    "资金穿透",
    "对公账户",
    "关联公司",
    "受益人",
    "利益输送",
    "异常流水",
  ],
  "国企职务犯罪 / 工程招投标（兼监察）": [
    "招投标",
    "对公账户",
    "关联公司",
    "利益输送",
    "电子证据",
    "资金回流",
  ],
  "其他": [
    "资金穿透",
    "异常流水",
    "电子证据",
  ],
};

export const QUICK_CASE_TYPE_OPTIONS = [
  "非法集资",
  "合同诈骗",
  "洗钱",
  "虚拟币 / 链上资金",
];

export const QUICK_TAG_PRESETS = [
  "资金穿透",
  "异常流水",
  "对公账户",
  "电子证据",
];

const CASE_TAG_SET = new Set<string>(CASE_TAG_PRESETS);

function normalizeCaseTag(value: string): string {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  return CASE_TAG_ALIAS_OVERRIDES[trimmed] ?? trimmed;
}

export function normalizeCaseType(value: string): string {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  const aliased = CASE_TYPE_ALIASES[trimmed] ?? trimmed;
  return aliased;
}

export function normalizeCaseTags(
  value: readonly string[] | string | null | undefined,
): string[] {
  const source = Array.isArray(value)
    ? value
    : String(value || "")
        .replace(/，/g, ",")
        .replace(/;/g, ",")
        .replace(/；/g, ",")
        .split(",");

  const dedup = new Set<string>();
  source.forEach((item) => {
    const normalized = normalizeCaseTag(String(item || ""));
    if (normalized && dedup.size < 32) {
      dedup.add(normalized);
    }
  });
  return Array.from(dedup);
}

export function getRecommendedCaseTags(caseType: string): string[] {
  const canonicalType = normalizeCaseType(caseType);
  return CASE_TAG_RECOMMENDATIONS_BY_TYPE[canonicalType] ?? [];
}

export function resolveCaseAnalysisSkeletonKey(
  caseType: string,
): CaseAnalysisSkeletonKey | null {
  const canonicalType = normalizeCaseType(caseType);
  return CASE_ANALYSIS_SKELETON_BY_TYPE[canonicalType] ?? null;
}

export function resolveCaseAnalysisSkeletonLabel(
  caseType: string,
): string {
  const skeletonKey = resolveCaseAnalysisSkeletonKey(caseType);
  return skeletonKey ? CASE_ANALYSIS_SKELETONS[skeletonKey].label : "";
}

export function isKnownCaseTag(tag: string): boolean {
  return CASE_TAG_SET.has(normalizeCaseTag(tag));
}
