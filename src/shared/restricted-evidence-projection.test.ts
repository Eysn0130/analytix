import { describe, expect, it } from 'vitest'
import {
  containsRestrictedEvidence,
  isRestrictedEvidenceObject
} from './restricted-evidence-projection'

describe('restricted evidence projection', () => {
  it('detects private purposes below neutral wrappers and key variants', () => {
    expect(containsRestrictedEvidence({
      review: {
        output: {
          Schema_Version: 2,
          PURPOSE: ' ANALYTIX.SOURCE-FIELD-BINDING/V2 ',
          sourceExactValue: '0012-3456789012345678'
        }
      }
    })).toBe(true)
    expect(containsRestrictedEvidence(
      '{"neutral":{"purpose":"analytix.dataset-snapshot-manifest/v2"}}'
    )).toBe(true)
  })

  it('detects purpose-stripped exact reference shapes', () => {
    const digest = 'a'.repeat(64)
    const sha256 = 'b'.repeat(64)
    expect(isRestrictedEvidenceObject({
      'Raw Artifact Manifest Digest': digest,
      raw_artifact_manifest_sha256: sha256,
      RAW_ARTIFACT_MANIFEST_BYTE_LENGTH: 42
    })).toBe(true)
    expect(containsRestrictedEvidence({
      neutral: {
        sourceExactValue: '0012-3456789012345678',
        sourceExactValueSha256: sha256,
        bindingDigest: digest
      }
    })).toBe(true)
    expect(containsRestrictedEvidence({
      schemaVersion: 3,
      facts: [{ factId: 'fact-a' }],
      acceptedSlotSourceBindings: [{
        sourceRecordId: `srow1_${sha256}`,
        sourceFileId: '0123456789abcdefabcd',
        sourceRowNumber: 7,
        field: 'card',
        bindingDigest: digest
      }],
      acceptedSlotSourceBindingSetDigest: sha256
    })).toBe(true)
    expect(containsRestrictedEvidence({
      sourceRecordId: `srow1_${sha256}`,
      sourceFileId: '0123456789abcdefabcd',
      sourceRowNumber: 7,
      field: 'account',
      bindingDigest: digest
    })).toBe(true)
  })

  it('detects canonical evidence V3 under a neutral PII classification', () => {
    expect(containsRestrictedEvidence({
      piiClassification: 'none',
      canonicalEvidence: {
        schemaVersion: 3,
        purpose: 'analytix.canonical-evidence/v3',
        facts: []
      }
    })).toBe(true)
  })

  it('detects restricted JSON appended to a log prefix', () => {
    expect(containsRestrictedEvidence(
      '[2026-07-18T00:00:00Z] [ERROR] detail: {"purpose":"analytix.source-row-lineage/v1"}\n'
    )).toBe(true)
  })

  it('detects balanced restricted JSON embedded in prose with nested escaped braces', () => {
    const privateV3 = {
      schemaVersion: 3,
      purpose: 'analytix.canonical-evidence/v3',
      note: 'literal braces: { value: "escaped \\"quote\\"" } and ]',
      facts: [],
      acceptedSlotSourceBindings: [{
        sourceRecordId: `srow1_${'a'.repeat(64)}`,
        sourceFileId: 'opaque-source-file',
        sourceRowNumber: 7,
        field: 'account',
        bindingDigest: 'b'.repeat(64)
      }],
      acceptedSlotSourceBindingSetDigest: 'c'.repeat(64)
    }

    expect(containsRestrictedEvidence(
      `prefix ${JSON.stringify({ wrapper: privateV3 })} suffix`
    )).toBe(true)
    expect(containsRestrictedEvidence(
      `prefix {not-json ${JSON.stringify(privateV3)}} suffix`
    )).toBe(true)
  })

  it('fails closed instead of ignoring JSON after the candidate budget', () => {
    const ordinaryCandidates = new Array(32).fill('{"kind":"ordinary"}').join(' ')
    const privateV3 = JSON.stringify({
      schemaVersion: 3,
      purpose: 'analytix.canonical-evidence/v3',
      facts: []
    })

    expect(containsRestrictedEvidence(`${ordinaryCandidates} ${privateV3}`)).toBe(true)
    expect(containsRestrictedEvidence(
      new Array(33).fill('{"kind":"ordinary"}').join(' ')
    )).toBe(true)
  })

  it('allows public hashes, standalone snapshot ids, and prose mentions', () => {
    const hash = 'a'.repeat(64)
    expect(containsRestrictedEvidence({ reportSha256: hash, datasetSnapshotId: 'snapshot-v2' })).toBe(false)
    expect(containsRestrictedEvidence({ rawArtifactManifestDigest: hash })).toBe(false)
    expect(containsRestrictedEvidence({
      rawArtifactManifestDigest: 'digest',
      rawArtifactManifestSha256: 'sha',
      rawArtifactManifestByteLength: 42
    })).toBe(false)
    expect(containsRestrictedEvidence({
      purpose: 'analytix.source-row-producer-policy/v1',
      policyDigest: hash
    })).toBe(false)
    expect(containsRestrictedEvidence(
      '请解释 analytix.raw-artifact-manifest/v1 与 lineageDigest 的含义'
    )).toBe(false)
    expect(containsRestrictedEvidence(
      'prefix {"kind":"ordinary","message":"literal } and ] inside a string"} suffix'
    )).toBe(false)
    expect(containsRestrictedEvidence(
      new Array(32).fill('{"kind":"ordinary"}').join(' ')
    )).toBe(false)
    expect(containsRestrictedEvidence('ordinary prose with { an unmatched marker')).toBe(false)
  })

  it('fails closed on cycles and over-depth serialized JSON', () => {
    const cyclic: Record<string, unknown> = {}
    cyclic.self = cyclic
    expect(containsRestrictedEvidence(cyclic)).toBe(true)
    const deep = '['.repeat(65) + ']'.repeat(65)
    expect(containsRestrictedEvidence(deep)).toBe(true)
    expect(containsRestrictedEvidence('x'.repeat(1024 * 1024 + 1))).toBe(true)
    expect(containsRestrictedEvidence(new Array(100_001).fill(null))).toBe(true)
    expect(containsRestrictedEvidence(JSON.stringify(new Array(100_001).fill(null)))).toBe(true)
    expect(containsRestrictedEvidence(
      '{"purpose":"analytix.source-row-lineage/v1","purpose":"analytix.public/v1"}'
    )).toBe(true)
  })
})
