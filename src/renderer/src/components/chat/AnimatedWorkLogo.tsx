import type { ReactElement } from 'react'
import { useEffect, useRef, useState } from 'react'
import type { UiPluginFigureSlot } from '@shared/ui-plugin'
import { useUiPluginFigure } from '../../store/ui-plugin-store'
import xiezhiProfileFigure from '../../../../asset/img/xiezhi_profile.png'

/* UI 插件按槽位覆盖默认 cameo 形象时的回退链 */
export const UI_PLUGIN_STATE_SLOTS: Record<CameoStateFigureKind, readonly UiPluginFigureSlot[]> = {
  greet: ['greet', 'swim'],
  sleep: ['sleep', 'sit', 'swim'],
  sit: ['sit', 'greet', 'swim']
}

export type WorkLogoSwimMode = 'propel' | 'sprint' | 'dive' | 'surf'

export const WORK_LOGO_SWIM_MODES: readonly WorkLogoSwimMode[] = [
  'propel',
  'sprint',
  'dive',
  'surf'
]

export const WORK_LOGO_SWIM_MODE_LABEL_KEYS: Record<WorkLogoSwimMode, string> = {
  propel: 'working',
  sprint: 'workingSprint',
  dive: 'workingDive',
  surf: 'workingSurf'
}

const WORK_LOGO_SWIM_MODE_INTERVAL_MS = 4200

export function useWorkLogoSwimMode(active: boolean): WorkLogoSwimMode {
  // 起点随机,避免每次都从「推进中」开始;之后按顺序轮播
  const [modeIndex, setModeIndex] = useState(() =>
    Math.floor(Math.random() * WORK_LOGO_SWIM_MODES.length)
  )

  useEffect(() => {
    if (!active) return
    const interval = window.setInterval(() => {
      setModeIndex((current) => (current + 1) % WORK_LOGO_SWIM_MODES.length)
    }, WORK_LOGO_SWIM_MODE_INTERVAL_MS)
    return () => window.clearInterval(interval)
  }, [active])

  return WORK_LOGO_SWIM_MODES[modeIndex] ?? 'propel'
}

export type CameoStateFigureKind = 'greet' | 'sleep' | 'sit'

const CAMEO_STATE_FIGURES: Record<CameoStateFigureKind, string> = {
  greet: xiezhiProfileFigure,
  sleep: xiezhiProfileFigure,
  sit: xiezhiProfileFigure
}

const CAMEO_STATE_MASCOT_FIGURES: Record<CameoStateFigureKind, string> = {
  greet: xiezhiProfileFigure,
  sleep: xiezhiProfileFigure,
  sit: xiezhiProfileFigure
}

/** 静态场景里的 cameo 形象:打招呼、睡觉、坐着 */
export function CameoStateFigure({
  kind,
  className = ''
}: {
  kind: CameoStateFigureKind
  className?: string
}): ReactElement {
  // UI 插件激活时按槽位覆盖默认 cameo 美术(mascot 内置模式走 CSS 双图切换,不经过这里)
  const cameoFigureSrc = useUiPluginFigure(UI_PLUGIN_STATE_SLOTS[kind], CAMEO_STATE_FIGURES[kind])
  return (
    <span
      className={['ds-cameo-state', `ds-cameo-state-${kind}`, className].filter(Boolean).join(' ')}
      aria-hidden="true"
    >
      <img
        className="ds-cameo-state-figure"
        src={cameoFigureSrc}
        alt=""
        draggable={false}
        decoding="async"
      />
      <img
        className="ds-mascot-state-figure"
        src={CAMEO_STATE_MASCOT_FIGURES[kind]}
        alt=""
        draggable={false}
        decoding="async"
      />
    </span>
  )
}

export type MascotCameoType = 'dash' | 'chase' | 'peek' | 'boba' | 'nap'
export type MascotCameoSide = 'left' | 'right'
export type MascotCameoSpec = { id: number; type: MascotCameoType; side: MascotCameoSide }

export const MASCOT_CAMEO_TYPES: readonly MascotCameoType[] = ['dash', 'chase', 'peek', 'boba', 'nap']

/* 每种戏码演完的总时长,与 CSS 里的 forwards 动画时长保持一致 */
export const MASCOT_CAMEO_DURATIONS_MS: Record<MascotCameoType, number> = {
  dash: 5200,
  chase: 6600,
  peek: 6200,
  boba: 7200,
  nap: 8200
}

const MASCOT_CAMEO_FIGURES: Record<Exclude<MascotCameoType, 'chase'>, string> = {
  dash: xiezhiProfileFigure,
  peek: xiezhiProfileFigure,
  boba: xiezhiProfileFigure,
  nap: xiezhiProfileFigure
}

const MASCOT_CAMEO_MIN_GAP_MS = 18000
const MASCOT_CAMEO_MAX_GAP_MS = 45000
const MASCOT_CAMEO_FIRST_GAP_MS = 7000

let mascotCameoSequence = 0

export function pickMascotCameo(): MascotCameoSpec {
  const type = MASCOT_CAMEO_TYPES[Math.floor(Math.random() * MASCOT_CAMEO_TYPES.length)] ?? 'dash'
  const side: MascotCameoSide = Math.random() < 0.5 ? 'left' : 'right'
  mascotCameoSequence += 1
  return { id: mascotCameoSequence, type, side }
}

/* 出没彩蛋的槽位回退链:插件模式取插件图,mascot 模式回退内置 cameo 美术 */
export const UI_PLUGIN_CAMEO_SLOTS: Record<Exclude<MascotCameoType, 'chase'>, readonly UiPluginFigureSlot[]> = {
  dash: ['run', 'swim'],
  peek: ['greet', 'swim'],
  boba: ['sit', 'greet', 'swim'],
  nap: ['sleep', 'sit', 'swim']
}

function MascotCameoFigure({
  type,
  side,
  second = false
}: {
  type: Exclude<MascotCameoType, 'chase'>
  side: MascotCameoSide
  second?: boolean
}): ReactElement {
  const src = useUiPluginFigure(UI_PLUGIN_CAMEO_SLOTS[type], MASCOT_CAMEO_FIGURES[type])
  return (
    <span
      className={[
        'ds-mascot-cameo',
        `ds-mascot-cameo-${type}`,
        `is-${side}`,
        second ? 'is-second' : ''
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <span className="ds-mascot-cameo-flip">
        <img className="ds-mascot-cameo-figure" src={src} alt="" draggable={false} decoding="async" />
      </span>
    </span>
  )
}

/** 单场 mascot 戏码;chase 是组合动画:两只对穿,第二只小一号晚一拍 */
export function MascotCameo({ cameo }: { cameo: Pick<MascotCameoSpec, 'type' | 'side'> }): ReactElement {
  if (cameo.type === 'chase') {
    const otherSide: MascotCameoSide = cameo.side === 'left' ? 'right' : 'left'
    return (
      <>
        <MascotCameoFigure type="dash" side={cameo.side} />
        <MascotCameoFigure type="dash" side={otherSide} second />
      </>
    )
  }
  return <MascotCameoFigure type={cameo.type} side={cameo.side} />
}

/** mascot 模式专属:主会话两侧不定时出没的 cameo 层(指针穿透,纯装饰) */
export function MascotCameoLayer(): ReactElement {
  const [cameo, setCameo] = useState<MascotCameoSpec | null>(null)

  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    let timer = 0
    const schedule = (delay: number): void => {
      timer = window.setTimeout(() => {
        const next = pickMascotCameo()
        setCameo(next)
        timer = window.setTimeout(() => {
          setCameo(null)
          schedule(
            MASCOT_CAMEO_MIN_GAP_MS + Math.random() * (MASCOT_CAMEO_MAX_GAP_MS - MASCOT_CAMEO_MIN_GAP_MS)
          )
        }, MASCOT_CAMEO_DURATIONS_MS[next.type])
      }, delay)
    }
    schedule(MASCOT_CAMEO_FIRST_GAP_MS + Math.random() * 8000)
    return () => window.clearTimeout(timer)
  }, [])

  return (
    <span className="ds-mascot-cameo-layer" aria-hidden="true">
      {cameo ? <MascotCameo key={cameo.id} cameo={cameo} /> : null}
    </span>
  )
}

export type CameoCelebrationVariant = 'cheer' | 'lap' | 'toast'

export const CAMEO_CELEBRATION_VARIANTS: readonly CameoCelebrationVariant[] = [
  'cheer',
  'lap',
  'toast'
]

/* 与 CSS 里 forwards 动画总时长一致 */
export const CAMEO_CELEBRATION_DURATIONS_MS: Record<CameoCelebrationVariant, number> = {
  cheer: 3200,
  lap: 3600,
  toast: 3400
}

/* 每种庆祝的双形象:普通模式用 cameo 美术,mascot 模式自动换 mascot 美术 */
const CAMEO_CELEBRATION_FIGURES: Record<CameoCelebrationVariant, { cameo: string; mascot: string }> = {
  cheer: { cameo: xiezhiProfileFigure, mascot: xiezhiProfileFigure },
  lap: { cameo: xiezhiProfileFigure, mascot: xiezhiProfileFigure },
  toast: { cameo: xiezhiProfileFigure, mascot: xiezhiProfileFigure }
}

/* 回合至少跑这么久才庆祝,避免秒回也放彩带 */
const CAMEO_CELEBRATION_MIN_TURN_MS = 2000

let cameoCelebrationSequence = 0

export function pickCameoCelebration(): { id: number; variant: CameoCelebrationVariant } {
  const variant =
    CAMEO_CELEBRATION_VARIANTS[Math.floor(Math.random() * CAMEO_CELEBRATION_VARIANTS.length)] ??
    'cheer'
  cameoCelebrationSequence += 1
  return { id: cameoCelebrationSequence, variant }
}

function CameoConfettiBurst(): ReactElement {
  return (
    <span className="ds-cameo-confetti">
      {Array.from({ length: 10 }, (_, index) => (
        <i key={index} />
      ))}
    </span>
  )
}

/* 庆祝戏码的插件槽位回退链 */
export const UI_PLUGIN_CELEBRATION_SLOTS: Record<CameoCelebrationVariant, readonly UiPluginFigureSlot[]> = {
  cheer: ['greet', 'swim'],
  lap: ['run', 'surf', 'swim'],
  toast: ['sit', 'greet', 'swim']
}

/** 单场庆祝:跃起欢呼 / 胜利冲浪(mascot 为快攻冲刺) / 举杯庆功 */
export function CameoCelebration({ variant }: { variant: CameoCelebrationVariant }): ReactElement {
  const figures = CAMEO_CELEBRATION_FIGURES[variant]
  const cameoFigureSrc = useUiPluginFigure(UI_PLUGIN_CELEBRATION_SLOTS[variant], figures.cameo)
  return (
    <span className={`ds-cameo-celebration ds-cameo-celebration-${variant}`}>
      <span className="ds-cameo-celebration-figure-wrap">
        <img
          className="ds-cameo-celebration-figure is-cameo"
          src={cameoFigureSrc}
          alt=""
          draggable={false}
          decoding="async"
        />
        <img
          className="ds-cameo-celebration-figure is-mascot"
          src={figures.mascot}
          alt=""
          draggable={false}
          decoding="async"
        />
        <CameoConfettiBurst />
      </span>
    </span>
  )
}

/** 回合完成庆祝层:active(busy)从 true 落回 false 且跑得够久时,随机放一段 */
export function CameoCelebrationLayer({
  active,
  suppressed = false
}: {
  active: boolean
  suppressed?: boolean
}): ReactElement {
  const [celebration, setCelebration] = useState<{
    id: number
    variant: CameoCelebrationVariant
  } | null>(null)
  const turnStartRef = useRef<number | null>(null)
  const hideTimerRef = useRef(0)

  useEffect(() => {
    if (active) {
      turnStartRef.current = Date.now()
      return
    }
    if (turnStartRef.current === null) return
    const elapsed = Date.now() - turnStartRef.current
    turnStartRef.current = null
    if (suppressed) return
    if (elapsed < CAMEO_CELEBRATION_MIN_TURN_MS) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return

    const next = pickCameoCelebration()
    setCelebration(next)
    window.clearTimeout(hideTimerRef.current)
    hideTimerRef.current = window.setTimeout(() => {
      setCelebration(null)
    }, CAMEO_CELEBRATION_DURATIONS_MS[next.variant])
  }, [active, suppressed])

  useEffect(() => () => window.clearTimeout(hideTimerRef.current), [])

  return (
    <span className="ds-cameo-celebration-layer" aria-hidden="true">
      {celebration ? <CameoCelebration key={celebration.id} variant={celebration.variant} /> : null}
    </span>
  )
}

const SIDEBAR_MASCOT_KINDS: readonly CameoStateFigureKind[] = ['sit', 'greet', 'sleep']
const SIDEBAR_MASCOT_INTERVAL_MS = 10000

/** 侧边栏角落的吉祥物:循环 坐着→打招呼→睡觉,mascot 模式自动换成内置 cameo 美术 */
export function SidebarMascot(): ReactElement {
  const [kindIndex, setKindIndex] = useState(() =>
    Math.floor(Math.random() * SIDEBAR_MASCOT_KINDS.length)
  )

  useEffect(() => {
    const interval = window.setInterval(() => {
      setKindIndex((current) => (current + 1) % SIDEBAR_MASCOT_KINDS.length)
    }, SIDEBAR_MASCOT_INTERVAL_MS)
    return () => window.clearInterval(interval)
  }, [])

  const kind = SIDEBAR_MASCOT_KINDS[kindIndex] ?? 'sit'
  return <CameoStateFigure key={kind} kind={kind} className="ds-sidebar-mascot" />
}

export type MascotWorkLogoVariant = 'dribble' | 'run' | 'boba'

export const MASCOT_WORK_LOGO_VARIANTS: readonly MascotWorkLogoVariant[] = [
  'dribble',
  'run',
  'boba'
]

const MASCOT_WORK_LOGO_FIGURES: Record<MascotWorkLogoVariant, string> = {
  dribble: xiezhiProfileFigure,
  run: xiezhiProfileFigure,
  boba: xiezhiProfileFigure
}

export const MASCOT_WORK_LOGO_VARIANT_LABEL_KEYS: Record<MascotWorkLogoVariant, string> = {
  dribble: 'mascotDribbling',
  run: 'mascotFastBreak',
  boba: 'mascotBobaTime'
}

const MASCOT_WORK_LOGO_VARIANT_INTERVAL_MS = 2800

export function pickMascotWorkLogoVariant(
  current?: MascotWorkLogoVariant
): MascotWorkLogoVariant {
  const candidates = MASCOT_WORK_LOGO_VARIANTS.filter((variant) => variant !== current)
  const pool = candidates.length > 0 ? candidates : MASCOT_WORK_LOGO_VARIANTS
  return pool[Math.floor(Math.random() * pool.length)] ?? 'dribble'
}

export function useMascotWorkLogoVariant(active: boolean): MascotWorkLogoVariant {
  const [variant, setVariant] = useState<MascotWorkLogoVariant>(() => pickMascotWorkLogoVariant())

  useEffect(() => {
    if (!active) return
    const interval = window.setInterval(() => {
      setVariant((current) => pickMascotWorkLogoVariant(current))
    }, MASCOT_WORK_LOGO_VARIANT_INTERVAL_MS)
    return () => window.clearInterval(interval)
  }, [active])

  return variant
}

export function AnimatedWorkLogo({
  active = false,
  className = '',
  mascotVariant,
  mode,
  phase = 'lead',
  size = 'sm'
}: {
  active?: boolean
  className?: string
  mascotVariant?: MascotWorkLogoVariant
  mode?: WorkLogoSwimMode
  phase?: 'lead' | 'trail'
  size?: 'sm' | 'md'
}): ReactElement {
  const rotatedMascotVariant = useMascotWorkLogoVariant(active && mascotVariant === undefined)
  const effectiveMascotVariant = mascotVariant ?? rotatedMascotVariant
  const rotatedSwimMode = useWorkLogoSwimMode(active && mode === undefined)
  const swimMode = mode ?? rotatedSwimMode
  const figureSrc = useUiPluginFigure(
    swimMode === 'surf' ? ['surf', 'swim'] : ['swim'],
    xiezhiProfileFigure
  )

  return (
    <span
      className={[
        'ds-work-logo',
        `ds-work-logo-${size}`,
        `ds-work-logo-phase-${phase}`,
        `ds-work-logo-mode-${swimMode}`,
        active ? 'is-active' : '',
        className
      ]
        .filter(Boolean)
        .join(' ')}
      aria-hidden="true"
    >
      <span className="ds-work-logo-gust" />
      <span className="ds-work-logo-current" />
      <span className="ds-work-logo-swell" />
      <span className="ds-work-logo-wave ds-work-logo-wave-back" />
      <span className="ds-work-logo-ripple" />
      <span className="ds-work-logo-wave ds-work-logo-wave-front" />
      <span className="ds-work-logo-breaker" />
      <span className="ds-work-logo-wake" />
      <span className="ds-work-logo-foam" />
      <span className="ds-work-logo-crest" />
      <span className="ds-work-logo-splash" />
      <span className="ds-work-logo-spray" />
      <span className="ds-work-logo-bubbles" />
      <img className="ds-work-logo-echo" src={figureSrc} alt="" draggable={false} decoding="async" />
      <span
        className={`ds-mascot-logo ds-mascot-logo-${effectiveMascotVariant}`}
        data-mascot-variant={effectiveMascotVariant}
      >
        <span className="ds-mascot-logo-shadow" />
        <img
          className="ds-mascot-figure"
          src={MASCOT_WORK_LOGO_FIGURES[effectiveMascotVariant]}
          alt=""
          draggable={false}
          decoding="async"
        />
      </span>
      <span className="ds-work-logo-track">
        <span className="ds-work-logo-body">
          <img className="ds-work-logo-image" src={figureSrc} alt="" draggable={false} decoding="async" />
        </span>
      </span>
    </span>
  )
}
