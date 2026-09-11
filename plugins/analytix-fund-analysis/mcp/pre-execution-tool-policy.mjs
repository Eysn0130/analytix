export const PRE_EXECUTION_BLOCKED_TOOL_NAMES = Object.freeze([
  "create_case_notebook",
  "export_cleaned_case_data",
  "run_full_case_analysis"
]);

const blockedToolNames = new Set(PRE_EXECUTION_BLOCKED_TOOL_NAMES);

export function isPreExecutionBlockedToolName(name) {
  return blockedToolNames.has(String(name || "").trim());
}
