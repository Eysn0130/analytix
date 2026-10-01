// Included only by the canonical importer test module. The supplied JSON stays
// immutable; derived CSV is a separately named, narrower CNY test input.
fn delivery_vectors() -> Vec<serde_json::Value> {
    include_str!("../tests/fixtures/core-funds-20260921/funds-facts.jsonl")
        .lines()
        .map(|line| serde_json::from_str(line).unwrap())
        .collect()
}

fn delivery_csv(rows: &[serde_json::Value], canonical: bool) -> Vec<u8> {
    let mut writer = csv::Writer::from_writer(Vec::new());
    writer
        .write_record(COLUMNS.iter().map(|column| column.header))
        .unwrap();
    for record in rows {
        let value = |key: &str| record[key].as_str().unwrap_or("");
        let mut row = vec![String::new(); COLUMNS.len()];
        row[ACCOUNT_INDEX] = if canonical {
            // The vector has two logical subjects sharing one raw account.
            // Give the derived positive control distinct valid account identities.
            format!("{}{}", value("raw_account"), value("subject_key")).replace('_', "")
        } else {
            value("raw_account").to_string()
        };
        row[2] = value("raw_subject_name").to_string();
        row[TXN_TIME_INDEX] = value("recorded_at")
            .replace('T', " ")
            .trim_end_matches('Z')
            .to_string();
        row[AMOUNT_INDEX] = value("amount").to_string();
        if canonical && row[AMOUNT_INDEX].contains('.') {
            row[AMOUNT_INDEX] = row[AMOUNT_INDEX]
                .trim_end_matches('0')
                .trim_end_matches('.')
                .to_string();
        }
        row[DIRECTION_INDEX] = if value("direction") == "in" {
            "进"
        } else {
            "出"
        }
        .to_string();
        row[CURRENCY_INDEX] = value("currency").to_string();
        row[8] = value("counterparty_key").to_string();
        row[10] = value("raw_counterparty_name").to_string();
        row[24] = value("source_record_id").to_string();
        row[31] = value("raw_memo").to_string();
        writer.write_record(row).unwrap();
    }
    writer.into_inner().unwrap()
}

#[cfg(unix)]
#[test]
fn delivery_vectors_reject_unsupported_inputs_before_database_output() {
    let all = delivery_vectors();
    for (label, records, canonical) in [
        ("original-vector", all.clone(), false),
        (
            "invalid-amount",
            all.iter()
                .filter(|r| r["row_id"] == "A011")
                .cloned()
                .collect(),
            true,
        ),
        (
            "unsupported-usd",
            all.iter()
                .filter(|r| r["row_id"] == "A008")
                .cloned()
                .collect(),
            true,
        ),
    ] {
        let (directory, _, mut output) = temp_unlinked_output(label);
        let source = delivery_csv(&records, canonical);
        let before = Sha256::digest(&source);
        let result = run_funds_build_canonical_csv_snapshot_v1_in(
            arguments(),
            &source,
            &mut output,
            &directory,
        );
        assert!(
            result.is_err(),
            "{label} must not be silently normalized or partly imported"
        );
        assert_eq!(output.metadata().unwrap().len(), 0);
        assert_eq!(Sha256::digest(&source), before);
        drop(output);
        fs::remove_dir(directory).unwrap();
    }
}

#[cfg(unix)]
#[test]
fn delivery_vectors_cny_derived_import_query_reopen_and_source_exact() {
    use crate::stats_query_store::account_flow::{
        analyze_account_flows, AnalyzeAccountFlowsHostArguments,
    };
    let all = delivery_vectors();
    let expected: serde_json::Value = serde_json::from_str(include_str!(
        "../tests/fixtures/core-funds-20260921/expected-queries.json"
    ))
    .unwrap();
    let mut producer_ids = Vec::new();
    for (query_index, case, snapshot, expected_rows) in [
        (0, "SYNTH_CASE_A", "SYNTH_SNAPSHOT_A1", 9),
        (1, "SYNTH_CASE_B", "SYNTH_SNAPSHOT_B1", 1),
        (2, "SYNTH_CASE_A", "SYNTH_SNAPSHOT_A2", 1),
    ] {
        // This selection is an explicitly derived positive control, not a claim
        // that the product accepts USD/invalid money or deduplicates observations.
        let rows: Vec<_> = all
            .iter()
            .filter(|r| {
                r["case_id"] == case
                    && r["snapshot_id"] == snapshot
                    && r["currency"] == "CNY"
                    && r["row_id"] != "A011"
                    && r["row_id"] != "A001_DUP"
            })
            .cloned()
            .collect();
        let source = delivery_csv(&rows, true);
        let source_hash = format!("{:x}", Sha256::digest(&source));
        let (directory, _, mut output) = temp_unlinked_output(snapshot);
        let mut build = arguments();
        build.case_id = case.to_string();
        build.source_revision = query_index as u64 + 1;
        let built =
            run_funds_build_canonical_csv_snapshot_v1_in(build, &source, &mut output, &directory)
                .expect("actual canonical import and DuckDB materialization");
        assert_eq!(built.source_artifact_sha256, source_hash);
        assert_eq!(built.source_row_count, expected_rows);
        producer_ids.push(built.materialization.producer_content_id.clone());
        let mut before = Vec::new();
        output.rewind().unwrap();
        output.read_to_end(&mut before).unwrap();
        let snapshot_hash = Sha256::digest(&before);
        let args = AnalyzeAccountFlowsHostArguments {
            case_id: case.to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 1,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: built.materialization.producer_content_id.clone(),
            expected_producer_manifest_sha256: built
                .materialization
                .producer_content_manifest_sha256
                .clone(),
            subject_ref: format!("cer1_{}", "a".repeat(64)),
            resolved_account_key: format!(
                "{}SYNTH_SUBJECT_1",
                rows[0]["raw_account"].as_str().unwrap()
            )
            .replace('_', ""),
            subject_resolution_digest: "5".repeat(64),
            start_inclusive: "2026-09-01T00:00:00Z".to_string(),
            // The product takes inclusive endpoints and the source has second
            // precision. This is the exact equivalent of the vector's [Sep,Oct).
            end_inclusive: "2026-09-30T23:59:59Z".to_string(),
            evidence_row_limit: 100,
            dataset_utc_offset_minutes: 0,
            expected_currency: "CNY".to_string(),
            minor_unit_scale: 2,
            scan_cap: 100,
        };
        let mut prior = None;
        for _ in 0..2 {
            let conn =
                crate::open_account_flow_readonly_connection(&output_descriptor_path(&output))
                    .unwrap();
            let result = analyze_account_flows(&conn, &args)
                .expect("actual immutable DuckDB account-flow query");
            let group = &expected["queries"][query_index]["expected"]["groups"][0];
            let minor = |field: &str| group[field].as_str().unwrap().replace('.', "");
            assert_eq!(
                result.inflow_minor.parse::<i128>().unwrap(),
                minor("inflow").parse::<i128>().unwrap()
            );
            assert_eq!(
                result.outflow_minor.parse::<i128>().unwrap(),
                minor("outflow").parse::<i128>().unwrap()
            );
            assert_eq!(
                result.net_minor.parse::<i128>().unwrap(),
                minor("net").parse::<i128>().unwrap()
            );
            assert_eq!(result.transaction_count, group["count"].as_u64().unwrap());
            assert_eq!(result.currency, "CNY");
            assert!(result.aggregate_complete && result.evidence_rows_complete);
            if let Some(previous) = &prior {
                assert_eq!(&result, previous);
            }
            prior = Some(result);
            let name: String = conn
                .query_row(
                    "SELECT account_open_name_raw FROM fc_transaction_raw ORDER BY id LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .unwrap();
            assert_eq!(name, rows[0]["raw_subject_name"].as_str().unwrap());
            let mut wrong_case = args.clone();
            wrong_case.case_id = "SYNTH_CASE_OTHER".to_string();
            assert!(analyze_account_flows(&conn, &wrong_case).is_err());
        }
        let mut after = Vec::new();
        output.rewind().unwrap();
        output.read_to_end(&mut after).unwrap();
        assert_eq!(Sha256::digest(&after), snapshot_hash);
        assert_eq!(format!("{:x}", Sha256::digest(&source)), source_hash);
        eprintln!(
            "derived CNY {snapshot}: source_sha256={source_hash} actual_rows={expected_rows}"
        );
        drop(output);
        fs::remove_dir(directory).unwrap();
    }
    assert_ne!(producer_ids[0], producer_ids[1]);
    assert_ne!(producer_ids[0], producer_ids[2]);
}
