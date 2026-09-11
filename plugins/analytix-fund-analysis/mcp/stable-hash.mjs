import crypto from "node:crypto";

export const STABLE_HASH_VERSION = "1.0.0";

export function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}
