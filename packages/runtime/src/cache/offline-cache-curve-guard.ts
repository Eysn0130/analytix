export type OfflineCacheCurveGuardCase = {
  id: string
  cacheHitPercentCurve: readonly number[]
  compactionGuardPaused?: boolean
  maxAllowedCollapses?: number
}

export type OfflineCacheCurveGuardInput = {
  thresholdPercent: number
  maxLowTailCases: number
  tailWindow: number
  cases: readonly OfflineCacheCurveGuardCase[]
}

export type OfflineCacheCurveGuardCaseResult = {
  id: string
  tailAveragePercent: number
  status: 'pass' | 'fail'
  collapseCount: number
  compactionGuardPaused: boolean
}

export type OfflineCacheCurveGuardResult = {
  fixtureOnly: true
  status: 'pass' | 'fail'
  lowTailCases: number
  cases: OfflineCacheCurveGuardCaseResult[]
}

export function evaluateOfflineCacheCurveGuard(
  input: OfflineCacheCurveGuardInput
): OfflineCacheCurveGuardResult {
  const cases = input.cases.map((testCase) => {
    const tailAveragePercent = tailAverage(testCase.cacheHitPercentCurve, input.tailWindow)
    const lowTail = tailAveragePercent < input.thresholdPercent
    const collapses = collapseCount(testCase.cacheHitPercentCurve)
    const collapseFailed = testCase.maxAllowedCollapses !== undefined &&
      collapses > testCase.maxAllowedCollapses
    return {
      id: testCase.id,
      tailAveragePercent,
      status: collapseFailed || lowTail ? 'fail' as const : 'pass' as const,
      collapseCount: collapses,
      compactionGuardPaused: testCase.compactionGuardPaused === true
    }
  })
  const lowTailCases = cases.filter((testCase) => testCase.tailAveragePercent < input.thresholdPercent).length
  const failed = input.maxLowTailCases !== 0 ||
    cases.some((testCase) => testCase.status === 'fail') || lowTailCases !== 0
  return {
    fixtureOnly: true,
    status: failed ? 'fail' : 'pass',
    lowTailCases,
    cases
  }
}

function tailAverage(values: readonly number[], windowSize: number): number {
  const tail = values.slice(Math.max(0, values.length - windowSize))
  if (tail.length === 0) return 0
  return tail.reduce((total, value) => total + value, 0) / tail.length
}

function collapseCount(values: readonly number[]): number {
  let collapses = 0
  for (let index = 1; index < values.length; index += 1) {
    if ((values[index] ?? 0) < (values[index - 1] ?? 0)) collapses += 1
  }
  return collapses
}
