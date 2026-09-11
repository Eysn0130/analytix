export interface StatsSelectableRow {
  id: string;
}

export interface StatsVisibleRowSelection<T extends StatsSelectableRow> {
  allSelected: boolean;
  rows: T[];
}

export function buildStatsVisibleRowSelection<T extends StatsSelectableRow>(
  visibleRows: T[],
  selectedRowIds: Set<string> | string[]
): StatsVisibleRowSelection<T> {
  const selectedCount = selectedRowIds instanceof Set ? selectedRowIds.size : selectedRowIds.length;
  if (!visibleRows.length || !selectedCount) {
    return {
      allSelected: false,
      rows: [],
    };
  }

  const selectedRowSet = selectedRowIds instanceof Set ? selectedRowIds : new Set(selectedRowIds);
  const rows: T[] = [];
  for (const row of visibleRows) {
    if (selectedRowSet.has(row.id)) {
      rows.push(row);
    }
  }

  return {
    allSelected: rows.length === visibleRows.length,
    rows,
  };
}
