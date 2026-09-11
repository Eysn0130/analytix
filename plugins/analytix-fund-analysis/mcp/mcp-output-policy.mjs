export const MCP_OUTPUT_POLICY_VERSION = "0.14.3-mcp-output-policy";

export const MCP_OUTPUT_POLICY_GUARDS = [
  "answer_card_only",
  "structuredContent_disabled_by_default",
  "raw_json_never_model_context_by_default",
  "restricted_pii_projection_required",
  "debug_payload_cannot_bypass_policy"
];

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

export function debugPayloadAllowedFromEnv(env = {}) {
  void env;
  return false;
}

export function structuredContentAllowed(args = {}, env = {}) {
  void args;
  void env;
  return false;
}

export function buildStructuredContentPolicy(args = {}, env = {}) {
  const requested = objectOf(args).include_debug === true;
  const allowed = structuredContentAllowed(args, env);
  return {
    version: MCP_OUTPUT_POLICY_VERSION,
    model_context: "answer_card_only",
    structured_content_policy: "structuredContent_disabled_fail_closed",
    raw_result_policy: "raw_json_never_model_context; restricted PII projection required before any public surface",
    include_debug_requested: requested,
    debug_env_required: "disabled",
    structured_content_allowed: allowed
  };
}
