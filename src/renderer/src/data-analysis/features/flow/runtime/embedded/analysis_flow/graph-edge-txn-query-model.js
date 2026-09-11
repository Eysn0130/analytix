(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) throw new Error("flow graph edge txn query model root missing");

  const DEFAULT_PLACEHOLDER_TOKEN_PREFIX = "__cp_placeholder__::";
  const EDGE_TXN_READONLY_PREVIEW_ROW_LIMIT = 5;

  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function normalizeEdgeTxnKey(value) {
    const raw = text(value);
    if (!raw) return "";
    let out = raw.replace(/\s+/g, "");
    const dash = out.indexOf("-");
    const under = out.indexOf("_");
    if (dash > 0 || under > 0) {
      const idx = dash > 0 && under > 0 ? Math.min(dash, under) : dash > 0 ? dash : under;
      out = out.slice(0, idx);
    }
    return out;
  }

  function resolveNormalizeKey(options = {}) {
    return typeof options.normalizeKey === "function" ? options.normalizeKey : normalizeEdgeTxnKey;
  }

  function normalizeEdgeTxnKeyList(values, options = {}) {
    const normalizeKey = resolveNormalizeKey(options);
    const source =
      values && typeof values !== "string" && typeof values[Symbol.iterator] === "function"
        ? Array.from(values)
        : Array.isArray(values)
          ? values
          : values
            ? [values]
            : [];
    const out = [];
    const seen = new Set();
    source.forEach((item) => {
      const key = normalizeKey(item);
      if (!key || seen.has(key)) return;
      seen.add(key);
      out.push(key);
    });
    return out;
  }

  function buildEdgeTxnCounterpartyQueries(input = {}, options = {}) {
    const normalizePlaceholderKind =
      typeof options.normalizePlaceholderKind === "function" ? options.normalizePlaceholderKind : text;
    const placeholderKind = normalizePlaceholderKind(input.cpPlaceholder?.kind || "");
    const placeholderName = text(input.cpPlaceholder?.name || "");
    if (placeholderKind) {
      return [
        {
          keyType: "account",
          keyValue: `${text(options.placeholderTokenPrefix) || DEFAULT_PLACEHOLDER_TOKEN_PREFIX}${placeholderKind}`,
          placeholderName,
          missingAccountOnly: false,
        },
      ];
    }

    const queries = [];
    const keyValues = normalizeEdgeTxnKeyList(input.cpKeys, options);
    if (keyValues.length) {
      queries.push({
        keyType: "account",
        keyValue: keyValues[0],
        keyValues,
        placeholderName: "",
        missingAccountOnly: false,
      });
    }
    const name = text(input.cpName || "");
    if (name) {
      queries.push({
        keyType: "name",
        keyValue: name,
        placeholderName: "",
        missingAccountOnly: true,
      });
    }
    return queries;
  }

  function resolveEdgeTxnRowOwnerKey(row, ownerKeys, options = {}) {
    const keys = normalizeEdgeTxnKeyList(ownerKeys, options);
    const ownerKeySet = new Set(keys);
    const candidates = [row?.account_key, row?.acct_key, row?.card_no, row?.acct_no];
    for (const candidate of candidates) {
      const key = resolveNormalizeKey(options)(candidate || "");
      if (key && ownerKeySet.has(key)) return key;
    }
    return keys.length === 1 ? keys[0] : "";
  }

  function refineEdgeTxnBackendRows(rows, query, options = {}) {
    const normalizeKey = resolveNormalizeKey(options);
    return (Array.isArray(rows) ? rows : []).filter((row) => {
      const rowCounterpartyName = text(row?.counterparty_name || "");
      if (query?.placeholderName && rowCounterpartyName && rowCounterpartyName !== query.placeholderName) return false;
      if (query?.missingAccountOnly && normalizeKey(row?.counterparty_acct || "")) return false;
      return true;
    });
  }

  function readTxnDetailLoadNow() {
    const perf = root.performance;
    if (perf && typeof perf.now === "function") return perf.now();
    return Date.now();
  }

  function buildTxnDetailLoadMetric(input = {}) {
    const rowsBefore = toNonNegativeInteger(input.rowsBefore);
    const rowsFetched = toNonNegativeInteger(input.rowsFetched);
    const reset = input.reset === true;
    const metric = {
      txnDetailCursorIn: hasTxnDetailCursor(input.cursorBefore) ? 1 : 0,
      txnDetailCursorOut: hasTxnDetailCursor(input.cursorAfter) ? 1 : 0,
      txnDetailDone: input.done === true ? 1 : 0,
      txnDetailOperation: text(input.operation || "detail") || "detail",
      txnDetailPageKind: text(input.pageKind || (reset ? "first-page" : "next-page")) || "next-page",
      txnDetailPageLimit: toNonNegativeInteger(input.pageLimit),
      txnDetailRequestCount: toNonNegativeInteger(input.requestCount, 1),
      txnDetailReset: reset ? 1 : 0,
      txnDetailRowsAfter: toNonNegativeInteger(input.rowsAfter, rowsBefore + rowsFetched),
      txnDetailRowsBefore: rowsBefore,
      txnDetailRowsFetched: rowsFetched,
      txnDetailSurface: text(input.surface || "unknown") || "unknown",
    };
    if (Number.isFinite(input.durationMs)) {
      metric.txnDetailDurationMs = roundTxnDetailMetricMs(Number(input.durationMs));
    }
    if (Number.isFinite(input.requestBuildMs)) {
      metric.txnDetailRequestBuildMs = roundTxnDetailMetricMs(Number(input.requestBuildMs));
    }
    return metric;
  }

  function hasTxnDetailCursor(value) {
    return !!value && typeof value === "object" && !Array.isArray(value);
  }

  function toNonNegativeInteger(value, fallback = 0) {
    const numberValue = Math.floor(Number(value ?? fallback));
    if (Number.isFinite(numberValue) && numberValue >= 0) return numberValue;
    return Math.max(0, Math.floor(Number(fallback) || 0));
  }

  function roundTxnDetailMetricMs(value) {
    return Math.round(Math.max(0, value) * 1000) / 1000;
  }

  function normalizeEdgeTxnCursorPageResult(value, limit = 0) {
    const factAnswerAllowed = value?.factAnswerAllowed === true || value?.fact_answer_allowed === true;
    const ok = value?.ok === true && factAnswerAllowed;
    const rows = ok && Array.isArray(value?.rows) ? value.rows : [];
    const nextCursorRaw = value?.nextCursor || value?.next_cursor || null;
    const nextCursor =
      nextCursorRaw && typeof nextCursorRaw === "object" && !Array.isArray(nextCursorRaw) ? nextCursorRaw : null;
    const hasDone = typeof value?.done === "boolean";
    const done = ok && (hasDone ? value.done : !nextCursor || (limit > 0 && rows.length < limit));
    return {
      ok,
      factAnswerAllowed,
      semanticStatus: text(value?.semanticStatus || value?.semantic_status || "blocked"),
      blocker: text(value?.blocker || value?.error || "host-evidence-receipt-required"),
      rows,
      done: !!done,
      nextCursor: ok ? nextCursor : null,
    };
  }

  function resolveEdgeTxnReadonlyPreviewRows(rows, limit = EDGE_TXN_READONLY_PREVIEW_ROW_LIMIT) {
    const source = Array.isArray(rows) ? rows : [];
    const safeLimit = Math.max(0, Math.floor(Number(limit) || 0));
    if (safeLimit <= 0) return [];
    return source.slice(0, safeLimit);
  }

  function buildEdgeTxnDirectionalRuleSets(input = {}, options = {}) {
    const flowFromKeys = normalizeEdgeTxnKeyList(input.flowFromKeys, options);
    const flowToKeys = normalizeEdgeTxnKeyList(input.flowToKeys, options);
    const flowFromName = text(input.flowFromName || "");
    const flowToName = text(input.flowToName || "");
    const flowTag = text(input.flowTag || "");
    const flowFromPlaceholder = input.flowFromPlaceholder || null;
    const flowToPlaceholder = input.flowToPlaceholder || null;
    return {
      strict: [
        {
          ownerKeys: flowFromKeys,
          dir: "out",
          cpKeys: flowToKeys,
          cpName: flowToName,
          cpPlaceholder: flowToPlaceholder,
          flow: flowTag,
        },
        {
          ownerKeys: flowToKeys,
          dir: "in",
          cpKeys: flowFromKeys,
          cpName: flowFromName,
          cpPlaceholder: flowFromPlaceholder,
          flow: flowTag,
        },
      ],
      fallback: [
        {
          ownerKeys: flowFromKeys,
          dir: "in",
          cpKeys: flowToKeys,
          cpName: flowToName,
          cpPlaceholder: flowToPlaceholder,
          flow: flowTag,
        },
        {
          ownerKeys: flowToKeys,
          dir: "out",
          cpKeys: flowFromKeys,
          cpName: flowFromName,
          cpPlaceholder: flowFromPlaceholder,
          flow: flowTag,
        },
      ],
    };
  }

  root.__ANALYTIX_FLOW_EDGE_TXN_QUERY_MODEL__ = {
    normalizeEdgeTxnKey,
    normalizeEdgeTxnKeyList,
    buildEdgeTxnCounterpartyQueries,
    resolveEdgeTxnRowOwnerKey,
    refineEdgeTxnBackendRows,
    readTxnDetailLoadNow,
    buildTxnDetailLoadMetric,
    normalizeEdgeTxnCursorPageResult,
    resolveEdgeTxnReadonlyPreviewRows,
    buildEdgeTxnDirectionalRuleSets,
  };
})();
