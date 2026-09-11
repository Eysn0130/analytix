/**
 * analytix identity reset starts from a fresh localStorage namespace. Legacy
 * desktop keys are intentionally not copied automatically; user data import
 * remains an explicit legacy import flow.
 */
export function migrateLegacyLocalStorageKeys(storage: Pick<Storage, 'length' | 'key' | 'getItem' | 'setItem'>): number {
  void storage
  return 0
}

// import 即执行(renderer 入口的第一行 import 就是这个模块)。
if (typeof window !== 'undefined' && window.localStorage) {
  try {
    migrateLegacyLocalStorageKeys(window.localStorage)
  } catch {
    // localStorage 不可用(隐私模式等):静默放弃,UI 退化为默认状态。
  }
}
