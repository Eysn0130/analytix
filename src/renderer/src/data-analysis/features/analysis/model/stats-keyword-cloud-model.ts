import { projectOrdinaryFieldValue } from "../../shared/ordinary-pii-projection";
import { readNonNegativeCaseFactInteger } from "./stats-case-fact-number-model";

export const MAX_REMARK_KEYWORD_CLOUD_ITEMS = 160;

export interface RemarkKeywordCloudItem {
  label: string;
  count: number;
}

export function prepareRemarkKeywordCloudItems(rawItems: unknown[]): RemarkKeywordCloudItem[] {
  return (Array.isArray(rawItems) ? rawItems : [])
    .flatMap((raw): RemarkKeywordCloudItem[] => {
      const item = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
      const label = projectOrdinaryFieldValue("remark_keyword", item.label);
      const count = readNonNegativeCaseFactInteger(item.count);
      return label && count != null ? [{ label, count }] : [];
    })
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label, "zh-CN"))
    .slice(0, MAX_REMARK_KEYWORD_CLOUD_ITEMS);
}
