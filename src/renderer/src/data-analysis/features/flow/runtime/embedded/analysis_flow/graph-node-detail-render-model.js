(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__;
  if (!piiProjection || typeof piiProjection.projectField !== "function" || typeof piiProjection.projectDetected !== "function") {
    throw new Error("ordinary PII projection missing for node detail");
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

  const TRACE_STATUS_LABELS = {
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

  function formatStatusField(value) {
    const text = formatDisplayValue(value);
    if (!text) return "";
    const key = text.toLowerCase();
    return TRACE_STATUS_LABELS[key] || text;
  }

  function pushDetailRow(lines, label, value, escapeHtml, options = {}) {
    const text = options.status ? formatStatusField(value) : formatDisplayValue(value);
    if (!text) return;
    lines.push(`<div class="k">${escapeHtml(label)}</div><div class="v">${escapeHtml(piiProjection.projectDetected(text))}</div>`);
  }

  function buildNodePreviewHtml(options = {}, deps = {}) {
    const escapeHtml = resolveFn(deps.escapeHtml, defaultEscapeHtml);
    const previewSize = Math.max(26, Math.min(52, Number(options.size) || 32));
    const previewLineWidth = Math.max(1, Math.min(8, Number(options.lineWidth) || 2));
    const fill = String(options.fill || "rgba(255,255,255,0.9)");
    const stroke = String(options.stroke || "#1f6feb");
    const pad = 6;
    const frameSize = previewSize + pad * 2;
    const center = frameSize / 2;
    const radius = Math.max(2, previewSize / 2 - previewLineWidth / 2);
    const haloRadius = radius + previewLineWidth * 0.5 + 1.25;
    return `
      <div class="nodePreview" style="width:${frameSize}px;height:${frameSize}px;">
        <svg class="nodePreviewGraphic" viewBox="0 0 ${frameSize} ${frameSize}" width="${frameSize}" height="${frameSize}" aria-hidden="true">
          <circle cx="${center.toFixed(2)}" cy="${center.toFixed(2)}" r="${haloRadius.toFixed(2)}" fill="rgba(15,23,42,0.08)"></circle>
          <circle cx="${center.toFixed(2)}" cy="${center.toFixed(2)}" r="${radius.toFixed(2)}" fill="${escapeHtml(fill)}"></circle>
          <circle
            cx="${center.toFixed(2)}"
            cy="${center.toFixed(2)}"
            r="${radius.toFixed(2)}"
            fill="none"
            stroke="${escapeHtml(stroke)}"
            stroke-width="${previewLineWidth.toFixed(2)}"
          ></circle>
        </svg>
      </div>
    `;
  }

  function buildNodeInfoHtml(payload = {}, deps = {}) {
    const escapeHtml = resolveFn(deps.escapeHtml, defaultEscapeHtml);
    const accountList = Array.isArray(payload.accountList)
      ? payload.accountList.filter(Boolean).map((value) => piiProjection.projectField("account_no", value))
      : [];
    const listMax = Math.max(1, Number(payload.listMax) || 18);
    const listHtml = accountList
      .slice(0, listMax)
      .map((id) => `<span class="nodeInfoTag">${escapeHtml(id)}</span>`)
      .join("");
    const moreHtml =
      accountList.length > listMax ? `<span class="nodeInfoMore">+${accountList.length - listMax}</span>` : "";
    const accountValueHtml = accountList.length > 1
      ? `<div class="nodeInfoList">${listHtml}${moreHtml}</div>`
      : `<div class="nodeInfoValue mono">${escapeHtml(piiProjection.projectField("account_no", payload.account || ""))}</div>`;
    return `
      <div class="nodeInfoGrid">
        <div class="nodeInfoLeft">
          ${buildNodePreviewHtml(payload.preview || {}, { escapeHtml })}
          <div class="nodeCategoryLabel">类别名称</div>
          <div class="nodeCategoryValue">${escapeHtml(piiProjection.projectDetected(payload.category || ""))}</div>
        </div>
        <div class="nodeInfoRight">
          <div class="nodeInfoRow">
            <div class="nodeInfoLabel">用户名</div>
            <div class="nodeInfoValue">${escapeHtml(piiProjection.projectDetected(payload.userName || ""))}</div>
          </div>
          <div class="nodeInfoRow">
            <div class="nodeInfoLabel">${escapeHtml(payload.accountLabel || "卡号")}</div>
            ${accountValueHtml}
          </div>
        </div>
      </div>
    `;
  }

  function buildNodeDetailHtml(model, options = {}, deps = {}) {
    const escapeHtml = resolveFn(deps.escapeHtml, defaultEscapeHtml);
    const fmtMoney = resolveFn(deps.fmtMoney, defaultMoney);
    const id = piiProjection.projectDetected(model?.id || "");
    const title = piiProjection.projectDetected(model?.title || model?.label || "");
    const type = String(model?.ntype || model?.type || "");
    const degree = String(options.degree || "");
    const lines = [];
    lines.push(`<div class="kv">`);
    lines.push(`<div class="k">ID</div><div class="v">${escapeHtml(id)}</div>`);
    if (title) lines.push(`<div class="k">名称</div><div class="v">${escapeHtml(title)}</div>`);
    if (type) lines.push(`<div class="k">类型</div><div class="v">${escapeHtml(type)}</div>`);
    if (degree) lines.push(`<div class="k">度</div><div class="v">${escapeHtml(degree)}</div>`);
    if (model?.total_amount != null) lines.push(`<div class="k">总金额</div><div class="v">${escapeHtml(fmtMoney(model.total_amount))}</div>`);
    if (model?.total_count != null) lines.push(`<div class="k">总次数</div><div class="v">${escapeHtml(model.total_count)}</div>`);
    pushDetailRow(lines, "主体角色", pickValue(model, ["role", "node_role", "subject_role"]), escapeHtml);
    pushDetailRow(
      lines,
      "账户/卡号",
      piiProjection.projectField("account_no", pickValue(model, ["account_no", "accountNo", "card_no", "cardNo"])),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "开户行/归属行",
      pickValue(model, ["bank_name", "bankName", "open_bank", "openBank", "branch_name", "branchName", "bank"]),
      escapeHtml
    );
    pushDetailRow(
      lines,
      "证据状态",
      pickValue(model, ["evidence_status", "evidenceStatus", "trace_status", "traceStatus"]),
      escapeHtml,
      { status: true }
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
    if (model?.note) {
      lines.push(`<div class="k">备注</div><div class="v">${escapeHtml(piiProjection.projectDetected(model.note))}</div>`);
    }
    lines.push(`</div>`);
    lines.push(`<div style="margin-top:12px; display:flex; gap:10px;">`);
    lines.push(`<button class="btn ghost" type="button" id="btnCopyNode">复制ID</button>`);
    lines.push(`<button class="btn" type="button" id="btnFocusNode">定位</button>`);
    lines.push(`</div>`);
    return lines.join("");
  }

  function buildNodeDetailDrawer(model, options = {}, deps = {}) {
    const buildDetailRow = resolveFn(deps.buildDetailRow, (label, value, rowOptions = {}) => ({
      label: String(label || ""),
      value: String(value == null ? "" : value),
      mono: !!rowOptions.mono,
      tone: String(rowOptions.tone || ""),
    }));
    const node = model || {};
    const id = String(node.id || "").trim();
    const degree = String(options.degree || "");
    return {
      kind: "nodeDetail",
      title: piiProjection.projectDetected(node.title || node.label || node.id || "节点"),
      rows: [
        buildDetailRow("ID", piiProjection.projectDetected(id), { mono: true }),
        ...(degree ? [buildDetailRow("度", degree)] : []),
      ],
      actions: [
        { id: "copy-node-id", label: "复制ID", variant: "ghost" },
        { id: "focus-node", label: "定位", variant: "default" },
      ],
      meta: { nodeId: id },
    };
  }

  window.__ANALYTIX_FLOW_NODE_DETAIL_RENDER_MODEL__ = {
    buildNodePreviewHtml,
    buildNodeInfoHtml,
    buildNodeDetailHtml,
    buildNodeDetailDrawer,
  };
})();
