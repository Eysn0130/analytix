#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { parse } from "acorn";
import closureManifestContract from "./production-mcp-entry-closure-manifest.cjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const CONTRACT_PATH = path.join(SCRIPT_DIR, "production-mcp-entry-closure.json");
const CONTRACT = closureManifestContract.loadProductionMcpEntryClosureContract(CONTRACT_PATH);

export function validProductionMcpEntryClosureContract(value) {
  return closureManifestContract.validateProductionMcpEntryClosureContract(value);
}

export const PRODUCTION_MCP_ENTRY_CLOSURE_FILES = Object.freeze([...CONTRACT.files]);

const FORBIDDEN_RUNTIME_IDENTIFIERS = new Set([
  "require",
  "eval",
  "Function",
  "globalThis",
  "module",
  "global",
  "WebAssembly",
  "Worker",
  "SharedWorker",
  "Reflect",
  "Proxy",
  "fetch",
  "XMLHttpRequest",
  "WebSocket",
  "EventSource"
]);
const FORBIDDEN_RUNTIME_MEMBER_NAMES = new Set([
  "require",
  "eval",
  "Function",
  "constructor",
  "__proto__",
  "prototype",
  "createRequire",
  "getBuiltinModule",
  "mainModule",
  "binding",
  "dlopen",
  "compileFunction",
  "runInThisContext",
  "runInNewContext"
]);
const BUILTIN_IMPORT_POLICIES = Object.freeze({
  "mcp/jsonrpc-stdio-runtime.mjs": Object.freeze({
    "node:util": Object.freeze(["named:TextDecoder:TextDecoder"])
  }),
  "mcp/mcp-request-handler-runtime.mjs": Object.freeze({
    "node:crypto": Object.freeze(["default:nodeCrypto"])
  })
});
const ALLOWED_NODE_BUILTIN_SPECIFIERS = new Set(
  Object.values(BUILTIN_IMPORT_POLICIES).flatMap((policy) => Object.keys(policy))
);
const OBJECT_CAPABILITY_ALLOWLIST = new Set(["entries", "freeze", "fromEntries", "hasOwn", "is", "keys"]);
const PROCESS_CAPABILITY_ALLOWLIST = new Set(["stdin", "stdout"]);
const BUFFER_CAPABILITY_ALLOWLIST = new Set(["alloc", "byteLength", "concat", "from", "isBuffer"]);

function staticPropertyName(node) {
  if (!node || typeof node !== "object") return "";
  if (node.type === "Identifier") return node.name;
  if (node.type === "Literal" && (typeof node.value === "string" || typeof node.value === "number")) {
    return String(node.value);
  }
  if (node.type === "BinaryExpression" && node.operator === "+") {
    const left = staticPropertyName(node.left);
    const right = staticPropertyName(node.right);
    return left && right ? `${left}${right}` : "";
  }
  if (node.type === "TemplateLiteral") {
    let value = "";
    for (let index = 0; index < node.quasis.length; index += 1) {
      value += node.quasis[index].value.cooked ?? node.quasis[index].value.raw;
      if (index < node.expressions.length) {
        const expression = staticPropertyName(node.expressions[index]);
        if (!expression) return "";
        value += expression;
      }
    }
    return value;
  }
  return "";
}

function staticComputedPropertyName(node) {
  if (!node || typeof node !== "object") return "";
  if (node.type === "Literal" && (typeof node.value === "string" || typeof node.value === "number")) {
    return String(node.value);
  }
  if (node.type === "BinaryExpression" && node.operator === "+") {
    const left = staticComputedPropertyName(node.left);
    const right = staticComputedPropertyName(node.right);
    return left && right ? `${left}${right}` : "";
  }
  if (node.type === "TemplateLiteral") {
    let value = "";
    for (let index = 0; index < node.quasis.length; index += 1) {
      value += node.quasis[index].value.cooked ?? node.quasis[index].value.raw;
      if (index < node.expressions.length) {
        const expression = staticComputedPropertyName(node.expressions[index]);
        if (!expression) return "";
        value += expression;
      }
    }
    return value;
  }
  return "";
}

function memberPropertyName(node) {
  return node.computed ? staticComputedPropertyName(node.property) : staticPropertyName(node.property);
}

function containsUnresolvedComputedMember(node) {
  if (!node || typeof node !== "object") return false;
  if (node.type === "MemberExpression" && node.computed && !staticComputedPropertyName(node.property)) return true;
  for (const [key, value] of Object.entries(node)) {
    if (key === "start" || key === "end" || key === "loc") continue;
    if (Array.isArray(value) && value.some(containsUnresolvedComputedMember)) return true;
    if (value && typeof value === "object" && typeof value.type === "string" && containsUnresolvedComputedMember(value)) {
      return true;
    }
  }
  return false;
}

function callTargetContainsUnresolvedComputedMember(node) {
  if (!node || typeof node !== "object") return false;
  if (node.type === "ChainExpression") return callTargetContainsUnresolvedComputedMember(node.expression);
  if (node.type === "MemberExpression") {
    return (node.computed && !staticComputedPropertyName(node.property)) ||
      callTargetContainsUnresolvedComputedMember(node.object);
  }
  if (node.type === "CallExpression" || node.type === "NewExpression") {
    return callTargetContainsUnresolvedComputedMember(node.callee);
  }
  if (node.type === "TaggedTemplateExpression") {
    return callTargetContainsUnresolvedComputedMember(node.tag);
  }
  return false;
}

function collectDynamicMemberBindings(program) {
  const bindings = [];
  function collect(node) {
    if (!node || typeof node !== "object") return;
    if (node.type === "VariableDeclarator" && node.id?.type === "Identifier" && node.init) {
      bindings.push({ name: node.id.name, value: node.init });
    }
    if (node.type === "AssignmentExpression" && node.operator === "=" && node.left?.type === "Identifier") {
      bindings.push({ name: node.left.name, value: node.right });
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === "start" || key === "end" || key === "loc") continue;
      if (Array.isArray(value)) value.forEach(collect);
      else if (value && typeof value === "object" && typeof value.type === "string") collect(value);
    }
  }
  collect(program);
  const tainted = new Set(bindings.filter(({ value }) => containsUnresolvedComputedMember(value)).map(({ name }) => name));
  let changed = true;
  while (changed) {
    changed = false;
    for (const { name, value } of bindings) {
      if (tainted.has(name) || value?.type !== "Identifier" || !tainted.has(value.name)) continue;
      tainted.add(name);
      changed = true;
    }
  }
  return tainted;
}

function callTargetUsesDynamicBinding(node, bindings) {
  if (!node || typeof node !== "object") return false;
  if (node.type === "Identifier") return bindings.has(node.name);
  if (node.type === "ChainExpression") return callTargetUsesDynamicBinding(node.expression, bindings);
  if (node.type === "MemberExpression" && !node.computed) {
    const propertyName = staticPropertyName(node.property);
    return ["apply", "bind", "call"].includes(propertyName) && callTargetUsesDynamicBinding(node.object, bindings);
  }
  if (node.type === "CallExpression" || node.type === "NewExpression") {
    return callTargetUsesDynamicBinding(node.callee, bindings);
  }
  return false;
}

function builtinImportShape(node) {
  return node.specifiers.map((specifier) => {
    if (specifier.type === "ImportDefaultSpecifier") return `default:${specifier.local.name}`;
    if (specifier.type === "ImportSpecifier") {
      const imported = specifier.imported.type === "Identifier" ? specifier.imported.name : String(specifier.imported.value);
      return `named:${imported}:${specifier.local.name}`;
    }
    return `forbidden:${specifier.type}`;
  }).sort();
}

function moduleSpecifiers(source, relativePath) {
  let program;
  try {
    program = parse(source, {
      ecmaVersion: "latest",
      sourceType: "module",
      allowAwaitOutsideFunction: false
    });
  } catch (error) {
    throw new Error(`${relativePath} cannot be parsed as a JavaScript module: ${error instanceof Error ? error.message : String(error)}`);
  }
  const specifiers = [];
  const observedBuiltinImports = new Map();
  const dynamicMemberBindings = collectDynamicMemberBindings(program);

  function visit(node, parent = null) {
    if (!node || typeof node !== "object") return;
    if (node.type === "ImportDeclaration" || node.type === "ExportAllDeclaration" || (
      node.type === "ExportNamedDeclaration" && node.source
    )) {
      if (typeof node.source?.value !== "string") {
        throw new Error(`${relativePath} uses a non-literal module specifier`);
      }
      specifiers.push(node.source.value);
      if (node.source.value.startsWith("node:")) {
        if (node.type !== "ImportDeclaration") {
          throw new Error(`${relativePath} re-exports a forbidden Node builtin capability`);
        }
        const expectedShape = BUILTIN_IMPORT_POLICIES[relativePath]?.[node.source.value];
        const actualShape = builtinImportShape(node);
        if (!expectedShape || JSON.stringify(actualShape) !== JSON.stringify([...expectedShape].sort())) {
          throw new Error(`${relativePath} imports forbidden bindings from ${node.source.value}`);
        }
        if (observedBuiltinImports.has(node.source.value)) {
          throw new Error(`${relativePath} imports ${node.source.value} more than once`);
        }
        observedBuiltinImports.set(node.source.value, actualShape);
      }
    }
    if (node.type === "ImportExpression") {
      throw new Error(`${relativePath} uses forbidden dynamic import()`);
    }
    if (node.type === "Identifier" && FORBIDDEN_RUNTIME_IDENTIFIERS.has(node.name)) {
      throw new Error(`${relativePath} references forbidden runtime loader ${node.name}`);
    }
    if (node.type === "Identifier" && node.name === "process") {
      const allowed = parent?.type === "MemberExpression" && parent.object === node && !parent.computed &&
        parent.property?.type === "Identifier" && PROCESS_CAPABILITY_ALLOWLIST.has(parent.property.name);
      if (!allowed) throw new Error(`${relativePath} references forbidden process authority`);
    }
    if (node.type === "Identifier" && node.name === "Object") {
      const allowed = parent?.type === "MemberExpression" && parent.object === node && !parent.computed &&
        parent.property?.type === "Identifier" && OBJECT_CAPABILITY_ALLOWLIST.has(parent.property.name);
      if (!allowed) throw new Error(`${relativePath} references forbidden Object reflection authority`);
    }
    if (node.type === "Identifier" && node.name === "Buffer") {
      const allowed = parent?.type === "MemberExpression" && parent.object === node && !parent.computed &&
        parent.property?.type === "Identifier" && BUFFER_CAPABILITY_ALLOWLIST.has(parent.property.name);
      if (!allowed) throw new Error(`${relativePath} references forbidden Buffer authority`);
    }
    if (node.type === "Identifier" && ["nodeCrypto", "nodePath"].includes(node.name)) {
      const allowedImport = parent?.type === "ImportDefaultSpecifier" && parent.local === node;
      const allowedMember = parent?.type === "MemberExpression" && parent.object === node && !parent.computed;
      if (!allowedImport && !allowedMember) {
        throw new Error(`${relativePath} aliases forbidden builtin authority ${node.name}`);
      }
    }
    if (node.type === "Identifier" && node.name === "TextDecoder") {
      const allowedImport = parent?.type === "ImportSpecifier";
      const allowedConstruction = parent?.type === "NewExpression" && parent.callee === node;
      if (!allowedImport && !allowedConstruction) {
        throw new Error(`${relativePath} aliases forbidden builtin authority TextDecoder`);
      }
    }
    if (node.type === "Property") {
      const propertyName = node.computed ? staticComputedPropertyName(node.key) : staticPropertyName(node.key);
      if (propertyName && FORBIDDEN_RUNTIME_MEMBER_NAMES.has(propertyName)) {
        throw new Error(`${relativePath} binds forbidden runtime property ${propertyName}`);
      }
    }
    if (node.type === "MemberExpression") {
      const propertyName = memberPropertyName(node);
      if (propertyName && FORBIDDEN_RUNTIME_MEMBER_NAMES.has(propertyName)) {
        throw new Error(`${relativePath} references forbidden runtime member ${propertyName}`);
      }
      if (node.computed && node.object?.type === "Identifier" && ["process", "globalThis", "global", "module"].includes(node.object.name)) {
        throw new Error(`${relativePath} uses computed access on forbidden runtime authority ${node.object.name}`);
      }
      if (node.object?.type === "Identifier" && node.object.name === "process" && (
        node.computed || !PROCESS_CAPABILITY_ALLOWLIST.has(propertyName)
      )) {
        throw new Error(`${relativePath} uses forbidden process capability ${propertyName || "computed"}`);
      }
      if (node.object?.type === "Identifier" && node.object.name === "Object" && (
        node.computed || !OBJECT_CAPABILITY_ALLOWLIST.has(propertyName)
      )) {
        throw new Error(`${relativePath} uses forbidden Object capability ${propertyName || "computed"}`);
      }
      if (node.object?.type === "Identifier" && node.object.name === "Buffer") {
        const directCall = parent?.type === "CallExpression" && parent.callee === node;
        if (node.computed || !BUFFER_CAPABILITY_ALLOWLIST.has(propertyName) || !directCall) {
          throw new Error(`${relativePath} uses forbidden Buffer capability ${propertyName || "computed"}`);
        }
      }
      if (node.object?.type === "Identifier" && node.object.name === "nodeCrypto" && (
        node.computed || propertyName !== "createHash"
      )) {
        throw new Error(`${relativePath} uses forbidden node:crypto capability ${propertyName || "computed"}`);
      }
      if (node.object?.type === "Identifier" && node.object.name === "nodePath" && (
        node.computed || propertyName !== "isAbsolute"
      )) {
        throw new Error(`${relativePath} uses forbidden node:path capability ${propertyName || "computed"}`);
      }
      if (node.computed && ["ArrowFunctionExpression", "FunctionExpression", "ClassExpression"].includes(node.object?.type)) {
        throw new Error(`${relativePath} uses computed reflection on executable code`);
      }
      if (node.object?.type === "MetaProperty") {
        if (node.computed || node.property?.type !== "Identifier" || node.property.name !== "url") {
          throw new Error(`${relativePath} uses forbidden import.meta capability`);
        }
      }
    }
    if (["CallExpression", "NewExpression", "TaggedTemplateExpression"].includes(node.type)) {
      const target = node.type === "TaggedTemplateExpression" ? node.tag : node.callee;
      if (callTargetContainsUnresolvedComputedMember(target) || callTargetUsesDynamicBinding(target, dynamicMemberBindings)) {
        throw new Error(`${relativePath} invokes an unresolved computed capability`);
      }
    }
    if (node.type === "MetaProperty") {
      const allowedImportMetaUrl = parent?.type === "MemberExpression" && parent.object === node &&
        parent.computed === false && parent.property?.type === "Identifier" && parent.property.name === "url";
      if (node.meta?.name !== "import" || node.property?.name !== "meta" || !allowedImportMetaUrl) {
        throw new Error(`${relativePath} uses forbidden import.meta capability`);
      }
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === "start" || key === "end" || key === "loc") continue;
      if (Array.isArray(value)) {
        for (const child of value) visit(child, node);
      } else if (value && typeof value === "object" && typeof value.type === "string") {
        visit(value, node);
      }
    }
  }

  visit(program);
  const expectedBuiltinImports = BUILTIN_IMPORT_POLICIES[relativePath] || {};
  if (JSON.stringify([...observedBuiltinImports.keys()].sort()) !== JSON.stringify(Object.keys(expectedBuiltinImports).sort())) {
    throw new Error(`${relativePath} builtin import capability set does not match the production allowlist`);
  }
  return specifiers;
}

function readRegularModule(modulePath) {
  const stat = fs.lstatSync(modulePath);
  if (!stat.isFile() || stat.isSymbolicLink()) {
    throw new Error(`production MCP module is not a regular non-symlink file: ${modulePath}`);
  }
  const checked = spawnSync(process.execPath, ["--check", modulePath], { encoding: "utf8" });
  if (checked.status !== 0) {
    throw new Error(`production MCP module syntax is invalid: ${modulePath}`);
  }
  return fs.readFileSync(modulePath, "utf8");
}

export function inspectProductionMcpEntryClosure(
  pluginRoot = DEFAULT_PLUGIN_ROOT,
  { entry = "mcp/server.mjs", expectedFiles = PRODUCTION_MCP_ENTRY_CLOSURE_FILES } = {}
) {
  const root = fs.realpathSync(path.resolve(pluginRoot));
  const mcpRoot = path.join(root, "mcp");
  const realMcpRoot = fs.realpathSync(mcpRoot);
  const queue = [entry];
  const visited = new Set();
  const edges = [];
  while (queue.length) {
    const relativePath = queue.shift();
    if (visited.has(relativePath)) continue;
    const absolutePath = path.resolve(root, relativePath);
    const realPath = fs.realpathSync(absolutePath);
    if (realPath !== realMcpRoot && !realPath.startsWith(`${realMcpRoot}${path.sep}`)) {
      throw new Error(`production MCP import escapes the mcp root: ${relativePath}`);
    }
    if (path.extname(realPath) !== ".mjs") {
      throw new Error(`production MCP import is not an .mjs module: ${relativePath}`);
    }
    const normalizedRelativePath = path.relative(root, realPath).split(path.sep).join("/");
    if (normalizedRelativePath !== relativePath) {
      throw new Error(`production MCP import changes identity through aliasing: ${relativePath}`);
    }
    visited.add(relativePath);
    const source = readRegularModule(realPath);
    for (const specifier of moduleSpecifiers(source, relativePath)) {
      if (specifier.startsWith("node:")) {
        if (!ALLOWED_NODE_BUILTIN_SPECIFIERS.has(specifier)) {
          throw new Error(`${relativePath} imports a forbidden Node builtin: ${specifier}`);
        }
        continue;
      }
      if (!specifier.startsWith("./") && !specifier.startsWith("../")) {
        throw new Error(`${relativePath} imports a non-builtin external module: ${specifier}`);
      }
      const targetPath = path.resolve(path.dirname(realPath), specifier);
      if (targetPath !== mcpRoot && !targetPath.startsWith(`${mcpRoot}${path.sep}`)) {
        throw new Error(`${relativePath} import escapes the mcp root: ${specifier}`);
      }
      const targetRelativePath = path.relative(root, targetPath).split(path.sep).join("/");
      edges.push({ from: relativePath, to: targetRelativePath });
      queue.push(targetRelativePath);
    }
  }
  const actual = [...visited].sort();
  const expected = [...expectedFiles].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`production MCP entry closure mismatch: actual=${actual.join(",")} expected=${expected.join(",")}`);
  }
  return { entry, files: actual, edges: edges.sort((left, right) => `${left.from}:${left.to}`.localeCompare(`${right.from}:${right.to}`)) };
}

function writeFixture(root, relativePath, source) {
  const target = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, source, "utf8");
}

function assertRejected(source, expectedMessage) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-entry-closure-"));
  try {
    writeFixture(root, "mcp/server.mjs", source);
    let message = "";
    try {
      inspectProductionMcpEntryClosure(root, { expectedFiles: ["mcp/server.mjs"] });
    } catch (error) {
      message = error instanceof Error ? error.message : String(error);
    }
    if (!message.includes(expectedMessage)) {
      throw new Error(`hostile closure fixture was not rejected as ${expectedMessage}: ${message || "accepted"}`);
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

export function runProductionMcpEntryClosureContract(pluginRoot = DEFAULT_PLUGIN_ROOT) {
  const actual = inspectProductionMcpEntryClosure(pluginRoot);
  const hostileLoaderCases = [
    ["await import('./hidden.mjs');\n", "dynamic import"],
    ["const loader = require('./hidden.mjs');\n", "forbidden runtime loader require"],
    ["import remote from 'https://example.invalid/hidden.mjs';\n", "non-builtin external module"],
    ["import { createRequire } from 'node:module';\n", "forbidden bindings from node:module"],
    ["import outside from '../outside.mjs';\n", "escapes the mcp root"],
    ["const loader = Function('return import(\\\"./hidden.mjs\\\")');\n", "forbidden runtime loader Function"],
    ["const indirect = (0, eval); await indirect(\"import('./hidden.mjs')\");\n", "forbidden runtime loader eval"],
    ["const loader = globalThis['Fun' + 'ction']; loader(\"return import('./hidden.mjs')\")();\n", "forbidden runtime member Function"],
    ["const loader = process.getBuiltinModule('node:module').createRequire(import.meta.url);\n", "forbidden runtime member createRequire"],
    ["const name = `ev${'al'}`; const indirect = eval; indirect(name);\n", "forbidden runtime loader eval"],
    ["const loader = (() => {}).constructor;\n", "forbidden runtime member constructor"],
    ["const value = import.meta.resolve('./hidden.mjs');\n", "forbidden import.meta capability"],
    ["const { getBuiltinModule: load } = process; load('node:child_process');\n", "binds forbidden runtime property getBuiltinModule"],
    ["const runtime = process; runtime['get' + 'BuiltinModule']('node:child_process');\n", "forbidden process authority"],
    ["Reflect.get(process, 'get' + 'BuiltinModule')('node:child_process');\n", "forbidden runtime loader Reflect"],
    ["Object.getOwnPropertyDescriptor(process, 'getBuiltinModule').value('node:child_process');\n", "forbidden Object capability getOwnPropertyDescriptor"],
    ["Reflect.get(() => {}, 'con' + 'structor')('return process')();\n", "forbidden runtime loader Reflect"],
    ["await fetch('https://example.invalid/payload.mjs');\n", "forbidden runtime loader fetch"],
    ["import fs from 'node:fs'; fs.writeFileSync('/tmp/escape', 'x');\n", "forbidden bindings from node:fs"],
    ["process.exit(0);\n", "forbidden process capability exit"],
    ["const key = ['con', 'structor'].join(''); const runtime = Buffer[key]('return process')(); runtime.exit(0);\n", "unresolved computed capability"],
    ["const decode = Buffer.from; decode('cmV0dXJuIHByb2Nlc3M=', 'base64');\n", "forbidden Buffer capability"],
    ["const callable = () => {}; const key = ['con', 'structor'].join(''); const runtime = callable[key]; runtime('return process')();\n", "unresolved computed capability"],
    ["const loaders = { safe() {} }; const key = ['re', 'quire'].join(''); loaders[key]('./hidden.mjs');\n", "unresolved computed capability"]
  ];
  for (const [source, expectedMessage] of hostileLoaderCases) assertRejected(source, expectedMessage);
  const hostileContractPaths = [
    "mcp/**/*.mjs",
    "mcp/../escape.mjs",
    "mcp/nested/file.mjs",
    "mcp/.hidden.mjs",
    "mcp/hidden\\\\module.mjs"
  ];
  for (const hostilePath of hostileContractPaths) {
    if (validProductionMcpEntryClosureContract({
      ...CONTRACT,
      files: [CONTRACT.entry, hostilePath]
    })) {
      throw new Error(`hostile production MCP contract path was accepted: ${hostilePath}`);
    }
  }
  return {
    status: "ok",
    contract: "ProductionMcpEntryClosureV1",
    entry: actual.entry,
    files: actual.files,
    edge_count: actual.edges.length,
    hostile_loader_cases_rejected: hostileLoaderCases.length,
    hostile_contract_paths_rejected: hostileContractPaths.length
  };
}

const invokedPath = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
  try {
    const pluginRootIndex = process.argv.indexOf("--plugin-root");
    const pluginRoot = pluginRootIndex >= 0 ? process.argv[pluginRootIndex + 1] : DEFAULT_PLUGIN_ROOT;
    if (!pluginRoot || (pluginRootIndex >= 0 && process.argv.length !== pluginRootIndex + 2)) {
      throw new Error("--plugin-root requires exactly one path");
    }
    console.log(JSON.stringify(runProductionMcpEntryClosureContract(path.resolve(pluginRoot)), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
