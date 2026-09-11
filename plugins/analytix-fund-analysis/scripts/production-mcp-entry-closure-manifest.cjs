const fs = require('node:fs')

const PRODUCTION_MCP_FILE_PATTERN = /^mcp\/[A-Za-z0-9][A-Za-z0-9._-]*\.mjs$/u
const CONTRACT_KEYS = Object.freeze(['schemaVersion', 'contract', 'entry', 'files'])

function validateProductionMcpEntryClosureContract(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value).sort()
  const expectedKeys = [...CONTRACT_KEYS].sort()
  return keys.length === expectedKeys.length &&
    keys.every((key, index) => key === expectedKeys[index]) &&
    value.schemaVersion === 1 &&
    value.contract === 'ProductionMcpEntryClosureV1' &&
    value.entry === 'mcp/server.mjs' &&
    Array.isArray(value.files) &&
    value.files.length > 0 &&
    new Set(value.files).size === value.files.length &&
    value.files.includes(value.entry) &&
    value.files.every((relativePath) => (
      typeof relativePath === 'string' &&
      PRODUCTION_MCP_FILE_PATTERN.test(relativePath)
    ))
}

function loadProductionMcpEntryClosureContract(contractPath) {
  const stat = fs.lstatSync(contractPath)
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size <= 0) {
    throw new Error(`ProductionMcpEntryClosureV1 is not a regular source file: ${contractPath}`)
  }
  let contract
  try {
    contract = JSON.parse(fs.readFileSync(contractPath, 'utf8'))
  } catch {
    throw new Error(`ProductionMcpEntryClosureV1 is not valid JSON: ${contractPath}`)
  }
  if (!validateProductionMcpEntryClosureContract(contract)) {
    throw new Error(`Invalid ProductionMcpEntryClosureV1 contract: ${contractPath}`)
  }
  return Object.freeze({
    schemaVersion: contract.schemaVersion,
    contract: contract.contract,
    entry: contract.entry,
    files: Object.freeze([...contract.files])
  })
}

module.exports = {
  loadProductionMcpEntryClosureContract,
  validateProductionMcpEntryClosureContract
}
