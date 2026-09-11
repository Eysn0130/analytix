#![recursion_limit = "256"]
#![allow(dead_code)]

use anyhow::{anyhow, bail, Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use std::env;
use std::io::{self, BufRead, Write};
use std::path::{Path, PathBuf};
use std::time::Instant;

mod account_info;
mod amount_balance;
pub mod export_realtime;
pub(crate) mod sql_update;
pub mod step_impact;

use account_info::{run_account_info, run_account_info_on_conn};
use amount_balance::{run_amount_balance, run_amount_balance_on_conn};
use sql_update::update_count;

#[derive(Debug)]
pub(crate) struct Args {
    pub(crate) command: String,
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) txn_file_ids: Vec<String>,
    pub(crate) acc_file_ids: Vec<String>,
}

#[derive(Debug)]
struct QualityFlagSummary {
    invalid_changed: i64,
    invalid_total: i64,
    duplicate_changed: i64,
    duplicate_total: i64,
    failed_changed: i64,
    reversal_changed: i64,
    failed_total: i64,
    reversal_total: i64,
}

#[derive(Debug)]
struct DcFlagSummary {
    normalized: i64,
    inferred: i64,
    normalized_total: i64,
    inferred_total: i64,
}

#[derive(Debug)]
struct AccountKeySummary {
    filled_count: i64,
    filled_total: i64,
    suffix_txn_count: i64,
    suffix_txn_total: i64,
    account_invalid_count: i64,
    account_invalid_total: i64,
    suffix_acc_count: i64,
    suffix_acc_total: i64,
}

#[derive(Clone, Copy, Debug, Default)]
struct NativeCompletionTimings {
    mark_txn_ms: i64,
    privacy_projection_ms: i64,
    mark_account_ms: i64,
    account_state_ms: i64,
    total_ms: i64,
}

struct TimedTxnConnection {
    conn: Connection,
    open_db_ms: i64,
    configure_ms: i64,
    ensure_txn_table_ms: i64,
    txn_scope_ms: i64,
}

pub fn run_cli_from_env() -> Result<()> {
    if env::args().nth(1).as_deref() == Some("worker") {
        return run_worker();
    }

    let args = parse_args()?;
    match args.command.as_str() {
        "clean-all" => {
            let payload = run_clean_all(&args)?;
            let mut stdout = io::stdout().lock();
            writeln!(stdout, "{payload}")?;
            stdout.flush()?;
            std::process::exit(0)
        }
        "amount-balance" => {
            let summary = run_amount_balance(&args)?;
            let timings = summary.timings;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "amount_updated": summary.amount_updated,
                    "balance_updated": summary.balance_updated,
                    "amount_failed": summary.amount_failed,
                    "balance_failed": summary.balance_failed,
                    "amount_fixed_total": summary.amount_fixed_total,
                    "amount_failed_total": summary.amount_failed_total,
                    "balance_fixed_total": summary.balance_fixed_total,
                    "balance_failed_total": summary.balance_failed_total,
                    "rows_affected": summary.rows_affected,
                    "amount_balance_desired_ms": timings.desired_ms,
                    "amount_balance_counts_ms": timings.counts_ms,
                    "amount_balance_update_ms": timings.update_ms,
                    "amount_balance_totals_ms": timings.totals_ms,
                })
            );
            Ok(())
        }
        "quality-flags" => {
            let summary = run_quality_flags(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "invalid_changed": summary.invalid_changed,
                    "invalid_total": summary.invalid_total,
                    "duplicate_changed": summary.duplicate_changed,
                    "duplicate_total": summary.duplicate_total,
                    "failed_changed": summary.failed_changed,
                    "reversal_changed": summary.reversal_changed,
                    "failed_total": summary.failed_total,
                    "reversal_total": summary.reversal_total,
                })
            );
            Ok(())
        }
        "dc-flag" => {
            let summary = run_dc_flag(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "normalized": summary.normalized,
                    "inferred": summary.inferred,
                    "normalized_total": summary.normalized_total,
                    "inferred_total": summary.inferred_total,
                })
            );
            Ok(())
        }
        "account-keys" => {
            let summary = run_account_keys(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "filled_count": summary.filled_count,
                    "filled_total": summary.filled_total,
                    "suffix_txn_count": summary.suffix_txn_count,
                    "suffix_txn_total": summary.suffix_txn_total,
                    "account_invalid_count": summary.account_invalid_count,
                    "account_invalid_total": summary.account_invalid_total,
                    "suffix_acc_count": summary.suffix_acc_count,
                    "suffix_acc_total": summary.suffix_acc_total,
                })
            );
            Ok(())
        }
        "account-info" => {
            let summary = run_account_info(&args)?;
            let timings = summary.timings;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "account_fill_count": summary.account_fill_count,
                    "account_fill_total": summary.account_fill_total,
                    "account_info_pending_ms": timings.pending_ms,
                    "account_info_key_maps_ms": timings.key_maps_ms,
                    "account_info_primary_match_ms": timings.primary_match_ms,
                    "account_info_projection_ms": timings.projection_ms,
                    "account_info_txn_unique_map_ms": timings.txn_unique_map_ms,
                    "account_info_txn_unique_match_ms": timings.txn_unique_match_ms,
                    "account_info_counterparty_map_ms": timings.counterparty_map_ms,
                    "account_info_counterparty_match_ms": timings.counterparty_match_ms,
                    "account_info_alpha_card_match_ms": timings.alpha_card_match_ms,
                    "account_info_acct_to_card_map_ms": timings.acct_to_card_map_ms,
                    "account_info_acct_to_card_match_ms": timings.acct_to_card_match_ms,
                    "account_info_fill_total_ms": timings.fill_total_ms,
                })
            );
            Ok(())
        }
        other => bail!("unsupported command: {other}"),
    }
}

fn run_worker() -> Result<()> {
    let stdin = io::stdin();
    let mut stdout = io::stdout().lock();
    for line in stdin.lock().lines() {
        let line = line?;
        if line.trim().is_empty() {
            continue;
        }
        let response = match run_worker_request(&line) {
            Ok(payload) => payload,
            Err(err) => json!({
                "ok": false,
                "error": err.to_string(),
            }),
        };
        writeln!(stdout, "{response}")?;
        stdout.flush()?;
    }
    Ok(())
}

pub fn run_data_engine_clean_all(
    case_id: &str,
    db_path: &Path,
    txn_file_ids: &[String],
    acc_file_ids: &[String],
) -> Result<serde_json::Value> {
    let args = Args {
        command: "clean-all".to_string(),
        case_id: case_id.to_string(),
        db_path: db_path.to_path_buf(),
        txn_file_ids: txn_file_ids.to_vec(),
        acc_file_ids: acc_file_ids.to_vec(),
    };
    run_clean_all(&args)
}

fn run_worker_request(line: &str) -> Result<Value> {
    let request: Value = serde_json::from_str(line).context("invalid worker request json")?;
    if request
        .get("command")
        .and_then(Value::as_str)
        .is_some_and(|command| command == "ping")
    {
        return Ok(json!({
            "ok": true,
            "command": "ping",
        }));
    }
    let args = args_from_worker_request(&request)?;
    match args.command.as_str() {
        "clean-all" => run_clean_all(&args),
        other => bail!("unsupported worker command: {other}"),
    }
}

fn args_from_worker_request(request: &Value) -> Result<Args> {
    let command = string_field(request, "command")?;
    let case_id = string_field(request, "case_id")?;
    let db_path = PathBuf::from(string_field(request, "db_path")?);
    let txn_file_ids = string_array_field(request, "txn_file_ids")?;
    let acc_file_ids = string_array_field(request, "acc_file_ids")?;
    if case_id.trim().is_empty() {
        bail!("missing case_id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing db_path");
    }
    Ok(Args {
        command,
        case_id,
        db_path,
        txn_file_ids,
        acc_file_ids,
    })
}

fn string_field(request: &Value, name: &str) -> Result<String> {
    request
        .get(name)
        .and_then(Value::as_str)
        .map(ToString::to_string)
        .ok_or_else(|| anyhow!("worker request missing string field: {name}"))
}

fn string_array_field(request: &Value, name: &str) -> Result<Vec<String>> {
    let Some(value) = request.get(name) else {
        return Ok(Vec::new());
    };
    let Some(items) = value.as_array() else {
        bail!("worker request field must be a string array: {name}");
    };
    let mut result = Vec::new();
    for item in items {
        let Some(text) = item.as_str() else {
            bail!("worker request field must be a string array: {name}");
        };
        let trimmed = text.trim();
        if !trimmed.is_empty() {
            result.push(trimmed.to_string());
        }
    }
    Ok(result)
}

fn parse_args() -> Result<Args> {
    let mut command = String::new();
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut txn_file_ids: Vec<String> = Vec::new();
    let mut acc_file_ids: Vec<String> = Vec::new();
    let mut iter = env::args().skip(1);
    while let Some(arg) = iter.next() {
        match arg.as_str() {
            "clean-all" | "amount-balance" | "quality-flags" | "dc-flag" | "account-keys"
            | "account-info"
                if command.is_empty() =>
            {
                command = arg;
            }
            "--case-id" => {
                case_id = iter
                    .next()
                    .ok_or_else(|| anyhow!("--case-id requires a value"))?;
            }
            "--db-path" => {
                db_path = PathBuf::from(
                    iter.next()
                        .ok_or_else(|| anyhow!("--db-path requires a value"))?,
                );
            }
            "--txn-file-id" => {
                let value = iter
                    .next()
                    .ok_or_else(|| anyhow!("--txn-file-id requires a value"))?;
                let trimmed = value.trim();
                if !trimmed.is_empty() {
                    txn_file_ids.push(trimmed.to_string());
                }
            }
            "--acc-file-id" => {
                let value = iter
                    .next()
                    .ok_or_else(|| anyhow!("--acc-file-id requires a value"))?;
                let trimmed = value.trim();
                if !trimmed.is_empty() {
                    acc_file_ids.push(trimmed.to_string());
                }
            }
            "--help" | "-h" => {
                println!(
                    "Usage: analytix-cleaning-ops <clean-all|amount-balance|quality-flags|dc-flag|account-keys|account-info> --case-id <id> --db-path <case.duckdb> [--txn-file-id <file_id> ...] [--acc-file-id <file_id> ...]"
                );
                std::process::exit(0);
            }
            _ => bail!("unknown argument: {arg}"),
        }
    }
    if command.is_empty() {
        bail!("missing command");
    }
    if case_id.trim().is_empty() {
        bail!("missing --case-id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing --db-path");
    }
    Ok(Args {
        command,
        case_id,
        db_path,
        txn_file_ids,
        acc_file_ids,
    })
}

fn run_clean_all(args: &Args) -> Result<serde_json::Value> {
    let clean_all_started = Instant::now();
    let txn_conn = open_txn_connection_timed(args)?;
    let conn = txn_conn.conn;
    let ensure_acct_state_started = Instant::now();
    ensure_required_table(&conn, "acct_state")?;
    let clean_all_acct_state_ms = elapsed_ms(ensure_acct_state_started);
    let acc_scope_started = Instant::now();
    create_acc_scope(&conn, &args.case_id, &args.acc_file_ids)?;
    let clean_all_acc_scope_ms = elapsed_ms(acc_scope_started);

    let begin_started = Instant::now();
    conn.execute_batch("BEGIN TRANSACTION")?;
    let clean_all_begin_ms = elapsed_ms(begin_started);
    let steps_started = Instant::now();
    let result = (|| -> Result<serde_json::Value> {
        let amount_started = Instant::now();
        let amount = run_amount_balance_on_conn(&conn, args)?;
        let amount_balance_ms = elapsed_ms(amount_started);
        let amount_timings = amount.timings;
        let quality_started = Instant::now();
        let quality = run_quality_flags_on_conn(&conn, args)?;
        let quality_flags_ms = elapsed_ms(quality_started);
        let dc_started = Instant::now();
        let dc = run_dc_flag_on_conn(&conn, args)?;
        let dc_flag_ms = elapsed_ms(dc_started);
        let keys_started = Instant::now();
        let keys = run_account_keys_on_conn(&conn, args)?;
        let account_keys_ms = elapsed_ms(keys_started);
        let info_started = Instant::now();
        let info = run_account_info_on_conn(&conn, args)?;
        let account_info_ms = elapsed_ms(info_started);
        let info_timings = info.timings;
        let rows_affected_total = amount.rows_affected
            + quality.invalid_changed
            + quality.duplicate_changed
            + quality.failed_changed
            + quality.reversal_changed
            + dc.normalized
            + dc.inferred
            + keys.filled_count
            + keys.suffix_txn_count
            + keys.account_invalid_count
            + keys.suffix_acc_count
            + info.account_fill_count;

        Ok(json!({
            "ok": true,
            "case_id": args.case_id,
            "amount_updated": amount.amount_updated,
            "balance_updated": amount.balance_updated,
            "amount_failed": amount.amount_failed,
            "balance_failed": amount.balance_failed,
            "amount_fixed_total": amount.amount_fixed_total,
            "amount_failed_total": amount.amount_failed_total,
            "balance_fixed_total": amount.balance_fixed_total,
            "balance_failed_total": amount.balance_failed_total,
            "rows_affected": amount.rows_affected,
            "invalid_changed": quality.invalid_changed,
            "invalid_total": quality.invalid_total,
            "duplicate_changed": quality.duplicate_changed,
            "duplicate_total": quality.duplicate_total,
            "failed_changed": quality.failed_changed,
            "reversal_changed": quality.reversal_changed,
            "failed_total": quality.failed_total,
            "reversal_total": quality.reversal_total,
            "normalized": dc.normalized,
            "inferred": dc.inferred,
            "normalized_total": dc.normalized_total,
            "inferred_total": dc.inferred_total,
            "filled_count": keys.filled_count,
            "filled_total": keys.filled_total,
            "suffix_txn_count": keys.suffix_txn_count,
            "suffix_txn_total": keys.suffix_txn_total,
            "account_invalid_count": keys.account_invalid_count,
            "account_invalid_total": keys.account_invalid_total,
            "suffix_acc_count": keys.suffix_acc_count,
            "suffix_acc_total": keys.suffix_acc_total,
            "account_fill_count": info.account_fill_count,
            "account_fill_total": info.account_fill_total,
            "rows_affected_total": rows_affected_total,
            "clean_all_open_db_ms": txn_conn.open_db_ms,
            "clean_all_configure_ms": txn_conn.configure_ms,
            "clean_all_ensure_txn_table_ms": txn_conn.ensure_txn_table_ms,
            "clean_all_txn_scope_ms": txn_conn.txn_scope_ms,
            "clean_all_acct_state_ms": clean_all_acct_state_ms,
            "clean_all_acc_scope_ms": clean_all_acc_scope_ms,
            "clean_all_begin_ms": clean_all_begin_ms,
            "amount_balance_ms": amount_balance_ms,
            "amount_balance_desired_ms": amount_timings.desired_ms,
            "amount_balance_counts_ms": amount_timings.counts_ms,
            "amount_balance_update_ms": amount_timings.update_ms,
            "amount_balance_totals_ms": amount_timings.totals_ms,
            "quality_flags_ms": quality_flags_ms,
            "dc_flag_ms": dc_flag_ms,
            "account_keys_ms": account_keys_ms,
            "account_info_ms": account_info_ms,
            "account_info_pending_ms": info_timings.pending_ms,
            "account_info_key_maps_ms": info_timings.key_maps_ms,
            "account_info_primary_match_ms": info_timings.primary_match_ms,
            "account_info_projection_ms": info_timings.projection_ms,
            "account_info_txn_unique_map_ms": info_timings.txn_unique_map_ms,
            "account_info_txn_unique_match_ms": info_timings.txn_unique_match_ms,
            "account_info_counterparty_map_ms": info_timings.counterparty_map_ms,
            "account_info_counterparty_match_ms": info_timings.counterparty_match_ms,
            "account_info_alpha_card_match_ms": info_timings.alpha_card_match_ms,
            "account_info_acct_to_card_map_ms": info_timings.acct_to_card_map_ms,
            "account_info_acct_to_card_match_ms": info_timings.acct_to_card_match_ms,
            "account_info_fill_total_ms": info_timings.fill_total_ms,
        }))
    })();

    match result {
        Ok(mut payload) => {
            let clean_all_steps_ms = elapsed_ms(steps_started);
            let completion_started = Instant::now();
            let completion = match apply_native_completion_on_conn(&conn, args) {
                Ok(completion) => completion,
                Err(err) => {
                    let _ = conn.execute_batch("ROLLBACK");
                    return Err(err);
                }
            };
            let native_completion_total_ms = elapsed_ms(completion_started);
            let commit_started = Instant::now();
            conn.execute_batch("COMMIT")?;
            let clean_all_commit_ms = elapsed_ms(commit_started);
            let connection_drop_started = Instant::now();
            drop(conn);
            let clean_all_connection_drop_ms = elapsed_ms(connection_drop_started);
            if let Some(object) = payload.as_object_mut() {
                object.insert("clean_all_steps_ms".to_string(), json!(clean_all_steps_ms));
                object.insert(
                    "clean_all_commit_ms".to_string(),
                    json!(clean_all_commit_ms),
                );
                object.insert(
                    "clean_all_connection_drop_ms".to_string(),
                    json!(clean_all_connection_drop_ms),
                );
                object.insert(
                    "clean_all_rust_total_ms".to_string(),
                    json!(elapsed_ms(clean_all_started)),
                );
                object.insert("native_completion_applied".to_string(), json!(1));
                object.insert(
                    "native_completion_mark_txn_ms".to_string(),
                    json!(completion.mark_txn_ms),
                );
                object.insert(
                    "native_completion_privacy_projection_ms".to_string(),
                    json!(completion.privacy_projection_ms),
                );
                object.insert(
                    "native_completion_mark_account_ms".to_string(),
                    json!(completion.mark_account_ms),
                );
                object.insert(
                    "native_completion_account_state_ms".to_string(),
                    json!(completion.account_state_ms),
                );
                object.insert(
                    "native_completion_total_ms".to_string(),
                    json!(native_completion_total_ms.max(completion.total_ms)),
                );
            }
            Ok(payload)
        }
        Err(err) => {
            let _ = conn.execute_batch("ROLLBACK");
            Err(err)
        }
    }
}

fn run_account_keys(args: &Args) -> Result<AccountKeySummary> {
    let conn = open_txn_connection(args)?;
    create_acc_scope(&conn, &args.case_id, &args.acc_file_ids)?;
    run_account_keys_on_conn(&conn, args)
}

fn run_account_keys_on_conn(conn: &Connection, args: &Args) -> Result<AccountKeySummary> {
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    if scope_rows <= 0 {
        bail!("no transaction rows in cleaning scope");
    }

    let mut filled_count = 0;
    filled_count += update_count(
        conn,
        "UPDATE fc_transaction_norm
            SET orig_card_no=COALESCE(NULLIF(orig_card_no,''), card_no),
                clean_card_no=COALESCE(NULLIF(clean_card_no,''), acct_no),
                clean_card_filled=1
          WHERE id IN (SELECT id FROM txn_scope)
            AND COALESCE(TRIM(card_no),'')=''
            AND COALESCE(TRIM(clean_card_no),'')=''
            AND COALESCE(TRIM(acct_no),'')<>''",
    )?;
    filled_count += update_count(
        conn,
        "UPDATE fc_transaction_norm
            SET clean_acct_no=COALESCE(NULLIF(clean_acct_no,''), card_no),
                clean_card_filled=1
          WHERE id IN (SELECT id FROM txn_scope)
            AND COALESCE(TRIM(acct_no),'')=''
            AND COALESCE(TRIM(clean_acct_no),'')=''
            AND COALESCE(TRIM(card_no),'')<>''",
    )?;
    let filled_total = scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={} AND clean_card_filled=1",
            sql_literal(&args.case_id)
        ),
    )?;

    let src_txn_card = "COALESCE(NULLIF(clean_card_no,''), card_no)";
    let src_txn_acct = "COALESCE(NULLIF(clean_acct_no,''), acct_no)";
    let suffix_txn_sql = suffix_update_sql(
        "fc_transaction_norm",
        "txn_scope",
        src_txn_card,
        src_txn_acct,
    );
    let suffix_txn_count = update_count(conn, &suffix_txn_sql)?;
    let suffix_txn_total = scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={} AND clean_suffix_fixed=1",
            sql_literal(&args.case_id)
        ),
    )?;

    let mut account_invalid_count = 0;
    let mut account_invalid_total = 0;
    let mut suffix_acc_count = 0;
    let mut suffix_acc_total = 0;
    if table_exists(conn, "fc_account_norm")? {
        account_invalid_count = update_count(
            conn,
            "UPDATE fc_account_norm
                SET clean_acct_invalid=1
              WHERE id IN (SELECT id FROM acc_scope)
                AND COALESCE(clean_acct_invalid, 0)=0
                AND (COALESCE(TRIM(card_no),'')='' OR TRIM(card_no) IN ('_', '-'))
                AND (COALESCE(TRIM(acct_no),'')='' OR TRIM(acct_no) IN ('_', '-'))",
        )?;
        account_invalid_total = scalar_i64(
            conn,
            &format!(
                "SELECT COUNT(1) FROM fc_account_norm WHERE case_id={} AND clean_acct_invalid=1",
                sql_literal(&args.case_id)
            ),
        )?;

        let src_acc_card = "COALESCE(NULLIF(clean_card_no,''), card_no)";
        let src_acc_acct = "COALESCE(NULLIF(clean_acct_no,''), acct_no)";
        let suffix_acc_sql =
            suffix_update_sql("fc_account_norm", "acc_scope", src_acc_card, src_acc_acct);
        suffix_acc_count = update_count(conn, &suffix_acc_sql)?;
        suffix_acc_total = scalar_i64(
            conn,
            &format!(
                "SELECT COUNT(1) FROM fc_account_norm WHERE case_id={} AND clean_suffix_fixed=1",
                sql_literal(&args.case_id)
            ),
        )?;
    }

    Ok(AccountKeySummary {
        filled_count,
        filled_total,
        suffix_txn_count,
        suffix_txn_total,
        account_invalid_count,
        account_invalid_total,
        suffix_acc_count,
        suffix_acc_total,
    })
}

fn apply_native_completion_on_conn(
    conn: &Connection,
    args: &Args,
) -> Result<NativeCompletionTimings> {
    let total_started = Instant::now();
    let mut timings = NativeCompletionTimings::default();
    let cleaned_at = scalar_string(
        conn,
        "SELECT strftime(CAST(NOW() AS TIMESTAMP), '%Y-%m-%d %H:%M:%S')",
    )?;
    let cleaned_at_sql = sql_literal(&cleaned_at);
    let case_id_sql = sql_literal(&args.case_id);

    let started = Instant::now();
    conn.execute_batch(&format!(
        "UPDATE fc_transaction_norm
            SET cleaned_at={cleaned_at}
          WHERE id IN (SELECT id FROM txn_scope)",
        cleaned_at = cleaned_at_sql,
    ))?;
    timings.mark_txn_ms = elapsed_ms(started);

    let started = Instant::now();
    conn.execute_batch(&format!(
        r#"
        CREATE TABLE IF NOT EXISTS privacy_projection_delta_log(
            case_id TEXT,
            op TEXT,
            file_id TEXT,
            row_hash TEXT,
            source TEXT,
            created_at TEXT
        );
        INSERT INTO privacy_projection_delta_log(case_id, op, file_id, row_hash, source, created_at)
        SELECT DISTINCT {case_id}, 'upsert', COALESCE(file_id, ''), row_hash, 'cleaning:run', {cleaned_at}
          FROM fc_transaction_norm
         WHERE id IN (SELECT id FROM txn_scope)
           AND COALESCE(row_hash, '')<>''
        "#,
        case_id = case_id_sql,
        cleaned_at = cleaned_at_sql,
    ))?;
    timings.privacy_projection_ms = elapsed_ms(started);

    let started = Instant::now();
    if table_exists(conn, "fc_account_norm")? {
        conn.execute_batch(&format!(
            "UPDATE fc_account_norm
                SET cleaned_at={cleaned_at}
              WHERE id IN (SELECT id FROM acc_scope)",
            cleaned_at = cleaned_at_sql,
        ))?;
    }
    timings.mark_account_ms = elapsed_ms(started);

    let started = Instant::now();
    update_acct_state_on_conn(conn, args, &cleaned_at)?;
    timings.account_state_ms = elapsed_ms(started);
    timings.total_ms = elapsed_ms(total_started);
    Ok(timings)
}

fn update_acct_state_on_conn(conn: &Connection, args: &Args, cleaned_at: &str) -> Result<()> {
    if !table_exists(conn, "acct_state")? {
        return Ok(());
    }
    let case_id = sql_literal(&args.case_id);
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    let case_rows = scalar_i64(
        conn,
        &format!("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={case_id}"),
    )?;
    if scope_rows > 0 && scope_rows == case_rows {
        return update_acct_state_full_case_on_conn(conn, &case_id, cleaned_at);
    }

    let acct_key_expr = acct_key_expr();
    let txn_ts_expr = format!("COALESCE(t.txn_ts, {})", ts_norm_expr("t.txn_time"));
    let bal_val = "COALESCE(t.balance_val, TRY_CAST(t.clean_balance AS DOUBLE), TRY_CAST(t.balance AS DOUBLE))";
    let dc_final = "COALESCE(NULLIF(t.dc_final,''), NULLIF(t.clean_dc_flag,''), t.dc_norm)";
    let sql = format!(
        r#"
        WITH scoped AS (
            SELECT {acct_key_expr} AS acct_key
              FROM fc_transaction_norm
             WHERE id IN (SELECT id FROM txn_scope)
        ),
        affected AS (
            SELECT DISTINCT acct_key
              FROM scoped
             WHERE acct_key IS NOT NULL AND acct_key<>''
        ),
        candidate_keys AS (
            SELECT {acct_key_expr} AS acct_key,
                   id
              FROM fc_transaction_norm
             WHERE case_id={case_id}
        ),
        candidates AS (
            SELECT k.acct_key,
                   {txn_ts_expr} AS txn_ts,
                   t.txn_time,
                   t.row_no,
                   t.id,
                   {bal_val} AS bal_val,
                   {dc_final} AS dc_final
              FROM candidate_keys AS k
              JOIN affected AS a ON a.acct_key=k.acct_key
              JOIN fc_transaction_norm AS t ON t.id=k.id
        ),
        picked AS (
            SELECT acct_key,
                   arg_max(
                       struct_pack(
                           txn_ts := txn_ts,
                           bal_val := bal_val,
                           dc_final := dc_final
                       ),
                       struct_pack(
                           has_txn_ts := CASE WHEN txn_ts IS NULL THEN 0 ELSE 1 END,
                           txn_ts := txn_ts,
                           has_row_no := CASE WHEN row_no IS NULL THEN 0 ELSE 1 END,
                           row_no := row_no,
                           has_txn_time := CASE WHEN txn_time IS NULL THEN 0 ELSE 1 END,
                           txn_time := txn_time,
                           id := id
                       )
                   ) AS row_value
              FROM candidates AS c
             WHERE acct_key IS NOT NULL AND acct_key<>''
             GROUP BY acct_key
        )
        INSERT INTO acct_state(case_id, acct_key, last_txn_ts, last_balance, last_dc, updated_at)
        SELECT {case_id}, acct_key, row_value.txn_ts, row_value.bal_val, row_value.dc_final, {cleaned_at}
          FROM picked
        ON CONFLICT(case_id, acct_key) DO UPDATE SET
            last_txn_ts=excluded.last_txn_ts,
            last_balance=excluded.last_balance,
            last_dc=excluded.last_dc,
            updated_at=excluded.updated_at
        "#,
        acct_key_expr = acct_key_expr,
        bal_val = bal_val,
        case_id = case_id,
        cleaned_at = sql_literal(cleaned_at),
        dc_final = dc_final,
        txn_ts_expr = txn_ts_expr,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn update_acct_state_full_case_on_conn(
    conn: &Connection,
    case_id: &str,
    cleaned_at: &str,
) -> Result<()> {
    let acct_key_expr = acct_key_expr();
    let txn_ts_expr = format!("COALESCE(txn_ts, {})", ts_norm_expr("txn_time"));
    let bal_val =
        "COALESCE(balance_val, TRY_CAST(clean_balance AS DOUBLE), TRY_CAST(balance AS DOUBLE))";
    let dc_final = "COALESCE(NULLIF(dc_final,''), NULLIF(clean_dc_flag,''), dc_norm)";
    let sql = format!(
        r#"
        WITH candidates AS (
            SELECT {acct_key_expr} AS acct_key,
                   {txn_ts_expr} AS txn_ts,
                   txn_time,
                   row_no,
                   id,
                   {bal_val} AS bal_val,
                   {dc_final} AS dc_final
              FROM fc_transaction_norm
             WHERE case_id={case_id}
        ),
        picked AS (
            SELECT acct_key,
                   arg_max(
                       struct_pack(
                           txn_ts := txn_ts,
                           bal_val := bal_val,
                           dc_final := dc_final
                       ),
                       struct_pack(
                           has_txn_ts := CASE WHEN txn_ts IS NULL THEN 0 ELSE 1 END,
                           txn_ts := txn_ts,
                           has_row_no := CASE WHEN row_no IS NULL THEN 0 ELSE 1 END,
                           row_no := row_no,
                           has_txn_time := CASE WHEN txn_time IS NULL THEN 0 ELSE 1 END,
                           txn_time := txn_time,
                           id := id
                       )
                   ) AS row_value
              FROM candidates AS c
             WHERE acct_key IS NOT NULL AND acct_key<>''
             GROUP BY acct_key
        )
        INSERT INTO acct_state(case_id, acct_key, last_txn_ts, last_balance, last_dc, updated_at)
        SELECT {case_id}, acct_key, row_value.txn_ts, row_value.bal_val, row_value.dc_final, {cleaned_at}
          FROM picked
        ON CONFLICT(case_id, acct_key) DO UPDATE SET
            last_txn_ts=excluded.last_txn_ts,
            last_balance=excluded.last_balance,
            last_dc=excluded.last_dc,
            updated_at=excluded.updated_at
        "#,
        acct_key_expr = acct_key_expr,
        bal_val = bal_val,
        case_id = case_id,
        cleaned_at = sql_literal(cleaned_at),
        dc_final = dc_final,
        txn_ts_expr = txn_ts_expr,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn run_dc_flag(args: &Args) -> Result<DcFlagSummary> {
    let conn = open_txn_connection(args)?;
    ensure_required_table(&conn, "acct_state")?;
    run_dc_flag_on_conn(&conn, args)
}

fn run_dc_flag_on_conn(conn: &Connection, args: &Args) -> Result<DcFlagSummary> {
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    if scope_rows <= 0 {
        bail!("no transaction rows in cleaning scope");
    }

    let normalized = update_count(
        conn,
        "UPDATE fc_transaction_norm
            SET orig_dc_flag=COALESCE(NULLIF(orig_dc_flag,''), dc_flag),
                clean_dc_flag=COALESCE(NULLIF(clean_dc_flag,''), dc_norm),
                clean_dc_normalized=CASE
                  WHEN dc_norm IS NOT NULL THEN 1
                  ELSE clean_dc_normalized END
          WHERE id IN (SELECT id FROM txn_scope)
            AND COALESCE(TRIM(clean_dc_flag),'')=''
            AND dc_norm IS NOT NULL",
    )?;

    let acct_key_expr = acct_key_expr();
    let amt_val =
        "COALESCE(TRY_CAST(clean_amount AS DOUBLE), amount_val, TRY_CAST(amount AS DOUBLE))";
    let bal_val =
        "COALESCE(balance_val, TRY_CAST(clean_balance AS DOUBLE), TRY_CAST(balance AS DOUBLE))";
    let txn_ts = format!("COALESCE(txn_ts, {})", ts_norm_expr("txn_time"));
    let order_by = "CASE WHEN txn_ts IS NULL THEN 1 ELSE 0 END, txn_ts, row_no, txn_time, id";

    let inferred_sql = format!(
        r#"
        WITH scope AS (
            SELECT id,
                   {txn_ts} AS txn_ts,
                   txn_time,
                   {amt_val} AS amt_val,
                   {bal_val} AS bal_val,
                   {acct_key_expr} AS acct_key,
                   row_no
            FROM fc_transaction_norm
            WHERE id IN (SELECT id FROM txn_scope)
              AND COALESCE(TRIM(clean_dc_flag),'')=''
        ),
        affected AS (
            SELECT DISTINCT acct_key
            FROM scope
            WHERE acct_key IS NOT NULL AND acct_key<>''
        ),
        seed AS (
            SELECT acct_key,
                   last_txn_ts AS txn_ts,
                   last_balance AS bal_val,
                   NULL::DOUBLE AS amt_val,
                   -1 AS row_no,
                   NULL AS txn_time
            FROM acct_state
            WHERE case_id={case_id} AND acct_key IN (SELECT acct_key FROM affected)
        ),
        base AS (
            SELECT id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no
            FROM scope
            WHERE acct_key IS NOT NULL AND acct_key<>''
        ),
        merged AS (
            SELECT NULL AS id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no FROM seed
            UNION ALL
            SELECT id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no FROM base
        ),
        ordered AS (
            SELECT *,
                   LAG(bal_val) OVER (PARTITION BY acct_key ORDER BY {order_by}) AS prev_bal
            FROM merged
        ),
        calc AS (
            SELECT id,
                   CASE
                      WHEN prev_bal IS NULL THEN
                        CASE WHEN amt_val IS NOT NULL AND amt_val < 0 THEN '出' ELSE NULL END
                      WHEN bal_val IS NULL THEN NULL
                      ELSE
                        CASE
                          WHEN amt_val IS NOT NULL THEN
                            CASE
                              WHEN abs((bal_val - prev_bal) - amt_val)
                                   <= greatest(0.01, abs(amt_val) * 0.01) THEN '进'
                              WHEN abs((bal_val - prev_bal) + amt_val)
                                   <= greatest(0.01, abs(amt_val) * 0.01) THEN '出'
                              WHEN (bal_val - prev_bal) > 0 THEN '进'
                              WHEN (bal_val - prev_bal) < 0 THEN '出'
                              ELSE NULL
                            END
                          ELSE
                            CASE
                              WHEN (bal_val - prev_bal) > 0 THEN '进'
                              WHEN (bal_val - prev_bal) < 0 THEN '出'
                              ELSE NULL
                            END
                        END
                   END AS inferred_flag
            FROM ordered
            WHERE id IS NOT NULL
        )
        UPDATE fc_transaction_norm AS t
           SET clean_dc_flag=calc.inferred_flag,
               clean_dc_inferred=1,
               orig_dc_flag=COALESCE(NULLIF(t.orig_dc_flag,''), t.dc_flag),
               dc_final=COALESCE(calc.inferred_flag, t.dc_final)
          FROM calc
         WHERE t.id=calc.id AND calc.inferred_flag IS NOT NULL
        "#,
        acct_key_expr = acct_key_expr,
        amt_val = amt_val,
        bal_val = bal_val,
        case_id = sql_literal(&args.case_id),
        order_by = order_by,
        txn_ts = txn_ts,
    );
    let inferred = update_count(conn, &inferred_sql)?;

    conn.execute_batch(
        "UPDATE fc_transaction_norm
            SET dc_final=COALESCE(NULLIF(clean_dc_flag,''), dc_norm)
          WHERE id IN (SELECT id FROM txn_scope)
            AND dc_final IS DISTINCT FROM COALESCE(NULLIF(clean_dc_flag,''), dc_norm)",
    )?;

    let normalized_total = scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={} AND clean_dc_normalized=1",
            sql_literal(&args.case_id)
        ),
    )?;
    let inferred_total = scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={} AND clean_dc_inferred=1",
            sql_literal(&args.case_id)
        ),
    )?;

    Ok(DcFlagSummary {
        normalized,
        inferred,
        normalized_total,
        inferred_total,
    })
}

fn run_quality_flags(args: &Args) -> Result<QualityFlagSummary> {
    let conn = open_txn_connection(args)?;
    run_quality_flags_on_conn(&conn, args)
}

fn run_quality_flags_on_conn(conn: &Connection, args: &Args) -> Result<QualityFlagSummary> {
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    if scope_rows <= 0 {
        bail!("no transaction rows in cleaning scope");
    }

    conn.execute_batch("DROP TABLE IF EXISTS quality_flag_candidates")?;
    conn.execute_batch(
        r#"
        CREATE TEMP TABLE quality_flag_candidates AS
        WITH base AS (
            SELECT id,
                   COALESCE(clean_invalid,0) AS clean_invalid,
                   COALESCE(clean_duplicate,0) AS clean_duplicate,
                   COALESCE(clean_failed,0) AS clean_failed,
                   COALESCE(clean_reversal,0) AS clean_reversal,
                   (
                     COALESCE(TRIM(txn_time),'')=''
                     OR COALESCE(TRIM(amount),'')=''
                     OR (
                       COALESCE(TRIM(COALESCE(NULLIF(clean_card_no,''), card_no)),'')=''
                       AND COALESCE(TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no)),'')=''
                     )
                   ) AS invalid_candidate,
                   COALESCE(clean_duplicate,0)=1 AS duplicate_candidate,
                   (
                     COALESCE(query_feedback_reason,'') LIKE '%失败%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%无%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%不%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%没有%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%未%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%查询无明细%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%找不到客户%'
                     OR COALESCE(query_feedback_reason,'') LIKE '%核心查无记录%'
                   ) AS failed_candidate,
                   COALESCE(query_feedback_reason,'') LIKE '%冲正%' AS reversal_candidate
              FROM fc_transaction_norm
             WHERE id IN (SELECT id FROM txn_scope)
        )
        SELECT id,
               invalid_candidate,
               duplicate_candidate,
               failed_candidate,
               reversal_candidate,
               invalid_candidate AND clean_invalid<>1 AS invalid_changed,
               duplicate_candidate AND clean_duplicate<>0 AS duplicate_changed,
               failed_candidate AND clean_failed<>1 AS failed_changed,
               reversal_candidate AND clean_reversal<>1 AS reversal_changed
          FROM base
         WHERE invalid_candidate
            OR duplicate_candidate
            OR failed_candidate
            OR reversal_candidate
        "#,
    )?;
    let (invalid_changed, duplicate_changed, failed_changed, reversal_changed) =
        amount_balance_count_row(
            conn,
            r#"
        SELECT
            SUM(CASE WHEN invalid_changed THEN 1 ELSE 0 END),
            SUM(CASE WHEN duplicate_changed THEN 1 ELSE 0 END),
            SUM(CASE WHEN failed_changed THEN 1 ELSE 0 END),
            SUM(CASE WHEN reversal_changed THEN 1 ELSE 0 END)
        FROM quality_flag_candidates
        "#,
        )?;

    conn.execute_batch(
        r#"
        UPDATE fc_transaction_norm AS t
           SET clean_invalid=CASE
                 WHEN q.invalid_candidate THEN 1 ELSE t.clean_invalid END,
               clean_duplicate=CASE
                 WHEN q.duplicate_candidate THEN 0 ELSE t.clean_duplicate END,
               clean_failed=CASE
                 WHEN q.failed_candidate THEN 1 ELSE t.clean_failed END,
               clean_reversal=CASE
                 WHEN q.reversal_candidate THEN 1 ELSE t.clean_reversal END
          FROM quality_flag_candidates AS q
         WHERE t.id=q.id
           AND (
             q.invalid_changed
             OR q.duplicate_changed
             OR q.failed_changed
             OR q.reversal_changed
           )
        "#,
    )?;

    let (invalid_total, duplicate_total, failed_total, reversal_total) = amount_balance_count_row(
        conn,
        &format!(
            r#"
                SELECT
                    SUM(CASE WHEN clean_invalid=1 THEN 1 ELSE 0 END),
                    SUM(CASE WHEN clean_duplicate=1 THEN 1 ELSE 0 END),
                    SUM(CASE WHEN clean_failed=1 THEN 1 ELSE 0 END),
                    SUM(CASE WHEN clean_reversal=1 THEN 1 ELSE 0 END)
                FROM fc_transaction_norm
                WHERE case_id={}
                "#,
            sql_literal(&args.case_id)
        ),
    )?;

    Ok(QualityFlagSummary {
        invalid_changed,
        invalid_total,
        duplicate_changed,
        duplicate_total,
        failed_changed,
        reversal_changed,
        failed_total,
        reversal_total,
    })
}

fn acct_key_expr() -> &'static str {
    "COALESCE(NULLIF(TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no_norm, acct_no)), ''), NULLIF(TRIM(COALESCE(NULLIF(clean_card_no,''), card_no_norm, card_no)), ''))"
}

fn ts_norm_expr(expr: &str) -> String {
    format!(
        "COALESCE(try_strptime({expr}, '%Y-%m-%d %H:%M:%S'),try_strptime({expr}, '%Y-%m-%d %H:%M'),try_strptime({expr}, '%Y-%m-%d'),try_strptime({expr}, '%Y%m%d%H%M%S'),try_strptime({expr}, '%Y%m%d%H%M'),try_strptime({expr}, '%Y%m%d'))"
    )
}

pub(crate) fn open_txn_connection(args: &Args) -> Result<Connection> {
    Ok(open_txn_connection_timed(args)?.conn)
}

fn open_txn_connection_timed(args: &Args) -> Result<TimedTxnConnection> {
    let open_started = Instant::now();
    let conn = Connection::open(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    let open_db_ms = elapsed_ms(open_started);
    let configure_started = Instant::now();
    configure_connection(&conn)?;
    let configure_ms = elapsed_ms(configure_started);
    let ensure_started = Instant::now();
    ensure_required_table(&conn, "fc_transaction_norm")?;
    let ensure_txn_table_ms = elapsed_ms(ensure_started);
    let scope_started = Instant::now();
    create_txn_scope(&conn, &args.case_id, &args.txn_file_ids)?;
    let txn_scope_ms = elapsed_ms(scope_started);
    Ok(TimedTxnConnection {
        conn,
        open_db_ms,
        configure_ms,
        ensure_txn_table_ms,
        txn_scope_ms,
    })
}

fn configure_connection(conn: &Connection) -> Result<()> {
    let threads = std::thread::available_parallelism()
        .map(|count| count.get().clamp(1, 16))
        .unwrap_or(4);
    conn.execute_batch(&format!(
        "
        PRAGMA threads={threads};
        SET preserve_insertion_order=false;
        ",
    ))?;
    Ok(())
}

fn ensure_required_table(conn: &Connection, table: &str) -> Result<()> {
    let exists = table_exists(conn, table)?;
    if !exists {
        bail!("{table} not found");
    }
    Ok(())
}

pub(crate) fn table_exists(conn: &Connection, table: &str) -> Result<bool> {
    let count = scalar_i64(
        conn,
        &format!(
            "SELECT 1
               FROM information_schema.tables
              WHERE table_schema='main' AND table_name={}
              LIMIT 1",
            sql_literal(table)
        ),
    )?;
    Ok(count > 0)
}

fn create_txn_scope(conn: &Connection, case_id: &str, txn_file_ids: &[String]) -> Result<()> {
    conn.execute_batch("DROP TABLE IF EXISTS txn_scope")?;
    let case_sql = sql_literal(case_id);
    let where_sql = if txn_file_ids.is_empty() {
        format!("case_id={case_sql}")
    } else {
        let ids = txn_file_ids
            .iter()
            .map(|value| sql_literal(value))
            .collect::<Vec<_>>()
            .join(",");
        format!("case_id={case_sql} AND file_id IN ({ids})")
    };
    conn.execute_batch(&format!(
        "CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm WHERE {where_sql}"
    ))?;
    Ok(())
}

pub(crate) fn create_acc_scope(
    conn: &Connection,
    case_id: &str,
    acc_file_ids: &[String],
) -> Result<()> {
    conn.execute_batch("DROP TABLE IF EXISTS acc_scope")?;
    if !table_exists(conn, "fc_account_norm")? {
        conn.execute_batch("CREATE TEMP TABLE acc_scope(id BIGINT)")?;
        return Ok(());
    }
    let case_sql = sql_literal(case_id);
    let where_sql = if acc_file_ids.is_empty() {
        format!("case_id={case_sql} AND 1=0")
    } else {
        let ids = acc_file_ids
            .iter()
            .map(|value| sql_literal(value))
            .collect::<Vec<_>>()
            .join(",");
        format!("case_id={case_sql} AND file_id IN ({ids})")
    };
    conn.execute_batch(&format!(
        "CREATE TEMP TABLE acc_scope AS SELECT id FROM fc_account_norm WHERE {where_sql}"
    ))?;
    Ok(())
}

fn suffix_base_expr(src: &str) -> String {
    format!(
        "CASE WHEN instr({src}, '-') > 0 OR instr({src}, '_') > 0 THEN \
           CASE WHEN (instr({src}, '_') > 0 AND (instr({src}, '-') = 0 OR instr({src}, '_') < instr({src}, '-'))) THEN \
             CASE WHEN instr({src}, '_') > 1 THEN substr({src}, 1, instr({src}, '_') - 1) ELSE NULL END \
           ELSE \
             CASE WHEN instr({src}, '-') > 1 THEN substr({src}, 1, instr({src}, '-') - 1) ELSE NULL END \
           END \
         ELSE NULL END"
    )
}

fn suffix_update_sql(table: &str, scope_table: &str, src_card: &str, src_acct: &str) -> String {
    let card_base = suffix_base_expr(src_card);
    let acct_base = suffix_base_expr(src_acct);
    format!(
        r#"
        WITH suffix_source AS (
            SELECT id,
                   clean_card_no,
                   clean_acct_no,
                   clean_suffix_fixed,
                   {card_base} AS card_base,
                   {acct_base} AS acct_base
              FROM {table}
             WHERE id IN (SELECT id FROM {scope_table})
               AND (
                 instr({src_card}, '-') > 1 OR instr({src_card}, '_') > 1
                 OR instr({src_acct}, '-') > 1 OR instr({src_acct}, '_') > 1
               )
        ),
        suffix_candidates AS (
            SELECT id, card_base, acct_base
              FROM suffix_source
             WHERE (
                 (card_base IS NOT NULL
                   AND (clean_card_no IS DISTINCT FROM card_base OR clean_suffix_fixed IS DISTINCT FROM 1))
                 OR (acct_base IS NOT NULL
                   AND (clean_acct_no IS DISTINCT FROM acct_base OR clean_suffix_fixed IS DISTINCT FROM 1))
               )
        )
        UPDATE {table} AS target
           SET clean_card_no=COALESCE(candidate.card_base, target.clean_card_no),
               clean_acct_no=COALESCE(candidate.acct_base, target.clean_acct_no),
               clean_suffix_fixed=1
          FROM suffix_candidates AS candidate
         WHERE target.id=candidate.id
        "#,
        acct_base = acct_base,
        card_base = card_base,
        scope_table = scope_table,
        src_acct = src_acct,
        src_card = src_card,
        table = table,
    )
}

pub(crate) fn valid_expr(expr: &str) -> String {
    format!("COALESCE(TRIM({expr}),'') NOT IN ('','-','—','_')")
}

fn amount_balance_count_row(conn: &Connection, sql: &str) -> Result<(i64, i64, i64, i64)> {
    let mut stmt = conn.prepare(sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let first: Option<i64> = row.get(0)?;
        let second: Option<i64> = row.get(1)?;
        let third: Option<i64> = row.get(2)?;
        let fourth: Option<i64> = row.get(3)?;
        Ok((
            first.unwrap_or(0),
            second.unwrap_or(0),
            third.unwrap_or(0),
            fourth.unwrap_or(0),
        ))
    } else {
        Ok((0, 0, 0, 0))
    }
}

pub(crate) fn scalar_i64(conn: &Connection, sql: &str) -> Result<i64> {
    let mut stmt = conn.prepare(sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let value: Option<i64> = row.get(0)?;
        Ok(value.unwrap_or(0))
    } else {
        Ok(0)
    }
}

fn scalar_string(conn: &Connection, sql: &str) -> Result<String> {
    let mut stmt = conn.prepare(sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let value: Option<String> = row.get(0)?;
        Ok(value.unwrap_or_default())
    } else {
        Ok(String::new())
    }
}

pub(crate) fn sql_literal(value: &str) -> String {
    format!("'{}'", value.replace('\'', "''"))
}

pub(crate) fn elapsed_ms(started: Instant) -> i64 {
    i64::try_from(started.elapsed().as_millis()).unwrap_or(i64::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn account_state_args() -> Args {
        Args {
            command: "clean-all".to_string(),
            case_id: "case-1".to_string(),
            db_path: PathBuf::new(),
            txn_file_ids: Vec::new(),
            acc_file_ids: Vec::new(),
        }
    }

    fn account_state_connection(scope_sql: &str) -> Result<Connection> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                file_id TEXT,
                txn_ts TIMESTAMP,
                txn_time TEXT,
                row_no BIGINT,
                balance_val DOUBLE,
                clean_balance TEXT,
                balance TEXT,
                dc_final TEXT,
                clean_dc_flag TEXT,
                dc_norm TEXT,
                clean_card_no TEXT,
                card_no_norm TEXT,
                card_no TEXT,
                clean_acct_no TEXT,
                acct_no_norm TEXT,
                acct_no TEXT
            );
            CREATE TABLE acct_state(
                case_id TEXT,
                acct_key TEXT,
                last_txn_ts TIMESTAMP,
                last_balance DOUBLE,
                last_dc TEXT,
                updated_at TEXT,
                PRIMARY KEY(case_id, acct_key)
            );
            INSERT INTO fc_transaction_norm VALUES
                (1, 'case-1', 'file-a', TIMESTAMP '2026-01-01 09:00:00', '2026-01-01 09:00:00', 1, 10.0, NULL, '10', '进', NULL, NULL, 'card-a', NULL, NULL, 'acct-a', NULL, NULL),
                (2, 'case-1', 'file-b', NULL, '2026-01-02 09:00:00', 2, 20.0, NULL, '20', '出', NULL, NULL, 'card-a', NULL, NULL, 'acct-a', NULL, NULL),
                (3, 'case-1', 'file-c', TIMESTAMP '2026-01-03 09:00:00', '2026-01-03 09:00:00', 3, 30.0, NULL, '30', '进', NULL, NULL, 'card-b', NULL, NULL, 'acct-b', NULL, NULL),
                (4, 'case-2', 'file-d', TIMESTAMP '2026-01-04 09:00:00', '2026-01-04 09:00:00', 4, 40.0, NULL, '40', '出', NULL, NULL, 'card-x', NULL, NULL, 'acct-x', NULL, NULL);
            "#,
        )?;
        conn.execute_batch(scope_sql)?;
        Ok(conn)
    }

    #[test]
    fn account_keys_suffix_fix_uses_single_candidate_projection() -> Result<()> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                card_no TEXT,
                orig_card_no TEXT,
                clean_card_no TEXT,
                acct_no TEXT,
                clean_acct_no TEXT,
                clean_card_filled BIGINT,
                clean_suffix_fixed BIGINT
            );
            INSERT INTO fc_transaction_norm VALUES
                (1, 'case-keys', '6222-001', NULL, '', '', '', 0, 0),
                (2, 'case-keys', 'cardok', NULL, '', 'acctok', '', 0, 0);
            CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm;
            "#,
        )?;
        let args = Args {
            command: "account-keys".to_string(),
            case_id: "case-keys".to_string(),
            db_path: PathBuf::new(),
            txn_file_ids: Vec::new(),
            acc_file_ids: Vec::new(),
        };

        let summary = run_account_keys_on_conn(&conn, &args)?;
        let fixed_row = conn
            .prepare(
                "SELECT clean_card_no, clean_acct_no, clean_card_filled, clean_suffix_fixed
                   FROM fc_transaction_norm
                  WHERE id=1",
            )?
            .query_row([], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, i64>(2)?,
                    row.get::<_, i64>(3)?,
                ))
            })?;

        assert_eq!(summary.filled_count, 1);
        assert_eq!(summary.suffix_txn_count, 1);
        assert_eq!(fixed_row, ("6222".to_string(), "6222".to_string(), 1, 1));
        Ok(())
    }

    #[test]
    fn quality_flags_filter_keeps_all_candidate_semantics() -> Result<()> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                clean_invalid BIGINT,
                clean_duplicate BIGINT,
                clean_failed BIGINT,
                clean_reversal BIGINT,
                txn_time TEXT,
                amount TEXT,
                clean_card_no TEXT,
                card_no TEXT,
                clean_acct_no TEXT,
                acct_no TEXT,
                query_feedback_reason TEXT
            );
            INSERT INTO fc_transaction_norm VALUES
                (1, 'case-quality', 0, 0, 0, 0, '', '', '', '', '', '', ''),
                (2, 'case-quality', 0, 1, 0, 0, '2026-01-01', '10', 'card', '', '', '', ''),
                (3, 'case-quality', 0, 0, 0, 0, '2026-01-01', '10', 'card', '', '', '', '查询失败'),
                (4, 'case-quality', 0, 0, 0, 0, '2026-01-01', '10', 'card', '', '', '', '冲正交易'),
                (5, 'case-quality', 0, 0, 0, 0, '2026-01-01', '10', 'card', '', '', '', '');
            CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm;
            "#,
        )?;
        let args = Args {
            command: "quality-flags".to_string(),
            case_id: "case-quality".to_string(),
            db_path: PathBuf::new(),
            txn_file_ids: Vec::new(),
            acc_file_ids: Vec::new(),
        };

        let summary = run_quality_flags_on_conn(&conn, &args)?;

        assert_eq!(summary.invalid_changed, 1);
        assert_eq!(summary.invalid_total, 1);
        assert_eq!(summary.duplicate_changed, 1);
        assert_eq!(summary.duplicate_total, 0);
        assert_eq!(summary.failed_changed, 1);
        assert_eq!(summary.failed_total, 1);
        assert_eq!(summary.reversal_changed, 1);
        assert_eq!(summary.reversal_total, 1);
        Ok(())
    }

    #[test]
    fn dc_flag_final_sync_preserves_direction_semantics() -> Result<()> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                orig_dc_flag TEXT,
                dc_flag TEXT,
                clean_dc_flag TEXT,
                clean_dc_normalized BIGINT,
                clean_dc_inferred BIGINT,
                dc_norm TEXT,
                dc_final TEXT,
                txn_ts TIMESTAMP,
                txn_time TEXT,
                amount_val DOUBLE,
                clean_amount TEXT,
                amount TEXT,
                balance_val DOUBLE,
                clean_balance TEXT,
                balance TEXT,
                clean_card_no TEXT,
                card_no_norm TEXT,
                card_no TEXT,
                clean_acct_no TEXT,
                acct_no_norm TEXT,
                acct_no TEXT,
                row_no BIGINT
            );
            CREATE TABLE acct_state(
                case_id TEXT,
                acct_key TEXT,
                last_txn_ts TIMESTAMP,
                last_balance DOUBLE,
                last_dc TEXT,
                updated_at TEXT
            );
            INSERT INTO fc_transaction_norm VALUES
                (1, 'case-dc', NULL, '进', '', 0, 0, '进', NULL, NULL, '', NULL, '', '', NULL, '', '', '', '', '', '', '', '', 1),
                (2, 'case-dc', NULL, '出', '出', 0, 0, '进', '出', NULL, '', NULL, '', '', NULL, '', '', '', '', '', '', '', '', 2);
            CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm;
            "#,
        )?;
        let args = Args {
            command: "dc-flag".to_string(),
            case_id: "case-dc".to_string(),
            db_path: PathBuf::new(),
            txn_file_ids: Vec::new(),
            acc_file_ids: Vec::new(),
        };

        let summary = run_dc_flag_on_conn(&conn, &args)?;
        let rows = conn
            .prepare("SELECT id, clean_dc_flag, dc_final FROM fc_transaction_norm ORDER BY id")?
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()?;

        assert_eq!(summary.normalized, 1);
        assert_eq!(summary.inferred, 0);
        assert_eq!(
            rows,
            vec![
                (1, "进".to_string(), "进".to_string()),
                (2, "出".to_string(), "出".to_string()),
            ]
        );
        Ok(())
    }

    #[test]
    fn full_scope_account_state_projects_latest_state_for_all_case_accounts() -> Result<()> {
        let conn = account_state_connection(
            "CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm WHERE case_id='case-1'",
        )?;
        update_acct_state_on_conn(&conn, &account_state_args(), "2026-05-02 10:00:00")?;

        let rows = conn
            .prepare(
                "SELECT acct_key, last_balance, last_dc FROM acct_state WHERE case_id='case-1' ORDER BY acct_key",
            )?
            .query_map([], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, f64>(1)?,
                    row.get::<_, String>(2)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()?;

        assert_eq!(
            rows,
            vec![
                ("acct-a".to_string(), 20.0, "出".to_string()),
                ("acct-b".to_string(), 30.0, "进".to_string()),
            ]
        );
        Ok(())
    }

    #[test]
    fn partial_scope_account_state_only_projects_affected_accounts() -> Result<()> {
        let conn = account_state_connection("CREATE TEMP TABLE txn_scope AS SELECT 1 AS id")?;
        update_acct_state_on_conn(&conn, &account_state_args(), "2026-05-02 10:00:00")?;

        let rows = conn
            .prepare(
                "SELECT acct_key, last_balance, last_dc FROM acct_state WHERE case_id='case-1' ORDER BY acct_key",
            )?
            .query_map([], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, f64>(1)?,
                    row.get::<_, String>(2)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()?;

        assert_eq!(rows, vec![("acct-a".to_string(), 20.0, "出".to_string())]);
        Ok(())
    }
}
