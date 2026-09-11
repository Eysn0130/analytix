export const TOOL_SCHEMA_VALIDATION_VERSION = "0.16.16";

const SUPPORTED_TYPES = new Set(["array", "boolean", "integer", "null", "number", "object", "string"]);
const SUPPORTED_SCHEMA_KEYWORDS = new Set([
  "$id",
  "$schema",
  "additionalProperties",
  "const",
  "description",
  "enum",
  "items",
  "maxItems",
  "maxLength",
  "maximum",
  "maxProperties",
  "minItems",
  "minLength",
  "minimum",
  "minProperties",
  "oneOf",
  "pattern",
  "properties",
  "required",
  "title",
  "type"
]);
const ANNOTATION_KEYWORDS = ["$id", "$schema", "description", "title"];

function hasOwn(value, key) {
  return Object.hasOwn(value, key);
}

function assertJsonValue(value, path, seen = new Set()) {
  if (value === null || typeof value === "string" || typeof value === "boolean") return;
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new Error(`${path} must be a finite JSON number`);
    return;
  }
  if (!value || typeof value !== "object") throw new Error(`${path} must be a JSON value`);
  if (seen.has(value)) throw new Error(`${path} must not contain a cycle`);
  seen.add(value);
  try {
    if (Array.isArray(value)) {
      value.forEach((entry, index) => assertJsonValue(entry, `${path}[${index}]`, seen));
      return;
    }
    for (const [key, entry] of Object.entries(value)) {
      assertJsonValue(entry, `${path}.${key}`, seen);
    }
  } finally {
    seen.delete(value);
  }
}

function jsonValueFingerprint(value) {
  if (value === null) return "null";
  if (typeof value === "string") return `string:${JSON.stringify(value)}`;
  if (typeof value === "boolean") return `boolean:${value}`;
  if (typeof value === "number") return `number:${String(value)}`;
  if (Array.isArray(value)) return `array:[${value.map(jsonValueFingerprint).join(",")}]`;
  const entries = Object.entries(value).sort(([left], [right]) => left.localeCompare(right));
  return `object:{${entries.map(([key, entry]) => `${JSON.stringify(key)}:${jsonValueFingerprint(entry)}`).join(",")}}`;
}

function jsonValuesEqual(left, right) {
  try {
    assertJsonValue(left, "$value");
    assertJsonValue(right, "$schemaValue");
    return jsonValueFingerprint(left) === jsonValueFingerprint(right);
  } catch {
    return false;
  }
}

function nonNegativeIntegerKeyword(schema, keyword, path) {
  if (!hasOwn(schema, keyword)) return undefined;
  const value = schema[keyword];
  if (!Number.isInteger(value) || value < 0) {
    throw new Error(`${path}.${keyword} must be a non-negative integer`);
  }
  return value;
}

function finiteNumberKeyword(schema, keyword, path) {
  if (!hasOwn(schema, keyword)) return undefined;
  const value = schema[keyword];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new Error(`${path}.${keyword} must be a finite number`);
  }
  return value;
}

function rejectKeywordsForOtherTypes(schema, type, path) {
  const owners = {
    additionalProperties: "object",
    items: "array",
    maxItems: "array",
    maxLength: "string",
    maximum: "number",
    maxProperties: "object",
    minItems: "array",
    minLength: "string",
    minimum: "number",
    minProperties: "object",
    pattern: "string",
    properties: "object",
    required: "object"
  };
  for (const [keyword, owner] of Object.entries(owners)) {
    if (!hasOwn(schema, keyword)) continue;
    const compatible = owner === type || (owner === "number" && type === "integer");
    if (!compatible) throw new Error(`${path}.${keyword} is not valid for schema type ${type || "oneOf"}`);
  }
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function valueType(value) {
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  if (Number.isInteger(value)) return "integer";
  return typeof value;
}

export function normalizeExternalSchema(schema, path = "$schema", seen = new Set()) {
  if (!schema || typeof schema !== "object" || Array.isArray(schema)) {
    throw new Error(`${path} must be an object schema`);
  }
  if (seen.has(schema)) throw new Error(`${path} must not contain a schema cycle`);
  seen.add(schema);
  try {
    for (const keyword of Object.keys(schema)) {
      if (!SUPPORTED_SCHEMA_KEYWORDS.has(keyword)) {
        throw new Error(`${path}.${keyword} is not a supported schema keyword`);
      }
    }
    for (const keyword of ANNOTATION_KEYWORDS) {
      const annotation = schema[keyword];
      if (hasOwn(schema, keyword) && (typeof annotation !== "string" || !annotation.trim())) {
        throw new Error(`${path}.${keyword} must be a non-empty string`);
      }
    }

    let type = "";
    if (hasOwn(schema, "type")) {
      if (typeof schema.type !== "string" || !schema.type.trim()) {
        throw new Error(`${path}.type must be a supported non-empty string`);
      }
      type = schema.type.trim();
      if (!SUPPORTED_TYPES.has(type)) throw new Error(`${path}.type is not supported: ${type}`);
    }
    let variants = [];
    if (hasOwn(schema, "oneOf")) {
      if (!Array.isArray(schema.oneOf) || !schema.oneOf.length) {
        throw new Error(`${path}.oneOf must be a non-empty array`);
      }
      variants = schema.oneOf;
    }
    if (!type && !variants.length) throw new Error(`${path}.type or .oneOf is required`);
    rejectKeywordsForOtherTypes(schema, type, path);

    const normalized = { ...schema };
    if (type) normalized.type = type;
    if (variants.length) {
      normalized.oneOf = variants.map((variant, index) => (
        normalizeExternalSchema(variant, `${path}.oneOf[${index}]`, seen)
      ));
    }
    if (hasOwn(schema, "const")) assertJsonValue(schema.const, `${path}.const`);
    if (hasOwn(schema, "enum")) {
      if (!Array.isArray(schema.enum) || !schema.enum.length) {
        throw new Error(`${path}.enum must be a non-empty array`);
      }
      const fingerprints = new Set();
      schema.enum.forEach((entry, index) => {
        assertJsonValue(entry, `${path}.enum[${index}]`);
        const fingerprint = jsonValueFingerprint(entry);
        if (fingerprints.has(fingerprint)) throw new Error(`${path}.enum values must be unique`);
        fingerprints.add(fingerprint);
      });
    }
    if (type === "object") {
      const properties = hasOwn(schema, "properties") ? schema.properties : {};
      if (!properties || typeof properties !== "object" || Array.isArray(properties)) {
        throw new Error(`${path}.properties must be an object`);
      }
      if (schema.additionalProperties !== false) {
        throw new Error(`${path}.additionalProperties must be false`);
      }
      normalized.properties = Object.fromEntries(
        Object.entries(properties).map(([key, child]) => [
          key,
          normalizeExternalSchema(child, `${path}.properties.${key}`, seen)
        ])
      );
      normalized.additionalProperties = false;
      if (hasOwn(schema, "required")) {
        if (!Array.isArray(schema.required) || schema.required.some((key) => typeof key !== "string" || !key)) {
          throw new Error(`${path}.required must contain non-empty strings`);
        }
        for (const key of schema.required) {
          if (!Object.hasOwn(normalized.properties, key)) {
            throw new Error(`${path}.required references unknown property ${key}`);
          }
        }
        if (new Set(schema.required).size !== schema.required.length) {
          throw new Error(`${path}.required must not contain duplicate properties`);
        }
        normalized.required = [...schema.required];
      }
      const minProperties = nonNegativeIntegerKeyword(schema, "minProperties", path);
      const maxProperties = nonNegativeIntegerKeyword(schema, "maxProperties", path);
      if (minProperties !== undefined && maxProperties !== undefined && minProperties > maxProperties) {
        throw new Error(`${path}.minProperties must not exceed maxProperties`);
      }
    } else if (type === "array") {
      if (!hasOwn(schema, "items")) throw new Error(`${path}.items is required`);
      normalized.items = normalizeExternalSchema(schema.items, `${path}.items`, seen);
      const minItems = nonNegativeIntegerKeyword(schema, "minItems", path);
      const maxItems = nonNegativeIntegerKeyword(schema, "maxItems", path);
      if (minItems !== undefined && maxItems !== undefined && minItems > maxItems) {
        throw new Error(`${path}.minItems must not exceed maxItems`);
      }
    } else if (type === "string") {
      const minLength = nonNegativeIntegerKeyword(schema, "minLength", path);
      const maxLength = nonNegativeIntegerKeyword(schema, "maxLength", path);
      if (minLength !== undefined && maxLength !== undefined && minLength > maxLength) {
        throw new Error(`${path}.minLength must not exceed maxLength`);
      }
      if (hasOwn(schema, "pattern")) {
        if (typeof schema.pattern !== "string") throw new Error(`${path}.pattern must be a string`);
        try {
          new RegExp(schema.pattern, "u");
        } catch {
          throw new Error(`${path}.pattern must be a valid Unicode regular expression`);
        }
      }
    } else if (type === "number" || type === "integer") {
      const minimum = finiteNumberKeyword(schema, "minimum", path);
      const maximum = finiteNumberKeyword(schema, "maximum", path);
      if (minimum !== undefined && maximum !== undefined && minimum > maximum) {
        throw new Error(`${path}.minimum must not exceed maximum`);
      }
    }
    return normalized;
  } finally {
    seen.delete(schema);
  }
}

function validateSchema(value, schema, path, failures) {
  const source = objectOf(schema);
  if (Array.isArray(source.oneOf)) {
    const matches = source.oneOf.filter((variant) => {
      const variantFailures = [];
      validateSchema(value, variant, path, variantFailures);
      return variantFailures.length === 0;
    }).length;
    if (matches !== 1) {
      failures.push(`${path} must match exactly one declared schema variant`);
      return;
    }
  }
  const expectedType = typeof source.type === "string" ? source.type.trim() : "";
  const actualType = valueType(value);
  if (expectedType === "number" && typeof value !== "number") failures.push(`${path} must be number`);
  else if (expectedType === "integer" && !Number.isInteger(value)) failures.push(`${path} must be integer`);
  else if (expectedType && !["number", "integer"].includes(expectedType) && actualType !== expectedType) {
    failures.push(`${path} must be ${expectedType}`);
  }
  if (failures.length) return;
  if (Object.hasOwn(source, "const") && !jsonValuesEqual(value, source.const)) {
    failures.push(`${path} must equal the declared constant`);
  }
  if (Array.isArray(source.enum) && !source.enum.some((entry) => jsonValuesEqual(entry, value))) {
    failures.push(`${path} must be one of the declared enum values`);
  }
  if (typeof value === "string") {
    const stringLength = [...value].length;
    if (Number.isInteger(source.minLength) && stringLength < source.minLength) failures.push(`${path} is shorter than minLength`);
    if (Number.isInteger(source.maxLength) && stringLength > source.maxLength) failures.push(`${path} is longer than maxLength`);
    if (typeof source.pattern === "string") {
      try {
        if (!(new RegExp(source.pattern, "u")).test(value)) failures.push(`${path} does not match pattern`);
      } catch {
        failures.push(`${path} schema pattern is invalid`);
      }
    }
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) failures.push(`${path} must be finite`);
    if (typeof source.minimum === "number" && value < source.minimum) failures.push(`${path} is below minimum`);
    if (typeof source.maximum === "number" && value > source.maximum) failures.push(`${path} is above maximum`);
  }
  if (Array.isArray(value)) {
    if (Number.isInteger(source.minItems) && value.length < source.minItems) failures.push(`${path} has too few items`);
    if (Number.isInteger(source.maxItems) && value.length > source.maxItems) failures.push(`${path} has too many items`);
    if (source.items) value.forEach((entry, index) => validateSchema(entry, source.items, `${path}[${index}]`, failures));
  }
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const properties = objectOf(source.properties);
    const required = Array.isArray(source.required) ? source.required : [];
    const propertyCount = Object.keys(value).length;
    if (Number.isInteger(source.minProperties) && propertyCount < source.minProperties) {
      failures.push(`${path} has too few properties`);
    }
    if (Number.isInteger(source.maxProperties) && propertyCount > source.maxProperties) {
      failures.push(`${path} has too many properties`);
    }
    for (const key of required) {
      if (!Object.hasOwn(value, key)) failures.push(`${path}.${key} is required`);
    }
    if (source.additionalProperties === false) {
      for (const key of Object.keys(value)) {
        if (!Object.hasOwn(properties, key)) failures.push(`${path}.${key} is not allowed`);
      }
    }
    for (const [key, entry] of Object.entries(value)) {
      if (properties[key]) validateSchema(entry, properties[key], `${path}.${key}`, failures);
    }
  }
}

export function schemaValidationFailures(value, schema, path = "$") {
  const normalized = normalizeExternalSchema(schema);
  const failures = [];
  validateSchema(value, normalized, path, failures);
  return failures;
}

export function assertToolCatalogSchemas(tools, outputContractByName) {
  if (!Array.isArray(tools) || !tools.length) throw new Error("tool catalog must not be empty");
  const names = new Set();
  for (const tool of tools) {
    const name = typeof tool?.name === "string" ? tool.name.trim() : "";
    if (!name) throw new Error("tool catalog contains a tool without a name");
    if (names.has(name)) throw new Error(`tool catalog contains duplicate tool ${name}`);
    names.add(name);
    normalizeExternalSchema(tool.inputSchema, `${name}.inputSchema`);
    normalizeExternalSchema(tool.outputSchema, `${name}.outputSchema`);
    if (!outputContractByName || typeof outputContractByName[name] !== "string") {
      throw new Error(`tool ${name} has no explicit output contract mapping`);
    }
  }
  const staleMappings = Object.keys(outputContractByName || {}).filter((name) => !names.has(name));
  if (staleMappings.length) throw new Error(`output contract mappings have no tool: ${staleMappings.join(", ")}`);
  return Object.freeze({ toolCount: names.size, inputSchemaCount: names.size, outputSchemaCount: names.size });
}
