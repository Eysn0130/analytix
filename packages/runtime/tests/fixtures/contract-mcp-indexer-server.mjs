import { readFile, writeFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'
import { z } from 'zod'

const statePath = process.env.ANALYTIX_CONTRACT_INDEXER_STATE || ''
const secret = process.env.ANALYTIX_CONTRACT_INDEXER_SECRET || ''

async function readState() {
  if (!statePath || !existsSync(statePath)) return { active: {}, tombstones: [], attempts: 0 }
  try {
    return JSON.parse(await readFile(statePath, 'utf8'))
  } catch {
    return { active: {}, tombstones: [], attempts: 0 }
  }
}

async function writeState(state) {
  if (!statePath) return
  await writeFile(statePath, JSON.stringify(state, null, 2), 'utf8')
}

async function updateState(mutator) {
  const state = await readState()
  await mutator(state)
  await writeState(state)
  return state
}

function text(payload) {
  return {
    content: [{ type: 'text', text: JSON.stringify(payload) }]
  }
}

const state = await updateState(async (current) => {
  current.attempts = (current.attempts || 0) + 1
})
const failUntil = Number(process.env.ANALYTIX_CONTRACT_INDEXER_FAIL_UNTIL_ATTEMPT || '0')
if (state.attempts <= failUntil) {
  console.error(`Authorization: ${secret} contract indexer cold start failed`)
  process.exit(1)
}

const server = new McpServer({ name: 'analytix-contract-indexer', version: '0.1.0' })

server.tool('index_seed', {
  files: z.array(z.object({ path: z.string(), digest: z.string() }))
}, async ({ files }) => {
  const next = await updateState(async (current) => {
    current.active ||= {}
    current.tombstones ||= []
    for (const file of files) {
      current.active[file.path] = file.digest
      current.tombstones = current.tombstones.filter((path) => path !== file.path)
    }
  })
  return text({ activePaths: Object.keys(next.active).sort() })
})

server.tool('index_tombstone', {
  path: z.string()
}, async ({ path }) => {
  const next = await updateState(async (current) => {
    current.active ||= {}
    current.tombstones ||= []
    delete current.active[path]
    if (!current.tombstones.includes(path)) current.tombstones.push(path)
  })
  return text({ activePaths: Object.keys(next.active).sort(), tombstoneCount: next.tombstones.length })
})

server.tool('index_resume', {
  files: z.array(z.object({ path: z.string(), digest: z.string() }))
}, async ({ files }) => {
  const next = await updateState(async (current) => {
    current.active ||= {}
    current.tombstones ||= []
    for (const file of files) {
      if (!current.tombstones.includes(file.path)) current.active[file.path] = file.digest
    }
  })
  return text({ activePaths: Object.keys(next.active).sort(), tombstoneCount: next.tombstones.length })
})

server.tool('index_status', {}, async () => {
  const current = await readState()
  return text({
    cwd: process.cwd(),
    activePaths: Object.keys(current.active || {}).sort(),
    tombstoneCount: (current.tombstones || []).length,
    attempts: current.attempts || 0
  })
})

server.tool('index_fail_diagnostic', {}, async () => {
  throw new Error(`Authorization: ${secret} contract indexer diagnostic`)
})

await server.connect(new StdioServerTransport())
