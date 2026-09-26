# Core/Funds delivery vectors (test only)

The three JSON files are byte-identical synthetic inputs from the user-supplied
2026-09-21 delivery archive. They are not installed resources or evidence receipts.
The archive SHA256 is `c5ae96f09e62374539b82011b4d570055883e6b20690c47fc3cea1a3e1b43d33`.

- funds-facts.jsonl: `6894ac103a21260de3260aba39a49c15749ee2ec1db52842f5764abc6fc3cc82`
- expected-queries.json: `c70d888a3602725bfb0c22801989596d776e3a2db2f43fdf101738e27deb5cef`
- canaries.json: `24d00ec8e1fa381fc8d48c11fb228c9f7362626282d1d89125e1b2a7ef142adf`

`src/funds_delivery_vectors_test.rs` exercises the actual canonical importer,
materializer, immutable DuckDB query and source preservation. It first rejects
unsupported original input and independently checks invalid money and USD refusal.
It then derives separately identified CNY positive controls for A1, B1 and A2.
Each uses a different database inode and actual producer manifest. Internal query
identity fields are test inputs, not Go-granted case/snapshot authority.

Derivation is explicit: select the case/snapshot, remove USD, invalid A011 and
repeated observation A001_DUP, strip redundant decimal zeros, and encode a valid
account from raw_account + subject_key without underscores. The vector's two
subjects share the same raw account, whereas the product query selects an account;
that test mapping is not a product identity resolution claim. Names/memos remain
source-exact. Second-precision timestamps map [Sep 1, Oct 1) to the product's
inclusive Sep 1 through Sep 30 23:59:59 contract. Same-amount distinct transactions
remain included. Source hashes are emitted, not silently relabeled as original input.

The reference CNY amounts/counts are compared unchanged. USD is unsupported by this
import profile; the reference's cross-observation deduplication is not a supported
CSV source-identity operation. These tests do not prove Go import/DSV2 admission,
Provider projection, Final Gate, protected desktop display, activation or installed
recovery. Those seams must consume the real production admission result separately.
