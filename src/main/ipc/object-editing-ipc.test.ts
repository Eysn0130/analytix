import { describe, expect, it, vi } from 'vitest'
import { createObjectEditingHandler } from './object-editing-ipc'
import { imageObjectSnapshotSchema, objectEditingPath, objectEditingReceiptSchema } from '../../../packages/runtime/src/contracts/object-editing'

const commit = { action: 'commit', sessionId: 'a'.repeat(48), operationId: 'save_1234', baseRevision: 'b'.repeat(64), content: 'local draft' }

describe('protected object IPC', () => {
  it('rejects caller authority and extra paths before transport', async () => {
    const transport = vi.fn()
    expect(await createObjectEditingHandler(transport)({ ...commit, path: '/other', principal: 'forged' })).toMatchObject({ ok: false, code: 'invalid_request' })
    expect(transport).not.toHaveBeenCalled()
  })

  it('uses only the protected lane and returns a confirmed receipt', async () => {
    const receipt = { operationId: commit.operationId, revision: 'c'.repeat(64), status: 'committed', savedAt: '2026-09-14T04:00:00Z' }
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, receipt }) }))
    expect(await createObjectEditingHandler(transport)(commit)).toEqual({ ok: true, receipt })
    expect(transport).toHaveBeenCalledExactlyOnceWith(objectEditingPath, JSON.stringify(commit))
  })

  it.each(['timeout', 'malformed', 'empty receipt'])('retains operation identity after %s, without returning unsafe errors', async (kind) => {
    const transport = vi.fn(async () => {
      if (kind === 'timeout') throw new Error('private path or server body')
      return { ok: true, status: 200, body: kind === 'malformed' ? 'private path or server body' : JSON.stringify({ ok: true, receipt: { operationId: commit.operationId, revision: '', status: 'committed', savedAt: '' } }) }
    })
    const result = await createObjectEditingHandler(transport)(commit)
    expect(result).toMatchObject({ ok: false, code: 'persistence_failure', receipt: { operationId: commit.operationId, status: 'unknown' } })
    expect(JSON.stringify(result)).not.toContain('private path')
  })

  it('rejects a saved claim without durable revision and time', () => {
    expect(objectEditingReceiptSchema.safeParse({ operationId: 'save_1234', revision: '', status: 'committed', savedAt: '' }).success).toBe(false)
  })
})

const token = 'd'.repeat(48)
const otherToken = 'e'.repeat(48)
const hash = 'f'.repeat(64)
const binding = { sessionId: commit.sessionId, scopeId: token, threadId: 'thread-1', purpose: 'edit', draftVersion: token }
const draft = { objectId: hash, baseRevision: commit.baseRevision, version: otherToken, content: 'private working copy' }
const scope = { scopeId: token, objectId: hash, threadId: binding.threadId, purpose: binding.purpose, draftVersion: token, baseRevision: commit.baseRevision, range: { start: 1, end: 4 }, parts: [{ kind: 'literal', text: 'safe' }], current: true }
const proposal = { proposalId: token, scopeId: token, draftVersion: token, parts: scope.parts, status: 'proposed' }
const privateCases = [
  { request: { action: 'draft-update', sessionId: commit.sessionId, baseRevision: commit.baseRevision, expectedVersion: token, content: draft.content }, payload: { draft } },
  { request: { action: 'draft-read', sessionId: commit.sessionId }, payload: { draft } },
  { request: { action: 'scope-capture', sessionId: commit.sessionId, draftVersion: token, threadId: binding.threadId, purpose: binding.purpose, range: scope.range }, payload: { scope } },
  { request: { action: 'scope-read', ...binding }, payload: { scope: { ...scope, current: false } } },
  { request: { action: 'scope-revoke', ...binding }, payload: { revoked: true } },
  { request: { action: 'proposal-create', ...binding, operationId: 'propose_001', parts: scope.parts }, payload: { proposal } },
  { request: { action: 'proposal-accept', ...binding, operationId: 'accept_001', proposalId: token }, payload: { decision: { proposalId: token, status: 'accepted', draftVersion: otherToken } } },
  { request: { action: 'proposal-reject', ...binding, operationId: 'reject_001', proposalId: token }, payload: { decision: { proposalId: token, status: 'rejected', draftVersion: otherToken } } }
]
const privateResponse = (payload: unknown, ok = true) => vi.fn(async () => ({ ok, status: ok ? 200 : 409, body: JSON.stringify(payload) }))

describe('private draft/scope/proposal response binding', () => {
  it.each(privateCases)('accepts and transmits the exact $request.action response', async ({ request, payload }) => {
    const transport = privateResponse({ ok: true, ...payload })
    expect(await createObjectEditingHandler(transport)(request)).toEqual({ ok: true, ...payload })
    expect(transport).toHaveBeenCalledExactlyOnceWith(objectEditingPath, JSON.stringify(request))
  })

  it.each(privateCases)('rejects a cross-action success for $request.action', async ({ request }) => {
    expect(await createObjectEditingHandler(privateResponse({ ok: true, closed: true }))(request)).toEqual({ ok: false, code: 'unavailable', message: 'Object service is unavailable.' })
  })

  it('rejects wrong scope thread, purpose, version, range and scope identity', async () => {
    for (const change of [{ threadId: 'thread-2' }, { purpose: 'discuss' }, { draftVersion: otherToken }, { range: { start: 0, end: 4 } }, { range: { start: 1, end: 5 } }, { current: false }]) {
      expect(await createObjectEditingHandler(privateResponse({ ok: true, scope: { ...scope, ...change } }))(privateCases[2].request)).toMatchObject({ ok: false, code: 'unavailable' })
    }
    for (const change of [{ scopeId: otherToken }, { threadId: 'thread-2' }, { purpose: 'discuss' }, { draftVersion: otherToken }]) {
      expect(await createObjectEditingHandler(privateResponse({ ok: true, scope: { ...scope, ...change } }))(privateCases[3].request)).toMatchObject({ ok: false, code: 'unavailable' })
    }
  })

  it('rejects a changed draft payload, proposal scope/version/parts and decision identity/status', async () => {
    for (const change of [{ baseRevision: hash }, { content: 'wrong private content' }]) {
      expect(await createObjectEditingHandler(privateResponse({ ok: true, draft: { ...draft, ...change } }))(privateCases[0].request)).toEqual({ ok: false, code: 'unavailable', message: 'Object service is unavailable.' })
    }
    for (const change of [{ scopeId: otherToken }, { draftVersion: otherToken }, { parts: [{ kind: 'literal', text: 'wrong' }] }]) {
      expect(await createObjectEditingHandler(privateResponse({ ok: true, proposal: { ...proposal, ...change } }))(privateCases[5].request)).toMatchObject({ ok: false, code: 'unavailable' })
    }
    for (const index of [6, 7]) {
      const decision = privateCases[index].payload.decision!
      for (const change of [{ proposalId: otherToken }, { status: decision.status === 'accepted' ? 'rejected' : 'accepted' }]) {
        expect(await createObjectEditingHandler(privateResponse({ ok: true, decision: { ...decision, ...change } }))(privateCases[index].request)).toMatchObject({ ok: false, code: 'unavailable' })
      }
    }
  })

  it('allows create replay after decision without requiring an obsolete proposed status', async () => {
    for (const status of ['accepted', 'rejected']) {
      expect(await createObjectEditingHandler(privateResponse({ ok: true, proposal: { ...proposal, status } }))(privateCases[5].request)).toEqual({ ok: true, proposal: { ...proposal, status } })
    }
  })

  it('preserves closed private failures and rejects receipts on proposal actions', async () => {
    const failure = { ok: false, code: 'scope_invalid', message: 'The object operation could not be completed.' }
    expect(await createObjectEditingHandler(privateResponse(failure, false))(privateCases[5].request)).toEqual(failure)
    const receipt = { operationId: 'propose_001', revision: '', status: 'unknown', savedAt: '' }
    expect(await createObjectEditingHandler(privateResponse({ ...failure, receipt }, false))(privateCases[5].request)).toMatchObject({ ok: false, code: 'unavailable' })
  })

  it.each(['commit', 'status'])('retains matching unknown/conflict receipts and rejects wrong operation for %s', async action => {
    const request = action === 'commit' ? commit : { action, sessionId: commit.sessionId, operationId: commit.operationId }
    for (const status of ['unknown', 'conflict']) {
      const failure = { ok: false, code: status === 'unknown' ? 'persistence_failure' : 'conflict', message: 'The object operation could not be completed.', receipt: { operationId: commit.operationId, revision: '', status, savedAt: '' } }
      expect(await createObjectEditingHandler(privateResponse(failure, false))(request)).toEqual(failure)
      const result = await createObjectEditingHandler(privateResponse({ ...failure, receipt: { ...failure.receipt, operationId: 'other_0001' } }, false))(request)
      expect(result).toMatchObject(action === 'commit' ? { ok: false, code: 'persistence_failure', receipt: { operationId: commit.operationId, status: 'unknown' } } : { ok: false, code: 'unavailable' })
    }
  })
})

describe('image object protected response bindings', () => {
  const dataBase64 = 'cGl4ZWxz', sourceRevision = '6ec9c2b0eb14010746c8bce8939303b382344b2962066126d4f2c5bb64c3d3da'
  const image = {sessionId:token,objectId:hash,threadId:'thread-1',path:'image.png',sourceRevision,mimeType:'image/png',width:100,height:80,dataBase64}
  const annotation = {objectId:hash,threadId:'thread-1',annotationRevision:hash,sourceRevision:hash,width:100,height:80,region:{x:0,y:1,width:40,height:20},note:'local note',updatedAt:'2026-09-21T06:00:00Z',current:true}
  it('validates the maximum admitted image payload without exhausting the regexp stack', () => {
    expect(imageObjectSnapshotSchema.safeParse({ ...image, dataBase64: 'A'.repeat(16 * 1024 * 1024) }).success).toBe(true)
    for (const malformed of ['AAA', 'A===', 'AA=A', 'AAA\n', 'AAA\r', 'AAAA\n', 'AAAA====']) {
      expect(imageObjectSnapshotSchema.safeParse({ ...image, dataBase64: malformed }).success).toBe(false)
    }
  })
  it('rejects image byte/hash mismatches and cross-family success', async () => {
    const request={action:'image-open',threadId:'thread-1',path:'image.png'}
    const {createHash}=await import('node:crypto')
    const good={...image,sourceRevision:createHash('sha256').update(Buffer.from(dataBase64,'base64')).digest('hex')}
    expect(await createObjectEditingHandler(privateResponse({ok:true,image:good}))(request)).toEqual({ok:true,image:good})
    for(const changed of [{...good,sourceRevision:'0'.repeat(64)},{...good,threadId:'thread-2'},{...good,dataBase64:'cGl4ZWxz===='}])expect(await createObjectEditingHandler(privateResponse({ok:true,image:changed}))(request)).toMatchObject({ok:false})
    expect(await createObjectEditingHandler(privateResponse({ok:true,closed:true}))(request)).toMatchObject({ok:false})
  })
  it('requires confirmed exact CAS note, source, thread and geometry', async () => {
    const request={action:'image-annotation-write',sessionId:token,threadId:annotation.threadId,sourceRevision:hash,expectedAnnotationRevision:'',region:annotation.region,note:annotation.note}
    expect(await createObjectEditingHandler(privateResponse({ok:true,annotation}))(request)).toEqual({ok:true,annotation})
    for(const changed of [{...annotation,current:false},{...annotation,note:'other'},{...annotation,sourceRevision:'0'.repeat(64)},{...annotation,region:{...annotation.region,width:41}},{...annotation,threadId:'thread-2'}])expect(await createObjectEditingHandler(privateResponse({ok:true,annotation:changed}))(request)).toMatchObject({ok:false})
  })
})
