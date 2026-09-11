export const PUBLICATION_RECEIPT_REQUIRED_CODE = "PUBLICATION_RECEIPT_REQUIRED";

export const PUBLICATION_RECEIPT_REQUIRED_TEXT = "正式报告未发布：当前结果没有通过宿主权威 PublicationReceipt registry 核验。仅可说明报告发布能力缺口，并继续逐项核验证据、数据快照、PII 投影与 claim gate；不得返回报告路径、把文件检查当事实验证或声称已经出具正式报告。";

export const REPORT_PUBLICATION_SCAN_MAX_NODES = 4_096;
export const REPORT_PUBLICATION_SCAN_MAX_BYTES = 1_048_576;

const STRONG_AUTHORITY_WORDS = new Set([
  "artifact",
  "manifest",
  "publication",
  "publish",
  "release",
  "report"
]);
const WEAK_AUTHORITY_WORDS = new Set(["delivery", "deliverable", "document", "render"]);
const DELIVERY_WORDS = new Set([
  "dest",
  "destination",
  "download",
  "file",
  "filename",
  "href",
  "location",
  "output",
  "path",
  "pointer",
  "target",
  "uri",
  "url"
]);
const CONTENT_WORDS = new Set([
  "body",
  "bytes",
  "content",
  "doc",
  "docx",
  "html",
  "markdown",
  "outline",
  "pdf",
  "text"
]);
const PROOF_WORDS = new Set([
  "checksum",
  "digest",
  "hash",
  "inspect",
  "inspection",
  "integrity",
  "proof",
  "receipt",
  "sha256",
  "signature",
  "verification",
  "verified"
]);
const STATE_WORDS = new Set([
  "complete",
  "completed",
  "delivered",
  "exists",
  "published",
  "ready",
  "state",
  "status",
  "success",
  "validated"
]);
const ACTION_WORDS = new Set(["create", "created", "final", "generate", "generated", "official", "write", "written"]);
const CONTROLLED_FILE_WORD_PATTERN = /(?:artifact|manifest|publication|report|正式报告|案件报告)/iu;
const CONTROLLED_FILE_EXTENSION_PATTERN = /\.(?:docx?|html?|json|md|pdf|zip)(?:[?#][^\s"'`<>]*)?(?:$|[\s"'`)\]}>,;])/iu;
const FORMAL_REPORT_FILE_EXTENSION_PATTERN = /\.(?:docx?|html?|md|pdf)(?:[?#][^\s"'`<>]*)?(?:$|[\s"'`)\]}>,;])/iu;
const PATH_MARKER_PATTERN = /(?:\b(?:artifact|file|https?|publication|report|s3):\/\/|(?:^|[\s"'`([])(?:(?:[a-z]:)?[\\/]|\.{1,2}[\\/]|~[\\/]))/iu;
const PUBLICATION_RECEIPT_TEXT_PATTERN = /(?:publication|report)[_\s-]*receipt/iu;
const SHA256_VALUE_PATTERN = /^[a-f0-9]{64}$/iu;
const INSPECTION_STATE_VALUE_PATTERN = /^(?:complete|completed|delivered|ok|pass|passed|published|ready|success|valid|validated|verified)$/iu;
const COMPACT_STRONG_AUTHORITY_PATTERN = /(?:artifact|manifest|publication|publish|release|report(?!ing)|发布|制品|清单|报告)/iu;
const COMPACT_WEAK_AUTHORITY_PATTERN = /(?:delivery|deliverable|document|render|交付|文档|渲染)/iu;
const COMPACT_DELIVERY_PATTERN = /(?:dest(?:ination)?|download|file(?:name)?|href|location|output|path|pointer|target|uri|url|下载|位置|文件|路径|输出)/iu;
const COMPACT_LOCATOR_PATTERN = /(?:dest(?:ination)?|file(?:name)?|href|location|path|pointer|target|uri|url|位置|文件|路径)/iu;
const COMPACT_CONTENT_PATTERN = /(?:body|bytes|content|docx?(?!ument)|html|markdown|outline|pdf|text|内容|正文)/iu;
const COMPACT_PROOF_PATTERN = /(?:checksum|digest|hash|inspect(?:ion)?|integrity|proof|receipt|sha256|signature|verification|verified|哈希|校验|检查|回执)/iu;
const COMPACT_STATE_PATTERN = /(?:complete|delivered|exists|published|ready|state|status|success|validated|状态|完成|已发布)/iu;
const COMPACT_ACTION_PATTERN = /(?:create|final|generate|generated|official|write|written|创建|生成|正式|写入)/iu;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function ownText(value, keys) {
  const source = objectOf(value);
  for (const key of keys) {
    try {
      const descriptor = Object.getOwnPropertyDescriptor(source, key);
      if (descriptor && !descriptor.get && !descriptor.set) {
        const result = text(descriptor.value);
        if (result) return result;
      }
    } catch {
      return "";
    }
  }
  return "";
}

function hasMaterialValue(value) {
  if (Array.isArray(value)) return value.length > 0;
  return value !== undefined && value !== null && value !== "" && value !== false;
}

function keyWords(key) {
  return String(key)
    .replace(/([a-z0-9])([A-Z])/gu, "$1 $2")
    .toLowerCase()
    .split(/[^a-z0-9\u3400-\u9fff]+/u)
    .filter(Boolean);
}

function wordsContain(words, candidates) {
  return words.some((word) => candidates.has(word));
}

function keySignalsStrongAuthority(words) {
  return wordsContain(words, STRONG_AUTHORITY_WORDS)
    || COMPACT_STRONG_AUTHORITY_PATTERN.test(words.join(""));
}

function keySignalsWeakAuthority(words) {
  return wordsContain(words, WEAK_AUTHORITY_WORDS)
    || COMPACT_WEAK_AUTHORITY_PATTERN.test(words.join(""));
}

function keyCreatesPublicationContext(words) {
  if (keySignalsStrongAuthority(words)) return true;
  const compact = words.join("");
  const weakAuthority = wordsContain(words, WEAK_AUTHORITY_WORDS)
    || COMPACT_WEAK_AUTHORITY_PATTERN.test(compact);
  return weakAuthority
    && (wordsContain(words, ACTION_WORDS)
      || wordsContain(words, DELIVERY_WORDS)
      || wordsContain(words, CONTENT_WORDS)
      || wordsContain(words, PROOF_WORDS)
      || COMPACT_ACTION_PATTERN.test(compact)
      || COMPACT_DELIVERY_PATTERN.test(compact)
      || COMPACT_CONTENT_PATTERN.test(compact)
      || COMPACT_PROOF_PATTERN.test(compact));
}

function keyCreatesSiblingPublicationContext(words) {
  return keySignalsStrongAuthority(words)
    && words.some((word) => word === "kind" || word === "type" || word === "类型");
}

function keySignalsMaterial(words) {
  const compact = words.join("");
  return wordsContain(words, DELIVERY_WORDS)
    || wordsContain(words, CONTENT_WORDS)
    || wordsContain(words, PROOF_WORDS)
    || wordsContain(words, STATE_WORDS)
    || wordsContain(words, ACTION_WORDS)
    || COMPACT_DELIVERY_PATTERN.test(compact)
    || COMPACT_CONTENT_PATTERN.test(compact)
    || COMPACT_PROOF_PATTERN.test(compact)
    || COMPACT_STATE_PATTERN.test(compact)
    || COMPACT_ACTION_PATTERN.test(compact);
}

function keyDirectlySignalsPublication(words) {
  const compact = words.join("");
  const strongAuthority = keySignalsStrongAuthority(words);
  const weakAuthority = keySignalsWeakAuthority(words);
  const material = keySignalsMaterial(words);
  const weakMaterial = wordsContain(words, DELIVERY_WORDS)
    || wordsContain(words, CONTENT_WORDS)
    || wordsContain(words, PROOF_WORDS)
    || wordsContain(words, ACTION_WORDS)
    || COMPACT_DELIVERY_PATTERN.test(compact)
    || COMPACT_CONTENT_PATTERN.test(compact)
    || COMPACT_PROOF_PATTERN.test(compact)
    || COMPACT_ACTION_PATTERN.test(compact);
  const inspectionProof = words.some((word) => word === "inspect" || word === "inspection");
  const compactInspectionProof = /inspect(?:ion)?/iu.test(compact)
    && COMPACT_PROOF_PATTERN.test(compact)
    && COMPACT_STATE_PATTERN.test(compact);
  const outputAlias = (compact.includes("output") || compact.includes("download") || compact.includes("输出") || compact.includes("下载"))
    && COMPACT_LOCATOR_PATTERN.test(compact);
  const actionDeliveryAlias = COMPACT_ACTION_PATTERN.test(compact) && COMPACT_LOCATOR_PATTERN.test(compact);
  return (strongAuthority && material)
    || (weakAuthority && weakMaterial)
    || inspectionProof
    || compactInspectionProof
    || outputAlias
    || actionDeliveryAlias;
}

function stringSignalsPublication(value) {
  if (PUBLICATION_RECEIPT_TEXT_PATTERN.test(value)) return true;
  if (PATH_MARKER_PATTERN.test(value) && FORMAL_REPORT_FILE_EXTENSION_PATTERN.test(value)) return true;
  if (!CONTROLLED_FILE_WORD_PATTERN.test(value)) return false;
  return PATH_MARKER_PATTERN.test(value) || CONTROLLED_FILE_EXTENSION_PATTERN.test(value);
}

function addBytes(state, value) {
  state.bytes += String(value).length * 2;
  return state.bytes <= REPORT_PUBLICATION_SCAN_MAX_BYTES;
}

function scanUnverifiedPublication(root) {
  const state = { bytes: 0, scheduled: 1 };
  const stack = [{ phase: "enter", publicationContext: false, value: root }];
  const ancestors = new WeakSet();

  while (stack.length) {
    const current = stack.pop();
    const value = current.value;
    if (current.phase === "exit") {
      ancestors.delete(value);
      continue;
    }
    if (typeof value === "string") {
      if (!addBytes(state, value)) return true;
      if (stringSignalsPublication(value)) return true;
      if (current.publicationContext
        && (PATH_MARKER_PATTERN.test(value)
          || CONTROLLED_FILE_EXTENSION_PATTERN.test(value)
          || SHA256_VALUE_PATTERN.test(value)
          || INSPECTION_STATE_VALUE_PATTERN.test(value))) {
        return true;
      }
      continue;
    }
    if (value == null || typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
      continue;
    }
    if (typeof value !== "object") return true;
    if (ancestors.has(value)) return true;
    ancestors.add(value);
    stack.push({ phase: "exit", value });

    let prototype;
    try {
      prototype = Object.getPrototypeOf(value);
    } catch {
      return true;
    }
    if (Array.isArray(value)) {
      if (value.length > REPORT_PUBLICATION_SCAN_MAX_NODES) return true;
    } else if (prototype !== Object.prototype && prototype !== null) {
      return true;
    }

    const children = [];
    let siblingContext = current.publicationContext;
    try {
      for (const key in value) {
        if (!Object.prototype.hasOwnProperty.call(value, key)) continue;
        if (!addBytes(state, key)) return true;
        state.scheduled += 1;
        if (state.scheduled > REPORT_PUBLICATION_SCAN_MAX_NODES) return true;
        const descriptor = Object.getOwnPropertyDescriptor(value, key);
        if (!descriptor || descriptor.get || descriptor.set) return true;
        const words = keyWords(key);
        const childValue = descriptor.value;
        if (keyCreatesSiblingPublicationContext(words) && hasMaterialValue(childValue)) {
          siblingContext = true;
        }
        children.push({ childValue, words });
      }
    } catch {
      return true;
    }

    for (const child of children) {
      const childHasValue = hasMaterialValue(child.childValue);
      const childContext = siblingContext || keyCreatesPublicationContext(child.words);
      if (childHasValue && keyDirectlySignalsPublication(child.words)) return true;
      if (childHasValue && childContext && keySignalsMaterial(child.words)) return true;
      stack.push({ phase: "enter", publicationContext: childContext, value: child.childValue });
    }
  }
  return false;
}

function scanFullCaseAnalysisName(root) {
  const state = { bytes: 0, scheduled: 1 };
  const stack = [{ phase: "enter", value: root }];
  const ancestors = new WeakSet();
  while (stack.length) {
    const current = stack.pop();
    const value = current.value;
    if (current.phase === "exit") {
      ancestors.delete(value);
      continue;
    }
    if (value == null || typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") continue;
    if (typeof value === "string") {
      if (!addBytes(state, value)) return true;
      continue;
    }
    if (typeof value !== "object" || ancestors.has(value)) return true;
    ancestors.add(value);
    stack.push({ phase: "exit", value });
    if (Array.isArray(value) && value.length > REPORT_PUBLICATION_SCAN_MAX_NODES) return true;
    try {
      for (const key in value) {
        if (!Object.prototype.hasOwnProperty.call(value, key)) continue;
        if (!addBytes(state, key)) return true;
        state.scheduled += 1;
        if (state.scheduled > REPORT_PUBLICATION_SCAN_MAX_NODES) return true;
        const descriptor = Object.getOwnPropertyDescriptor(value, key);
        if (!descriptor || descriptor.get || descriptor.set) return true;
        const words = keyWords(key);
        if ((words.join("") === "tool" || words.join("") === "skillid")
          && text(descriptor.value) === "run_full_case_analysis") {
          return true;
        }
        stack.push({ phase: "enter", value: descriptor.value });
      }
    } catch {
      return true;
    }
  }
  return false;
}

export function isFullCaseAnalysisPayload(payload) {
  return scanFullCaseAnalysisName(payload);
}

export function hasUnverifiedReportPublication(value) {
  return scanUnverifiedPublication(value);
}

function publicationVisibleArgs(args = {}) {
  const source = objectOf(args);
  const visible = { ...source };
  delete visible._analytix;
  delete visible.__analytix;
  delete visible.analytix_runtime_context;
  return visible;
}

export function requiresPublicationBlock(payload, args = {}) {
  if (hasUnverifiedReportPublication(payload) || hasUnverifiedReportPublication(publicationVisibleArgs(args))) return true;
  if (ownText(payload, ["error_code", "errorCode"]) === PUBLICATION_RECEIPT_REQUIRED_CODE) return true;
  return isFullCaseAnalysisPayload(payload);
}

export function reportPublicationBlockedPayload(payload) {
  return {
    tool: ownText(payload, ["tool"]) || undefined,
    skill_id: ownText(payload, ["skill_id", "skillId"]) || undefined,
    case_id: ownText(payload, ["case_id", "caseId"]) || undefined,
    status: "blocked",
    semantic_status: "blocked",
    error_code: PUBLICATION_RECEIPT_REQUIRED_CODE,
    reason: PUBLICATION_RECEIPT_REQUIRED_TEXT,
    isError: true,
    safeToAnswer: false,
    safe_to_answer_current_task: false,
    write_blocked: true,
    report_gate_status: "publication_receipt_required",
    warnings: [PUBLICATION_RECEIPT_REQUIRED_TEXT]
  };
}
