import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { X } from 'lucide-react'
import type {
  HubAccountSnapshot,
  HubAuthChallenge,
  HubAuthChallengeMode
} from '@shared/hub-account'
import analytixAppIcon from '../../../asset/brand/analytix-exact-app-icon-512.png'
import analytixWordmark from '../../../asset/brand/analytix-logo-transparent.png'
import { useHubAccountStore } from './hub-account-store'
import './animated-login.css'

type AnimatedHubLoginPageProps = {
  onAuthenticated: (snapshot: HubAccountSnapshot) => void | Promise<void>
  className?: string
  stage?: 'prepared' | 'entering' | 'entered'
}

type BilingualCopy = {
  primary: string
  secondary: string
}

type LoginPanelMode = 'login' | 'register' | 'forgot-password'
type ChallengeNoteTone = 'neutral' | 'success' | 'error'

type LoginInputDraft = {
  version: 1
  loginEmail: string
  registerFullName: string
  registerOrganization: string
  registerPhoneNumber: string
  registerEmail: string
  forgotPasswordEmail: string
  updatedAt: string
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const PHONE_ALLOWED_PATTERN = /^[0-9+()\-\s]{7,20}$/
const LOGIN_INPUT_DRAFT_STORAGE_KEY = 'analytix:login-input-draft:v1'
const HUB_AUTH_REMEMBER_DAYS = 30
const LOOK_AT_EACH_OTHER_MS = 800
const ERROR_RECOVERY_MS = 2500
const SHAKE_DELAY_MS = 350
const BLINK_MIN_MS = 3000
const BLINK_JITTER_MS = 4000
const PEEK_MIN_MS = 2000
const PEEK_JITTER_MS = 3000
const PEEK_DURATION_MS = 800
const SHAKE_CLASS = 'shake-head'
const LOGIN_ERROR_EVENT = 'analytix-login-error'

const COPY = {
  headerDescription: {
    primary: '使用你的 Analytix 账号继续登录',
    secondary: 'Sign in with your Analytix account'
  },
  emailLabel: { primary: '邮箱', secondary: 'Email' },
  passwordLabel: { primary: '密码', secondary: 'Password' },
  nameLabel: { primary: '姓名', secondary: 'Name' },
  organizationLabel: { primary: '所属单位', secondary: 'Organization' },
  phoneLabel: { primary: '联系电话', secondary: 'Phone' },
  verificationCodeLabel: { primary: '验证码', secondary: 'Code' },
  newPasswordLabel: { primary: '新密码', secondary: 'New Password' },
  confirmPasswordLabel: { primary: '确认密码', secondary: 'Confirm Password' },
  rememberMe: { primary: '30 天内保持登录', secondary: 'Remember for 30 days' },
  forgotPassword: { primary: '忘记密码', secondary: 'Forgot password' },
  login: { primary: '登录', secondary: 'Log In' },
  loggingIn: { primary: '正在登录', secondary: 'Signing in...' },
  registerHeading: { primary: '创建账号', secondary: 'Create your Analytix account' },
  registerDescription: {
    primary: '注册后可直接使用同一 Analytix Hub 账号登录桌面端',
    secondary: 'Create one account for Analytix Hub and Desktop'
  },
  registerAgreement: {
    primary: '我已阅读并同意服务条款与隐私政策',
    secondary: 'Agree to terms and privacy policy'
  },
  createAccount: { primary: '创建账号', secondary: 'Create Account' },
  creatingAccount: { primary: '正在创建', secondary: 'Creating...' },
  sendVerificationCode: { primary: '获取验证码', secondary: 'Send Code' },
  sendingVerificationCode: { primary: '发送中', secondary: 'Sending...' },
  resendVerificationCode: { primary: '重新发送', secondary: 'Resend' },
  forgotPasswordHeading: {
    primary: '重置密码',
    secondary: 'Reset your password with email verification'
  },
  forgotPasswordDescription: {
    primary: '通过注册邮箱验证码重设你的 Analytix 登录密码',
    secondary: 'Reset your password with an email verification code'
  },
  resetPassword: { primary: '确认重置', secondary: 'Save New Password' },
  resettingPassword: { primary: '正在重置', secondary: 'Resetting...' },
  backToLogin: { primary: '返回登录', secondary: 'Back to Login' },
  privacy: { primary: '隐私政策', secondary: 'Privacy Policy' },
  terms: { primary: '服务条款', secondary: 'Terms of Service' },
  contact: { primary: '联系我们', secondary: 'Contact' },
  showPassword: { primary: '显示密码', secondary: 'Show password' },
  hidePassword: { primary: '隐藏密码', secondary: 'Hide password' },
  verifyChallenge: { primary: '验证图块', secondary: 'Verify Tiles' },
  verifyingChallenge: { primary: '验证中', secondary: 'Checking...' },
  refreshChallenge: { primary: '换一组', secondary: 'Refresh' },
  feedback: {
    invalidEmail: { primary: '请输入有效的邮箱地址', secondary: 'Please enter a valid email address.' },
    invalidPassword: { primary: '密码至少需要 6 位', secondary: 'Password must be at least 6 characters.' },
    invalidName: { primary: '请输入至少 2 个字符的姓名', secondary: 'Please enter a valid full name.' },
    invalidPhone: { primary: '请输入有效的联系电话', secondary: 'Please enter a valid contact phone number.' },
    invalidVerificationCode: { primary: '请输入 6 位邮箱验证码', secondary: 'Please enter the 6-digit verification code.' },
    invalidCredentials: { primary: '邮箱或密码不正确，请重试', secondary: 'Invalid email or password. Please try again.' },
    passwordMismatch: { primary: '两次输入的新密码不一致', secondary: 'The new passwords do not match.' },
    verificationCodeSent: { primary: '验证码已发送，请前往邮箱查收', secondary: 'Verification code sent.' },
    forgotPasswordCodeSent: {
      primary: '如果该邮箱已注册，验证码已发送，请前往邮箱查收',
      secondary: 'If the email exists, a verification code has been sent.'
    },
    forgotPasswordResetSuccess: { primary: '密码已重置，请使用新密码重新登录', secondary: 'Password reset completed.' },
    missingChallenge: { primary: '请先完成图片验证', secondary: 'Please finish the visual verification first.' },
    challengeSelectionRequired: { primary: '请先选择符合要求的图块', secondary: 'Select matching tiles first.' },
    challengeVerified: { primary: '图片验证已通过，可以继续操作', secondary: 'Visual verification passed.' },
    challengeActivated: { primary: '连续异常次数过多，已启用图片验证', secondary: 'Visual verification is now required.' },
    challengeRefreshFailed: { primary: '图片验证加载失败，请刷新后重试', secondary: 'Failed to load the visual challenge.' }
  }
} as const

const LEGAL_PANELS = {
  privacy: {
    label: COPY.privacy,
    officialUrl: 'https://analytix.top/protocol/privacy',
    action: { primary: '官网版本', secondary: 'Official Policy' },
    summary: '本政策说明 Analytix 在账号注册、客户端授权、专业版开通和 Agent 分析服务中如何处理必要信息。',
    sections: [
      {
        heading: '我们收集的信息',
        body: [
          '账号信息：邮箱、手机号、登录状态、实名认证状态和用户主动填写的组织信息。',
          '服务信息：客户端版本、设备授权状态、专业版套餐、订单号、支付状态和额度使用记录。',
          '分析信息：为完成 Agent 分析所需的任务输入、模型调用记录、消耗统计和异常日志。'
        ]
      },
      {
        heading: '信息使用目的',
        body: [
          '用于完成账号登录、安全验证、授权发放、专业版权益写入和支付状态同步。',
          '用于核算 AI Credits、排查异常调用、提升服务稳定性和防止滥用。'
        ]
      }
    ]
  },
  terms: {
    label: COPY.terms,
    officialUrl: 'https://analytix.top/protocol/terms',
    action: { primary: '官网版本', secondary: 'Official Terms' },
    summary: '本条款适用于用户访问 analytix.top、登录控制台、下载客户端、使用授权和 Agent 分析相关服务。',
    sections: [
      {
        heading: '账号与使用',
        body: [
          '用户应使用真实、准确、有效的信息注册和维护账号，并妥善保管登录凭证。',
          '用户不得将账号用于违法违规、侵权、攻击、绕过限制或干扰平台稳定性的行为。',
          '用户应确保上传、输入或分析的数据来自合法授权的业务场景。'
        ]
      },
      {
        heading: '服务边界',
        body: [
          'Analytix 提供资金分析相关的 Agent 工作平台、客户端授权、模型网关和专业版能力。',
          '平台输出内容用于辅助分析和决策，不构成法律、投资、审计或其他专业意见。'
        ]
      }
    ]
  },
  contact: {
    label: COPY.contact,
    officialUrl: 'mailto:support@analytix.top',
    action: { primary: '联系支持', secondary: 'Contact Support' },
    summary: '如需账号、认证、额度或桌面端支持，请通过 Analytix Hub 官网联系服务团队。',
    sections: [
      { heading: '官网', body: ['https://analytix.top'] },
      { heading: '邮箱', body: ['support@analytix.top'] }
    ]
  }
} as const

function TextPair({ primary, secondary, className }: BilingualCopy & { className?: string }): React.ReactElement {
  return (
    <span className={['text-pair', className].filter(Boolean).join(' ')}>
      <span className="text-primary">{primary}</span>
      <span className="text-secondary">{secondary}</span>
    </span>
  )
}

function emptyDraft(): LoginInputDraft {
  return {
    version: 1,
    loginEmail: '',
    registerFullName: '',
    registerOrganization: '',
    registerPhoneNumber: '',
    registerEmail: '',
    forgotPasswordEmail: '',
    updatedAt: ''
  }
}

function clean(value: unknown, maxLength = 160): string {
  return String(value ?? '').slice(0, maxLength)
}

function normalizeDraft(value: unknown): LoginInputDraft {
  if (!value || typeof value !== 'object') return emptyDraft()
  const record = value as Record<string, unknown>
  return {
    version: 1,
    loginEmail: clean(record.loginEmail).trim(),
    registerFullName: clean(record.registerFullName),
    registerOrganization: clean(record.registerOrganization),
    registerPhoneNumber: clean(record.registerPhoneNumber, 40),
    registerEmail: clean(record.registerEmail).trim(),
    forgotPasswordEmail: clean(record.forgotPasswordEmail).trim(),
    updatedAt: clean(record.updatedAt, 40)
  }
}

function readDraft(): LoginInputDraft {
  try {
    return normalizeDraft(JSON.parse(window.localStorage.getItem(LOGIN_INPUT_DRAFT_STORAGE_KEY) || 'null'))
  } catch {
    return emptyDraft()
  }
}

function writeDraft(draft: LoginInputDraft): void {
  try {
    window.localStorage.setItem(LOGIN_INPUT_DRAFT_STORAGE_KEY, JSON.stringify(draft))
  } catch {
    /* Input drafts are best-effort convenience only. */
  }
}

function isValidEmail(value: string): boolean {
  return EMAIL_PATTERN.test(value.trim())
}

function isValidContactPhone(value: string): boolean {
  const normalized = value.trim()
  const digitsOnly = normalized.replace(/\D/g, '')
  return digitsOnly.length >= 7 && digitsOnly.length <= 15 && PHONE_ALLOWED_PATTERN.test(normalized)
}

function normalizeReferralCode(value: string): string {
  return value.trim().toUpperCase().replace(/[^A-Z0-9]/g, '')
}

function messageFromError(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function accountApi() {
  return typeof window !== 'undefined' ? window.analytix?.account : undefined
}

function openExternal(url: string): void {
  if (typeof window === 'undefined') return
  if (typeof window.analytix?.app?.openExternal === 'function') {
    void window.analytix.app.openExternal(url).catch(() => undefined)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

export function AnimatedHubLoginPage({
  onAuthenticated,
  className,
  stage = 'entered'
}: AnimatedHubLoginPageProps): React.ReactElement {
  const rootRef = useRef<HTMLDivElement | null>(null)
  const draftRef = useRef(readDraft())
  const authChallengeProofRef = useRef('')
  const challengeOverlayTimerRef = useRef<number | null>(null)
  const registerCooldownTimerRef = useRef<number | null>(null)
  const forgotPasswordCooldownTimerRef = useRef<number | null>(null)
  const login = useHubAccountStore((state) => state.login)
  const register = useHubAccountStore((state) => state.register)

  const [panelMode, setPanelMode] = useState<LoginPanelMode>('login')
  const [activeLegalPanel, setActiveLegalPanel] = useState<keyof typeof LEGAL_PANELS | null>(null)
  const [loginEmail, setLoginEmail] = useState(draftRef.current.loginEmail || draftRef.current.registerEmail || draftRef.current.forgotPasswordEmail)
  const [loginPassword, setLoginPassword] = useState('')
  const [passwordVisible, setPasswordVisible] = useState(false)
  const [rememberLogin, setRememberLogin] = useState(true)
  const [loginSubmitting, setLoginSubmitting] = useState(false)
  const [loginError, setLoginError] = useState('')
  const [registerFullName, setRegisterFullName] = useState(draftRef.current.registerFullName)
  const [registerOrganization, setRegisterOrganization] = useState(draftRef.current.registerOrganization)
  const [registerPhoneNumber, setRegisterPhoneNumber] = useState(draftRef.current.registerPhoneNumber)
  const [registerEmail, setRegisterEmail] = useState(draftRef.current.registerEmail || loginEmail)
  const [registerVerificationCode, setRegisterVerificationCode] = useState('')
  const [registerPassword, setRegisterPassword] = useState('')
  const [registerPasswordVisible, setRegisterPasswordVisible] = useState(false)
  const [registerAgreement, setRegisterAgreement] = useState(false)
  const [registerSendingCode, setRegisterSendingCode] = useState(false)
  const [registerSubmitting, setRegisterSubmitting] = useState(false)
  const [registerCooldownUntil, setRegisterCooldownUntil] = useState(0)
  const [registerCooldownTick, setRegisterCooldownTick] = useState(0)
  const [registerMessage, setRegisterMessage] = useState<BilingualCopy | null>(null)
  const [registerMessageTone, setRegisterMessageTone] = useState<ChallengeNoteTone>('neutral')
  const [forgotPasswordEmail, setForgotPasswordEmail] = useState(draftRef.current.forgotPasswordEmail || loginEmail)
  const [forgotPasswordCode, setForgotPasswordCode] = useState('')
  const [forgotPasswordNextPassword, setForgotPasswordNextPassword] = useState('')
  const [forgotPasswordConfirmPassword, setForgotPasswordConfirmPassword] = useState('')
  const [forgotPasswordSendingCode, setForgotPasswordSendingCode] = useState(false)
  const [forgotPasswordSubmitting, setForgotPasswordSubmitting] = useState(false)
  const [forgotPasswordCooldownUntil, setForgotPasswordCooldownUntil] = useState(0)
  const [forgotPasswordCooldownTick, setForgotPasswordCooldownTick] = useState(0)
  const [forgotPasswordMessage, setForgotPasswordMessage] = useState<BilingualCopy | null>(null)
  const [forgotPasswordMessageTone, setForgotPasswordMessageTone] = useState<ChallengeNoteTone>('neutral')
  const [authChallengeRequired, setAuthChallengeRequired] = useState(false)
  const [authChallenge, setAuthChallenge] = useState<HubAuthChallenge | null>(null)
  const [selectedChallengeTileIds, setSelectedChallengeTileIds] = useState<string[]>([])
  const [authChallengeProof, setAuthChallengeProof] = useState('')
  const [challengeSuccessVisible, setChallengeSuccessVisible] = useState(false)
  const [challengeStatus, setChallengeStatus] = useState<'idle' | 'loading' | 'ready' | 'verifying' | 'verified'>('idle')
  const [challengeNote, setChallengeNote] = useState<BilingualCopy | null>(null)
  const [challengeNoteTone, setChallengeNoteTone] = useState<ChallengeNoteTone>('neutral')
  const [referralCode] = useState(() => normalizeReferralCode(new URLSearchParams(window.location.search).get('ref') || ''))

  const activeAuthMode: HubAuthChallengeMode = panelMode === 'register' ? 'register' : 'login'

  const emitLoginCharacterError = useCallback(() => {
    window.setTimeout(() => {
      rootRef.current?.dispatchEvent(new CustomEvent(LOGIN_ERROR_EVENT))
    }, 0)
  }, [])

  const rememberDraft = useCallback((patch: Partial<Omit<LoginInputDraft, 'version' | 'updatedAt'>>) => {
    const nextDraft = normalizeDraft({
      ...draftRef.current,
      ...patch,
      version: 1,
      updatedAt: new Date().toISOString()
    })
    draftRef.current = nextDraft
    writeDraft(nextDraft)
  }, [])

  const resetChallenge = useCallback(() => {
    authChallengeProofRef.current = ''
    setAuthChallengeProof('')
    setAuthChallenge(null)
    setSelectedChallengeTileIds([])
    setChallengeSuccessVisible(false)
    setChallengeStatus('idle')
    setChallengeNote(null)
    setChallengeNoteTone('neutral')
  }, [])

  const loadChallenge = useCallback(async (mode: HubAuthChallengeMode) => {
    const api = accountApi()
    if (!api) return
    authChallengeProofRef.current = ''
    setAuthChallengeProof('')
    setAuthChallenge(null)
    setSelectedChallengeTileIds([])
    setChallengeStatus('loading')
    setChallengeNote(null)
    setChallengeNoteTone('neutral')
    const result = await api.fetchChallenge(mode)
    if (!result.ok) {
      setChallengeStatus('ready')
      setChallengeNote({ primary: result.message, secondary: COPY.feedback.challengeRefreshFailed.secondary })
      setChallengeNoteTone('error')
      return
    }
    setAuthChallenge(result.challenge)
    setChallengeStatus('ready')
  }, [])

  const refreshChallengeRequirement = useCallback(async (mode: HubAuthChallengeMode, force = false) => {
    const api = accountApi()
    if (!api) return
    const result = await api.fetchChallengeState(mode)
    if (!result.ok) {
      setAuthChallengeRequired(false)
      resetChallenge()
      return
    }
    setAuthChallengeRequired(result.state.challengeRequired)
    if (!result.state.challengeRequired) {
      resetChallenge()
      return
    }
    if (force || !authChallengeProofRef.current) {
      await loadChallenge(mode)
    }
  }, [loadChallenge, resetChallenge])

  useEffect(() => {
    void refreshChallengeRequirement(activeAuthMode)
  }, [activeAuthMode, refreshChallengeRequirement])

  useEffect(() => {
    return () => {
      if (challengeOverlayTimerRef.current !== null) window.clearTimeout(challengeOverlayTimerRef.current)
      if (registerCooldownTimerRef.current !== null) window.clearTimeout(registerCooldownTimerRef.current)
      if (forgotPasswordCooldownTimerRef.current !== null) window.clearTimeout(forgotPasswordCooldownTimerRef.current)
    }
  }, [])

  useEffect(() => {
    if (stage !== 'entered' || panelMode !== 'login') return

    const root = rootRef.current
    if (!root) return

    const query = <T extends Element>(selector: string): T | null => root.querySelector(selector) as T | null
    const elements = {
      form: query<HTMLFormElement>('#login-form'),
      emailInput: query<HTMLInputElement>('#email'),
      passwordInput: query<HTMLInputElement>('#password'),
      toggleBtn: query<HTMLButtonElement>('#toggle-password'),
      eyeIcon: query<SVGSVGElement>('#eye-icon'),
      eyeOffIcon: query<SVGSVGElement>('#eye-off-icon'),
      scene: query<HTMLDivElement>('#characters-scene'),
      purple: query<HTMLDivElement>('#char-purple'),
      black: query<HTMLDivElement>('#char-black'),
      orange: query<HTMLDivElement>('#char-orange'),
      yellow: query<HTMLDivElement>('#char-yellow'),
      purpleEyes: query<HTMLDivElement>('#purple-eyes'),
      purpleEyeL: query<HTMLDivElement>('#purple-eye-l'),
      purpleEyeR: query<HTMLDivElement>('#purple-eye-r'),
      purplePupilL: query<HTMLDivElement>('#purple-pupil-l'),
      purplePupilR: query<HTMLDivElement>('#purple-pupil-r'),
      blackEyes: query<HTMLDivElement>('#black-eyes'),
      blackEyeL: query<HTMLDivElement>('#black-eye-l'),
      blackEyeR: query<HTMLDivElement>('#black-eye-r'),
      blackPupilL: query<HTMLDivElement>('#black-pupil-l'),
      blackPupilR: query<HTMLDivElement>('#black-pupil-r'),
      orangeEyes: query<HTMLDivElement>('#orange-eyes'),
      orangePupilL: query<HTMLDivElement>('#orange-pupil-l'),
      orangePupilR: query<HTMLDivElement>('#orange-pupil-r'),
      orangeMouth: query<HTMLDivElement>('#orange-mouth'),
      yellowEyes: query<HTMLDivElement>('#yellow-eyes'),
      yellowPupilL: query<HTMLDivElement>('#yellow-pupil-l'),
      yellowPupilR: query<HTMLDivElement>('#yellow-pupil-r'),
      yellowMouth: query<HTMLDivElement>('#yellow-mouth')
    }

    if (Object.values(elements).some((element) => element === null)) return

    const shakeElements = [
      elements.purpleEyes!,
      elements.blackEyes!,
      elements.orangeEyes!,
      elements.yellowEyes!,
      elements.yellowMouth!,
      elements.orangeMouth!
    ]
    const characterState = {
      mouseX: window.innerWidth / 2,
      mouseY: window.innerHeight / 2,
      isEmailFocused: false,
      isLookingAtEachOther: false,
      isPasswordFocused: false,
      isLoginError: false,
      isPurpleBlinking: false,
      isBlackBlinking: false,
      isPurplePeeking: false
    }
    const timers: Record<
      | 'emailReaction'
      | 'purpleBlinkStart'
      | 'purpleBlinkEnd'
      | 'blackBlinkStart'
      | 'blackBlinkEnd'
      | 'purplePeekStart'
      | 'purplePeekEnd'
      | 'shakeStart'
      | 'errorRecover',
      number | null
    > = {
      emailReaction: null,
      purpleBlinkStart: null,
      purpleBlinkEnd: null,
      blackBlinkStart: null,
      blackBlinkEnd: null,
      purplePeekStart: null,
      purplePeekEnd: null,
      shakeStart: null,
      errorRecover: null
    }
    let disposed = false
    const randomDelay = (base: number, spread: number): number => Math.random() * spread + base
    const clearTimer = (name: keyof typeof timers): void => {
      if (timers[name] === null) return
      window.clearTimeout(timers[name] as number)
      timers[name] = null
    }
    const setTimer = (name: keyof typeof timers, callback: () => void, delay: number): void => {
      clearTimer(name)
      timers[name] = window.setTimeout(() => {
        timers[name] = null
        if (!disposed) callback()
      }, delay)
    }
    const sceneIsVisible = (): boolean => elements.scene!.offsetParent !== null
    const passwordIsVisible = (): boolean => elements.passwordInput!.type === 'text'
    const calcPosition = (element: HTMLElement): { faceX: number; faceY: number; bodySkew: number } => {
      const rect = element.getBoundingClientRect()
      const cx = rect.left + rect.width / 2
      const cy = rect.top + rect.height / 3
      const dx = characterState.mouseX - cx
      const dy = characterState.mouseY - cy
      return {
        faceX: Math.max(-15, Math.min(15, dx / 20)),
        faceY: Math.max(-10, Math.min(10, dy / 30)),
        bodySkew: Math.max(-6, Math.min(6, -dx / 120))
      }
    }
    const calcPupilOffset = (element: HTMLElement, maxDist: number): { x: number; y: number } => {
      const rect = element.getBoundingClientRect()
      const cx = rect.left + rect.width / 2
      const cy = rect.top + rect.height / 2
      const dx = characterState.mouseX - cx
      const dy = characterState.mouseY - cy
      const dist = Math.min(Math.sqrt(dx * dx + dy * dy), maxDist)
      const angle = Math.atan2(dy, dx)
      return { x: Math.cos(angle) * dist, y: Math.sin(angle) * dist }
    }
    const syncPasswordToggleAffordance = (): void => {
      const visible = passwordIsVisible()
      elements.eyeIcon!.style.display = visible ? 'none' : 'block'
      elements.eyeOffIcon!.style.display = visible ? 'block' : 'none'
      elements.toggleBtn!.setAttribute(
        'aria-label',
        visible
          ? `${COPY.hidePassword.primary} ${COPY.hidePassword.secondary}`
          : `${COPY.showPassword.primary} ${COPY.showPassword.secondary}`
      )
      elements.toggleBtn!.setAttribute('aria-pressed', String(visible))
    }
    const updateCharacters = (): void => {
      if (!sceneIsVisible()) return
      syncPasswordToggleAffordance()
      const purplePos = calcPosition(elements.purple!)
      const blackPos = calcPosition(elements.black!)
      const orangePos = calcPosition(elements.orange!)
      const yellowPos = calcPosition(elements.yellow!)
      const isShowingPassword = elements.passwordInput!.value.length > 0 && passwordIsVisible()
      const isLookingAway = characterState.isPasswordFocused && !passwordIsVisible()

      if (isShowingPassword) {
        elements.purple!.style.transform = 'skewX(0deg)'
        elements.purple!.style.height = '370px'
      } else if (isLookingAway) {
        elements.purple!.style.transform = 'skewX(-14deg) translateX(-20px)'
        elements.purple!.style.height = '410px'
      } else if (characterState.isEmailFocused) {
        elements.purple!.style.transform = `skewX(${purplePos.bodySkew - 12}deg) translateX(40px)`
        elements.purple!.style.height = '410px'
      } else {
        elements.purple!.style.transform = `skewX(${purplePos.bodySkew}deg)`
        elements.purple!.style.height = '370px'
      }

      elements.purpleEyeL!.style.height = characterState.isPurpleBlinking ? '2px' : '18px'
      elements.purpleEyeR!.style.height = characterState.isPurpleBlinking ? '2px' : '18px'
      if (characterState.isLoginError) {
        elements.purpleEyes!.style.left = '30px'
        elements.purpleEyes!.style.top = '55px'
        elements.purplePupilL!.style.transform = 'translate(-3px, 4px)'
        elements.purplePupilR!.style.transform = 'translate(-3px, 4px)'
      } else if (isLookingAway) {
        elements.purpleEyes!.style.left = '20px'
        elements.purpleEyes!.style.top = '25px'
        elements.purplePupilL!.style.transform = 'translate(-5px, -5px)'
        elements.purplePupilR!.style.transform = 'translate(-5px, -5px)'
      } else if (isShowingPassword) {
        elements.purpleEyes!.style.left = '20px'
        elements.purpleEyes!.style.top = '35px'
        const pupilX = characterState.isPurplePeeking ? 4 : -4
        const pupilY = characterState.isPurplePeeking ? 5 : -4
        elements.purplePupilL!.style.transform = `translate(${pupilX}px, ${pupilY}px)`
        elements.purplePupilR!.style.transform = `translate(${pupilX}px, ${pupilY}px)`
      } else if (characterState.isLookingAtEachOther) {
        elements.purpleEyes!.style.left = '55px'
        elements.purpleEyes!.style.top = '65px'
        elements.purplePupilL!.style.transform = 'translate(3px, 4px)'
        elements.purplePupilR!.style.transform = 'translate(3px, 4px)'
      } else {
        elements.purpleEyes!.style.left = `${45 + purplePos.faceX}px`
        elements.purpleEyes!.style.top = `${40 + purplePos.faceY}px`
        const offset = calcPupilOffset(elements.purpleEyeL!, 5)
        elements.purplePupilL!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
        elements.purplePupilR!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
      }

      if (isShowingPassword) {
        elements.black!.style.transform = 'skewX(0deg)'
      } else if (isLookingAway) {
        elements.black!.style.transform = 'skewX(12deg) translateX(-10px)'
      } else if (characterState.isLookingAtEachOther) {
        elements.black!.style.transform = `skewX(${blackPos.bodySkew * 1.5 + 10}deg) translateX(20px)`
      } else if (characterState.isEmailFocused) {
        elements.black!.style.transform = `skewX(${blackPos.bodySkew * 1.5}deg)`
      } else {
        elements.black!.style.transform = `skewX(${blackPos.bodySkew}deg)`
      }

      elements.blackEyeL!.style.height = characterState.isBlackBlinking ? '2px' : '16px'
      elements.blackEyeR!.style.height = characterState.isBlackBlinking ? '2px' : '16px'
      if (characterState.isLoginError) {
        elements.blackEyes!.style.left = '15px'
        elements.blackEyes!.style.top = '40px'
        elements.blackPupilL!.style.transform = 'translate(-3px, 4px)'
        elements.blackPupilR!.style.transform = 'translate(-3px, 4px)'
      } else if (isLookingAway) {
        elements.blackEyes!.style.left = '10px'
        elements.blackEyes!.style.top = '20px'
        elements.blackPupilL!.style.transform = 'translate(-4px, -5px)'
        elements.blackPupilR!.style.transform = 'translate(-4px, -5px)'
      } else if (isShowingPassword) {
        elements.blackEyes!.style.left = '10px'
        elements.blackEyes!.style.top = '28px'
        elements.blackPupilL!.style.transform = 'translate(-4px, -4px)'
        elements.blackPupilR!.style.transform = 'translate(-4px, -4px)'
      } else if (characterState.isLookingAtEachOther) {
        elements.blackEyes!.style.left = '32px'
        elements.blackEyes!.style.top = '12px'
        elements.blackPupilL!.style.transform = 'translate(0px, -4px)'
        elements.blackPupilR!.style.transform = 'translate(0px, -4px)'
      } else {
        elements.blackEyes!.style.left = `${26 + blackPos.faceX}px`
        elements.blackEyes!.style.top = `${32 + blackPos.faceY}px`
        const offset = calcPupilOffset(elements.blackEyeL!, 4)
        elements.blackPupilL!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
        elements.blackPupilR!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
      }

      if (characterState.isLoginError) {
        elements.orangeMouth!.style.left = `${80 + orangePos.faceX}px`
        elements.orangeMouth!.style.top = '130px'
      }
      elements.orange!.style.transform = isShowingPassword ? 'skewX(0deg)' : `skewX(${orangePos.bodySkew}deg)`
      if (characterState.isLoginError) {
        elements.orangeEyes!.style.left = '60px'
        elements.orangeEyes!.style.top = '95px'
        elements.orangePupilL!.style.transform = 'translate(-3px, 4px)'
        elements.orangePupilR!.style.transform = 'translate(-3px, 4px)'
      } else if (isLookingAway) {
        elements.orangeEyes!.style.left = '50px'
        elements.orangeEyes!.style.top = '75px'
        elements.orangePupilL!.style.transform = 'translate(-5px, -5px)'
        elements.orangePupilR!.style.transform = 'translate(-5px, -5px)'
      } else if (isShowingPassword) {
        elements.orangeEyes!.style.left = '50px'
        elements.orangeEyes!.style.top = '85px'
        elements.orangePupilL!.style.transform = 'translate(-5px, -4px)'
        elements.orangePupilR!.style.transform = 'translate(-5px, -4px)'
      } else {
        elements.orangeEyes!.style.left = `${82 + orangePos.faceX}px`
        elements.orangeEyes!.style.top = `${90 + orangePos.faceY}px`
        const offset = calcPupilOffset(elements.orangePupilL!, 5)
        elements.orangePupilL!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
        elements.orangePupilR!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
      }

      elements.yellow!.style.transform = isShowingPassword ? 'skewX(0deg)' : `skewX(${yellowPos.bodySkew}deg)`
      if (characterState.isLoginError) {
        elements.yellowEyes!.style.left = '35px'
        elements.yellowEyes!.style.top = '45px'
        elements.yellowPupilL!.style.transform = 'translate(-3px, 4px)'
        elements.yellowPupilR!.style.transform = 'translate(-3px, 4px)'
        elements.yellowMouth!.style.left = '30px'
        elements.yellowMouth!.style.top = '92px'
        elements.yellowMouth!.style.transform = 'rotate(-8deg)'
      } else if (isLookingAway) {
        elements.yellowEyes!.style.left = '20px'
        elements.yellowEyes!.style.top = '30px'
        elements.yellowPupilL!.style.transform = 'translate(-5px, -5px)'
        elements.yellowPupilR!.style.transform = 'translate(-5px, -5px)'
        elements.yellowMouth!.style.left = '15px'
        elements.yellowMouth!.style.top = '78px'
        elements.yellowMouth!.style.transform = 'rotate(0deg)'
      } else if (isShowingPassword) {
        elements.yellowEyes!.style.left = '20px'
        elements.yellowEyes!.style.top = '35px'
        elements.yellowPupilL!.style.transform = 'translate(-5px, -4px)'
        elements.yellowPupilR!.style.transform = 'translate(-5px, -4px)'
        elements.yellowMouth!.style.left = '10px'
        elements.yellowMouth!.style.top = '88px'
        elements.yellowMouth!.style.transform = 'rotate(0deg)'
      } else {
        elements.yellowEyes!.style.left = `${52 + yellowPos.faceX}px`
        elements.yellowEyes!.style.top = `${40 + yellowPos.faceY}px`
        const offset = calcPupilOffset(elements.yellowPupilL!, 5)
        elements.yellowPupilL!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
        elements.yellowPupilR!.style.transform = `translate(${offset.x}px, ${offset.y}px)`
        elements.yellowMouth!.style.left = `${40 + yellowPos.faceX}px`
        elements.yellowMouth!.style.top = `${88 + yellowPos.faceY}px`
        elements.yellowMouth!.style.transform = 'rotate(0deg)'
      }
    }
    const shouldPurplePeek = (): boolean =>
      passwordIsVisible() &&
      elements.passwordInput!.value.length > 0 &&
      !characterState.isLoginError
    const stopPurplePeek = (): void => {
      clearTimer('purplePeekStart')
      clearTimer('purplePeekEnd')
      if (!characterState.isPurplePeeking) return
      characterState.isPurplePeeking = false
      updateCharacters()
    }
    const schedulePurplePeek = (): void => {
      if (!shouldPurplePeek() || timers.purplePeekStart || timers.purplePeekEnd) return
      setTimer('purplePeekStart', () => {
        if (!shouldPurplePeek()) return
        characterState.isPurplePeeking = true
        updateCharacters()
        setTimer('purplePeekEnd', () => {
          characterState.isPurplePeeking = false
          updateCharacters()
          schedulePurplePeek()
        }, PEEK_DURATION_MS)
      }, randomDelay(PEEK_MIN_MS, PEEK_JITTER_MS))
    }
    const syncPurplePeek = (): void => {
      if (shouldPurplePeek()) {
        schedulePurplePeek()
      } else {
        stopPurplePeek()
      }
    }
    const refreshEmailReaction = (): void => {
      if (!characterState.isEmailFocused) return
      characterState.isLookingAtEachOther = true
      setTimer('emailReaction', () => {
        characterState.isLookingAtEachOther = false
        updateCharacters()
      }, LOOK_AT_EACH_OTHER_MS)
      updateCharacters()
    }
    const scheduleBlink = (
      startTimerKey: 'purpleBlinkStart' | 'blackBlinkStart',
      endTimerKey: 'purpleBlinkEnd' | 'blackBlinkEnd',
      flagKey: 'isPurpleBlinking' | 'isBlackBlinking'
    ): void => {
      setTimer(startTimerKey, () => {
        characterState[flagKey] = true
        updateCharacters()
        setTimer(endTimerKey, () => {
          characterState[flagKey] = false
          updateCharacters()
          scheduleBlink(startTimerKey, endTimerKey, flagKey)
        }, 150)
      }, randomDelay(BLINK_MIN_MS, BLINK_JITTER_MS))
    }
    const triggerLoginError = (): void => {
      clearTimer('shakeStart')
      clearTimer('errorRecover')
      stopPurplePeek()
      shakeElements.forEach((element) => element.classList.remove(SHAKE_CLASS))
      void root.offsetHeight
      characterState.isLoginError = true
      characterState.isPasswordFocused = false
      characterState.isLookingAtEachOther = false
      elements.orangeMouth!.classList.add('visible')
      updateCharacters()
      setTimer('shakeStart', () => {
        shakeElements.forEach((element) => element.classList.add(SHAKE_CLASS))
      }, SHAKE_DELAY_MS)
      setTimer('errorRecover', () => {
        characterState.isLoginError = false
        elements.orangeMouth!.classList.remove('visible')
        shakeElements.forEach((element) => element.classList.remove(SHAKE_CLASS))
        syncPurplePeek()
        updateCharacters()
      }, ERROR_RECOVERY_MS)
    }
    const handleMouseMove = (event: MouseEvent): void => {
      characterState.mouseX = event.clientX
      characterState.mouseY = event.clientY
      if (!characterState.isEmailFocused && !characterState.isLoginError) updateCharacters()
    }
    const handleEmailFocus = (): void => {
      characterState.isEmailFocused = true
      refreshEmailReaction()
    }
    const handleEmailBlur = (): void => {
      characterState.isEmailFocused = false
      characterState.isLookingAtEachOther = false
      clearTimer('emailReaction')
      updateCharacters()
    }
    const handleEmailInput = (): void => {
      refreshEmailReaction()
    }
    const handlePasswordFocus = (): void => {
      characterState.isPasswordFocused = true
      updateCharacters()
    }
    const handlePasswordBlur = (): void => {
      characterState.isPasswordFocused = false
      updateCharacters()
    }
    const handlePasswordInput = (): void => {
      syncPurplePeek()
      updateCharacters()
    }
    const handleTogglePassword = (): void => {
      window.setTimeout(() => {
        syncPurplePeek()
        updateCharacters()
      }, 0)
    }
    const handleLoginErrorEvent = (): void => {
      triggerLoginError()
    }

    document.addEventListener('mousemove', handleMouseMove)
    root.addEventListener(LOGIN_ERROR_EVENT, handleLoginErrorEvent)
    elements.emailInput!.addEventListener('focus', handleEmailFocus)
    elements.emailInput!.addEventListener('blur', handleEmailBlur)
    elements.emailInput!.addEventListener('input', handleEmailInput)
    elements.passwordInput!.addEventListener('focus', handlePasswordFocus)
    elements.passwordInput!.addEventListener('blur', handlePasswordBlur)
    elements.passwordInput!.addEventListener('input', handlePasswordInput)
    elements.toggleBtn!.addEventListener('click', handleTogglePassword)
    scheduleBlink('purpleBlinkStart', 'purpleBlinkEnd', 'isPurpleBlinking')
    scheduleBlink('blackBlinkStart', 'blackBlinkEnd', 'isBlackBlinking')
    syncPasswordToggleAffordance()
    updateCharacters()

    return () => {
      disposed = true
      ;(Object.keys(timers) as Array<keyof typeof timers>).forEach(clearTimer)
      document.removeEventListener('mousemove', handleMouseMove)
      root.removeEventListener(LOGIN_ERROR_EVENT, handleLoginErrorEvent)
      elements.emailInput!.removeEventListener('focus', handleEmailFocus)
      elements.emailInput!.removeEventListener('blur', handleEmailBlur)
      elements.emailInput!.removeEventListener('input', handleEmailInput)
      elements.passwordInput!.removeEventListener('focus', handlePasswordFocus)
      elements.passwordInput!.removeEventListener('blur', handlePasswordBlur)
      elements.passwordInput!.removeEventListener('input', handlePasswordInput)
      elements.toggleBtn!.removeEventListener('click', handleTogglePassword)
    }
  }, [panelMode, stage])

  useEffect(() => {
    if (registerCooldownTimerRef.current !== null) window.clearTimeout(registerCooldownTimerRef.current)
    const remainingMs = registerCooldownUntil - Date.now()
    if (remainingMs <= 0) return
    registerCooldownTimerRef.current = window.setTimeout(() => {
      setRegisterCooldownTick((current) => current + 1)
      registerCooldownTimerRef.current = null
    }, Math.min(1000, remainingMs))
  }, [registerCooldownTick, registerCooldownUntil])

  useEffect(() => {
    if (forgotPasswordCooldownTimerRef.current !== null) window.clearTimeout(forgotPasswordCooldownTimerRef.current)
    const remainingMs = forgotPasswordCooldownUntil - Date.now()
    if (remainingMs <= 0) return
    forgotPasswordCooldownTimerRef.current = window.setTimeout(() => {
      setForgotPasswordCooldownTick((current) => current + 1)
      forgotPasswordCooldownTimerRef.current = null
    }, Math.min(1000, remainingMs))
  }, [forgotPasswordCooldownTick, forgotPasswordCooldownUntil])

  const showRegister = () => {
    setRegisterEmail(loginEmail)
    rememberDraft({ registerEmail: loginEmail })
    setRegisterMessage(null)
    setPanelMode('register')
  }

  const showForgotPassword = () => {
    setForgotPasswordEmail(loginEmail)
    rememberDraft({ forgotPasswordEmail: loginEmail })
    setForgotPasswordMessage(null)
    setPanelMode('forgot-password')
  }

  const returnToLogin = (email = '') => {
    const nextEmail = email.trim()
    if (nextEmail) {
      setLoginEmail(nextEmail)
      rememberDraft({ loginEmail: nextEmail })
    }
    setPanelMode('login')
    setLoginError('')
  }

  const ensureChallengeProof = async (): Promise<boolean> => {
    if (!authChallengeRequired) return true
    if (authChallengeProofRef.current) return true
    setChallengeNote(COPY.feedback.missingChallenge)
    setChallengeNoteTone('error')
    if (!authChallenge) await loadChallenge(activeAuthMode)
    return false
  }

  const handleLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (loginSubmitting) return
    const email = loginEmail.trim().toLowerCase()
    if (!isValidEmail(email)) {
      setLoginError(COPY.feedback.invalidEmail.primary)
      emitLoginCharacterError()
      return
    }
    if (loginPassword.length < 6) {
      setLoginError(COPY.feedback.invalidPassword.primary)
      emitLoginCharacterError()
      return
    }
    if (!await ensureChallengeProof()) return
    rememberDraft({ loginEmail: email })
    setLoginSubmitting(true)
    setLoginError('')
    try {
      const snapshot = await login({
        email,
        password: loginPassword,
        authChallengeProof: authChallengeProofRef.current,
        rememberForDays: rememberLogin ? HUB_AUTH_REMEMBER_DAYS : 0
      })
      await onAuthenticated(snapshot)
    } catch (error) {
      const message = messageFromError(error)
      setLoginError(message || COPY.feedback.invalidCredentials.primary)
      emitLoginCharacterError()
      authChallengeProofRef.current = ''
      setAuthChallengeProof('')
      await refreshChallengeRequirement('login', /图片|验证|challenge/i.test(message))
    } finally {
      setLoginSubmitting(false)
    }
  }

  const handleRegisterSendCode = async () => {
    if (registerSendingCode || registerCooldownUntil > Date.now()) return
    const email = registerEmail.trim().toLowerCase()
    if (!isValidEmail(email)) {
      setRegisterMessage(COPY.feedback.invalidEmail)
      setRegisterMessageTone('error')
      return
    }
    if (!await ensureChallengeProof()) return
    setRegisterSendingCode(true)
    try {
      const result = await accountApi()?.sendRegisterVerificationCode({
        email,
        authChallengeProof: authChallengeProofRef.current
      })
      if (!result?.ok) throw new Error(result?.message || '验证码发送失败。')
      setRegisterMessage(COPY.feedback.verificationCodeSent)
      setRegisterMessageTone('success')
      setRegisterCooldownUntil(Date.now() + Math.max(30, result.retryAfterSeconds || 60) * 1000)
    } catch (error) {
      setRegisterMessage({ primary: messageFromError(error), secondary: 'Failed to send verification code.' })
      setRegisterMessageTone('error')
      await refreshChallengeRequirement('register', true)
    } finally {
      setRegisterSendingCode(false)
    }
  }

  const handleRegisterSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (registerSubmitting) return
    const fullName = registerFullName.trim()
    const phoneNumber = registerPhoneNumber.trim()
    const email = registerEmail.trim().toLowerCase()
    const verificationCode = registerVerificationCode.replace(/\D/g, '').slice(0, 6)
    if (fullName.length < 2) {
      setRegisterMessage(COPY.feedback.invalidName)
      setRegisterMessageTone('error')
      return
    }
    if (!isValidContactPhone(phoneNumber)) {
      setRegisterMessage(COPY.feedback.invalidPhone)
      setRegisterMessageTone('error')
      return
    }
    if (!isValidEmail(email)) {
      setRegisterMessage(COPY.feedback.invalidEmail)
      setRegisterMessageTone('error')
      return
    }
    if (verificationCode.length !== 6) {
      setRegisterMessage(COPY.feedback.invalidVerificationCode)
      setRegisterMessageTone('error')
      return
    }
    if (registerPassword.length < 6) {
      setRegisterMessage(COPY.feedback.invalidPassword)
      setRegisterMessageTone('error')
      return
    }
    if (!registerAgreement) {
      setRegisterMessage(COPY.registerAgreement)
      setRegisterMessageTone('error')
      return
    }
    if (!await ensureChallengeProof()) return
    setRegisterSubmitting(true)
    try {
      rememberDraft({
        registerFullName: fullName,
        registerOrganization,
        registerPhoneNumber: phoneNumber,
        registerEmail: email
      })
      const snapshot = await register({
        fullName,
        organization: registerOrganization.trim(),
        phoneNumber,
        email,
        password: registerPassword,
        verificationCode,
        authChallengeProof: authChallengeProofRef.current,
        referralCode,
        rememberForDays: HUB_AUTH_REMEMBER_DAYS
      })
      await onAuthenticated(snapshot)
    } catch (error) {
      setRegisterMessage({ primary: messageFromError(error), secondary: 'Account creation failed.' })
      setRegisterMessageTone('error')
      authChallengeProofRef.current = ''
      setAuthChallengeProof('')
      await refreshChallengeRequirement('register', true)
    } finally {
      setRegisterSubmitting(false)
    }
  }

  const handleForgotPasswordSendCode = async () => {
    if (forgotPasswordSendingCode || forgotPasswordCooldownUntil > Date.now()) return
    const email = forgotPasswordEmail.trim().toLowerCase()
    if (!isValidEmail(email)) {
      setForgotPasswordMessage(COPY.feedback.invalidEmail)
      setForgotPasswordMessageTone('error')
      return
    }
    setForgotPasswordSendingCode(true)
    try {
      const result = await accountApi()?.sendPasswordResetVerificationCode({ email })
      if (!result?.ok) throw new Error(result?.message || '验证码发送失败。')
      setForgotPasswordMessage(COPY.feedback.forgotPasswordCodeSent)
      setForgotPasswordMessageTone('success')
      setForgotPasswordCooldownUntil(Date.now() + Math.max(30, result.retryAfterSeconds || 60) * 1000)
    } catch (error) {
      setForgotPasswordMessage({ primary: messageFromError(error), secondary: 'Failed to send verification code.' })
      setForgotPasswordMessageTone('error')
    } finally {
      setForgotPasswordSendingCode(false)
    }
  }

  const handleForgotPasswordConfirm = async () => {
    if (forgotPasswordSubmitting) return
    const email = forgotPasswordEmail.trim().toLowerCase()
    const verificationCode = forgotPasswordCode.replace(/\D/g, '').slice(0, 6)
    if (!isValidEmail(email)) {
      setForgotPasswordMessage(COPY.feedback.invalidEmail)
      setForgotPasswordMessageTone('error')
      return
    }
    if (verificationCode.length !== 6) {
      setForgotPasswordMessage(COPY.feedback.invalidVerificationCode)
      setForgotPasswordMessageTone('error')
      return
    }
    if (forgotPasswordNextPassword.length < 6) {
      setForgotPasswordMessage(COPY.feedback.invalidPassword)
      setForgotPasswordMessageTone('error')
      return
    }
    if (forgotPasswordNextPassword !== forgotPasswordConfirmPassword) {
      setForgotPasswordMessage(COPY.feedback.passwordMismatch)
      setForgotPasswordMessageTone('error')
      return
    }
    setForgotPasswordSubmitting(true)
    try {
      const result = await accountApi()?.confirmPasswordReset({
        email,
        verificationCode,
        nextPassword: forgotPasswordNextPassword
      })
      if (!result?.ok) throw new Error(result?.message || '重置密码失败。')
      setForgotPasswordMessage(COPY.feedback.forgotPasswordResetSuccess)
      setForgotPasswordMessageTone('success')
      window.setTimeout(() => returnToLogin(email), 1400)
    } catch (error) {
      setForgotPasswordMessage({ primary: messageFromError(error), secondary: 'Password reset failed.' })
      setForgotPasswordMessageTone('error')
    } finally {
      setForgotPasswordSubmitting(false)
    }
  }

  const toggleChallengeTile = (tileId: string): void => {
    if (challengeStatus === 'loading' || challengeStatus === 'verifying') return
    setSelectedChallengeTileIds((current) =>
      current.includes(tileId)
        ? current.filter((item) => item !== tileId)
        : [...current, tileId].sort()
    )
    authChallengeProofRef.current = ''
    setAuthChallengeProof('')
    setChallengeSuccessVisible(false)
    setChallengeStatus('ready')
    setChallengeNote(null)
  }

  const submitChallengeVerification = async () => {
    if (!authChallenge || challengeStatus === 'loading' || challengeStatus === 'verifying') return
    if (selectedChallengeTileIds.length === 0) {
      setChallengeNote(COPY.feedback.challengeSelectionRequired)
      setChallengeNoteTone('error')
      return
    }
    setChallengeStatus('verifying')
    const result = await accountApi()?.verifyChallenge({
      mode: activeAuthMode,
      challengeId: authChallenge.challengeId,
      selectedTileIds: selectedChallengeTileIds
    })
    if (!result?.ok) {
      setChallengeStatus('ready')
      setChallengeNote({ primary: result?.message || COPY.feedback.challengeRefreshFailed.primary, secondary: 'Verification failed.' })
      setChallengeNoteTone('error')
      return
    }
    authChallengeProofRef.current = result.verification.proofToken
    setAuthChallengeProof(result.verification.proofToken)
    setChallengeSuccessVisible(true)
    setChallengeStatus('verified')
    setChallengeNote(COPY.feedback.challengeVerified)
    setChallengeNoteTone('success')
    if (challengeOverlayTimerRef.current !== null) window.clearTimeout(challengeOverlayTimerRef.current)
    challengeOverlayTimerRef.current = window.setTimeout(() => {
      setChallengeSuccessVisible(false)
      challengeOverlayTimerRef.current = null
    }, 950)
  }

  const registerCooldownRemainingSeconds = Math.max(0, Math.ceil((registerCooldownUntil - Date.now()) / 1000))
  const forgotPasswordCooldownRemainingSeconds = Math.max(0, Math.ceil((forgotPasswordCooldownUntil - Date.now()) / 1000))
  const registerSendCodeCopy = registerSendingCode
    ? COPY.sendingVerificationCode
    : registerCooldownRemainingSeconds > 0
      ? { primary: `${registerCooldownRemainingSeconds}s 后重发`, secondary: 'Wait' }
      : registerCooldownUntil > 0
        ? COPY.resendVerificationCode
        : COPY.sendVerificationCode
  const forgotPasswordSendCodeCopy = forgotPasswordSendingCode
    ? COPY.sendingVerificationCode
    : forgotPasswordCooldownRemainingSeconds > 0
      ? { primary: `${forgotPasswordCooldownRemainingSeconds}s 后重发`, secondary: 'Wait' }
      : forgotPasswordCooldownUntil > 0
        ? COPY.resendVerificationCode
        : COPY.sendVerificationCode
  const challengeOverlayVisible =
    authChallengeRequired &&
    (panelMode === 'login' || panelMode === 'register') &&
    (!authChallengeProof || challengeSuccessVisible)
  const activeLegalContent = activeLegalPanel ? LEGAL_PANELS[activeLegalPanel] : null

  return (
    <div
      ref={rootRef}
      className={[
        'analytix-animated-login',
        'is-mode-interactive',
        `is-stage-${stage}`,
        `is-panel-${panelMode}`,
        className
      ].filter(Boolean).join(' ')}
    >
      <div className="login-window-drag-region" aria-hidden="true" />
      <div className="login-page">
        <div className="left-panel">
          <div className="logo">
            <span className="logo-mark-shell" aria-hidden="true">
              <img className="login-brand-symbol" src={analytixAppIcon} alt="" draggable={false} />
            </span>
            <img className="login-brand-wordmark" src={analytixWordmark} alt="Analytix" draggable={false} />
          </div>
          <div className="characters-wrapper">
            <div className="characters-scene" id="characters-scene">
              <div className="character char-purple" id="char-purple">
                <div className="eyes" id="purple-eyes" style={{ left: 45, top: 40, gap: 28 }}>
                  <div className="eyeball" id="purple-eye-l" style={{ width: 18, height: 18 }}><div className="pupil" id="purple-pupil-l" style={{ width: 7, height: 7 }} /></div>
                  <div className="eyeball" id="purple-eye-r" style={{ width: 18, height: 18 }}><div className="pupil" id="purple-pupil-r" style={{ width: 7, height: 7 }} /></div>
                </div>
              </div>
              <div className="character char-black" id="char-black">
                <div className="eyes" id="black-eyes" style={{ left: 26, top: 32, gap: 20 }}>
                  <div className="eyeball" id="black-eye-l" style={{ width: 16, height: 16 }}><div className="pupil" id="black-pupil-l" style={{ width: 6, height: 6 }} /></div>
                  <div className="eyeball" id="black-eye-r" style={{ width: 16, height: 16 }}><div className="pupil" id="black-pupil-r" style={{ width: 6, height: 6 }} /></div>
                </div>
              </div>
              <div className="character char-orange" id="char-orange">
                <div className="eyes" id="orange-eyes" style={{ left: 82, top: 90, gap: 28 }}>
                  <div className="bare-pupil" id="orange-pupil-l" />
                  <div className="bare-pupil" id="orange-pupil-r" />
                </div>
                <div className="orange-mouth" id="orange-mouth" style={{ left: 90, top: 120 }} />
              </div>
              <div className="character char-yellow" id="char-yellow">
                <div className="eyes" id="yellow-eyes" style={{ left: 52, top: 40, gap: 20 }}>
                  <div className="bare-pupil" id="yellow-pupil-l" />
                  <div className="bare-pupil" id="yellow-pupil-r" />
                </div>
                <div className="yellow-mouth" id="yellow-mouth" style={{ left: 40, top: 88 }} />
              </div>
            </div>
          </div>
          <div className="footer-links">
            <button type="button" className="footer-link-button" onClick={() => setActiveLegalPanel('privacy')}><TextPair {...COPY.privacy} /></button>
            <button type="button" className="footer-link-button" onClick={() => setActiveLegalPanel('terms')}><TextPair {...COPY.terms} /></button>
            <button type="button" className="footer-link-button" onClick={() => setActiveLegalPanel('contact')}><TextPair {...COPY.contact} /></button>
          </div>
        </div>
        <div className="right-panel">
          <div className="form-container">
            {panelMode === 'register' ? null : <div className="sparkle-icon" aria-hidden="true">✦</div>}
            {panelMode === 'register' ? (
              <div className="auth-inline-panel">
                <div className="form-header compact-form-header">
                  <h1 className="login-heading">{COPY.registerHeading.primary}</h1>
                  <TextPair {...COPY.registerDescription} className="header-description-copy" />
                </div>
                <form className="auth-inline-form" onSubmit={(event) => void handleRegisterSubmit(event)} noValidate>
                  <div className="form-group">
                    <label htmlFor="register-full-name"><TextPair {...COPY.nameLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="register-full-name" value={registerFullName} onChange={(event) => { setRegisterFullName(event.target.value); rememberDraft({ registerFullName: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="register-organization"><TextPair {...COPY.organizationLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="register-organization" value={registerOrganization} onChange={(event) => { setRegisterOrganization(event.target.value); rememberDraft({ registerOrganization: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="register-phone-number"><TextPair {...COPY.phoneLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="register-phone-number" value={registerPhoneNumber} onChange={(event) => { setRegisterPhoneNumber(event.target.value); rememberDraft({ registerPhoneNumber: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="register-email"><TextPair {...COPY.emailLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="register-email" type="email" value={registerEmail} onChange={(event) => { setRegisterEmail(event.target.value); rememberDraft({ registerEmail: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="register-verification-code"><TextPair {...COPY.verificationCodeLabel} className="field-label-copy" /></label>
                    <div className="verification-code-row">
                      <div className="input-wrapper verification-code-input"><input id="register-verification-code" inputMode="numeric" value={registerVerificationCode} onChange={(event) => setRegisterVerificationCode(event.target.value.replace(/\D/g, '').slice(0, 6))} data-copy-allowed="true" /></div>
                      <button type="button" className="send-code-button" onClick={() => void handleRegisterSendCode()} disabled={registerSendingCode || registerCooldownRemainingSeconds > 0}><TextPair {...registerSendCodeCopy} /></button>
                    </div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="register-password"><TextPair {...COPY.passwordLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper">
                      <input id="register-password" type={registerPasswordVisible ? 'text' : 'password'} value={registerPassword} onChange={(event) => setRegisterPassword(event.target.value)} data-copy-allowed="true" />
                      <button type="button" className="toggle-password" onClick={() => setRegisterPasswordVisible((value) => !value)}>{registerPasswordVisible ? '隐藏' : '显示'}</button>
                    </div>
                  </div>
                  <label className="remember-me register-agreement">
                    <input type="checkbox" checked={registerAgreement} onChange={(event) => setRegisterAgreement(event.target.checked)} />
                    <TextPair {...COPY.registerAgreement} />
                  </label>
                  <div className={['auth-inline-message', registerMessage ? 'is-visible' : '', registerMessageTone === 'success' ? 'is-success' : '', registerMessageTone === 'error' ? 'is-error' : ''].filter(Boolean).join(' ')} role="status">
                    {registerMessage ? <TextPair {...registerMessage} /> : null}
                  </div>
                  <button type="submit" className="btn-login auth-inline-primary" disabled={registerSubmitting}>
                    <span className="btn-text"><TextPair {...(registerSubmitting ? COPY.creatingAccount : COPY.createAccount)} className="button-copy" /></span>
                    <div className="btn-hover-content"><TextPair {...(registerSubmitting ? COPY.creatingAccount : COPY.createAccount)} className="button-copy" /></div>
                  </button>
                </form>
                <div className="signup-link">
                  <button type="button" className="auth-switch-link" onClick={() => returnToLogin(registerEmail.trim())}><TextPair {...COPY.backToLogin} /></button>
                </div>
              </div>
            ) : panelMode === 'forgot-password' ? (
              <div className="auth-inline-panel">
                <button type="button" className="license-import-back" onClick={() => returnToLogin(forgotPasswordEmail.trim())}><TextPair {...COPY.backToLogin} /></button>
                <div className="form-header compact-form-header">
                  <h1 className="login-heading">{COPY.forgotPasswordHeading.primary}</h1>
                  <TextPair {...COPY.forgotPasswordDescription} className="header-description-copy" />
                </div>
                <form className="auth-inline-form" noValidate>
                  <div className="form-group">
                    <label htmlFor="forgot-password-email"><TextPair {...COPY.emailLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="forgot-password-email" type="email" value={forgotPasswordEmail} onChange={(event) => { setForgotPasswordEmail(event.target.value); rememberDraft({ forgotPasswordEmail: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="forgot-password-code"><TextPair {...COPY.verificationCodeLabel} className="field-label-copy" /></label>
                    <div className="verification-code-row">
                      <div className="input-wrapper verification-code-input"><input id="forgot-password-code" inputMode="numeric" value={forgotPasswordCode} onChange={(event) => setForgotPasswordCode(event.target.value.replace(/\D/g, '').slice(0, 6))} data-copy-allowed="true" /></div>
                      <button type="button" className="send-code-button" onClick={() => void handleForgotPasswordSendCode()} disabled={forgotPasswordSendingCode || forgotPasswordCooldownRemainingSeconds > 0}><TextPair {...forgotPasswordSendCodeCopy} /></button>
                    </div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="forgot-password-next"><TextPair {...COPY.newPasswordLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="forgot-password-next" type="password" value={forgotPasswordNextPassword} onChange={(event) => setForgotPasswordNextPassword(event.target.value)} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="forgot-password-confirm"><TextPair {...COPY.confirmPasswordLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="forgot-password-confirm" type="password" value={forgotPasswordConfirmPassword} onChange={(event) => setForgotPasswordConfirmPassword(event.target.value)} data-copy-allowed="true" /></div>
                  </div>
                  <div className={['auth-inline-message', forgotPasswordMessage ? 'is-visible' : '', forgotPasswordMessageTone === 'success' ? 'is-success' : '', forgotPasswordMessageTone === 'error' ? 'is-error' : ''].filter(Boolean).join(' ')} role="status">
                    {forgotPasswordMessage ? <TextPair {...forgotPasswordMessage} /> : null}
                  </div>
                  <button type="button" className="btn-login auth-inline-primary" onClick={() => void handleForgotPasswordConfirm()} disabled={forgotPasswordSubmitting}>
                    <span className="btn-text"><TextPair {...(forgotPasswordSubmitting ? COPY.resettingPassword : COPY.resetPassword)} className="button-copy" /></span>
                    <div className="btn-hover-content"><TextPair {...(forgotPasswordSubmitting ? COPY.resettingPassword : COPY.resetPassword)} className="button-copy" /></div>
                  </button>
                </form>
              </div>
            ) : (
              <>
                <div className="form-header">
                  <h1 className="login-heading">欢迎回来</h1>
                  <TextPair {...COPY.headerDescription} className="header-description-copy" />
                </div>
                <form id="login-form" onSubmit={(event) => void handleLoginSubmit(event)} noValidate>
                  <div className="form-group">
                    <label htmlFor="email" id="email-label"><TextPair {...COPY.emailLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper"><input id="email" type="email" value={loginEmail} onChange={(event) => { setLoginEmail(event.target.value); setLoginError(''); rememberDraft({ loginEmail: event.target.value }) }} data-copy-allowed="true" /></div>
                  </div>
                  <div className="form-group">
                    <label htmlFor="password" id="password-label"><TextPair {...COPY.passwordLabel} className="field-label-copy" /></label>
                    <div className="input-wrapper">
                      <input id="password" type={passwordVisible ? 'text' : 'password'} value={loginPassword} onChange={(event) => { setLoginPassword(event.target.value); setLoginError('') }} data-copy-allowed="true" />
                      <button
                        type="button"
                        className="toggle-password"
                        id="toggle-password"
                        aria-label={passwordVisible ? `${COPY.hidePassword.primary} ${COPY.hidePassword.secondary}` : `${COPY.showPassword.primary} ${COPY.showPassword.secondary}`}
                        aria-controls="password"
                        aria-pressed={passwordVisible}
                        onClick={() => setPasswordVisible((value) => !value)}
                      >
                        <svg id="eye-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ display: passwordVisible ? 'none' : 'block' }}>
                          <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                          <circle cx="12" cy="12" r="3" />
                        </svg>
                        <svg id="eye-off-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ display: passwordVisible ? 'block' : 'none' }}>
                          <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                          <line x1="1" y1="1" x2="23" y2="23" />
                        </svg>
                      </button>
                    </div>
                  </div>
                  <div className="form-options">
                    <label className="remember-me">
                      <input id="remember-login" type="checkbox" checked={rememberLogin} onChange={(event) => setRememberLogin(event.target.checked)} />
                      <TextPair {...COPY.rememberMe} />
                    </label>
                    <button type="button" className="forgot-link" onClick={showForgotPassword}><TextPair {...COPY.forgotPassword} /></button>
                  </div>
                  <div className={['error-msg', loginError ? 'visible' : ''].filter(Boolean).join(' ')} id="error-msg" role="alert" aria-live="polite">{loginError}</div>
                  <button type="submit" className="btn-login" id="btn-login" disabled={loginSubmitting}>
                    <span className="btn-text"><TextPair {...(loginSubmitting ? COPY.loggingIn : COPY.login)} className="button-copy" /></span>
                    <div className="btn-hover-content"><TextPair {...(loginSubmitting ? COPY.loggingIn : COPY.login)} className="button-copy" /></div>
                  </button>
                </form>
                <div className="login-secondary-actions">
                  <button type="button" className="auth-switch-link" onClick={showRegister}>创建账号</button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
      {challengeOverlayVisible ? (
        <div className="auth-challenge-overlay" role="presentation" aria-hidden="false">
          <div className="auth-challenge-backdrop" />
          <div className={['auth-challenge-modal', challengeSuccessVisible ? 'is-success-mode' : ''].filter(Boolean).join(' ')} role="dialog" aria-modal="true" aria-labelledby="auth-challenge-title">
            <div className="auth-challenge-modal-head">
              <div>
                <div className="auth-challenge-kicker">Visual Check</div>
                <h2 className="auth-challenge-modal-title" id="auth-challenge-title">{challengeSuccessVisible ? '图片验证已通过' : '请完成图片验证'}</h2>
              </div>
              {!challengeSuccessVisible ? (
                <button type="button" className="challenge-refresh-button" onClick={() => void loadChallenge(activeAuthMode)} disabled={challengeStatus === 'loading' || challengeStatus === 'verifying'}>
                  <TextPair {...COPY.refreshChallenge} />
                </button>
              ) : null}
            </div>
            <div className="auth-challenge-modal-body">
              <TextPair {...(challengeSuccessVisible ? COPY.feedback.challengeVerified : authChallenge?.prompt ?? COPY.feedback.missingChallenge)} className="auth-challenge-copy" />
              {!challengeSuccessVisible ? (
                <div className="auth-challenge-grid" id="auth-challenge-grid" aria-live="polite">
                  {authChallenge?.tiles?.length ? authChallenge.tiles.map((tile, index) => {
                    const selected = selectedChallengeTileIds.includes(tile.id)
                    return (
                      <button key={tile.id} type="button" className={['auth-challenge-tile', selected ? 'is-selected' : ''].filter(Boolean).join(' ')} onClick={() => toggleChallengeTile(tile.id)} disabled={challengeStatus === 'loading' || challengeStatus === 'verifying'}>
                        <img alt="" src={tile.imageUri} />
                        <span className="auth-challenge-tile-badge">{index + 1}</span>
                      </button>
                    )
                  }) : Array.from({ length: 6 }).map((_, index) => <div key={`challenge-skeleton-${index}`} className="auth-challenge-skeleton" />)}
                </div>
              ) : (
                <div className="auth-challenge-success-panel">
                  <div className="auth-challenge-success-icon" aria-hidden="true">✓</div>
                  <p className="auth-challenge-success-text">验证浮层即将关闭，你可以继续操作。</p>
                </div>
              )}
            </div>
            <div className="auth-challenge-footer">
              {!challengeSuccessVisible ? (
                <button type="button" className={['challenge-verify-button', challengeStatus === 'verified' ? 'is-verified' : ''].filter(Boolean).join(' ')} onClick={() => void submitChallengeVerification()} disabled={challengeStatus === 'loading' || challengeStatus === 'verifying' || !authChallenge}>
                  <TextPair {...(challengeStatus === 'verifying' ? COPY.verifyingChallenge : COPY.verifyChallenge)} />
                </button>
              ) : null}
              <div className={['auth-challenge-note', challengeNote ? 'is-visible' : '', challengeNoteTone === 'success' ? 'is-success' : '', challengeNoteTone === 'error' ? 'is-error' : ''].filter(Boolean).join(' ')}>
                {challengeNote ? <TextPair {...challengeNote} /> : null}
              </div>
            </div>
          </div>
        </div>
      ) : null}
      {activeLegalContent ? (
        <div className="login-legal-overlay" role="presentation" onClick={() => setActiveLegalPanel(null)}>
          <section
            className="login-legal-panel"
            role="dialog"
            aria-modal="true"
            aria-labelledby="login-legal-title"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="login-legal-panel__header">
              <div className="login-legal-panel__eyebrow"><TextPair {...activeLegalContent.label} /></div>
              <button
                type="button"
                className="login-legal-panel__close"
                aria-label="关闭 Close"
                onClick={() => setActiveLegalPanel(null)}
              >
                <X size={18} strokeWidth={2.2} aria-hidden="true" />
              </button>
            </div>
            <div className="login-legal-panel__intro">
              <h2 id="login-legal-title">{activeLegalContent.label.primary}</h2>
              <p>{activeLegalContent.summary}</p>
              <button
                type="button"
                className="login-legal-panel__official-link"
                onClick={() => openExternal(activeLegalContent.officialUrl)}
              >
                <TextPair {...activeLegalContent.action} />
              </button>
            </div>
            <div className="login-legal-panel__body">
              {activeLegalContent.sections.map((section) => (
                <section key={section.heading} className="login-legal-section">
                  <h3>{section.heading}</h3>
                  {section.body.map((line) => <p key={line}>{line}</p>)}
                </section>
              ))}
            </div>
          </section>
        </div>
      ) : null}
    </div>
  )
}

export default AnimatedHubLoginPage
