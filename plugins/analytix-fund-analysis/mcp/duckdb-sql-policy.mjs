const MAX_SQL_BYTES = 128 * 1024;

const FORBIDDEN_KEYWORDS = new Set([
  "alter", "attach", "call", "checkpoint", "copy", "create", "delete", "detach",
  "drop", "export", "import", "insert", "install", "load", "merge", "pragma",
  "reset", "set", "truncate", "update", "vacuum"
]);

const SAFE_FUNCTIONS = new Set([
  "abs", "approx_count_distinct", "avg", "bool_and", "bool_or", "ceil", "ceiling",
  "coalesce", "concat", "concat_ws", "contains", "count", "date_diff", "date_part",
  "date_trunc", "dense_rank", "ends_with", "epoch", "first", "first_value", "floor",
  "greatest", "if", "ifnull", "isfinite", "lag", "last", "last_value", "lead", "least", "left",
  "length", "list", "lower", "ltrim", "max", "md5", "min", "nullif", "printf",
  "rank", "regexp_extract", "regexp_extract_all", "regexp_full_match", "regexp_matches",
  "regexp_replace", "replace", "right", "round", "row_number", "rtrim", "sha256",
  "split_part", "starts_with", "strftime", "strptime", "string_agg", "substr", "sum",
  "typeof", "upper"
]);

const SQL_SYNTAX_CALLS = new Set([
  "and", "as", "by", "case", "cast", "cube", "decimal", "else", "end", "except", "exists",
  "extract", "filter", "from", "group", "grouping_sets", "having", "in", "intersect",
  "interval", "join", "limit", "not", "numeric", "on", "or", "order", "over", "position",
  "qualify", "rollup", "select", "substring", "then", "time", "timestamp", "trim",
  "try_cast", "union", "values", "varchar", "when", "where", "with", "within"
]);

const RELATION_CLAUSE_END = new Set([
  "except", "group", "having", "intersect", "limit", "offset", "order", "qualify",
  "returning", "union", "where", "window"
]);

function policyError(message) {
  const error = new Error(`Local DuckDB workbench SQL policy rejected input: ${message}`);
  error.code = "DUCKDB_SQL_POLICY_REJECTED";
  return error;
}

function isWordStart(character) {
  return /[A-Za-z_]/u.test(character);
}

function isWordPart(character) {
  return /[A-Za-z0-9_]/u.test(character);
}

function scanQuoted(source, start, quote, escapeBackslash = false) {
  let cursor = start + 1;
  while (cursor < source.length) {
    if (escapeBackslash && source[cursor] === "\\") {
      cursor += 2;
      continue;
    }
    if (source[cursor] !== quote) {
      cursor += 1;
      continue;
    }
    if (source[cursor + 1] === quote) {
      cursor += 2;
      continue;
    }
    return cursor + 1;
  }
  throw policyError(quote === "'" ? "unterminated string literal" : "unterminated quoted identifier");
}

function scanDuckdbSql(sql, { allowEmpty = false } = {}) {
  const source = String(sql ?? "");
  if (!source.trim() && !allowEmpty) throw policyError("SQL is empty");
  if (Buffer.byteLength(source, "utf8") > MAX_SQL_BYTES) throw policyError("SQL exceeds the byte limit");

  const tokens = [];
  const withoutComments = [...source];
  let cursor = 0;
  while (cursor < source.length) {
    const character = source[cursor];
    if (/\s/u.test(character)) {
      cursor += 1;
      continue;
    }
    if (character === "-" && source[cursor + 1] === "-") {
      let end = cursor + 2;
      while (end < source.length && source[end] !== "\n" && source[end] !== "\r") end += 1;
      for (let index = cursor; index < end; index += 1) withoutComments[index] = " ";
      cursor = end;
      continue;
    }
    if (character === "/" && source[cursor + 1] === "*") {
      const start = cursor;
      let depth = 1;
      cursor += 2;
      while (cursor < source.length && depth > 0) {
        if (source[cursor] === "/" && source[cursor + 1] === "*") {
          depth += 1;
          cursor += 2;
        } else if (source[cursor] === "*" && source[cursor + 1] === "/") {
          depth -= 1;
          cursor += 2;
        } else {
          cursor += 1;
        }
      }
      if (depth !== 0) throw policyError("unterminated block comment");
      for (let index = start; index < cursor; index += 1) {
        if (withoutComments[index] !== "\n" && withoutComments[index] !== "\r") withoutComments[index] = " ";
      }
      continue;
    }
    if (character === "'") {
      const previous = tokens.at(-1);
      const escapeBackslash = previous?.kind === "word" && previous.lower === "e" && previous.end === cursor;
      const end = scanQuoted(source, cursor, "'", escapeBackslash);
      tokens.push({ kind: "string", value: source.slice(cursor, end), lower: "", start: cursor, end });
      cursor = end;
      continue;
    }
    if (character === '"') {
      const end = scanQuoted(source, cursor, '"');
      const value = source.slice(cursor + 1, end - 1).replace(/""/gu, '"');
      tokens.push({ kind: "identifier", value, lower: value.toLowerCase(), quoted: true, start: cursor, end });
      cursor = end;
      continue;
    }
    if (isWordStart(character)) {
      let end = cursor + 1;
      while (end < source.length && isWordPart(source[end])) end += 1;
      const value = source.slice(cursor, end);
      tokens.push({ kind: "word", value, lower: value.toLowerCase(), quoted: false, start: cursor, end });
      cursor = end;
      continue;
    }
    if (/[0-9]/u.test(character)) {
      let end = cursor + 1;
      while (end < source.length && /[0-9A-Fa-f_xX.eE+-]/u.test(source[end])) end += 1;
      tokens.push({ kind: "number", value: source.slice(cursor, end), lower: "", start: cursor, end });
      cursor = end;
      continue;
    }
    const two = source.slice(cursor, cursor + 2);
    if (["::", "<=", ">=", "<>", "!=", "||", "&&", "->"].includes(two)) {
      tokens.push({ kind: "symbol", value: two, lower: two, start: cursor, end: cursor + 2 });
      cursor += 2;
      continue;
    }
    if ("(),.;*+-/%=<>!|&^~".includes(character)) {
      tokens.push({ kind: "symbol", value: character, lower: character, start: cursor, end: cursor + 1 });
      cursor += 1;
      continue;
    }
    throw policyError(`unsupported token at byte ${Buffer.byteLength(source.slice(0, cursor), "utf8")}`);
  }
  return { source, tokens, withoutComments: withoutComments.join("") };
}

function annotateFrames(tokens) {
  const frames = [{ id: 0, parent: -1 }];
  const stack = [0];
  const pairs = new Map();
  const opens = [];
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    token.frame = stack.at(-1);
    if (token.value === "(") {
      const nextFrame = frames.length;
      frames.push({ id: nextFrame, parent: stack.at(-1) });
      token.childFrame = nextFrame;
      opens.push(index);
      stack.push(nextFrame);
    } else if (token.value === ")") {
      if (stack.length === 1 || opens.length === 0) throw policyError("unbalanced closing parenthesis");
      const open = opens.pop();
      const closingFrame = stack.pop();
      token.frame = closingFrame;
      pairs.set(open, index);
      pairs.set(index, open);
    }
  }
  if (stack.length !== 1) throw policyError("unbalanced opening parenthesis");
  return { pairs };
}

function identifierToken(token) {
  return token?.kind === "word" || token?.kind === "identifier";
}

function parseCTEs(tokens, pairs) {
  const names = new Set();
  if (tokens[0]?.lower !== "with") return { names, mainIndex: 0 };
  let index = 1;
  if (tokens[index]?.lower === "recursive") index += 1;
  while (index < tokens.length) {
    const name = tokens[index];
    if (!identifierToken(name)) throw policyError("WITH requires a CTE identifier");
    if (names.has(name.lower)) throw policyError("duplicate CTE identifier");
    names.add(name.lower);
    index += 1;
    if (tokens[index]?.value === "(") {
      const close = pairs.get(index);
      if (close === undefined) throw policyError("invalid CTE column list");
      index = close + 1;
    }
    if (tokens[index]?.lower !== "as") throw policyError("CTE must use AS (...)");
    index += 1;
    if (tokens[index]?.lower === "not" && tokens[index + 1]?.lower === "materialized") index += 2;
    else if (tokens[index]?.lower === "materialized") index += 1;
    if (tokens[index]?.value !== "(") throw policyError("CTE body must be parenthesized");
    const close = pairs.get(index);
    if (close === undefined) throw policyError("invalid CTE body");
    index = close + 1;
    if (tokens[index]?.value !== ",") break;
    index += 1;
  }
  if (tokens[index]?.lower !== "select") throw policyError("WITH statement must terminate in SELECT");
  return { names, mainIndex: index };
}

function parseRelation(tokens, start, ctes) {
  let index = start;
  if (tokens[index]?.lower === "lateral") index += 1;
  if (tokens[index]?.value === "(") return { table: "", end: index };
  if (!identifierToken(tokens[index])) throw policyError("FROM/JOIN requires a concrete relation");
  const parts = [tokens[index].lower];
  index += 1;
  while (tokens[index]?.value === ".") {
    if (!identifierToken(tokens[index + 1])) throw policyError("invalid qualified relation name");
    parts.push(tokens[index + 1].lower);
    index += 2;
  }
  if (tokens[index]?.value === "(") throw policyError(`table function ${parts.join(".")} is not allowed`);
  if (parts.length > 2 || (parts.length === 2 && parts[0] !== "main")) {
    throw policyError(`relation ${parts.join(".")} is outside the main schema`);
  }
  const table = parts.at(-1);
  if (!ctes.has(table) && !isAllowedDuckdbWorkbenchTable(table)) {
    throw policyError(`relation ${parts.join(".")} is outside cleaned/analysis scope`);
  }
  return { table: ctes.has(table) ? "" : table, end: index - 1 };
}

function analyzeRelations(tokens, ctes) {
  const selectFrames = new Set();
  const fromFrames = new Set();
  const baseTables = new Set();
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token.kind === "word" && token.lower === "select") selectFrames.add(token.frame);
    if (token.kind === "word" && RELATION_CLAUSE_END.has(token.lower)) fromFrames.delete(token.frame);
    let relationStart = -1;
    if (token.kind === "word" && token.lower === "from" && selectFrames.has(token.frame)) {
      fromFrames.add(token.frame);
      relationStart = index + 1;
    } else if (token.kind === "word" && token.lower === "join" && fromFrames.has(token.frame)) {
      relationStart = index + 1;
    } else if (token.value === "," && fromFrames.has(token.frame)) {
      relationStart = index + 1;
    }
    if (relationStart < 0) continue;
    const relation = parseRelation(tokens, relationStart, ctes);
    if (relation.table) baseTables.add(relation.table);
  }
  if (baseTables.size === 0) {
    throw policyError("at least one cleaned fc_*_norm or approved analysis_* relation is required");
  }
  return [...baseTables].sort();
}

function analyzeFunctions(tokens) {
  const functions = new Set();
  for (let index = 0; index < tokens.length - 1; index += 1) {
    const token = tokens[index];
    if (!identifierToken(token) || tokens[index + 1]?.value !== "(") continue;
    if (token.quoted || tokens[index - 1]?.value === ".") {
      throw policyError(`qualified or quoted function ${token.value} is not allowed`);
    }
    if (SQL_SYNTAX_CALLS.has(token.lower)) continue;
    if (!SAFE_FUNCTIONS.has(token.lower)) throw policyError(`function ${token.value} is not allowlisted`);
    functions.add(token.lower);
  }
  return [...functions].sort();
}

export function isAllowedDuckdbWorkbenchTable(tableName) {
  const table = String(tableName ?? "").trim().toLowerCase();
  return /^analysis_[a-z0-9_]*$/u.test(table) || /^fc_[a-z0-9_]*_norm$/u.test(table);
}

export function stripDuckdbSqlComments(sql) {
  return scanDuckdbSql(sql, { allowEmpty: true }).withoutComments;
}

export function analyzeDuckdbReadOnlySql(sql) {
  const scanned = scanDuckdbSql(sql);
  const tokens = scanned.tokens.slice();
  if (tokens.at(-1)?.value === ";") tokens.pop();
  if (tokens.some((token) => token.value === ";")) throw policyError("multiple statements are not allowed");
  if (tokens.length === 0) throw policyError("SQL is empty");
  const { pairs } = annotateFrames(tokens);
  const { names: ctes, mainIndex } = parseCTEs(tokens, pairs);
  if (tokens[mainIndex]?.lower !== "select") throw policyError("only SELECT/CTE statements are allowed");
  for (const token of tokens) {
    if (token.kind === "word" && FORBIDDEN_KEYWORDS.has(token.lower)) {
      throw policyError(`keyword ${token.value} is not allowed`);
    }
  }
  const functions = analyzeFunctions(tokens);
  const baseTables = analyzeRelations(tokens, ctes);
  return {
    policy_engine: "tokenized_duckdb_readonly_scope_guard_v3",
    statement_class: tokens[0]?.lower === "with" ? "with_select" : "select",
    readonly_start: true,
    multiple_statements_blocked: true,
    external_access_blocked: true,
    allowed_view_policy: "cleaned_and_analysis_only",
    allowed_table_pattern: "analysis_* or fc_*_norm",
    base_tables: baseTables,
    base_table_count: baseTables.length,
    function_allowlist_enforced: true,
    functions
  };
}

export const DUCKDB_SQL_POLICY_LIMITS = Object.freeze({ maxSqlBytes: MAX_SQL_BYTES });
