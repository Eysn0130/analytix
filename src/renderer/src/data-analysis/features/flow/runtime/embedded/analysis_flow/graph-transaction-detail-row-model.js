(() => {
  function requireFunction(deps, name) {
    const fn = deps && deps[name];
    if (typeof fn !== "function") {
      throw new Error(`flow txn detail row model missing ${name}`);
    }
    return fn;
  }

  function normalizeTxnRowDirection(row) {
    const direct = String((row && (row.__dir || row.__signClass)) || "").trim().toLowerCase();
    if (direct === "out" || direct === "in") return direct;
    const dc = String((row && (row.dc_flag || row.dcFlag)) || "").trim();
    if (dc.includes("出")) return "out";
    if (dc.includes("进") || dc.includes("入")) return "in";
    const signedAmount = toTxnNumber(row && row.__signedAmount);
    if (Number.isFinite(signedAmount) && signedAmount < 0) return "out";
    return "in";
  }

  function toTxnNumber(value) {
    if (typeof value === "number") return value;
    if (typeof value === "string" && value.trim()) return Number(value);
    return Number(value);
  }

  function buildTxnModalCell(row, column, deps) {
    const key = String((column && column.key) || "");
    const isTxnMoneyCol = requireFunction(deps, "isTxnMoneyCol");
    const isTxnMonoCol = requireFunction(deps, "isTxnMonoCol");
    const formatTxnMoney = requireFunction(deps, "formatTxnMoney");
    const isMoney = isTxnMoneyCol(key);
    const dir = normalizeTxnRowDirection(row);
    const cell = {
      text: "-",
      mono: isTxnMonoCol(key),
      align: isMoney ? "right" : "left",
      tone: "",
    };

    if (key === "dc_flag") {
      cell.text = String((row && row.dc_flag) || "");
      cell.tone = dir;
      return cell;
    }

    if (isMoney) {
      const raw = row ? row[key] : "";
      const numberValue = toTxnNumber(raw);
      if (Number.isFinite(numberValue)) {
        let displayValue = numberValue;
        if (key === "amount") {
          displayValue = dir === "in" ? Math.abs(numberValue) : -Math.abs(numberValue);
          cell.tone = dir;
        }
        cell.text = formatTxnMoney(displayValue);
      } else if (raw != null && String(raw).trim()) {
        cell.text = String(raw);
      }
      return cell;
    }

    const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__;
    if (!piiProjection || typeof piiProjection.projectField !== "function") {
      throw new Error("ordinary PII projection missing for transaction detail");
    }
    const value = row == null || row[key] == null ? "" : piiProjection.projectField(key, row[key]);
    cell.text = value || "-";
    return cell;
  }

  function buildTxnModalRow(row, index, columns, deps) {
    const cells = {};
    columns.forEach((column) => {
      const key = String((column && column.key) || "");
      cells[key] = buildTxnModalCell(row, column, deps);
    });
    return {
      key: String((row && (row.__txid || row.txn_id || row.id)) || `row-${index}`),
      cells,
    };
  }

  function buildTxnModalRows(txnModal, deps = {}) {
    const sortTxnModalRows = requireFunction(deps, "sortTxnModalRows");
    const sourceRows = Array.isArray(txnModal?.rows) ? txnModal.rows : [];
    const rows = sortTxnModalRows(sourceRows, txnModal?.sortCol, txnModal?.sortDir);
    const columns = Array.isArray(deps.txnColumns) ? deps.txnColumns : [];
    return rows.map((row, index) => buildTxnModalRow(row, index, columns, deps));
  }

  window.__ANALYTIX_FLOW_TXN_DETAIL_ROW_MODEL__ = {
    normalizeTxnRowDirection,
    buildTxnModalCell,
    buildTxnModalRow,
    buildTxnModalRows,
  };
})();
