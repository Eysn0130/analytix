# Economic Investigation Analysis Contract

This reference defines the domain task library for analytix current-case-project fund
analysis. It is production guidance, not an eval fixture, report template, or
old-case fact source.

## Product Position

Analytix main modules own import, cleaning, indexing, permission, export, and UI
workflows. The fund-analysis plugin reads the current case project through
the workspace `.analytix/case-project.json` binding and `analytix_funds` MCP tools, audits analytical readiness, and turns deterministic
case facts into public-security/economic-investigation analysis.

Production analysis uses Analytix-cleaned fact stores and analysis indexes:
`fc_*_norm` tables, `analysis_*` indexes, and backend-compiled fact cards.
`fc_*_raw` tables, raw source rows, weak sources, and unverified calculations
may support import/cleaning diagnostics or gap analysis, but they are not
ordinary investigation evidence. Raw source tables belong to Analytix
import/cleaning modules; the plugin may report source/import/cleaning coverage
or blockers through MCP summaries, but production fund-analysis claims need a
cleaned or verified source boundary.

The plugin should compress work that previously required many manual DuckDB
rounds: scope review, object dossiers, source/destination tracing, rescope and
recompute, negative search, one-more-hop continuation, claim review, and report
materialization. It must never copy fixed old-case subjects, amounts,
transaction ids, time windows, or golden wording into production references.

Short prompts can imply a large workflow:

- `某卡资金研判` means an account dossier, not a single number.
- `某人/某公司资金研判` means a subject dossier across direct and candidate
  accounts.
- `全案分析/完整研判/跑完整功能树` means the full-case analysis tree.
- `继续追一层/资金来源/资金去向` means auditable tracing, not a closed answer.

## Core Investigation Lanes

| Lane | Required coverage | Common handoff |
| --- | --- | --- |
| Case preflight | current case project, data volume, time span, cleaned-table/index coverage, table or scope gaps, available subject/account/contact/address lanes | `case-context`, `data-quality` |
| Account dossier | account identity, account-opening facts, time span, inflow/outflow totals and counts, top counterparties, behavior features, source/destination leads, gaps, next checks | `account-dossier`, `fund-tracing`, `investigation-lab` |
| Subject dossier | direct accounts, candidate accounts, identity/contact/address links, related companies/persons if supported, account ranking, inflow/outflow, common channels, source/destination, anomalies,补调对象 | `subject-dossier`, `counterparty-analysis` |
| Full-case analysis | data scale, time span, cleaned-table/index coverage, account/contact/address coverage, full-case inflow/outflow, holder/account rankings, each key subject's account situation, Top20 counterparties, suspicious features, fund-flow context, continuation queue | `full-case-analysis` |
| Topic analysis | public-to-private, total-vs-classified wages/labor/reimbursement, project funds, cost normality review, litigation/enforcement, coercive measures, cash, financial products, payment channels, asset-end leads, control/holding leads | `investigation-lab`, `counterparty-analysis` |
| Trace continuation | upstream source, downstream destination, one more hop, return-flow, stop reason, missing account/field, subpoena target | `fund-tracing` |
| Negative search | searched layers and fields, hit/not-found boundary, weak matches, next enrichment | `investigation-lab`, `data-quality` |
| Amount challenge | old口径, new口径, recomputed support, unsupported part, corrected wording | `data-quality`, `quick-fact`, `claim-review` |
| Delivery pack | facts, tables, feature cards, flow cards, evidence request list, safe report paragraphs, claim QA | `report-builder`, `claim-review` |

## Cleaned DuckDB Fact Layers

Current-case-project fund analysis is not only a transaction-detail task. Codex should
think across the case DuckDB fact layers, while every user-visible fact remains
bound to the MCP-backed scope returned by Analytix.

| Fact layer | What it can support | Boundary |
| --- | --- | --- |
| Cleaned transaction detail: `fc_transaction_norm` and `analysis_txn_detail_idx` | amount, direction, time, source account, counterparty, balance if available, summary/remark/merchant/transaction type, transaction id, cleaned row scope, indexed counterparty keys | detail rows support transaction edges or examples; bounded previews cannot be summed into full-case totals |
| Transaction environment / device / channel: cleaned `ip_addr`, `mac_addr`, `teller_no`, `branch_name`, `location`, `merchant_name`, `merchant_no`, and receipt/channel fields in `fc_transaction_norm`, `analysis_txn_detail_idx`, and rule indexes | shared IP/MAC/device clues, same teller/branch/location concentrations, merchant/channel clues, payment-terminal or cash-counter context | environment overlap is a lead, not proof of actual operator, device owner, organized gang, or same physical person; merchant/channel clues require platform or institution records |
| Account-opening / account registry: `fc_account_norm`, `fc_sub_account_norm`, `analysis_account_dim` | holder name, certificate/id fields, account/card, bank/branch, account type, open/close time, status, public/private account label, sub-account clues | identity and scope evidence; it is not transaction proof and does not prove actual control by itself |
| Person identity/contact/address: `fc_person_norm`, `fc_person_contact_norm`, `fc_person_address_norm` | customer name, id number, employer, legal representative/company clues, phone, address, contact overlap, address overlap | identity and correlation leads; phone/address overlap is not control, ownership, or fund flow without supporting evidence |
| Task feedback and coercive measures: `fc_task_success_norm`, `fc_task_fail_norm`, `fc_coercive_measure_norm` | bank feedback coverage, failed-query gaps, subject/acct query scope, freeze/seizure/measure clues, amount/date/agency fields when available | coverage and enforcement clues; a failed query is a data gap, not absence of an account; a measure is not proof of offense |
| Holder / subject index | direct holder accounts, normalized names, candidate aliases, company/person labels, subject groups, account-set coverage | candidate aliases and candidate-linked accounts must remain candidate until evidence closes the gap |
| Counterparty index | counterparty names/accounts, missing-name rates, account-like counterparties, merchant/channel labels, common counterparties | missing or masked names are data gaps, not hidden identity proof |
| Casegraph / relationship layer | person-company-account-contact-address relations, imported relationship material, user-provided hypotheses, graph edges, external clues if loaded | graph context suggests scope and hypotheses; unsupported graph edges do not prove fund flow |
| Pipeline and reconciliation layer | import/source counts, cleaned detail, analysis index, report scope, duplicate/card-replacement candidates, non-indexed cleaned-table gaps | each total must name the layer and口径; candidate duplicate families cannot silently reduce totals |

Minimum preflight language should separate `cleaned transaction detail`,
`analysis index`, `account-opening facts`, `person/contact/address facts`,
`task feedback`, `coercive-measure clues`, `casegraph facts`, and `report scope`
when those layers affect the answer. Do not tell ordinary investigation users
that raw source fields were searched; if the data-quality lane needs source
provenance, phrase it as cleaned pipeline coverage.

When IP/MAC, teller, branch, location, merchant, or receipt fields are present,
preflight should report their coverage as `transaction-environment coverage`.
If coverage is sparse, say it is optional enrichment. Do not block ordinary
fund analysis merely because environment fields are missing.

## Fund Analysis Method Library

Use these methods as investigative lenses. They should trigger deterministic
facts, feature status, downgrade reasons, and next proof; they must not become
legal conclusions.

### Account Role Classification

Account roles are fund-flow roles, not criminal identities. A single account may
hold multiple candidate roles across different time windows.

| Candidate role | Signals to test | Output boundary |
| --- | --- | --- |
| Living-card account | payroll/living expense/rent/utilities/education/medical/food, small recurring personal consumption, stable low-risk counterparties | can downgrade suspicion; suspicious only when mixed with project funds, rapid pass-through, cash breaks, or asset endpoints |
| Business-use personal account | company/project funds enter personal account, reimbursement/labor/material summaries, company costs paid from personal card | business-use lead; needs ledger, approval, invoice, role evidence for stronger conclusions |
| Transit/pass-through account | fast-in-fast-out, low balance retention, same-day/next-day outbound, many unrelated sources and destinations | pass-through feature, not final control or illegal purpose |
| Convergence/collection account | many sources aggregate to one holder/account, high concentration, repeated small/medium inflows | collection feature; source identities and purpose require proof |
| Dispersion/payout account | one or few sources disperse to many endpoints, repeated payout patterns | payout feature; endpoint purpose and beneficiary require enrichment |
| Cash-heavy account | frequent ATM/counter cash in/out, cash breaks near relevant flows, missing counterparty | cash clue; physical cash identity requires external evidence |
| Financial-product account | wealth management, fund, securities, insurance, redemption, dividend, rollover | financial-product flow; ownership/source remains bounded to transaction facts |
| Payment-channel account | Alipay, WeChat, Tenpay, UnionPay, clearing institution, merchant/payee channel | platform-channel clue; platform account/control needs payment institution records |
| Device/channel-coordinated account | shared IP/MAC, same teller/branch/location concentrations, repeated merchant/channel clues, same receipt/terminal patterns | coordination lead; actual operator, device owner, or gang relationship requires login, device, platform, surveillance, or interrogation evidence |
| Asset-consumption account | vehicle, real estate, renovation, property fee, parking, insurance, loan repayment | asset lead; ownership needs registry, contract, invoice, or loan material |
| Dormant/sudden-active account | long inactivity followed by dense high-value movement | suspicious feature only after time-span baseline and coverage review |
| Related-company/internal-cycle account | repeated person-company or company-company transfers among related entities | relationship and transaction purpose require company records and external proof |

### Cash Same-Deposit/Withdraw And Cash Bridge

`现金同存同取` and cash bridge analysis should test near-time and near-amount
cash pairs:

- same day or next day, with explicit time gap;
- same or near amount, with explicit amount gap and tolerance;
- counter/ATM/channel/branch if available;
- source withdrawal account and target deposit account;
- transaction ids or substitutable fact fields;
- competing candidates and missing fields.

Even a strong pair remains a `cash bridge candidate` unless external evidence
links the physical cash. Do not write same physical cash, cash laundering, or
cash delivery as confirmed from bank flow alone.

### Asset, Consumption, And Financial-Product Endpoints

Asset and financial-product analysis should scan for vehicle, real estate,
renovation, property, parking, insurance, loan repayment, wealth management,
fund, securities, redemption, dividend, and rollover clues. Output should
separate:

- transaction fact: amount, date, payer, payee, summary, channel;
- endpoint lead: vehicle/house/financial product/insurance/loan clue;
- ownership gap: registry, contract, invoice, policy, loan, platform, or bank
  reply still needed;
- relation gap: whether the asset or product belongs to the suspect, related
  person, company, or an unrelated party.

### Cross-Border / Overseas Transfer Clues

Cross-border clues include foreign-currency movement, overseas bank/remittance
counterparties, trade/FX summaries, offshore keywords, cross-border payment
platform clues, and virtual-asset fiat on/off-ramp clues. A clue is not proof of
overseas transfer or underground banking. Stronger conclusions require bank
remittance material, payment-platform records, customs/trade documents, FX
records, virtual-asset platform records, or external identity/control evidence.

### Project, Public-To-Private, And Cost Normality Review

For project, contract, bid, duty-embezzlement, tax, and corruption lenses, the
plugin must distinguish ordinary business cost from suspicious benefit transfer.
Test normal material/labor/logistics/tax/refund/loan repayment patterns before
upgrading a company-to-person flow. Suspicion increases when project funds move
to related persons, personal cards pay company costs, funds rapidly pass through
to cash/assets, or reimbursement/wage summaries contradict role and timing.

### Identity, Contact, And Address Correlation

Contact and address tables help Codex see investigative context that pure bank
flows miss. Use them to find same phone, same address, employer, legal
representative, or certificate overlap among persons, companies, accounts, and
counterparties. Output must split:

- strong identity match: same certificate/id or MCP-confirmed account holder;
- medium correlation: same phone/address/employer/legal representative with
  compatible names or account facts;
- weak lead: same name fragment, partial id, same address text, or contact
  overlap without transaction support;
- forbidden upgrade: phone/address/contact overlap written as actual control,
  nominee holding, conspiracy, ownership, or fund-flow proof.

### Group Association, Device/IP/MAC, And Transaction-Environment Correlation

Economic-investigation users often ask whether multiple people, companies, or
accounts belong to one group, team, card-running network, project circle, or
actual controller. Treat this as a correlation problem, not a legal conclusion.
Use cleaned fields and casegraph context to test:

- shared or repeated IP/MAC/device clues;
- same teller, branch, location, receipt, terminal, merchant, or channel
  concentrations;
- same phone, address, employer, legal representative, contact, or opening
  information;
- repeated upstream/downstream counterparties, synchronized transfer windows,
  split/round amount patterns, and common cash or payment-channel endpoints;
- external relationship material already loaded into casegraph.

Output a Group Association Lead Card with `strong / medium / weak / not found`
status. Strong means deterministic identity/account evidence plus transaction
support. Medium means multiple independent overlaps with compatible timing.
Weak means a single overlap such as shared IP, same phone, or same branch.
Never write shared IP/MAC, phone, address, branch, or merchant overlap as
organized gang, actual control, co-offending, card-running role, or same
operator without external device/platform/identity evidence.

### Payment Channel, Merchant, Virtual-Asset, And Platform Leads

Payment and merchant fields are high-value investigative lenses when present:
Alipay, WeChat/Tenpay, UnionPay, NetUnion, clearing institutions, merchant
names/numbers, account-like platform counterparties, digital wallet keywords,
virtual-asset fiat on/off-ramp clues, OTC or exchange-like names, and platform
settlement summaries. Output should split:

- bank-flow fact: amount, date, payer/payee, channel, merchant/platform clue;
- platform lead: payment institution, merchant, wallet, exchange, OTC, or
  aggregator to subpoena;
- evidence gap: platform account registration, order/merchant record, device/IP,
  KYC, wallet address, exchange record, or trade document;
- forbidden upgrade: platform/channel clue written as confirmed platform control,
  virtual-asset transfer, gambling settlement, underground banking, or laundering.

### Task Feedback, Coverage, And Coercive Measures

Full-case and subject dossiers should check whether query/feedback tables show
successful and failed bank feedback, whether key subjects or accounts are absent
because of failed/partial feedback, and whether coercive-measure records affect
asset/freeze/enforcement interpretation. A failure row is a coverage gap; it
does not prove no account exists. A freeze/seizure/coercive-measure row is an
investigative event clue; it does not prove criminal liability.

### Company-To-Person Two-Layer Statistics

When users ask for wages, salary, labor, reimbursement, travel, subsidy, or
public-to-private settlement, do not collapse all company-to-person payments
into one label. Use the two-layer pattern:

- total company-to-person receipts within the user-defined company/subject
  scope, including rows with blank or unclassifiable summaries;
- identifiable subset where summary, remark, transaction type, counterparties,
  or rule hits support wages/salary/labor/reimbursement/travel/subsidy wording;
- unclassified remainder with amount/count and proof gap;
- dedupe boundary when personal receipt side and company payment side both show
  the same transaction.

### Negative Search In Cleaned Fields

Negative search should inspect cleaned and indexed fields: holder/opening name,
holder id, account/card, counterparty name/account/id, summary, remark,
transaction type, merchant, branch/location, person/contact/address fields,
task feedback fields, coercive-measure fields, and casegraph aliases. It must
state the searched cleaned layers and fields. It must not claim to have searched
raw tables in production output.

## Completion Packages By Task

Short user prompts can imply complete work packages. A package is complete only
when the covered facts are either returned or explicitly marked unavailable.

| Task | Completion package |
| --- | --- |
| Full-case analysis | data scale, time span, cleaned-table/index/report coverage, account-opening coverage, person/contact/address coverage, task feedback and coercive-measure coverage, subject/person/company inventory, each key subject's account situation, account role classification, full-case inflow/outflow amount/count, account/holder/counterparty Top20, public-to-private two-layer statistics, cost normality review, cash, asset, financial-product, payment-channel, cross-border clues, typology leads, fund-flow context, negative findings, quality gaps, continuation/evidence-request list |
| Single-account analysis | account-opening identity and status, account type/bank/branch when available, transaction coverage/time span, inflow/outflow amount/count, top counterparties, role classification, source/destination leads, cash/asset/financial/payment-channel clues, negative findings, data gaps, next proof |
| Person/company subject analysis | identity and account-opening facts, contact/address/employer/legal-representative correlation, direct accounts, candidate accounts, per-account role, account portfolio ranking, total inflow/outflow, top counterparties, source/destination and related-subject channels, public-to-private or business-use clues, cash/asset/financial/cross-border leads, missing-field/ownership/control gaps, next subpoenas |
| Topic / suspicious-feature scan | tested feature families, supporting facts, evidence status, typology lens, downgrade reason, negative searched fields, next proof |
| Visual/table evidence | selected evidence surface, source facts, table/chart inventory, scope/unit/time window/metric/direction, evidence status, unsupported visual claims, next owner skill |
| Report materialization | result-first Chinese material, precise tables, 万元 narrative summaries when useful, no raw database field names, no SQL/tool steps, claim QA, attachment/evidence request tables, legal-boundary wording |

## Impeccable-Style Quality Rules For Fund Analysis

Use these rule ids in implementation comments, anti-pattern detectors, eval
labels, and critique passes when possible.

| Rule id | Failure signal | Corrective pattern |
| --- | --- | --- |
| `fund-scope-layer-mix` | raw, normalized, cleaned, index, report scope, and account-opening facts are mixed without口径 | name the fact layer and downgrade unsupported totals |
| `fund-raw-table-bypass` | ordinary investigation uses `fc_*_raw`, source detail rows, weak sources, or unverified calculations as evidence | use MCP-backed `fc_*_norm` / `analysis_*` facts and pipeline coverage summaries |
| `fund-opening-as-transaction-proof` | account-opening or registry facts are used as proof of transaction behavior or actual control | use opening data for identity/scope only; require flow/control evidence for stronger claims |
| `fund-contact-address-control-upgrade` | shared phone, address, employer, or legal representative becomes actual control/ownership | label identity/contact/address correlation and request external control proof |
| `fund-device-ipmac-control-upgrade` | shared IP/MAC, teller, branch, location, merchant, or terminal overlap becomes actual operator, controller, gang, or co-offending proof | label transaction-environment correlation and request device/platform/identity evidence |
| `fund-platform-virtualasset-upgrade` | payment-channel, merchant, wallet, OTC, or exchange-like clue becomes confirmed platform control, virtual-asset transfer, gambling settlement, laundering, or underground banking | label platform/virtual-asset lead and request payment institution, KYC, order, wallet, or chain evidence |
| `fund-candidate-owner-upgrade` | candidate-linked accounts, same-name hits, aliases, or graph hints become confirmed ownership | keep direct/candidate labels and request identity/control proof |
| `fund-account-role-overclaim` | living card, transit account, collection account, or cash-heavy account is written as a legal/criminal role | write candidate fund-flow role with feature facts and downgrade reason |
| `fund-cash-bridge-upgrade` | near-time cash withdrawal/deposit is written as the same physical cash | write cash bridge candidate with time/amount/channel gaps |
| `fund-life-card-over-suspicion` | normal living-card behavior is described as suspicious without contradiction | state normality/downgrade unless mixed with high-risk flows |
| `fund-asset-ownership-upgrade` | vehicle/house/insurance/loan payment becomes asset ownership | request registry, contract, invoice, policy, or loan evidence |
| `fund-cross-border-upgrade` | FX/offshore/platform clue becomes confirmed overseas transfer or underground banking | require remittance, payment, customs/trade, FX, platform, or control evidence |
| `fund-total-vs-classified-mix` | total company-to-person payments are written as wages/reimbursement because some summaries match | split total, identifiable subset, unclassified remainder, and dedupe boundary |
| `fund-cost-normality-skip` | project material/labor payments are treated as suspicious without normal cost review | separate ordinary cost appearance from intersecting benefit-transfer leads |
| `fund-task-feedback-coverage-skip` | missing account/subject facts ignore failed or partial bank feedback | report task feedback coverage and failed-query gaps before negative conclusions |
| `fund-plan-only-fullcase` | full-case analysis returns only a plan or summary | run or request the full-case package and mark unavailable lanes explicitly |
| `fund-report-gate-pollution` | ordinary analysis outputs report gate, doctor, eval, write_blocked, or internal labels | return user-facing facts, boundaries, and next actions only |
| `fund-visual-overclaim` | Top tables, heatmaps, dashboards, or graph-like visuals imply a supported transaction path, final destination, control relation, or legal conclusion | label the visual as ranking/aggregation/correlation/lead; use supported edges only for transaction paths |
| `fund-visual-context-missing` | evidence table or chart lacks scope, unit, time window, metric, direction, or evidence status | add a reader-facing title and口径 line before treating it as deliverable material |

## Economic-Crime Typology Matrix

Typologies are investigative lenses, not legal conclusions. A hit means a lead
or feature family to test; it does not prove the named offense.

| Typology lens | Typical fund patterns | Useful feature families | Forbidden upgrade | Proof/enrichment needs |
| --- | --- | --- | --- | --- |
| Telecom fraud / two-card / running-score | many small or medium inflows, fast pass-through, payment channels, cash breaks, many unrelated counterparties | high-frequency, fast-in-fast-out, split amounts, account role, payment channel, missing counterparty, device/IP/MAC overlap | calling an account a fraud account, runner, or criminal gang member from bank flow alone | upstream victim report, platform/account registration, device/IP/contact, interrogation, payment institution records |
| Online gambling settlement | dense payment-channel flows, night activity, repeated round amounts, many small counterparties, rapid aggregation | time concentration, amount concentration, third-party channel, cash/virtual-asset clues | stating gambling operation or bet settlement without external proof | platform data, chat/order records, merchant/payment institution data |
| Money laundering / underground banking | layered transfers, rapid conversion, cash/foreign trade/payment-channel endpoints, circular or paired flows | layering, return flow, cash bridge, financial product, virtual-asset clue, cross-subject channel | concluding laundering or underground banking成立 | predicate-crime material, OTC/exchange/payment records, control evidence, external counterpart verification |
| Illegal fundraising | many personal inflows to organizer or related company, periodic returns, interest-like payments, refund/rollover | concentration, investor-like counterparties, regular repayment, publicity/contract clues | calling principal/interest or illegal fundraising without contracts/publicity proof | contracts, promotion material, investor statements, ledger, company records |
| Pyramid scheme / membership rebate | member-like inflows, layered payouts, repeated rebate amounts, leader accounts | layered subject graph, repeated payout, rank-like groups | stating hierarchy or membership relation solely from flows | membership list, platform/team data, communication records |
| Duty embezzlement / misappropriation | company funds to personal/related accounts, reimbursement/wage-like summaries, later asset or cash endpoints | public-to-private, project/company channel, reimbursement keywords, asset lead, control lead | concluding embezzlement or misappropriation without duty/authority evidence | company ledger, contracts, approvals, invoices, role evidence |
| Bribery / corruption | funds or benefits to public-office contacts or related persons, same-name/contact weak matches, asset purchases | identity/contact correlation, weak/strong match split, asset lead, timing around project or approval | calling a same-name person public official or bribery target without identity proof | identity verification, employment/public-office proof, project approval material, communication/interrogation |
| False invoice / tax crime | public-private cycling, invoice-like summaries, abnormal company-to-company loops, cash return | circular flow, project/company channel, invoice/contract keyword, tax-period concentration | concluding虚开/偷逃税 from bank flow alone | invoice, tax, contract, goods/service, warehouse/logistics and ledger evidence |
| Contract/project/bid case | project payments, related-company transfers, reimbursements, personal accounts around project nodes | project keyword, related-party channel, source/destination trace, asset endpoint | treating every project payment as illicit benefit | contract, bidding files, acceptance, invoice, company ledger, relationship proof |
| Organized group / team association | multiple subjects share accounts, counterparties, phone/address, IP/MAC, branch/location, merchants, or synchronized transfer windows | group association, casegraph, shared environment, common channels, role split | calling a group an organized crime gang or co-offending team from overlap alone | identity/control proof, communications, device/platform records, surveillance/interrogation, external relationship material |
| Virtual asset / OTC exchange | bank inflow/outflow around exchange/OTC/payment-channel names, cash or cross-border clues, rapid conversion | platform lead, merchant/channel, cross-border, layering, device/IP | stating confirmed crypto transfer, wallet ownership, or underground banking | exchange/KYC/order records, wallet address evidence, chain analysis, platform/device records |
| Invoice / contract / tax-and-ticket chain | project/company flows around invoice, tax, contract, goods, logistics, reimbursement, and cash-return clues | document keyword, tax period, company loop, public-to-private, cost normality | concluding false invoicing, fake contract, or tax evasion from summaries alone | invoices, tax filings, contracts, acceptance, logistics, warehouse, service proof, company ledger |

## Abnormal Feature Catalog

Use the feature name as vocabulary, then attach supporting facts, status, and
downgrade reason.

| Feature family | Signals to test | Evidence status boundary |
| --- | --- | --- |
| Scale and concentration | outsized inflow/outflow, high share of total, top counterparty concentration | statistical feature, not offense fact |
| Frequency and time concentration | dense bursts, night/weekend activity, incident-window spikes | needs defined window and baseline |
| Large / round / threshold amounts | repeated large or round numbers, near-threshold split transfers | needs threshold and count/amount scope |
| Fast-in-fast-out | short holding time, high same-day or next-day outbound ratio, low balance retention | pass-through feature, not ownership conclusion |
| Split aggregation / dispersion | many sources to one destination, one source to many endpoints | channel feature; endpoint still needs proof |
| Return flow / circular flow | funds return to origin or related subject through supported edges | only supported edges can be drawn as paths |
| Same-fact / replacement-card / duplicate | same amount/time/counterparty or card replacement candidates | candidate duplicate cannot reduce totals without rule support |
| Public-to-private settlement | company/project funds into personal accounts, personal accounts paying company costs | business-use lead; not tax or crime conclusion |
| Wage/labor/reimbursement/travel | payroll-like or reimbursement summaries, repeated staff names, expense categories | may be legitimate unless contradicted by facts |
| Litigation/enforcement/refund | court, execution, refund, settlement, guarantee, deduction keywords | legal process lead, not case liability conclusion |
| Financial products | wealth management, fund, securities, insurance, redemption, dividend, rollover | visible financial-product flow, not final source/ownership |
| Cash break | ATM/counter cash deposit or withdrawal near relevant flows | cash bridge candidate, not same physical cash |
| Payment channel | Alipay, WeChat, Tenpay, UnionPay, clearing institution, account-like payee | channel clue; platform account/control needs records |
| Device/IP/MAC overlap | same IP, MAC, terminal, device-like field, teller, branch, location, or receipt concentration across subjects/accounts | transaction-environment lead; operator/control needs external proof |
| Asset endpoint | vehicle, real estate, renovation, property fee, parking, insurance, loan repayment | asset lead; ownership requires registry/contracts |
| Missing counterparty | blank name, account-only, masked merchant, cleaned/index field mismatch | data gap, not hidden identity proof |
| Related-company cycling | repeated transfers among related companies/persons/project accounts | relationship and purpose require external evidence |
| Identity/contact correlation | same name, phone, ID fragment, address, company role, contact-list clue | weak/strong match must be labeled separately |
| Group association | shared accounts, counterparties, contact/address, IP/MAC, merchant/channel, synchronized windows | association lead, not gang/criminal conclusion |
| Dormant/sudden-active account | long inactive period then dense movement | suspicious feature only after scope/baseline |
| High-frequency small payments | many low-value rows to many counterparties | typology lead; needs context and field search |

## Source And Destination Tracing

`资金来源` means visible upstream evidence, not final economic origin. `资金去向`
means visible downstream evidence, not final benefit or asset ownership.

For tracing outputs:

- Define seed, direction, depth, time window, amount tolerance, and supported
  edge criteria.
- Separate upstream source, downstream destination, return-flow, endpoint, and
  stop reason.
- Use `supported` only for transaction edges returned by trace/fundgraph tools.
- Use `candidate` for amount/time proximity, shared counterparty, keyword, cash
  bridge, financial-product clue, asset endpoint, or missing-account stop.
- Always provide a continuation queue when the next step needs bank, payment
  institution, asset registry, contract, invoice, platform, phone/device, or
  interview evidence.

## Negative Search Contract

A negative search can only say no hit was found in the searched current-case-project
layers and fields. It must state the fields searched, such as holder/account
name, counterparty name, account/id fields, summary, remark, purpose, merchant,
transaction type, IP/MAC/device/environment fields, teller/branch/location,
person/contact/address fields, task feedback, coercive measure fields,
cleaned/index fields, and casegraph aliases.

Output:

- searched object and variants;
- searched cleaned data layers and fields;
- exact hits, weak hits, and no-hit fields;
- boundary wording: `在已检字段未命中`, not `不存在`;
- next enrichment, such as cleaned pipeline coverage audit, payment platform
  records, external identity proof, or additional bank statements.

## Amount Challenge Contract

When the user challenges a number, recompute instead of defending the old text.
Return:

- original claim and old口径;
- requested new口径: scope, time, direction, success filter, dedupe, unit,
  gross/net, account/holder/counterparty set;
- supported amount/count and unsupported portion;
- why the answer changed or why it cannot be recomputed;
- corrected wording for reports or answers.

## Delivery Pack

Public-security/economic-investigation delivery is result material, not a tool
log. A delivery pack can include:

- Case Scope Card: data volume, time span, source/clean/index coverage, quality
  warnings.
- Subject/Account Table: direct/candidate boundary, accounts, inflow/outflow,
  main counterparties.
- Counterparty Table: direction, amount/count, shared channels, missing fields,
  subpoena priority.
- Feature Table: feature family, supporting facts, status, typology lens,
  downgrade reason, next proof.
- Flow Table: supported edge, candidate stop, amount/time gap, stop reason,
  next hop.
- Evidence Request Table: target institution/source, requested material, reason,
  related amount/risk, priority.
- Visual Evidence Pack: Top20 tables, concentration bars, time-window charts,
  subject-account-counterparty matrices, case dashboard cards, and appendix
  workbook sheets when they help the reader inspect evidence.
- Report Paragraphs: facts first, analysis second, boundary third, proof action
  last.

Use Chinese case-material wording. Paragraph summaries may use 万元 for
readability; tables should preserve precise amounts and units when available.
Avoid raw database fields, SQL steps, plugin routes, internal ids, doctor/eval
terms, and report-gate templates in ordinary user-facing material.

### Visual/Table Evidence Rules

Use visuals to make evidence easier to inspect, not to create new evidence.
Borrow Data Analytics-style chart discipline, but keep Analytix's proof
boundary:

- Use exact tables for Top20, subject/account/counterparty lists, continuation
  lists, evidence request lists, and report appendices.
- Use sorted bar or leaderboard charts for concentration and ranking only when
  the reader benefits from comparison; keep the exact table beside or behind the
  chart.
- Use time charts only when the query returns enough time buckets to show a
  shape. With sparse periods, use a table or short narrative comparison.
- Use matrices or heatmaps for subject-account-counterparty overlap or
  environment correlation, but label them as correlation/lead views, not paths.
- Use Mermaid/fund-flow diagrams only for supported transaction edges returned
  by fundgraph tools; a ranking, matrix, or shared IP/MAC overlap must never be
  drawn as an arrow flow.
- Every table/chart/dashboard card must state scope, unit, time window, metric,
  direction, and evidence status. If one is unavailable, say so.
- Attachment/workbook-style outputs should normally include Summary, Scope,
  Subject/Account, Counterparty Top20, Feature Leads, Flow/Trace, Evidence
  Requests, Claim QA, and Source Limitations sheets or sections when applicable.
- Public-security material should introduce a table or chart with one short
  explanatory paragraph before the table/visual appears.

## Legal And Proof Boundary

Bank-flow analysis can support investigative leads. It does not by itself prove
crime constitution, illegal gains, tax evasion, final ownership, actual control,
bribery, asset ownership, or subjective intent. Use `可疑特征`, `线索`,
`需复核`, `待补证`, and `建议调取`; do not use conclusive legal wording unless
external proof and user intent require a separate legal memo outside the fund
fact layer.
