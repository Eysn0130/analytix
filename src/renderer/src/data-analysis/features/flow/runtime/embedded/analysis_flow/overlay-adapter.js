(function initAnalytixFlowOverlayAdapter(global) {
  "use strict";

  var piiProjection = global.__ANALYTIX_ORDINARY_PII_PROJECTION__;
  if (!piiProjection || typeof piiProjection.projectField !== "function" || typeof piiProjection.projectDetected !== "function") {
    throw new Error("ordinary PII projection missing for flow overlay");
  }

  /**
   * @typedef {{ left:number, top:number, right:number, bottom:number, width:number, height:number }} AnchorRect
   * @typedef {{ x:number, y:number }} Point
   * @typedef {{ id:string, label:string, disabled:boolean, separator:boolean }} ContextMenuItem
   * @typedef {{ value:string, label:string, dash?:number[] }} StyleOption
   * @typedef {{ id:string, label:string }} StyleTab
   * @typedef {{ label:string, value:string, mono?:boolean, tone?:string }} DetailRow
   * @typedef {{ id:string, label:string, variant:string }} DetailAction
   * @typedef {{ open:boolean, title:string, kind:string, rows:DetailRow[], actions:DetailAction[] }} DetailDrawerState
   * @typedef {{ time:string, amountText:string, amountTone:string }} EdgeTxnRow
   * @typedef {{ title:string, amountText:string, amountTone:string, countText:string, countClickable:boolean, loading:boolean, error:string, rows:EdgeTxnRow[], requestKey?:string }} EdgeTxnOverlayState
   * @typedef {{ key:string, title:string, width:number, sortable:boolean, sorted:boolean, sortDir:string }} TxnColumn
   * @typedef {{ text:string, mono:boolean, align:string, tone:string }} TxnCell
   * @typedef {{ key:string, cells:Record<string, TxnCell> }} TxnRow
   */

  function clampNumber(value, min, max) {
    var num = Number(value);
    if (!Number.isFinite(num)) return min;
    return Math.min(max, Math.max(min, num));
  }

  function cloneAnchorRect(rect) {
    if (!rect || typeof rect !== "object") return null;
    var left = Number(rect.left);
    var top = Number(rect.top);
    var right = Number(rect.right);
    var bottom = Number(rect.bottom);
    var width = Number(rect.width);
    var height = Number(rect.height);
    if (![left, top, right, bottom, width, height].every(Number.isFinite)) {
      return null;
    }
    return { left: left, top: top, right: right, bottom: bottom, width: width, height: height };
  }

  function clonePoint(point) {
    if (!point || typeof point !== "object") return null;
    var x = Number(point.x);
    var y = Number(point.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
    return { x: x, y: y };
  }

  function cloneContextMenuItems(items) {
    return (Array.isArray(items) ? items : []).map(function mapItem(item) {
      return {
        id: String((item && item.id) || ""),
        label: String((item && item.label) || ""),
        disabled: !!(item && item.disabled),
        separator: !!(item && item.separator),
      };
    });
  }

  function cloneStyleOptions(options) {
    return (Array.isArray(options) ? options : []).map(function mapOption(option) {
      if (option && typeof option === "object") {
        var next = {
          value: String(option.value == null ? "" : option.value),
          label: String(option.label == null ? option.value == null ? "" : option.value : option.label),
        };
        if (Array.isArray(option.dash)) {
          next.dash = option.dash.map(function mapDash(value) {
            return Number(value) || 0;
          });
        }
        return next;
      }
      return {
        value: String(option == null ? "" : option),
        label: String(option == null ? "" : option),
      };
    });
  }

  function cloneDetailRows(rows) {
    return (Array.isArray(rows) ? rows : []).map(function mapRow(row) {
      return {
        label: String((row && row.label) || ""),
        value: String((row && row.value) || ""),
        mono: !!(row && row.mono),
        tone: String((row && row.tone) || ""),
      };
    });
  }

  function cloneDetailActions(actions) {
    return (Array.isArray(actions) ? actions : []).map(function mapAction(action) {
      return {
        id: String((action && action.id) || ""),
        label: String((action && action.label) || ""),
        variant: String((action && action.variant) || "default"),
      };
    });
  }

  function cloneEdgeTxnRows(rows) {
    return (Array.isArray(rows) ? rows : []).map(function mapRow(row) {
      return {
        time: String((row && row.time) || ""),
        amountText: String((row && row.amountText) || ""),
        amountTone: String((row && row.amountTone) || ""),
      };
    });
  }

  function buildDetailRow(label, value, options) {
    var nextOptions = options && typeof options === "object" ? options : {};
    return {
      label: String(label || ""),
      value: piiProjection.projectDetected(value),
      mono: !!nextOptions.mono,
      tone: String(nextOptions.tone || ""),
    };
  }

  var TRACE_STATUS_LABELS = {
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
    var source = model && typeof model === "object" ? model : {};
    for (var index = 0; index < keys.length; index += 1) {
      var key = keys[index];
      if (Object.prototype.hasOwnProperty.call(source, key) && hasText(source[key])) return source[key];
    }
    return "";
  }

  function formatDisplayValue(value) {
    if (Array.isArray(value)) {
      return value
        .map(function mapItem(item) {
          return String(item == null ? "" : item).trim();
        })
        .filter(Boolean)
        .join(" / ");
    }
    return String(value == null ? "" : value).trim();
  }

  function formatMoneyValue(value, deps) {
    var nextDeps = deps && typeof deps === "object" ? deps : {};
    if (typeof value === "number" && Number.isFinite(value)) {
      return typeof nextDeps.formatMoney === "function" ? nextDeps.formatMoney(value) : String(value);
    }
    var text = formatDisplayValue(value);
    if (!text) return "";
    var num = Number(text.replace(/[,，￥¥\s]/g, ""));
    if (Number.isFinite(num) && /^[-+]?[\d,，￥¥\s.]+$/.test(text)) {
      return typeof nextDeps.formatMoney === "function" ? nextDeps.formatMoney(num) : String(num);
    }
    return text;
  }

  function formatStatusValue(value) {
    var text = formatDisplayValue(value);
    if (!text) return "";
    return TRACE_STATUS_LABELS[text.toLowerCase()] || text;
  }

  function pushDrawerRow(rows, label, value, options) {
    var nextOptions = options && typeof options === "object" ? options : {};
    var text = nextOptions.money ? formatMoneyValue(value, nextOptions.deps) : formatDisplayValue(value);
    if (nextOptions.status) text = formatStatusValue(text);
    if (!text) return;
    rows.push(buildDetailRow(label, text, { mono: !!nextOptions.mono, tone: String(nextOptions.tone || "") }));
  }

  function buildNodeDetailDrawer(model, meta, deps) {
    var node = model && typeof model === "object" ? model : {};
    var nextMeta = meta && typeof meta === "object" ? meta : {};
    var nextDeps = deps && typeof deps === "object" ? deps : {};
    var id = String(node.id || "").trim();
    var title = piiProjection.projectDetected(node.title || node.label || node.id || "节点");
    var typeLabel = String(nextMeta.typeLabel || "");
    var degree = String(nextMeta.degree || "");
    var rows = [];
    if (id) rows.push(buildDetailRow("ID", piiProjection.projectDetected(id), { mono: true }));
    if (title) rows.push(buildDetailRow("名称", title));
    if (typeLabel) rows.push(buildDetailRow("类型", typeLabel));
    if (degree) rows.push(buildDetailRow("度", degree));
    if (node.total_amount != null) {
      rows.push(
        buildDetailRow(
          "总金额",
          typeof nextDeps.formatMoney === "function" ? nextDeps.formatMoney(node.total_amount) : String(node.total_amount)
        )
      );
    }
    if (node.total_count != null) rows.push(buildDetailRow("总次数", String(node.total_count)));
    pushDrawerRow(rows, "主体角色", pickValue(node, ["role", "node_role", "subject_role"]));
    pushDrawerRow(
      rows,
      "账户/卡号",
      piiProjection.projectField("account_no", pickValue(node, ["account_no", "accountNo", "card_no", "cardNo"]))
    );
    pushDrawerRow(
      rows,
      "开户行/归属行",
      pickValue(node, ["bank_name", "bankName", "open_bank", "openBank", "branch_name", "branchName", "bank"])
    );
    pushDrawerRow(
      rows,
      "证据状态",
      pickValue(node, ["evidence_status", "evidenceStatus", "trace_status", "traceStatus"]),
      { status: true }
    );
    pushDrawerRow(
      rows,
      "穿透停止原因",
      pickValue(node, ["trace_stop_reason", "stop_reason", "stopReason", "terminal_reason"])
    );
    pushDrawerRow(
      rows,
      "下一步调取",
      pickValue(node, ["next_action", "retrieval_action", "nextAction", "retrievalAction"])
    );
    pushDrawerRow(
      rows,
      "调取账号",
      piiProjection.projectField(
        "retrieval_account",
        pickValue(node, ["retrieval_account", "target_account_no", "account_to_retrieve", "retrievalAccount"])
      )
    );
    pushDrawerRow(
      rows,
      "归属行/受理机构",
      pickValue(node, ["retrieval_bank", "target_bank", "receiving_bank", "retrievalBank"])
    );
    if (node.note) rows.push(buildDetailRow("备注", String(node.note)));
    return {
      kind: "nodeDetail",
      title: title,
      rows: rows,
      actions: [
        { id: "copy-node-id", label: "复制ID", variant: "ghost" },
        { id: "focus-node", label: "定位", variant: "default" },
      ],
      meta: { nodeId: id },
    };
  }

  function buildEdgeDetailDrawer(model, deps) {
    var edge = model && typeof model === "object" ? model : {};
    var nextDeps = deps && typeof deps === "object" ? deps : {};
    var edgeId = String(edge.id || "").trim();
    var source = String(edge.source || "").trim();
    var target = String(edge.target || "").trim();
    var isDouble = String(edge.mode || "").toLowerCase() === "double";
    var lane =
      typeof nextDeps.resolveEdgeLane === "function" ? String(nextDeps.resolveEdgeLane(edge, "") || "") : "";
    var lineAmount = 0;
    if (isDouble && !lane) {
      lineAmount =
        typeof nextDeps.edgeAmountTotal === "function" ? Number(nextDeps.edgeAmountTotal(edge)) || 0 : Number(edge.amount) || 0;
    } else {
      lineAmount =
        typeof nextDeps.edgeAmountByLane === "function"
          ? Number(nextDeps.edgeAmountByLane(edge, lane)) || 0
          : Number(edge.amount) || 0;
    }
    var count = Number(edge.count) || 0;
    var range = typeof nextDeps.formatRange === "function" ? String(nextDeps.formatRange(edge.first_time, edge.last_time) || "") : "";
    var laneText = "单向";
    if (isDouble) {
      if (lane === "bottom") laneText = "下行（" + target + " → " + source + "）";
      else if (lane === "top") laneText = "上行（" + source + " → " + target + "）";
      else laneText = "双向（未区分线道）";
    } else {
      var arrow = String(edge.edgeArrow || edge.arrow || "").toLowerCase();
      laneText = arrow === "start" ? "方向（" + target + " → " + source + "）" : "方向（" + source + " → " + target + "）";
    }

    var rows = [];
    if (edgeId) rows.push(buildDetailRow("ID", edgeId, { mono: true }));
    if (source) rows.push(buildDetailRow("源节点", source, { mono: true }));
    if (target) rows.push(buildDetailRow("目标节点", target, { mono: true }));
    rows.push(buildDetailRow("线道", laneText));
    rows.push(
      buildDetailRow(
        "金额",
        typeof nextDeps.formatMoney === "function" ? nextDeps.formatMoney(lineAmount) : String(lineAmount)
      )
    );
    if (count > 0) rows.push(buildDetailRow("次数", String(count)));
    if (range) rows.push(buildDetailRow("时间", range));
    pushDrawerRow(
      rows,
      "线索金额",
      pickValue(edge, ["claim_amount", "claimed_amount", "report_amount", "reportAmount", "claimAmount"]),
      { money: true, deps: nextDeps }
    );
    pushDrawerRow(
      rows,
      "流水金额",
      pickValue(edge, [
        "evidence_amount",
        "verified_amount",
        "database_amount",
        "actual_amount",
        "evidenceAmount",
        "verifiedAmount",
      ]),
      { money: true, deps: nextDeps }
    );
    pushDrawerRow(rows, "入账合计", pickValue(edge, ["in_amount_verified", "evidence_in_amount"]), {
      money: true,
      deps: nextDeps,
    });
    pushDrawerRow(rows, "转出合计", pickValue(edge, ["out_amount_verified", "evidence_out_amount"]), {
      money: true,
      deps: nextDeps,
    });
    pushDrawerRow(rows, "净额", pickValue(edge, ["net_amount", "evidence_net_amount"]), {
      money: true,
      deps: nextDeps,
    });
    pushDrawerRow(
      rows,
      "证据状态",
      pickValue(edge, ["evidence_status", "evidenceStatus", "trace_status", "traceStatus"]),
      { status: true }
    );
    pushDrawerRow(
      rows,
      "证据口径",
      pickValue(edge, ["evidence_basis", "basis", "amount_scope", "amountScope", "analysis_basis"])
    );
    pushDrawerRow(
      rows,
      "穿透停止原因",
      pickValue(edge, ["trace_stop_reason", "stop_reason", "stopReason", "terminal_reason"])
    );
    pushDrawerRow(
      rows,
      "下一步调取",
      pickValue(edge, ["next_action", "retrieval_action", "nextAction", "retrievalAction"])
    );
    pushDrawerRow(
      rows,
      "调取对象",
      pickValue(edge, ["retrieval_target", "target_subject", "retrievalTarget", "targetSubject"])
    );
    pushDrawerRow(
      rows,
      "调取账号",
      piiProjection.projectField(
        "retrieval_account",
        pickValue(edge, ["retrieval_account", "target_account_no", "account_to_retrieve", "retrievalAccount"])
      )
    );
    pushDrawerRow(
      rows,
      "归属行/受理机构",
      pickValue(edge, ["retrieval_bank", "target_bank", "receiving_bank", "retrievalBank"])
    );
    pushDrawerRow(rows, "备注", pickValue(edge, ["analysis_note", "note", "notes", "remark"]));
    return {
      kind: "edgeDetail",
      title: piiProjection.projectDetected(edge.label || edge.id || "连线详情"),
      rows: rows,
      actions: [],
      meta: null,
    };
  }

  function buildNodeInfoPayload(model, style, deps) {
    var node = model && typeof model === "object" ? model : {};
    var nextStyle = style && typeof style === "object" ? style : {};
    var nextDeps = deps && typeof deps === "object" ? deps : {};
    var stroke = String(node.stroke || nextStyle.nodeColor || "#1f6feb");
    var fill = String(node.fill || nextStyle.nodeFill || "rgba(255,255,255,0.05)");
    var lineWidth = Number(node.lineWidth || nextStyle.nodeWidth || 2);
    var radius = Number(node.r || nextStyle.nodeSize || 18);
    var size = Math.round(Math.max(26, Math.min(52, radius * 2 || 32)));
    var accountList =
      typeof nextDeps.getNodeAccountList === "function" ? nextDeps.getNodeAccountList(node) || [] : [];
    var isGroup = accountList.length > 1;
    var category = isGroup
      ? "账户合集"
      : typeof nextDeps.getNodeCategoryName === "function"
        ? String(nextDeps.getNodeCategoryName(node) || "")
        : "";
    var userName = isGroup
      ? "账户合集"
      : typeof nextDeps.getNodeUserName === "function"
        ? String(nextDeps.getNodeUserName(node) || "")
        : "";
    var account = typeof nextDeps.getNodeAccount === "function" ? String(nextDeps.getNodeAccount(node) || "") : "";
    var listMax = 18;
    return {
      preview: {
        size: size,
        fill: fill,
        stroke: stroke,
        lineWidth: Number.isFinite(lineWidth) ? lineWidth : 2,
      },
      category: piiProjection.projectDetected(category),
      userName: piiProjection.projectDetected(userName),
      accountLabel: isGroup ? "卡号（" + accountList.length + "）" : "卡号",
      accountValue: isGroup ? "" : piiProjection.projectField("account_no", account),
      accountValueMono: !isGroup,
      accounts: isGroup
        ? accountList.slice(0, listMax).map(function projectAccount(value) {
            return piiProjection.projectField("account_no", value);
          })
        : [],
      moreCount: isGroup && accountList.length > listMax ? accountList.length - listMax : 0,
    };
  }

  function buildEdgeTxnOverlayPayload(model, lane, rows, opts, deps) {
    var edge = model && typeof model === "object" ? model : {};
    var selectedLane = String(lane || "");
    var nextRows = Array.isArray(rows) ? rows : [];
    var nextOpts = opts && typeof opts === "object" ? opts : {};
    var nextDeps = deps && typeof deps === "object" ? deps : {};
    var signedEnabled =
      typeof nextDeps.isSignedEdgeDetailEnabled === "function" ? !!nextDeps.isSignedEdgeDetailEnabled(edge) : false;
    var signed =
      typeof nextDeps.edgeSignedMeta === "function"
        ? nextDeps.edgeSignedMeta(edge, selectedLane) || {}
        : {
            selectedLane: selectedLane,
            laneAmount: Number(edge.amount) || 0,
            signedAmount: Number(edge.amount) || 0,
            sign: 1,
            signClass: "",
          };
    var effectiveLane = String(signed.selectedLane || selectedLane || "");
    var laneAmount = Number(signed.laneAmount) || 0;
    var signedAmount = Number(signed.signedAmount) || 0;
    var loading = !!nextOpts.loading;
    var error = String(nextOpts.error || "").trim();
    var enableCountLink = !!nextOpts.enableCountLink;
    var fallbackCount = Math.max(0, Number(edge.count) || 0);
    var hasRows = nextRows.length > 0;
    var showFallbackCount = !loading && !error && !hasRows && fallbackCount > 0;
    var totalCount = hasRows ? nextRows.length : showFallbackCount ? fallbackCount : 0;
    var totalCountText = loading ? "加载中..." : String(totalCount);
    var entries = [];
    var previewRows =
      typeof nextDeps.resolveEdgeTxnReadonlyPreviewRows === "function"
        ? nextDeps.resolveEdgeTxnReadonlyPreviewRows(nextRows)
        : nextRows.slice(0, 5);

    if (hasRows) {
      previewRows.forEach(function eachRow(row) {
        var timeText =
          typeof nextDeps.normalizeTxnTimeText === "function"
            ? String(nextDeps.normalizeTxnTimeText((row && row.txn_time) || "") || "--")
            : String((row && row.txn_time) || "--");
        if (signedEnabled) {
          var rawSigned = Number(row && row.__signedAmount);
          var amountSigned = Number.isFinite(rawSigned)
            ? rawSigned
            : Number(signed.sign) >= 0
              ? Math.abs(Number(row && row.amount) || 0)
              : -Math.abs(Number(row && row.amount) || 0);
          entries.push({
            time: timeText || "--",
            amountText:
              typeof nextDeps.formatSignedMoney === "function"
                ? String(nextDeps.formatSignedMoney(amountSigned) || "")
                : String(amountSigned),
            amountTone: amountSigned >= 0 ? "in" : "out",
          });
          return;
        }
        entries.push({
          time: timeText || "--",
          amountText:
            "￥" +
            (typeof nextDeps.formatMoney === "function"
              ? String(nextDeps.formatMoney(Math.abs(Number(row && row.amount) || 0)) || "")
              : String(Math.abs(Number(row && row.amount) || 0))),
          amountTone: "",
        });
      });
    }

    return {
      title: "资金交易",
      amountText: signedEnabled
        ? typeof nextDeps.formatSignedMoney === "function"
          ? String(nextDeps.formatSignedMoney(signedAmount) || "")
          : String(signedAmount)
        : "￥" + (typeof nextDeps.formatMoney === "function" ? String(nextDeps.formatMoney(laneAmount) || "") : String(laneAmount)),
      amountTone: signedEnabled ? String(signed.signClass || "") : "",
      countText: totalCountText,
      countClickable: enableCountLink && !loading && totalCount > 0,
      loading: loading,
      error: error,
      rows: entries,
      requestKey:
        nextOpts.requestKey != null && String(nextOpts.requestKey || "").trim()
          ? String(nextOpts.requestKey || "")
          : typeof nextDeps.buildRequestKey === "function"
          ? String(nextDeps.buildRequestKey(edge, effectiveLane) || "")
          : String(nextOpts.requestKey || ""),
    };
  }

  function buildFilterOverlayState(state) {
    return {
      open: !!state.reactOverlay?.filter?.open,
      anchorRect: cloneAnchorRect(state.reactOverlay?.filter?.anchorRect),
    };
  }

  function buildMenuOverlayState(state) {
    return {
      open: !!state.reactOverlay?.menu?.open,
      anchorRect: cloneAnchorRect(state.reactOverlay?.menu?.anchorRect),
      sourceKey: String(state.reactOverlay?.menu?.sourceKey || ""),
      items: cloneContextMenuItems(state.reactOverlay?.menu?.items),
    };
  }

  function buildExportOverlayState(state) {
    return {
      open: !!state.reactOverlay?.exportPanel?.open,
      anchorRect: cloneAnchorRect(state.reactOverlay?.exportPanel?.anchorRect),
      scale: clampNumber(Number(state.exportScale) || 1, 1, 4),
    };
  }

  function buildContextMenuOverlayState(state) {
    return {
      open: !!state.reactOverlay?.contextMenu?.open,
      x: Number(state.reactOverlay?.contextMenu?.x) || 0,
      y: Number(state.reactOverlay?.contextMenu?.y) || 0,
      items: cloneContextMenuItems(state.reactOverlay?.contextMenu?.items),
    };
  }

  function buildStylePopoverState(state, deps) {
    var overlay = state.reactOverlay?.stylePopover || {};
    var open = !!overlay.open;
    var anchorRect = cloneAnchorRect(overlay.anchorRect);
    var sourceKey = String(overlay.sourceKey || "");
    var style = state.ribbonStyle || state.graphStyle || {};
    var base = {
      open: open,
      anchorRect: anchorRect,
      sourceKey: sourceKey,
      kind: "",
      currentValue: "",
      options: [],
      commonColors: [],
      recentColors: [],
      iconTabs: [],
      activeTab: "",
      iconQuery: "",
      icons: [],
    };

    if (!open || !anchorRect || !sourceKey) {
      return base;
    }

    if (sourceKey === "style-font-family") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "list",
        currentValue: String(style.fontFamily || ""),
        options: cloneStyleOptions(deps.fontList),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-font-size") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "list",
        currentValue: String(style.fontSize == null ? "" : style.fontSize),
        options: cloneStyleOptions(deps.fontSizeList),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-text-color" || sourceKey === "style-outline-color" || sourceKey === "style-node-fill") {
      var colorTarget = sourceKey === "style-text-color" ? "textColor" : sourceKey === "style-outline-color" ? "outlineColor" : "nodeFill";
      var fallbackColor =
        colorTarget === "textColor" ? "#1f2937" : colorTarget === "outlineColor" ? "#1f6feb" : "#ffffff";
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "color",
        currentValue: String(
          typeof deps.getColorValue === "function" ? deps.getColorValue(colorTarget) || fallbackColor : fallbackColor
        ),
        options: [],
        commonColors: Array.isArray(deps.commonColors) ? deps.commonColors.slice() : [],
        recentColors: (Array.isArray(state.recentColors) ? state.recentColors : []).map(function mapColor(color) {
          return String(color || "");
        }),
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-line-style") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "lineStyle",
        currentValue: String(style.edgeDash || ""),
        options: cloneStyleOptions(deps.lineStyleOptions),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-line-width") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "list",
        currentValue: String(style.edgeWidth == null ? "" : style.edgeWidth),
        options: cloneStyleOptions(deps.lineWidthOptions),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-line-arrow") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "arrow",
        currentValue: String(style.edgeArrow || ""),
        options: cloneStyleOptions(deps.arrowOptions),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-node-shape") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "shape",
        currentValue: String(style.nodeShape || ""),
        options: cloneStyleOptions(deps.nodeShapeOptions),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-icon-size") {
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "list",
        currentValue: String(style.nodeSize == null ? "" : style.nodeSize),
        options: cloneStyleOptions(deps.nodeSizeOptions),
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      };
    }
    if (sourceKey === "style-icon-library") {
      var activeTab = String(state.iconLibrary?.tab || "company");
      var query = String(state.iconLibrary?.query || "").trim().toLowerCase();
      var categories = Array.isArray(deps.iconLibrary) ? deps.iconLibrary : [];
      var category = categories.find(function findCategory(item) {
        return item && item.id === activeTab;
      }) || categories[0] || { icons: [] };
      var icons = (Array.isArray(category.icons) ? category.icons : []).filter(function filterIcon(iconId) {
        return !query || String(iconId || "").toLowerCase().includes(query);
      });
      return {
        open: open,
        anchorRect: anchorRect,
        sourceKey: sourceKey,
        kind: "iconLibrary",
        currentValue: String(style.iconSymbol || ""),
        options: [],
        commonColors: [],
        recentColors: [],
        iconTabs: categories.map(function mapCategory(item) {
          return {
            id: String((item && item.id) || ""),
            label: String((item && item.label) || ""),
          };
        }),
        activeTab: activeTab,
        iconQuery: String(state.iconLibrary?.query || ""),
        icons: icons.map(function mapIcon(iconId) {
          return String(iconId || "");
        }),
      };
    }

    return base;
  }

  function buildDetailDrawerState(state) {
    var drawer = state.reactOverlay?.drawer || {};
    return {
      open: !!drawer.open,
      title: String(drawer.title || ""),
      kind: String(drawer.kind || ""),
      rows: cloneDetailRows(drawer.rows),
      actions: cloneDetailActions(drawer.actions),
    };
  }

  function buildEdgeTxnOverlayState(source) {
    var raw = source && typeof source === "object" ? source : {};
    return {
      title: String(raw.title || ""),
      amountText: String(raw.amountText || ""),
      amountTone: String(raw.amountTone || ""),
      countText: String(raw.countText || ""),
      countClickable: !!raw.countClickable,
      loading: !!raw.loading,
      error: String(raw.error || ""),
      rows: cloneEdgeTxnRows(raw.rows),
      requestKey: String(raw.requestKey || ""),
    };
  }

  function buildNodeInfoOverlayState(state) {
    var nodeInfo = state.reactOverlay?.nodeInfo || {};
    return {
      open: !!nodeInfo.open,
      point: clonePoint(nodeInfo.point),
      preview: nodeInfo.preview
        ? {
            size: clampNumber(Number(nodeInfo.preview.size) || 32, 26, 52),
            fill: String(nodeInfo.preview.fill || "rgba(255,255,255,0.05)"),
            stroke: String(nodeInfo.preview.stroke || "#1f6feb"),
            lineWidth: clampNumber(Number(nodeInfo.preview.lineWidth) || 2, 1, 8),
          }
        : null,
      category: piiProjection.projectDetected(nodeInfo.category || ""),
      userName: piiProjection.projectDetected(nodeInfo.userName || ""),
      accountLabel: String(nodeInfo.accountLabel || ""),
      accountValue: piiProjection.projectField("account_no", nodeInfo.accountValue || ""),
      accountValueMono: !!nodeInfo.accountValueMono,
      accounts: Array.isArray(nodeInfo.accounts)
        ? nodeInfo.accounts.map(function mapAccount(value) {
            return piiProjection.projectField("account_no", value);
          })
        : [],
      moreCount: Math.max(0, Number(nodeInfo.moreCount) || 0),
    };
  }

  function buildNodeHoverPresentation(model, accounts) {
    var source = model && typeof model === "object" ? model : {};
    return {
      title: piiProjection.projectDetected(source.name || source.title || "账户合集"),
      accounts: (Array.isArray(accounts) ? accounts : []).map(function mapAccount(value) {
        return piiProjection.projectField("account_no", value);
      }),
    };
  }

  function buildEdgeInfoOverlayState(state) {
    var edgeInfo = state.reactOverlay?.edgeInfo || {};
    var edgeTxn = buildEdgeTxnOverlayState(edgeInfo);
    return {
      open: !!edgeInfo.open,
      point: clonePoint(edgeInfo.point),
      title: edgeTxn.title,
      amountText: edgeTxn.amountText,
      amountTone: edgeTxn.amountTone,
      countText: edgeTxn.countText,
      countClickable: edgeTxn.countClickable,
      loading: edgeTxn.loading,
      error: edgeTxn.error,
      rows: edgeTxn.rows,
    };
  }

  function buildEdgeLabelOverlayState(state) {
    var edgeLabel = state.reactOverlay?.edgeLabel || {};
    var readonlyState = buildEdgeTxnOverlayState(edgeLabel.readonly);
    return {
      open: !!edgeLabel.open,
      editable: edgeLabel.editable !== false,
      title: piiProjection.projectDetected(edgeLabel.title || "编辑连线文本"),
      value: piiProjection.projectField("node_id", edgeLabel.value || ""),
      placeholder: String(edgeLabel.placeholder || "输入文本内容…"),
      hint: String(edgeLabel.hint || ""),
      confirmLabel: String(edgeLabel.confirmLabel || "确定"),
      cancelLabel: String(edgeLabel.cancelLabel || (edgeLabel.editable === false ? "关闭" : "取消")),
      readonly: {
        open: !!(edgeLabel.readonly && edgeLabel.readonly.open),
        title: readonlyState.title,
        amountText: readonlyState.amountText,
        amountTone: readonlyState.amountTone,
        countText: readonlyState.countText,
        countClickable: readonlyState.countClickable,
        loading: readonlyState.loading,
        error: readonlyState.error,
        rows: readonlyState.rows,
      },
    };
  }

  function buildTxnModalHint(txnModal, deps) {
    var loaded = Array.isArray(txnModal.rows) ? txnModal.rows.length : 0;
    var expected = Number(txnModal.expectedTotal) || 0;
    var sortLabel =
      typeof deps.getTxnSortLabel === "function"
        ? deps.getTxnSortLabel(txnModal.sortCol, txnModal.sortDir)
        : txnModal.sortCol === "amount"
          ? "按金额绝对值排序"
          : "按时间排序";
    if (txnModal.loading && loaded === 0) {
      return "加载中... · " + sortLabel;
    }
    if (txnModal.error) {
      return String(txnModal.error || "") + " · " + sortLabel;
    }
    if (expected > 0) {
      return "共 " + Math.max(expected, loaded) + " 条 · " + sortLabel;
    }
    return "共 " + loaded + " 条 · " + sortLabel;
  }

  function buildTxnModalColumns(txnModal, deps) {
    var cols = Array.isArray(deps.txnColumns) ? deps.txnColumns : [];
    if (!cols.length) {
      var renderModel = global.__ANALYTIX_FLOW_TXN_DETAIL_RENDER_MODEL__ || {};
      cols = Array.isArray(renderModel.TXN_DETAIL_COLS) ? renderModel.TXN_DETAIL_COLS : [];
    }
    var widths = typeof deps.ensureTxnDetailWidths === "function" ? deps.ensureTxnDetailWidths() : [];
    return cols.map(function mapColumn(column, index) {
      var key = String((column && column.key) || "");
      var sorted = key && String(txnModal.sortCol || "") === key;
      return {
        key: key,
        title: String((column && column.title) || key),
        width: clampNumber(Number(widths[index]) || Number(column && column.w) || 120, 64, 480),
        sortable: typeof deps.isTxnSortableCol === "function" ? !!deps.isTxnSortableCol(key) : false,
        sorted: sorted,
        sortDir: sorted ? (txnModal.sortDir === "desc" ? "desc" : "asc") : "",
      };
    });
  }

  function buildTxnModalOverlayState(state, deps) {
    var txnModal = state.txnModal || {};
    if (!deps || typeof deps.buildTxnModalRows !== "function") {
      throw new Error("flow overlay adapter missing buildTxnModalRows");
    }
    var columns = buildTxnModalColumns(txnModal, deps);
    var rowDeps = Object.assign({}, deps, { txnColumns: columns });
    return {
      open: !!txnModal.open,
      title: String(txnModal.title || ""),
      hint: buildTxnModalHint(txnModal, deps),
      loading: !!txnModal.loading,
      error: String(txnModal.error || ""),
      columns: columns,
      rows: deps.buildTxnModalRows(txnModal, rowDeps),
      emptyText: String(txnModal.error || "").trim() || "暂无交易明细",
      sortCol: String(txnModal.sortCol || "txn_time"),
      sortDir: txnModal.sortDir === "desc" ? "desc" : "asc",
    };
  }

  function buildOverlayState(state, deps) {
    return {
      filter: buildFilterOverlayState(state),
      stylePopover: buildStylePopoverState(state, deps || {}),
      menu: buildMenuOverlayState(state),
      exportPanel: buildExportOverlayState(state),
      contextMenu: buildContextMenuOverlayState(state),
      drawer: buildDetailDrawerState(state),
      nodeInfo: buildNodeInfoOverlayState(state),
      edgeInfo: buildEdgeInfoOverlayState(state),
      edgeLabel: buildEdgeLabelOverlayState(state),
      txnModal: buildTxnModalOverlayState(state, deps || {}),
    };
  }

  global.__ANALYTIX_FLOW_OVERLAY_ADAPTER__ = {
    cloneAnchorRect: cloneAnchorRect,
    buildDetailRow: buildDetailRow,
    buildNodeDetailDrawer: buildNodeDetailDrawer,
    buildEdgeDetailDrawer: buildEdgeDetailDrawer,
    buildNodeInfoPayload: buildNodeInfoPayload,
    buildNodeHoverPresentation: buildNodeHoverPresentation,
    buildEdgeTxnOverlayPayload: buildEdgeTxnOverlayPayload,
    buildOverlayState: buildOverlayState,
  };
})(window);
