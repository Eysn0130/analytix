export interface StatsTreeSelectionItem {
  id?: string;
}

export interface StatsTreeSelectionGroup {
  id?: string;
  itemKeys?: string[];
  items?: StatsTreeSelectionItem[];
}

function normalizeKey(value: unknown): string {
  return String(value || "").trim();
}

export function uniqueStatsTreeKeys(values: unknown[]): string[] {
  return Array.from(new Set(values.map(normalizeKey).filter(Boolean)));
}

export function sameStatsTreeKeys(left: string[], right: string[]): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) {
      return false;
    }
  }
  return true;
}

export function getStatsTreeGroupAccountKeys(group: StatsTreeSelectionGroup): string[] {
  const itemKeys = Array.isArray(group.itemKeys) ? group.itemKeys : [];
  if (itemKeys.length) {
    return uniqueStatsTreeKeys(itemKeys);
  }
  const items = Array.isArray(group.items) ? group.items : [];
  return uniqueStatsTreeKeys(items.map((item) => item.id));
}

export function addExpandedStatsTreeGroup(prev: string[], groupId: string): string[] {
  const id = normalizeKey(groupId);
  if (!id || prev.includes(id)) {
    return prev;
  }
  return [...prev, id];
}

export function toggleExpandedStatsTreeGroup(prev: string[], groupId: string): string[] {
  const id = normalizeKey(groupId);
  if (!id) {
    return prev;
  }
  const next = new Set(prev);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  return Array.from(next);
}

export function canCollapseStatsTreeGroup(group: StatsTreeSelectionGroup | undefined, selectedAccounts: Set<string>): boolean {
  if (!group) {
    return true;
  }
  return !getStatsTreeGroupAccountKeys(group).some((key) => selectedAccounts.has(key));
}

export function toggleStatsTreeAccountSelection(prev: string[], accountKey: string): string[] {
  const key = normalizeKey(accountKey);
  if (!key) {
    return prev;
  }
  const next = new Set(prev);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  const nextValues = Array.from(next);
  return sameStatsTreeKeys(prev, nextValues) ? prev : nextValues;
}

export function selectStatsTreeGroupAccounts(prev: string[], group: StatsTreeSelectionGroup, checked: boolean): string[] {
  const keys = getStatsTreeGroupAccountKeys(group);
  if (!keys.length) {
    return prev;
  }
  const next = new Set(prev);
  keys.forEach((key) => {
    if (checked) {
      next.add(key);
    } else {
      next.delete(key);
    }
  });
  const nextValues = Array.from(next);
  return sameStatsTreeKeys(prev, nextValues) ? prev : nextValues;
}

export function selectVisibleStatsTreeAccounts(groups: StatsTreeSelectionGroup[]): {
  expandedGroupIds: string[];
  selectedAccounts: string[];
} {
  const expandedGroupIds: string[] = [];
  const selectedAccounts: string[] = [];
  groups.forEach((group) => {
    const groupId = normalizeKey(group.id);
    if (groupId) {
      expandedGroupIds.push(groupId);
    }
    selectedAccounts.push(...getStatsTreeGroupAccountKeys(group));
  });
  return {
    expandedGroupIds: uniqueStatsTreeKeys(expandedGroupIds),
    selectedAccounts: uniqueStatsTreeKeys(selectedAccounts),
  };
}

export function selectDefaultStatsTreeAccounts(groups: StatsTreeSelectionGroup[]): {
  expandedGroupIds: string[];
  selectedAccounts: string[];
} {
  const group = groups.find((item) => getStatsTreeGroupAccountKeys(item).length > 0);
  if (!group) {
    return {
      expandedGroupIds: [],
      selectedAccounts: [],
    };
  }
  const groupId = normalizeKey(group.id);
  return {
    expandedGroupIds: groupId ? [groupId] : [],
    selectedAccounts: getStatsTreeGroupAccountKeys(group),
  };
}

export function countSelectedStatsTreeGroups(groups: StatsTreeSelectionGroup[], selectedAccounts: Set<string>): number {
  let count = 0;
  groups.forEach((group) => {
    if (getStatsTreeGroupAccountKeys(group).some((key) => selectedAccounts.has(key))) {
      count += 1;
    }
  });
  return count;
}
