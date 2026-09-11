#!/usr/bin/env node

import assert from "node:assert/strict";

import {
  arrayLengthOrUndefined,
  finiteNumber,
  nonNegativeInteger,
  parseNumericArgument
} from "./golden-qa.mjs";

for (const value of [undefined, null, "", " ", false, true, {}, [], Number.NaN, Number.POSITIVE_INFINITY]) {
  assert.equal(finiteNumber(value), undefined, `invalid numeric value ${String(value)} must remain absent`);
}
assert.equal(finiteNumber(0), 0, "explicit zero must survive");
assert.equal(nonNegativeInteger(0), 0, "explicit zero count must survive");
assert.equal(nonNegativeInteger(-1), undefined, "negative counts must remain invalid");
assert.equal(arrayLengthOrUndefined(undefined), undefined, "missing arrays must not become zero");
assert.equal(arrayLengthOrUndefined(null), undefined, "null arrays must not become zero");
assert.equal(arrayLengthOrUndefined([]), 0, "an explicitly present empty array may report zero");
assert.equal(arrayLengthOrUndefined([{}]), 1);
assert.equal(parseNumericArgument("0", "--expect", { integer: true }), 0);
assert.throws(
  () => parseNumericArgument("", "--expect", { integer: true }),
  /invalid numeric argument/u
);
assert.throws(
  () => parseNumericArgument("NaN", "--expect", { integer: true }),
  /invalid numeric argument/u
);

process.stdout.write("golden QA numeric boundary contract passed\n");
