export function canonicalNonnegativeCount(value: unknown): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    return null;
  }
  return value;
}

export function canonicalProgress(value: unknown): number | null {
  const progress = canonicalNonnegativeCount(value);
  return progress !== null && progress <= 100 ? progress : null;
}

export function sumCompleteCounts(values: ReadonlyArray<number | null>): number | null {
  if (values.length === 0 || values.some((value) => value === null)) {
    return null;
  }
  let total = 0;
  for (const value of values) {
    if (value === null || total > Number.MAX_SAFE_INTEGER - value) {
      return null;
    }
    total += value;
  }
  return total;
}

export function reconcileEquivalentCounts(left: number | null, right: number | null): number | null {
  if (left !== null && right !== null && left !== right) {
    return null;
  }
  return left ?? right;
}
