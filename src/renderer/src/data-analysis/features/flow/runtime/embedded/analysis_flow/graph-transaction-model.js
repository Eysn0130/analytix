(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) throw new Error("flow graph transaction model root missing");

  function normalizeKey(value) {
    const raw = String(value || "").trim();
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

  function normalizeTxnTimeText(value) {
    const s = String(value || "").trim();
    if (!s) return "";
    const m = s.match(/^(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}:\d{2}:\d{2}))?$/);
    if (!m) return s;
    return m[2] ? `${m[1]} ${m[2]}` : `${m[1]} 00:00:00`;
  }

  function parseTxnTimeValue(value) {
    const s = String(value || "").trim();
    if (!s) return null;
    const m = s.match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}):(\d{2}))?$/);
    if (!m) return null;
    const y = Number(m[1]);
    const mo = Number(m[2]) - 1;
    const d = Number(m[3]);
    const hh = Number(m[4] || 0);
    const mm = Number(m[5] || 0);
    const ss = Number(m[6] || 0);
    const dt = new Date(y, mo, d, hh, mm, ss);
    const ms = dt.getTime();
    return Number.isFinite(ms) ? ms : null;
  }

  function finiteAmount(value) {
    if (value == null || (typeof value === "string" && !value.trim())) return null;
    const amount = Number(value);
    return Number.isFinite(amount) ? amount : null;
  }

  function normalizeAccountTxnRows(rows) {
    return (rows || []).flatMap((r) => {
      const dc = String(r.dc_flag || r.dcFlag || "").trim();
      const dir = dc.includes("进")
        ? "in"
        : dc.includes("出")
          ? "out"
          : "";
      const amount = finiteAmount(r.amount);
      if (!dir || amount == null) return [];
      const txid = r.__txid || r.txn_id || r.id || "";
      const txnTime = r.txn_time || r.txnTime || r.txn_ts || "";
      return [{ ...r, amount, __dir: dir, __txid: txid, txn_time: txnTime }];
    });
  }

  function normalizeTxnModalRows(rows) {
    const normalized = normalizeAccountTxnRows(rows || []);
    return normalized.map((row) => {
      const dir = row.__dir === "out" ? "out" : "in";
      const dc = String(row?.dc_flag || row?.dcFlag || "").trim() || (dir === "in" ? "进" : "出");
      return {
        ...row,
        __dir: dir,
        __txid: String(row?.__txid || row?.txn_id || row?.id || "").trim(),
        txn_time: normalizeTxnTimeText(row?.txn_time || row?.txnTime || row?.txn_ts || ""),
        dc_flag: dc,
      };
    });
  }

  function sortTxnModalRows(rows, sortCol, sortDir) {
    const list = (rows || []).slice();
    const dir = sortDir === "desc" ? -1 : 1;
    if (sortCol === "amount") {
      list.sort((a, b) => {
        const av = finiteAmount(a?.amount);
        const bv = finiteAmount(b?.amount);
        if (av == null && bv == null) return 0;
        if (av == null) return 1;
        if (bv == null) return -1;
        if (Math.abs(av) !== Math.abs(bv)) return (Math.abs(av) - Math.abs(bv)) * dir;
        const ta = parseTxnTimeValue(a?.txn_time || "") || 0;
        const tb = parseTxnTimeValue(b?.txn_time || "") || 0;
        return (ta - tb) * dir;
      });
      return list;
    }
    list.sort((a, b) => {
      const ta = parseTxnTimeValue(a?.txn_time || "") || 0;
      const tb = parseTxnTimeValue(b?.txn_time || "") || 0;
      if (ta !== tb) return (ta - tb) * dir;
      const sa = String(a?.__txid || a?.txn_id || "");
      const sb = String(b?.__txid || b?.txn_id || "");
      return sa.localeCompare(sb, "zh-CN");
    });
    return list;
  }

  function matchInboundTxn(rows, baseTxn) {
    if (!baseTxn) return null;
    const baseTxId = String(baseTxn.txn_id || "").trim();
    const baseAmountValue = finiteAmount(baseTxn.amount);
    if (baseAmountValue == null) return null;
    const baseAmount = Math.abs(baseAmountValue);
    const baseTime = parseTxnTimeValue(baseTxn.time);
    const cpAccount = normalizeKey(baseTxn?.from?.account || "");
    const cpName = String(baseTxn?.from?.name || "").trim();

    let candidates = rows.filter((r) => r.__dir === "in");
    if (baseTxId) {
      const direct = candidates.find((r) => String(r.txn_id || r.__txid || "").trim() === baseTxId);
      if (direct) {
        direct.__matchReason = "txn_id";
        return direct;
      }
    }
    const reasonParts = [];
    reasonParts.push("amount");
    if (cpAccount) reasonParts.push("counterparty_acct");
    else if (cpName) reasonParts.push("counterparty_name");
    if (baseTime) reasonParts.push("time±60s");
    candidates = candidates.filter((r) => {
      const amountValue = finiteAmount(r.amount);
      if (amountValue == null || Math.abs(Math.abs(amountValue) - baseAmount) > 0.01) return false;
      if (cpAccount) {
        const cp = normalizeKey(r.counterparty_acct || "");
        if (cp && cp !== cpAccount) return false;
      } else if (cpName) {
        const nm = String(r.counterparty_name || "").trim();
        if (nm && !nm.includes(cpName)) return false;
      }
      if (baseTime) {
        const rt = parseTxnTimeValue(r.txn_time);
        if (!rt || Math.abs(rt - baseTime) > 60 * 1000) return false;
      }
      return true;
    });
    if (!candidates.length) return null;
    if (baseTime) {
      candidates.sort((a, b) => {
        const ta = parseTxnTimeValue(a.txn_time) || 0;
        const tb = parseTxnTimeValue(b.txn_time) || 0;
        return Math.abs(ta - baseTime) - Math.abs(tb - baseTime);
      });
    }
    const picked = candidates[0];
    if (picked) {
      picked.__matchReason = reasonParts.length ? reasonParts.join("+") : "inbound";
    }
    return picked;
  }

  function normalizeDrillConfig(config = {}) {
    const policyRaw = String(config.policy || "auto").toLowerCase();
    const policy = ["auto", "balance", "time"].includes(policyRaw) ? policyRaw : "auto";
    let windowDays = Number(config.windowDays);
    if (!Number.isFinite(windowDays) || windowDays < 0) windowDays = 7;
    if (windowDays > 3650) windowDays = 3650;
    return { policy, windowDays };
  }

  function computeDrillOutflows(rows, baseTxn, config = {}) {
    const normalizedConfig = normalizeDrillConfig(config);
    const baseAmountValue = finiteAmount(baseTxn?.amount);
    if (baseAmountValue == null) {
      return { inbound: null, outflows: [], baseBalance: null, anchorTime: null, policy: normalizedConfig.policy, windowDays: normalizedConfig.windowDays, maxTime: null };
    }
    const inbound = matchInboundTxn(rows, baseTxn);
    const anchorTime = parseTxnTimeValue(inbound?.txn_time || baseTxn?.time || "");
    const baseAmount = Math.abs(baseAmountValue);
    const inboundBalance = finiteAmount(inbound?.balance);
    const baseBalance = inboundBalance == null ? null : inboundBalance - baseAmount;
    const policy = normalizedConfig.policy;
    const windowDays = normalizedConfig.windowDays;
    const useTimeWindow = windowDays > 0 && (policy === "time" || policy === "auto" || policy === "balance");
    const maxTime =
      useTimeWindow && anchorTime != null ? anchorTime + windowDays * 24 * 60 * 60 * 1000 : null;
    const useBalanceStop = (policy === "balance" || policy === "auto") && baseBalance != null;

    const sorted = rows
      .slice()
      .sort((a, b) => {
        const ta = parseTxnTimeValue(a.txn_time);
        const tb = parseTxnTimeValue(b.txn_time);
        if (ta == null && tb == null) return 0;
        if (ta == null) return 1;
        if (tb == null) return -1;
        return ta - tb;
      });

    const outflows = [];
    for (const row of sorted) {
      if (row.__dir !== "out") continue;
      const t = parseTxnTimeValue(row.txn_time);
      if (anchorTime != null && t != null && t < anchorTime) continue;
      if (anchorTime != null && t == null) continue;
      if (maxTime != null && t != null && t > maxTime) continue;
      const amountValue = finiteAmount(row.amount);
      if (amountValue == null || Math.abs(amountValue) === 0) continue;
      outflows.push(row);
      if (useBalanceStop) {
        const after = finiteAmount(row.balance);
        if (after != null && after < baseBalance - 0.0001) break;
      }
    }
    return { inbound, outflows, baseBalance, anchorTime, policy, windowDays, maxTime };
  }

  root.__ANALYTIX_FLOW_GRAPH_TRANSACTION_MODEL__ = Object.freeze({
    normalizeTxnTimeText,
    parseTxnTimeValue,
    normalizeAccountTxnRows,
    normalizeTxnModalRows,
    sortTxnModalRows,
    matchInboundTxn,
    computeDrillOutflows,
  });
})();
