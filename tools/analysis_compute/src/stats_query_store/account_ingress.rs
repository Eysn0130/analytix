use std::collections::BTreeSet;
use std::fmt;

use anyhow::{anyhow, bail, Result};
use duckdb::types::ValueRef;
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

use super::session::{VerifiedStatsQueryRequest, VerifiedStatsQuerySession};

pub(crate) const RESOLVE_ACCOUNT_INGRESS_COMMAND: &str = "resolve_account_ingress";

pub(crate) const ACCOUNT_INGRESS_QUERY_CONTRACT: &str =
    "analytix.account-ingress-resolution-query/v1";
pub(crate) const ACCOUNT_INGRESS_RESULT_CONTRACT: &str =
    "analytix.account-ingress-resolution-result/v1";

const ACCOUNT_INGRESS_QUERY_SQL_HASH_DOMAIN: &[u8] =
    b"analytix.account-ingress-resolution-query-sql/v1";
#[cfg(test)]
const ACCOUNT_INGRESS_QUERY_SQL_HASH_V1: &str =
    "5fef7c06fe02fd67de5699e1a826f02103fe7c872f586b1f902ff967cf33f18d";
const ACCOUNT_INGRESS_RANGE_HASH_DOMAIN: &[u8] = b"analytix.account-ingress-resolution-range/v1";
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_CANDIDATES: usize = 64;
const MIN_CANDIDATE_DIGITS: usize = 8;
const MAX_CANDIDATE_DIGITS: usize = 32;
const MAX_SAFE_SEMANTIC_BYTES: usize = 256;
const MAX_TOTAL_SAFE_SEMANTIC_BYTES: usize = 64 * 1024;

// The complete candidate is supplied only as a bound parameter. Resolution
// requires a source-proven acct_no witness; an account_key produced only by
// card fallback is insufficient, and a cross-account card collision is an
// integrity failure. No SQL text, table name, path, projection, ordering, or
// limit is caller-selectable.
const ACCOUNT_INGRESS_QUERY_SQL: &str = "\
SELECT a.bank_name,
       a.acct_type,
       EXISTS (
         SELECT 1
           FROM analysis_txn_detail_idx c
          WHERE c.card_no = ?
            AND c.acct_key <> a.account_key
       ) AS cross_account_card_conflict
  FROM analysis_account_dim a
 WHERE a.account_key = ?
   AND EXISTS (
         SELECT 1
           FROM analysis_txn_detail_idx w
          WHERE w.acct_no = ?
            AND w.acct_key = a.account_key
       )
 ORDER BY a.account_key ASC
 LIMIT 2
";

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct ResolveAccountIngressCandidate {
    pub ordinal: u32,
    pub normalized_candidate: String,
}

impl fmt::Debug for ResolveAccountIngressCandidate {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("ResolveAccountIngressCandidate")
            .field("ordinal", &self.ordinal)
            .field("normalized_candidate", &"[REDACTED]")
            .finish()
    }
}

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct ResolveAccountIngressHostArguments {
    pub case_id: String,
    pub dataset_snapshot_id: String,
    pub context_epoch: u64,
    pub context_digest: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub candidates: Vec<ResolveAccountIngressCandidate>,
}

impl fmt::Debug for ResolveAccountIngressHostArguments {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("ResolveAccountIngressHostArguments")
            .field("case_id", &self.case_id)
            .field("dataset_snapshot_id", &self.dataset_snapshot_id)
            .field("context_epoch", &self.context_epoch)
            .field("context_digest", &self.context_digest)
            .field("case_binding_hash", &self.case_binding_hash)
            .field(
                "expected_producer_content_id",
                &self.expected_producer_content_id,
            )
            .field(
                "expected_producer_manifest_sha256",
                &self.expected_producer_manifest_sha256,
            )
            .field("candidate_count", &self.candidates.len())
            .finish()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountIngressResolutionDisposition {
    Resolved,
    NotFound,
    Ambiguous,
    IntegrityFailure,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AccountIngressResolution {
    pub ordinal: u32,
    pub disposition: AccountIngressResolutionDisposition,
    pub entity_type: String,
    pub bank_institution: String,
    pub account_type: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AccountIngressResolutionProvenance {
    pub dataset_snapshot_id: String,
    pub context_epoch: u64,
    pub context_digest: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub duckdb_content_snapshot_digest: String,
    pub duckdb_snapshot_manifest_sha256: String,
    pub materialization_identity: String,
    pub source_signature: String,
    pub result_signature: String,
    pub producer_content_id: String,
    pub producer_manifest_sha256: String,
    pub query_contract: String,
    pub query_sql_hash: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct ResolveAccountIngressHostResult {
    pub schema_version: u32,
    pub contract: String,
    pub resolutions: Vec<AccountIngressResolution>,
    pub provenance: AccountIngressResolutionProvenance,
}

pub(crate) fn parse_resolve_account_ingress_host_arguments(
    value: &Value,
) -> Result<ResolveAccountIngressHostArguments> {
    serde_json::from_value(value.clone())
        .map_err(|_| anyhow!("account_ingress_request_contract_invalid"))
}

pub(crate) fn validate_resolve_account_ingress_host_arguments(
    arguments: &ResolveAccountIngressHostArguments,
) -> Result<()> {
    if arguments.case_id.is_empty()
        || arguments.case_id != arguments.case_id.trim()
        || arguments.case_id.len() > MAX_CASE_ID_BYTES
        || arguments.case_id.chars().any(char::is_control)
        || !valid_prefixed_sha256(&arguments.dataset_snapshot_id, "dsv2_")
        || arguments.context_epoch == 0
        || !crate::is_sha256_hex(&arguments.context_digest)
        || !crate::is_sha256_hex(&arguments.case_binding_hash)
        || !valid_prefixed_sha256(&arguments.expected_producer_content_id, "fpc1_")
        || !crate::is_sha256_hex(&arguments.expected_producer_manifest_sha256)
        || arguments.candidates.is_empty()
        || arguments.candidates.len() > MAX_CANDIDATES
    {
        bail!("account_ingress_request_contract_invalid");
    }

    let mut previous_ordinal = None;
    let mut unique_candidates = BTreeSet::new();
    for candidate in &arguments.candidates {
        if previous_ordinal.is_some_and(|previous| candidate.ordinal <= previous)
            || candidate.normalized_candidate.len() < MIN_CANDIDATE_DIGITS
            || candidate.normalized_candidate.len() > MAX_CANDIDATE_DIGITS
            || !candidate
                .normalized_candidate
                .bytes()
                .all(|byte| byte.is_ascii_digit())
            || !unique_candidates.insert(candidate.normalized_candidate.as_str())
        {
            bail!("account_ingress_request_contract_invalid");
        }
        previous_ordinal = Some(candidate.ordinal);
    }
    Ok(())
}

pub(crate) fn resolve_account_ingress(
    conn: &Connection,
    arguments: &ResolveAccountIngressHostArguments,
) -> Result<ResolveAccountIngressHostResult> {
    resolve_account_ingress_inner(conn, arguments).map_err(sanitize_account_ingress_error)
}

fn resolve_account_ingress_inner(
    conn: &Connection,
    arguments: &ResolveAccountIngressHostArguments,
) -> Result<ResolveAccountIngressHostResult> {
    validate_resolve_account_ingress_host_arguments(arguments)?;
    let mut session = VerifiedStatsQuerySession::begin(conn, arguments)?;
    let snapshot = session.snapshot_binding();
    if snapshot.producer_content_id != arguments.expected_producer_content_id
        || snapshot.producer_manifest_sha256 != arguments.expected_producer_manifest_sha256
    {
        bail!("account_ingress_snapshot_authority_binding_mismatch");
    }
    let provenance = AccountIngressResolutionProvenance {
        dataset_snapshot_id: arguments.dataset_snapshot_id.clone(),
        context_epoch: arguments.context_epoch,
        context_digest: arguments.context_digest.clone(),
        case_binding_hash: arguments.case_binding_hash.clone(),
        expected_producer_content_id: arguments.expected_producer_content_id.clone(),
        expected_producer_manifest_sha256: arguments.expected_producer_manifest_sha256.clone(),
        duckdb_content_snapshot_digest: snapshot.duckdb_content_snapshot_digest.clone(),
        duckdb_snapshot_manifest_sha256: snapshot.duckdb_snapshot_manifest_sha256.clone(),
        materialization_identity: snapshot.materialization_identity.clone(),
        source_signature: snapshot.source_signature.clone(),
        result_signature: snapshot.result_signature.clone(),
        producer_content_id: snapshot.producer_content_id.clone(),
        producer_manifest_sha256: snapshot.producer_manifest_sha256.clone(),
        query_contract: ACCOUNT_INGRESS_QUERY_CONTRACT.to_string(),
        query_sql_hash: account_ingress_query_sql_hash(),
    };

    let mut resolutions = Vec::with_capacity(arguments.candidates.len());
    let mut semantic_bytes = 0usize;
    for candidate in &arguments.candidates {
        let (resolution, observed_rows) = query_candidate(session.conn(), candidate)?;
        semantic_bytes = semantic_bytes
            .checked_add(resolution.bank_institution.len())
            .and_then(|value| value.checked_add(resolution.account_type.len()))
            .filter(|value| *value <= MAX_TOTAL_SAFE_SEMANTIC_BYTES)
            .ok_or_else(|| anyhow!("account_ingress_result_limit_exceeded"))?;
        let range_hash = candidate_range_hash(candidate);
        session.record_observed_range(
            ACCOUNT_INGRESS_QUERY_CONTRACT,
            &range_hash,
            observed_rows,
        )?;
        resolutions.push(resolution);
    }
    session.commit()?;
    Ok(ResolveAccountIngressHostResult {
        schema_version: 1,
        contract: ACCOUNT_INGRESS_RESULT_CONTRACT.to_string(),
        resolutions,
        provenance,
    })
}

fn query_candidate(
    conn: &Connection,
    candidate: &ResolveAccountIngressCandidate,
) -> Result<(AccountIngressResolution, i64)> {
    let mut statement = conn
        .prepare(ACCOUNT_INGRESS_QUERY_SQL)
        .map_err(|_| anyhow!("account_ingress_query_failed"))?;
    let mut rows = statement
        .query(params![
            candidate.normalized_candidate.as_str(),
            candidate.normalized_candidate.as_str(),
            candidate.normalized_candidate.as_str(),
        ])
        .map_err(|_| anyhow!("account_ingress_query_failed"))?;
    let mut matches = Vec::with_capacity(2);
    while let Some(row) = rows
        .next()
        .map_err(|_| anyhow!("account_ingress_query_failed"))?
    {
        if matches.len() >= 2 {
            bail!("account_ingress_query_limit_invalid");
        }
        let bank = safe_semantic_from_value(
            row.get_ref(0)
                .map_err(|_| anyhow!("account_ingress_result_contract_invalid"))?,
        );
        let account_type = safe_semantic_from_value(
            row.get_ref(1)
                .map_err(|_| anyhow!("account_ingress_result_contract_invalid"))?,
        );
        let cross_account_card_conflict = row
            .get::<_, bool>(2)
            .map_err(|_| anyhow!("account_ingress_result_contract_invalid"))?;
        matches.push((bank, account_type, cross_account_card_conflict));
    }
    let observed_rows = i64::try_from(matches.len())
        .map_err(|_| anyhow!("account_ingress_result_contract_invalid"))?;
    let empty = || AccountIngressResolution {
        ordinal: candidate.ordinal,
        disposition: AccountIngressResolutionDisposition::NotFound,
        entity_type: String::new(),
        bank_institution: String::new(),
        account_type: String::new(),
    };
    match matches.as_slice() {
        [] => Ok((empty(), observed_rows)),
        [(Ok(bank), Ok(account_type), false)] => Ok((
            AccountIngressResolution {
                ordinal: candidate.ordinal,
                disposition: AccountIngressResolutionDisposition::Resolved,
                entity_type: "bank_account_number".to_string(),
                bank_institution: bank.clone(),
                account_type: account_type.clone(),
            },
            observed_rows,
        )),
        [(_bank, _account_type, _cross_account_card_conflict)] => Ok((
            AccountIngressResolution {
                ordinal: candidate.ordinal,
                disposition: AccountIngressResolutionDisposition::IntegrityFailure,
                entity_type: String::new(),
                bank_institution: String::new(),
                account_type: String::new(),
            },
            observed_rows,
        )),
        _ => Ok((
            AccountIngressResolution {
                ordinal: candidate.ordinal,
                disposition: AccountIngressResolutionDisposition::Ambiguous,
                entity_type: String::new(),
                bank_institution: String::new(),
                account_type: String::new(),
            },
            observed_rows,
        )),
    }
}

fn safe_semantic_from_value(value: ValueRef<'_>) -> Result<String> {
    let bytes = match value {
        ValueRef::Null => return Ok(String::new()),
        ValueRef::Text(value) => value,
        _ => bail!("account_ingress_result_contract_invalid"),
    };
    if bytes.len() > MAX_SAFE_SEMANTIC_BYTES {
        bail!("account_ingress_unsafe_semantic");
    }
    let value = std::str::from_utf8(bytes)
        .map_err(|_| anyhow!("account_ingress_result_contract_invalid"))?;
    if value != value.trim()
        || value.chars().any(|character| {
            character.is_control()
                || matches!(
                    character,
                    '\u{200b}'
                        | '\u{200c}'
                        | '\u{200d}'
                        | '\u{2060}'
                        | '\u{feff}'
                        | '\u{202a}'..='\u{202e}'
                        | '\u{2066}'..='\u{2069}'
                )
        })
        || looks_like_complete_financial_identifier(value)
    {
        bail!("account_ingress_unsafe_semantic");
    }
    Ok(value.to_string())
}

fn looks_like_complete_financial_identifier(value: &str) -> bool {
    let mut digit_run = 0usize;
    for character in value.chars() {
        if character.is_numeric() {
            digit_run += 1;
            if digit_run >= MIN_CANDIDATE_DIGITS {
                return true;
            }
        } else if character.is_whitespace() || character.is_ascii_punctuation() {
            continue;
        } else {
            digit_run = 0;
        }
    }
    false
}

impl VerifiedStatsQueryRequest for ResolveAccountIngressHostArguments {
    fn case_id(&self) -> &str {
        &self.case_id
    }

    fn command(&self) -> &'static str {
        RESOLVE_ACCOUNT_INGRESS_COMMAND
    }

    fn canonical_scope(&self) -> Value {
        json!({
            "datasetSnapshotId": self.dataset_snapshot_id,
            "contextEpoch": self.context_epoch,
            "contextDigest": self.context_digest,
            "caseBindingHash": self.case_binding_hash,
            "expectedProducerContentId": self.expected_producer_content_id,
            "expectedProducerManifestSha256": self.expected_producer_manifest_sha256,
            "candidates": self.candidates.iter().map(|candidate| json!({
                "ordinal": candidate.ordinal,
                "normalizedCandidate": candidate.normalized_candidate,
            })).collect::<Vec<_>>(),
        })
    }

    fn has_required_scope(&self) -> bool {
        !self.candidates.is_empty() && self.candidates.len() <= MAX_CANDIDATES
    }
}

fn candidate_range_hash(candidate: &ResolveAccountIngressCandidate) -> String {
    hash_framed(
        ACCOUNT_INGRESS_RANGE_HASH_DOMAIN,
        &[
            account_ingress_query_sql_hash().as_bytes(),
            candidate.ordinal.to_string().as_bytes(),
            candidate.normalized_candidate.as_bytes(),
        ],
    )
}

pub(crate) fn account_ingress_query_sql_hash() -> String {
    hash_framed(
        ACCOUNT_INGRESS_QUERY_SQL_HASH_DOMAIN,
        &[ACCOUNT_INGRESS_QUERY_SQL.as_bytes()],
    )
}

fn hash_framed(domain: &[u8], values: &[&[u8]]) -> String {
    let mut hasher = Sha256::new();
    hash_one(&mut hasher, domain);
    for value in values {
        hash_one(&mut hasher, value);
    }
    format!("{:x}", hasher.finalize())
}

fn hash_one(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

fn sanitize_account_ingress_error(error: anyhow::Error) -> anyhow::Error {
    let error_chain = format!("{error:#}");
    let code = [
        "account_ingress_request_contract_invalid",
        "account_ingress_snapshot_authority_binding_mismatch",
        "account_ingress_query_failed",
        "account_ingress_query_limit_invalid",
        "account_ingress_result_contract_invalid",
        "account_ingress_result_limit_exceeded",
        "account_ingress_unsafe_semantic",
    ]
    .into_iter()
    .find(|code| error_chain.contains(code))
    .unwrap_or_else(|| {
        if error_chain.contains("stats_query_connection_not_readonly") {
            "account_ingress_snapshot_connection_invalid"
        } else if error_chain.contains("stats_query_external_access_not_disabled") {
            "account_ingress_external_access_not_disabled"
        } else if error_chain.contains("stats_query_") {
            "account_ingress_snapshot_verification_failed"
        } else {
            "account_ingress_internal_failed"
        }
    });
    anyhow!(code)
}

fn valid_prefixed_sha256(value: &str, prefix: &str) -> bool {
    value.strip_prefix(prefix).is_some_and(crate::is_sha256_hex)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn arguments() -> ResolveAccountIngressHostArguments {
        ResolveAccountIngressHostArguments {
            case_id: "case-a".to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 7,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: format!("fpc1_{}", "4".repeat(64)),
            expected_producer_manifest_sha256: "5".repeat(64),
            candidates: vec![ResolveAccountIngressCandidate {
                ordinal: 4,
                normalized_candidate: "6222021234567890123".to_string(),
            }],
        }
    }

    #[test]
    fn arguments_are_strict_bounded_and_debug_redacted() {
        let valid = arguments();
        validate_resolve_account_ingress_host_arguments(&valid).expect("valid arguments");
        assert!(!format!("{valid:?}").contains("6222021234567890123"));

        let mut duplicate = valid.clone();
        duplicate.candidates.push(ResolveAccountIngressCandidate {
            ordinal: 5,
            normalized_candidate: duplicate.candidates[0].normalized_candidate.clone(),
        });
        assert!(validate_resolve_account_ingress_host_arguments(&duplicate).is_err());

        let mut reordered = valid.clone();
        reordered.candidates.push(ResolveAccountIngressCandidate {
            ordinal: 3,
            normalized_candidate: "6217009876543210987".to_string(),
        });
        assert!(validate_resolve_account_ingress_host_arguments(&reordered).is_err());

        let mut too_many = valid;
        too_many.candidates = (0..=MAX_CANDIDATES)
            .map(|index| ResolveAccountIngressCandidate {
                ordinal: index as u32,
                normalized_candidate: format!("{:019}", index + 1),
            })
            .collect();
        assert!(validate_resolve_account_ingress_host_arguments(&too_many).is_err());
    }

    #[test]
    fn fixed_query_is_parameterized_and_semantics_reject_identifiers() {
        assert!(ACCOUNT_INGRESS_QUERY_SQL.contains("a.account_key = ?"));
        assert!(ACCOUNT_INGRESS_QUERY_SQL.contains("w.acct_no = ?"));
        assert!(ACCOUNT_INGRESS_QUERY_SQL.contains("c.card_no = ?"));
        assert!(!ACCOUNT_INGRESS_QUERY_SQL.contains("{}"));
        assert_eq!(
            account_ingress_query_sql_hash(),
            ACCOUNT_INGRESS_QUERY_SQL_HASH_V1
        );
        assert!(safe_semantic_from_value(ValueRef::Text("中国银行".as_bytes())).is_ok());
        assert!(
            safe_semantic_from_value(ValueRef::Text("账号6222021234567890123".as_bytes())).is_err()
        );
    }

    #[test]
    fn fixed_query_requires_account_witness_and_fails_closed_on_card_confusion() {
        const ACCOUNT: &str = "6222021234567890123";
        const CARD_ONLY: &str = "6217009876543210987";
        const OTHER_ACCOUNT: &str = "6228480402564890018";
        let conn = Connection::open_in_memory().expect("open account-ingress fixture");
        conn.execute_batch(&format!(
            "CREATE TABLE analysis_account_dim( \
               account_key TEXT, bank_name TEXT, acct_type TEXT \
             ); \
             INSERT INTO analysis_account_dim VALUES \
               ('{ACCOUNT}', '银行甲', '结算账户'), \
               ('{CARD_ONLY}', '银行乙', '银行卡'), \
               ('{OTHER_ACCOUNT}', '银行丙', '结算账户'); \
             CREATE TABLE analysis_txn_detail_idx( \
               acct_key TEXT, acct_no TEXT, card_no TEXT \
             ); \
             INSERT INTO analysis_txn_detail_idx VALUES \
               ('{ACCOUNT}', '{ACCOUNT}', NULL), \
               ('{CARD_ONLY}', NULL, '{CARD_ONLY}'), \
               ('{OTHER_ACCOUNT}', '{OTHER_ACCOUNT}', '{ACCOUNT}');"
        ))
        .expect("seed account and card provenance");

        let account_candidate = ResolveAccountIngressCandidate {
            ordinal: 1,
            normalized_candidate: ACCOUNT.to_string(),
        };
        let (confused, _) = query_candidate(&conn, &account_candidate).expect("query confusion");
        assert_eq!(
            confused.disposition,
            AccountIngressResolutionDisposition::IntegrityFailure
        );

        let card_candidate = ResolveAccountIngressCandidate {
            ordinal: 2,
            normalized_candidate: CARD_ONLY.to_string(),
        };
        let (card_only, _) = query_candidate(&conn, &card_candidate).expect("query card-only");
        assert_eq!(
            card_only.disposition,
            AccountIngressResolutionDisposition::NotFound
        );

        let unambiguous_candidate = ResolveAccountIngressCandidate {
            ordinal: 3,
            normalized_candidate: OTHER_ACCOUNT.to_string(),
        };
        let (unambiguous, _) =
            query_candidate(&conn, &unambiguous_candidate).expect("query unambiguous account");
        assert_eq!(
            unambiguous.disposition,
            AccountIngressResolutionDisposition::Resolved
        );
    }
}
