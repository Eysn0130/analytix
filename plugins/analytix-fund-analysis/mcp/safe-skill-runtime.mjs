import { redactAgentPayload } from "./agent-context-hygiene.mjs";
import {
  envelopeWarnings,
  evidenceRefsFromEnvelope,
  skillEnvelopeData,
  unwrapSkillEnvelope
} from "./agent-payload-compiler.mjs";
import {
  executeLocalDuckdbWorkbenchSkill,
  supportsLocalDuckdbWorkbenchSkill
} from "./duckdb-workbench-runtime.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";

export const SAFE_SKILL_RUNTIME_VERSION = "0.14.4";

export function createSafeSkillRuntime({ executeSkill, env = {} }) {
  async function safeSkill(skillId, input, { signal } = {}) {
    throwIfAborted(signal);
    try {
      const result = await executeSkill(skillId, input, { signal });
      throwIfAborted(signal);
      const envelope = unwrapSkillEnvelope(result);
      return {
        ok: true,
        skill_id: skillId,
        input: redactAgentPayload(input),
        envelope: redactAgentPayload(envelope),
        data: redactAgentPayload(skillEnvelopeData(result)),
        warnings: redactAgentPayload(envelopeWarnings(envelope)),
        evidence_refs: redactAgentPayload(evidenceRefsFromEnvelope(envelope, result))
      };
    } catch (error) {
      throwIfAborted(signal);
      if (supportsLocalDuckdbWorkbenchSkill(skillId)) {
        try {
          const result = await executeLocalDuckdbWorkbenchSkill(skillId, input, { env, signal });
          throwIfAborted(signal);
          const envelope = unwrapSkillEnvelope(result);
          return {
            ok: true,
            skill_id: skillId,
            input: redactAgentPayload(input),
            envelope: redactAgentPayload(envelope),
            data: redactAgentPayload(skillEnvelopeData(result)),
            warnings: [
              ...redactAgentPayload(envelopeWarnings(envelope)),
              {
                code: "LOCAL_DUCKDB_SEMANTIC_FALLBACK",
                severity: "warning",
                message: "后端语义服务不可用；本次只接受当前案件 DuckDB 的受约束本地结果，未发布上游错误文本。"
              }
            ],
            evidence_refs: redactAgentPayload(evidenceRefsFromEnvelope(envelope, result))
          };
        } catch (localError) {
          throwIfAborted(signal);
          return {
            ok: false,
            skill_id: skillId,
            input: redactAgentPayload(input),
            envelope: {},
            data: {},
            warnings: [
              {
                code: "CASEGRAPH_CHILD_TOOL_FAILED",
                severity: "warning",
                message: "上游语义技能执行失败；未发布其原始错误文本。"
              },
              {
                code: "LOCAL_DUCKDB_SEMANTIC_FALLBACK_FAILED",
                severity: "warning",
                message: "当前案件 DuckDB 受约束本地执行失败；未发布其原始错误文本。"
              }
            ],
            evidence_refs: {}
          };
        }
      }
      return {
        ok: false,
        skill_id: skillId,
        input: redactAgentPayload(input),
        envelope: {},
        data: {},
        warnings: [
          {
            code: "CASEGRAPH_CHILD_TOOL_FAILED",
            severity: "warning",
            message: "上游语义技能执行失败；未发布其原始错误文本。"
          }
        ],
        evidence_refs: {}
      };
    }
  }

  return { safeSkill };
}
