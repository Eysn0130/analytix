(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__;
  if (!piiProjection || typeof piiProjection.projectField !== "function" || typeof piiProjection.projectDetected !== "function") {
    throw new Error("ordinary PII projection missing for edge detail");
  }

  function defaultEscapeHtml(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function resolveFn(fn, fallback) {
    return typeof fn === "function" ? fn : fallback;
  }

  function defaultMoney(value) {
    const num = Number(value);
    return Number.isFinite(num) ? num.toFixed(2) : "0.00";
  }

  const EVIDENCE_STATUS_LABELS = {
    verified: "已由流水印证",
    matched: "已由流水印证",
    partial: "局部印证",
    derived: "流水推导",
    inferred: "流水推导",
    source_missing: "需补调源账户",
    retrieval_needed: "需调取后续账户",
    stop: "穿透停止点",
    terminal: "穿透停止点",
    conflict: "金额口径不一致",
  };

  function hasText(value) {
    return String(value == null ? "" : value).trim() !== "";
  }

  function pickValue(model, keys) {
    const source = model && typeof model === "object" ? model : {};
    for (const key of keys) {
      if (Object.prototype.hasOwnProperty.call(source, key) && hasText(source[key])) return source[key];
    }
    return "";
  }

  function formatDisplayValue(value) {
    if (Array.isArray(value)) return value.map((item) => String(item == null ? "" : item).trim()).filter(Boolean).join(" / ");
    return String(value == null ? "" : value).trim();
  }

  function formatMoneyField(value, fmtMoney) {
    if (typeof value === "number" && Number.isFinite(value)) return `￥${fmtMoney(value)}`;
    const text = formatDisplayValue(value);
    if (!text) return "";
    const num = Number(text.replace(/[,，￥¥\s]/g, ""));
    return Number.isFinite(num) && /^[-+]?[\d,，￥¥\s.]+$/.test(text) ? `￥${fmtMoney(num)}` : text;
  }

  function formatStatusField(value) {
    const text = formatDisplayValue(value);
    if (!text) return "";
    const key = text.toLowerCase();
    return EVIDENCE_STATUS_LABELS[key] || text;
  }

  function pushDetailRow(lines, label, value, escapeHtml, options = {}) {
    const raw = options.money ? formatMoneyField(value, options.fmtMoney || defaultMoney) : formatDisplayValue(value);
    const text = options.status ? formatStatusField(raw) : raw;
    if (!text) return;
    lines.push(`<div class="k">${escapeHtml(label)}</div><div class="v">${escapeHtml(piiProjection.projectDetected(text))}</div>`);
  }

  function buildEdgeTxnReadonlyHtml(model, lane = "", rows = [], opts = {}, deps = {}) {
    const escapeHtml = resolveFn(deps.escapeHtml, defaultEscapeHtml);
    const fmtMoney = resolveFn(deps.fmtMoney, defaultMoney);
    const normalizeTxnTimeText = resolveFn(deps.normalizeTxnTimeText, (value) => String(value || "").trim());
    const isSignedEdgeDetailEnabled = resolveFn(deps.isSignedEdgeDetailEnabled, () => false);
    const resolveEdgeTxnReadonlyPreviewRows = resolveFn(deps.resolveEdgeTxnReadonlyPreviewRows, (value) =>
      Array.isArray(value) ? value.slice(0, 5) : []
    );
    const edgeSignedMeta = resolveFn(deps.edgeSignedMeta, () => ({
      selectedLane: lane,
      laneAmount: Number(model?.amount) || 0,
      signedAmount: Number(model?.amount) || 0,
      sign: 1,
      signClass: "in",
    }));
    const formatSignedMoney = resolveFn(deps.formatSignedMoney, (value) => `￥${fmtMoney(value)}`);

    const signedEnabled = isSignedEdgeDetailEnabled(model);
    const signed = edgeSignedMeta(model, lane);
    const laneAmount = signed.laneAmount;
    const signedAmount = signed.signedAmount;
    const loading = !!opts.loading;
    const error = piiProjection.projectDetected(opts.error || "");
    const enableCountLink = !!opts.enableCountLink;
    const fallbackCount = Math.max(0, Number(model?.count) || 0);
    const hasRows = Array.isArray(rows) && rows.length > 0;
    const previewRows = resolveEdgeTxnReadonlyPreviewRows(rows);
    const showFallbackCount = !loading && !error && !hasRows && fallbackCount > 0;
    const totalCount = hasRows ? rows.length : showFallbackCount ? fallbackCount : 0;
    const totalCountText = loading ? "加载中..." : String(totalCount);

    const lines = [];
    lines.push(`<div class="edgeTxnHeader">`);
    lines.push(`<div class="edgeTxnCategory">资金交易</div>`);
    lines.push(`<div class="edgeTxnSummary">`);
    if (signedEnabled) {
      lines.push(
        `<div class="edgeTxnMetric"><div class="edgeTxnMetricLabel">金额合计</div><div class="edgeTxnMetricValue edgeTxnMetricValue--${escapeHtml(
          signed.signClass
        )}">${escapeHtml(formatSignedMoney(signedAmount))}</div></div>`
      );
    } else {
      lines.push(
        `<div class="edgeTxnMetric"><div class="edgeTxnMetricLabel">金额合计</div><div class="edgeTxnMetricValue">${escapeHtml(
          `￥${fmtMoney(laneAmount)}`
        )}</div></div>`
      );
    }
    if (enableCountLink && !loading && totalCount > 0) {
      lines.push(
        `<div class="edgeTxnMetric"><div class="edgeTxnMetricLabel">次数</div><button class="edgeTxnMetricValue edgeTxnCountLink" type="button" data-action="open-edge-txn-detail">${escapeHtml(
          totalCountText
        )}</button></div>`
      );
    } else {
      lines.push(
        `<div class="edgeTxnMetric"><div class="edgeTxnMetricLabel">次数</div><div class="edgeTxnMetricValue">${escapeHtml(
          totalCountText
        )}</div></div>`
      );
    }
    lines.push(`</div>`);
    lines.push(`</div>`);
    lines.push(`<div class="edgeTxnSectionTitle">交易情况</div>`);
    lines.push(`<div class="edgeTxnList edgeTxnList--cap5">`);
    if (loading) {
      lines.push(`<div class="edgeTxnEmpty">正在加载交易明细…</div>`);
    } else if (error) {
      lines.push(`<div class="edgeTxnEmpty">${escapeHtml(error)}</div>`);
    } else if (!rows.length) {
      lines.push(`<div class="edgeTxnEmpty">暂无可展示的交易明细</div>`);
    } else {
      previewRows.forEach((row) => {
        const t = normalizeTxnTimeText(row?.txn_time || "");
        if (signedEnabled) {
          const rawSigned = Number(row?.__signedAmount);
          const amountSigned = Number.isFinite(rawSigned)
            ? rawSigned
            : signed.sign >= 0
              ? Math.abs(Number(row?.amount) || 0)
              : -Math.abs(Number(row?.amount) || 0);
          const cls = amountSigned >= 0 ? "in" : "out";
          lines.push(
            `<div class="edgeTxnRow"><div class="edgeTxnTime">${escapeHtml(
              t || "--"
            )}</div><div class="edgeTxnAmount edgeTxnAmount--${cls}">${escapeHtml(
              formatSignedMoney(amountSigned)
            )}</div></div>`
          );
        } else {
          const amount = Math.abs(Number(row?.amount) || 0);
          lines.push(
            `<div class="edgeTxnRow"><div class="edgeTxnTime">${escapeHtml(
              t || "--"
            )}</div><div class="edgeTxnAmount">${escapeHtml(`￥${fmtMoney(amount)}`)}</div></div>`
          );
        }
      });
    }
    lines.push(`</div>`);
    return lines.join("");
  }

  function buildEdgeDetailHtml(model, deps = {}) {
    const escapeHtml = resolveFn(deps.escapeHtml, defaultEscapeHtml);
    const fmtMoney = resolveFn(deps.fmtMoney, defaultMoney);
    const fmtShortRange = resolveFn(deps.fmtShortRange, () => "");
    const resolveEdgeLane = resolveFn(deps.resolveEdgeLane, () => "");
    const edgeAmountTotal = resolveFn(deps.edgeAmountTotal, (edge) => Number(edge?.amount) || 0);
    const edgeAmountByLane = resolveFn(deps.edgeAmountByLane, (edge) => Number(edge?.amount) || 0);

    const edgeId = piiProjection.projectDetected(model?.id || "");
    const source = piiProjection.projectDetected(model?.source || "");
    const target = piiProjection.projectDetected(model?.target || "");
    const isDouble = String(model?.mode || "").toLowerCase() === "double";
    const lane = resolveEdgeLane(model, "");
    const lineAmount = isDouble && !lane ? edgeAmountTotal(model) : edgeAmountByLane(model, lane);
    const count = Number(model?.count) || 0;
    const range = fmtShortRange(model?.first_time, model?.last_time);

    let laneText = "单向";
    if (isDouble) {
      if (lane === "bottom") laneText = `下行（${target} → ${source}）`;
      else if (lane === "top") laneText = `上行（${source} → ${target}）`;
      else laneText = "双向（未区分线道）";
    } else {
      const arrow = String(model?.edgeArrow || model?.arrow || "").toLowerCase();
      laneText = arrow === "start" ? `方向（${target} → ${source}）` : `方向（${source} → ${target}）`;
    }

    const lines = [];
    lines.push(`<div class="kv">`);
    if (edgeId) lines.push(`<div class="k">ID</div><div class="v">${escapeHtml(edgeId)}</div>`);
    if (source) lines.push(`<div class="k">源节点</div><div class="v">${escapeHtml(source)}</div>`);
    if (target) lines.push(`<div class="k">目标节点</div><div class="v">${escapeHtml(target)}</div>`);
    lines.push(`<div class="k">线道</div><div class="v">${escapeHtml(laneText)}</div>`);
    lines.push(`<div class="k">金额</div><div class="v">${escapeHtml(fmtMoney(lineAmount))}</div>`);
    if (count > 0) lines.push(`<div class="k">次数</div><div class="v">${escapeHtml(String(count))}</div>`);
    if (range) lines.push(`<div class="k">时间</div><div class="v">${escapeHtml(range)}</div>`);
    pushDetailRow(
      lines,
      "线索金额",
      pickValue(model, ["claim_amount", "claimed_amount", "report_amount", "reportAmount", "claimAmount"]),
      escapeHtml,
      { money: true, fmtMoney }
    );
    pushDetailRow(
      lines,
      "流水金额",
      pickValue(model, [
        "evidence_amount",
        "verified_amount",
        "database_amount",
        "actual_amount",
        "evidenceAmount",
        "verifiedAmount",
      ]),
      escapeHtml,
      { money: true, fmtMoney }
    );
    pushDetailRow(lines, "入账合计", pickValue(model, ["in_amount_verified", "evidence_in_amount"]), escapeHtml, {
      money: true,
      fmtMoney,
    });
    pushDetailRow(lines, "转出合计", pickValue(model, ["out_amount_verified", "evidence_out_amount"]), escapeHtml, {
      money: true,
      fmtMoney,
    });
    pushDetailRow(lines, "净额", pickValue(model, ["net_amount", "evidence_net_amount"]), escapeHtml, {
      money: true,
      fmtMoney,
    });
    pushDetailRow(
      lines,
      "证据状态",
      pickValue(model, ["evidence_status", "evidenceStatus", "trace_status", "traceStatus"]),
      escapeHtml,
      { status: true }
    );
    pushDetailRow(
      lines,
      "证据口径",
      pickValue(model, ["evidence_basis", "basis", "amount_scope", "amountScope", "analysis_basis"]),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "穿透停止原因",
      pickValue(model, ["trace_stop_reason", "stop_reason", "stopReason", "terminal_reason"]),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "下一步调取",
      pickValue(model, ["next_action", "retrieval_action", "nextAction", "retrievalAction"]),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "调取对象",
      pickValue(model, ["retrieval_target", "target_subject", "retrievalTarget", "targetSubject"]),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "调取账号",
      piiProjection.projectField(
        "retrieval_account",
        pickValue(model, ["retrieval_account", "target_account_no", "account_to_retrieve", "retrievalAccount"])
      ),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "归属行/受理机构",
      pickValue(model, ["retrieval_bank", "target_bank", "receiving_bank", "retrievalBank"]),
      escapeHtml
    );
    pushDetailRow(lines, "备注", pickValue(model, ["analysis_note", "note", "notes", "remark"]), escapeHtml);
    lines.push(`</div>`);
    return lines.join("");
  }

  window.__ANALYTIX_FLOW_EDGE_DETAIL_RENDER_MODEL__ = {
    buildEdgeTxnReadonlyHtml,
    buildEdgeDetailHtml,
  };
})();
