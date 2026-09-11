(() => {
  const TXN_COL_MIN_W = 60;
  const TXN_COL_MAX_W = 520;
  const TXN_DETAIL_SORTABLE_COL_KEYS = new Set(["txn_time", "amount"]);
  const TXN_DETAIL_MONEY_COL_KEYS = new Set(["amount", "balance", "counterparty_balance"]);
  const TXN_DETAIL_MONO_COL_KEYS = new Set([
    "card_no",
    "acct_no",
    "txn_time",
    "counterparty_acct",
    "voucher_no",
    "terminal_no",
    "ip_addr",
    "mac_addr",
    "txn_id",
    "log_id",
    "voucher_id",
    "teller_no",
    "merchant_no",
  ]);
  const TXN_DETAIL_COLS = [
    { key: "card_no", title: "交易卡号", w: 180 },
    { key: "acct_no", title: "交易账号", w: 180 },
    { key: "account_open_name", title: "账户开户名称", w: 160 },
    { key: "opener_id_no", title: "开户人证件号码", w: 200 },
    { key: "txn_time", title: "交易时间", w: 176 },
    { key: "amount", title: "交易金额", w: 120 },
    { key: "balance", title: "交易余额", w: 120 },
    { key: "dc_flag", title: "收付标志", w: 92 },
    { key: "counterparty_acct", title: "交易对手账卡号", w: 200 },
    { key: "cash_flag", title: "现金标志", w: 92 },
    { key: "counterparty_name", title: "对手户名", w: 160 },
    { key: "counterparty_id_no", title: "对手身份证号", w: 200 },
    { key: "counterparty_bank", title: "对手开户银行", w: 190 },
    { key: "summary", title: "摘要说明", w: 220 },
    { key: "currency", title: "交易币种", w: 100 },
    { key: "branch_name", title: "交易网点名称", w: 180 },
    { key: "branch_code", title: "交易网点代码", w: 140 },
    { key: "location", title: "交易发生地", w: 150 },
    { key: "is_success", title: "交易是否成功", w: 120 },
    { key: "voucher_no", title: "传票号", w: 140 },
    { key: "terminal_no", title: "终端号", w: 130 },
    { key: "ip_addr", title: "IP地址", w: 160 },
    { key: "mac_addr", title: "MAC地址", w: 180 },
    { key: "counterparty_balance", title: "对手交易余额", w: 130 },
    { key: "txn_id", title: "交易流水号", w: 190 },
    { key: "log_id", title: "日志号", w: 150 },
    { key: "voucher_type", title: "凭证种类", w: 130 },
    { key: "voucher_id", title: "凭证号", w: 150 },
    { key: "teller_no", title: "交易柜员号", w: 120 },
    { key: "merchant_name", title: "商户名称", w: 180 },
    { key: "merchant_no", title: "商户号", w: 150 },
    { key: "remark", title: "备注", w: 180 },
    { key: "txn_type", title: "交易类型", w: 140 },
    { key: "query_feedback_reason", title: "查询反馈结果原因", w: 220 },
  ];
  function defaultFormatMoney(value) {
    const num = Number(value);
    return Number.isFinite(num) ? num.toFixed(2) : "0.00";
  }

  function resolveFmtMoney(fmtMoney) {
    return typeof fmtMoney === "function" ? fmtMoney : defaultFormatMoney;
  }

  function formatTxnMoney(value, options = {}) {
    return `￥${resolveFmtMoney(options.fmtMoney)(value)}`;
  }

  function isTxnSortableCol(colKey) {
    return TXN_DETAIL_SORTABLE_COL_KEYS.has(String(colKey || ""));
  }

  function isTxnMoneyCol(colKey) {
    return TXN_DETAIL_MONEY_COL_KEYS.has(String(colKey || ""));
  }

  function isTxnMonoCol(colKey) {
    return TXN_DETAIL_MONO_COL_KEYS.has(String(colKey || ""));
  }

  function getTxnSortLabel(sortCol, sortDir) {
    const dir = sortDir === "desc" ? "降序" : "升序";
    return sortCol === "amount" ? `按金额绝对值${dir}` : `按时间${dir}`;
  }

  window.__ANALYTIX_FLOW_TXN_DETAIL_RENDER_MODEL__ = {
    TXN_COL_MIN_W,
    TXN_COL_MAX_W,
    TXN_DETAIL_SORTABLE_COL_KEYS,
    TXN_DETAIL_MONEY_COL_KEYS,
    TXN_DETAIL_MONO_COL_KEYS,
    TXN_DETAIL_COLS,
    formatTxnMoney,
    isTxnSortableCol,
    isTxnMoneyCol,
    isTxnMonoCol,
    getTxnSortLabel,
  };
})();
