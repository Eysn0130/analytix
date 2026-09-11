import { describe, expect, it } from 'vitest'
import {
  SDD_DRAFT_FILE_NAME,
  buildSddDraftRelativePath,
  isSddDraftRelativePath,
  isSddImageRelativePath,
  isSddPrototypeRelativePath,
  normalizeSddRelativePath,
  sddDraftRelativePathForPlanPath,
  sddDraftTraceRelativePath,
  sddRequirementUnitDir,
  sddUnitChatDir,
  sddUnitImageDir,
  sddUnitProtoDir
} from './sdd'

const UUID = '123e4567-e89b-12d3-a456-426614174000'
const DRAFT = `.analytixsdd/requirements/${UUID}/${SDD_DRAFT_FILE_NAME}`

describe('sdd shared paths', () => {
  it('builds a canonical requirement-unit draft path', () => {
    expect(buildSddDraftRelativePath(UUID)).toBe(DRAFT)
  })

  it('validates only uuid-backed requirement drafts under requirements/', () => {
    expect(isSddDraftRelativePath(DRAFT)).toBe(true)
    expect(isSddDraftRelativePath(`.analytixsdd/requirements/not-a-uuid/requirement.md`)).toBe(false)
    expect(isSddDraftRelativePath(`.analytixsdd/requirements/${UUID}/other.md`)).toBe(false)
    expect(isSddDraftRelativePath(`.analytixsdd/requirements/${UUID}/nested/requirement.md`)).toBe(false)
    // The pre-unit layout is explicitly retired (clean switch, no migration).
    expect(isSddDraftRelativePath(`.analytixsdd/draft/${UUID}/requirement.md`)).toBe(false)
  })

  it('derives the unit directories from the draft path', () => {
    expect(sddRequirementUnitDir(DRAFT)).toBe(`.analytixsdd/requirements/${UUID}`)
    expect(sddUnitImageDir(DRAFT)).toBe(`.analytixsdd/requirements/${UUID}/img`)
    expect(sddUnitProtoDir(DRAFT)).toBe(`.analytixsdd/requirements/${UUID}/proto`)
    expect(sddUnitChatDir(DRAFT)).toBe(`.analytixsdd/requirements/${UUID}/chat`)
    expect(sddDraftTraceRelativePath(DRAFT)).toBe(`.analytixsdd/requirements/${UUID}/trace.json`)
    expect(sddRequirementUnitDir(`.analytixsdd/draft/${UUID}/requirement.md`)).toBeNull()
    expect(sddUnitImageDir('not-a-draft.md')).toBeNull()
  })

  it('maps SDD plan paths back to the requirement unit', () => {
    expect(sddDraftRelativePathForPlanPath(`.analytixsdd/plan/sdd-${UUID}.md`)).toBe(DRAFT)
    expect(sddDraftRelativePathForPlanPath(`.analytixsdd/plan/sdd-${UUID}-2.md`)).toBe(DRAFT)
    expect(sddDraftRelativePathForPlanPath('.analytixsdd/plan/other.md')).toBeNull()
  })

  it('validates per-unit image and prototype paths', () => {
    expect(normalizeSddRelativePath(`./.analytixsdd\\requirements\\${UUID}\\img\\a.png`)).toBe(
      `.analytixsdd/requirements/${UUID}/img/a.png`
    )
    expect(isSddImageRelativePath(`.analytixsdd/requirements/${UUID}/img/wireframe.png`)).toBe(true)
    expect(isSddImageRelativePath(`.analytixsdd/requirements/${UUID}/img/nested/wireframe.png`)).toBe(true)
    expect(isSddImageRelativePath(`.analytixsdd/requirements/${UUID}/img/../escape.png`)).toBe(false)
    expect(isSddImageRelativePath(`.analytixsdd/requirements/not-a-uuid/img/a.png`)).toBe(false)
    expect(isSddImageRelativePath('.analytixsdd/img/wireframe.png')).toBe(false)
    expect(isSddImageRelativePath('img/wireframe.png')).toBe(false)

    expect(isSddPrototypeRelativePath(`.analytixsdd/requirements/${UUID}/proto/p.html`)).toBe(true)
    expect(isSddPrototypeRelativePath('.analytixsdd/proto/p.html')).toBe(false)
    expect(isSddPrototypeRelativePath(`.analytixsdd/requirements/${UUID}/img/p.html`)).toBe(false)
  })
})
