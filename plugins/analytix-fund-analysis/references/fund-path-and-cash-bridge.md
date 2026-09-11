# Fund Path And Cash Bridge

Use this reference for `/analytix path` and `/analytix cash-bridge`.

## Fund Path

A report-grade path needs:

- seed account or transaction;
- direction;
- time window;
- amount tolerance;
- depth;
- each hop's account, counterparty, amount, time, balance feasibility, and transaction id when available;
- cut-off point, if funds leave visible data;
- warnings for missing or ambiguous data.

Mermaid is only an index. Always pair it with a compact transaction/fact table.

## Commingled Funds

When funds are mixed in an intermediate account:

- never say the same physical money was definitely transferred;
- describe an allocation model or candidate path;
- show time gap, amount gap, and balance feasibility;
- mention competing candidates when present.

Suggested wording:

> 在当前参数下，该路径具备资金承接可能性，属于混同资金链路候选，需结合账户余额、后续交易和外部证据复核。

## Cash Bridge

Use `同日/次日取现-存现对应特征` or `同存同取特征` for cash separation leads.

A candidate pair should include:

- withdrawal account;
- withdrawal time and amount;
- deposit account;
- deposit time and amount;
- time gap;
- amount gap or tolerance;
- branch/location/channel if returned;
- transaction ids or concrete account/time/amount fields when available;
- confidence and caveats.

Suggested wording:

> 该组交易呈现取现后同日/次日存现的对应特征，可作为现金桥接转移线索；因现金具有物理隔断属性，不能仅凭流水认定为同一笔资金。

## Output Shape

For paths and cash bridges:

1. parameters;
2. short narrative;
3. Mermaid diagram when useful;
4. transaction/fact table;
5. limits and next verification requests.
