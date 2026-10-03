import { createElement, forwardRef, type ComponentType, type SVGProps } from 'react'
import type { LucideProps } from 'lucide-react'
import Archive from './svg/Archive'
import ArrowCurvedLeft from './svg/ArrowCurvedLeft'
import ArrowDown from './svg/ArrowDown'
import ArrowLeft from './svg/ArrowLeft'
import ArrowRight from './svg/ArrowRight'
import ArrowRotateCcw from './svg/ArrowRotateCcw'
import ArrowRotateCw from './svg/ArrowRotateCw'
import ArrowUp from './svg/ArrowUp'
import AtSign from './svg/AtSign'
import AvatarProfile from './svg/AvatarProfile'
import BarChart from './svg/BarChart'
import Bell from './svg/Bell'
import BookOpen from './svg/BookOpen'
import Brain from './svg/Brain'
import Branch from './svg/Branch'
import BranchAlt from './svg/BranchAlt'
import Bug from './svg/Bug'
import Cabinet from './svg/Cabinet'
import Calendar from './svg/Calendar'
import Chat from './svg/Chat'
import ChatCompose from './svg/ChatCompose'
import ChatTripleDots from './svg/ChatTripleDots'
import Check from './svg/Check'
import CheckCircle from './svg/CheckCircle'
import ChevronDown from './svg/ChevronDown'
import ChevronLeft from './svg/ChevronLeft'
import ChevronRight from './svg/ChevronRight'
import ChevronUp from './svg/ChevronUp'
import CircleDashed from './svg/CircleDashed'
import ClappingBoardClosed from './svg/ClappingBoardClosed'
import Clock from './svg/Clock'
import Code from './svg/Code'
import CollapseLarge from './svg/CollapseLarge'
import ColorTheme from './svg/ColorTheme'
import Compare from './svg/Compare'
import CompareArrows from './svg/CompareArrows'
import ComposeEditSquare from './svg/ComposeEditSquare'
import Connect from './svg/Connect'
import Copy from './svg/Copy'
import CreditCard from './svg/CreditCard'
import Cube from './svg/Cube'
import Cursor from './svg/Cursor'
import Desktop from './svg/Desktop'
import Document from './svg/Document'
import DotsHorizontal from './svg/DotsHorizontal'
import Download from './svg/Download'
import EditPencil from './svg/EditPencil'
import EmptyCircle from './svg/EmptyCircle'
import Error from './svg/Error'
import ExclamationMarkCircle from './svg/ExclamationMarkCircle'
import ExitLogout from './svg/ExitLogout'
import Expand from './svg/Expand'
import ExpandLarge from './svg/ExpandLarge'
import ExternalLink from './svg/ExternalLink'
import Eye from './svg/Eye'
import EyeOff from './svg/EyeOff'
import FileBlank from './svg/FileBlank'
import FileCode from './svg/FileCode'
import FileDocument from './svg/FileDocument'
import FilePresentation from './svg/FilePresentation'
import FileSpreadsheet from './svg/FileSpreadsheet'
import Filter from './svg/Filter'
import Flask from './svg/Flask'
import Folder from './svg/Folder'
import FolderDocumentsFinder from './svg/FolderDocumentsFinder'
import FolderOpen from './svg/FolderOpen'
import FolderPlus from './svg/FolderPlus'
import GenerateSuggestedEdits from './svg/GenerateSuggestedEdits'
import Globe from './svg/Globe'
import Grid from './svg/Grid'
import HandRaised from './svg/HandRaised'
import Identity from './svg/Identity'
import ImageSquare from './svg/ImageSquare'
import InfoCircle from './svg/InfoCircle'
import Invoice from './svg/Invoice'
import Key from './svg/Key'
import Keyboard from './svg/Keyboard'
import Lightbulb from './svg/Lightbulb'
import Lock from './svg/Lock'
import LockKeyHole from './svg/LockKeyHole'
import MicLgDictate from './svg/MicLgDictate'
import Minus from './svg/Minus'
import Mobile from './svg/Mobile'
import Moon from './svg/Moon'
import Music from './svg/Music'
import Nodes from './svg/Nodes'
import NotebookPencil from './svg/NotebookPencil'
import On from './svg/On'
import PageBlank from './svg/PageBlank'
import Paperclip from './svg/Paperclip'
import PauseOutline from './svg/PauseOutline'
import PauseSm from './svg/PauseSm'
import Pin from './svg/Pin'
import PinFilled from './svg/PinFilled'
import PlayOutline from './svg/PlayOutline'
import PlaySm from './svg/PlaySm'
import Plugin from './svg/Plugin'
import PlusCircle from './svg/PlusCircle'
import PlusComposer from './svg/PlusComposer'
import PopOutWindow from './svg/PopOutWindow'
import PullRequestMerged from './svg/PullRequestMerged'
import Quote from './svg/Quote'
import QuoteReplyFilledQuoteXs from './svg/QuoteReplyFilledQuoteXs'
import Reload from './svg/Reload'
import RobotHead from './svg/RobotHead'
import Search from './svg/Search'
import SettingsCog from './svg/SettingsCog'
import SettingsSlider from './svg/SettingsSlider'
import SettingsWrench from './svg/SettingsWrench'
import Share from './svg/Share'
import ShieldCheck from './svg/ShieldCheck'
import Sidebar from './svg/Sidebar'
import SidebarCollapseRight from './svg/SidebarCollapseRight'
import SidebarOpenRightAlt from './svg/SidebarOpenRightAlt'
import SimpleSmile from './svg/SimpleSmile'
import Sparkles from './svg/Sparkles'
import Spelling from './svg/Spelling'
import Stack from './svg/Stack'
import Stop from './svg/Stop'
import Storage from './svg/Storage'
import Sun from './svg/Sun'
import Tasks from './svg/Tasks'
import Terminal from './svg/Terminal'
import Text from './svg/Text'
import Timer from './svg/Timer'
import Tools from './svg/Tools'
import Trash from './svg/Trash'
import Unarchive from './svg/Unarchive'
import Undo from './svg/Undo'
import Upgrade from './svg/Upgrade'
import Upscale from './svg/Upscale'
import User from './svg/User'
import Users from './svg/Users'
import Video from './svg/Video'
import Voice5BarsSoundwave from './svg/Voice5BarsSoundwave'
import Warning from './svg/Warning'
import Widget from './svg/Widget'
import X from './svg/X'
import XCircle from './svg/XCircle'

// Pinned public MIT vectors. Current ChatGPT Web/Codex pixel identity is not claimed.
// Preserve upstream filled-outline geometry; legacy Lucide stroke props must not repaint it.
function adaptOfficialIcon(Source: ComponentType<SVGProps<SVGSVGElement>>, name: string) {
  const Icon = forwardRef<SVGSVGElement, LucideProps>((props, ref) => {
    const svgProps = { ...props, 'data-analytix-icon': name }
    delete svgProps.size
    delete svgProps.strokeWidth
    delete svgProps.absoluteStrokeWidth
    delete svgProps.fill
    delete svgProps.stroke
    return createElement(Source, {
      ...svgProps, ref,
      'aria-hidden': props['aria-hidden'] ?? (props['aria-label'] || props['aria-labelledby'] || props.role === 'img' ? undefined : true),
      focusable: props.focusable ?? false,
      style: { ...props.style, fill: 'currentColor', stroke: 'none' },
      width: props.width ?? props.size ?? 20,
      height: props.height ?? props.size ?? 20,
      fill: 'currentColor', stroke: 'none'
    })
  })
  Icon.displayName = `AnalytixOfficial${name}`
  return Icon
}

export const OpenAiUiIcons = {
  archive: adaptOfficialIcon(Archive, 'Archive'),
  arrowCurvedLeft: adaptOfficialIcon(ArrowCurvedLeft, 'ArrowCurvedLeft'),
  arrowDown: adaptOfficialIcon(ArrowDown, 'ArrowDown'),
  arrowLeft: adaptOfficialIcon(ArrowLeft, 'ArrowLeft'),
  arrowRight: adaptOfficialIcon(ArrowRight, 'ArrowRight'),
  arrowRotateCcw: adaptOfficialIcon(ArrowRotateCcw, 'ArrowRotateCcw'),
  arrowRotateCw: adaptOfficialIcon(ArrowRotateCw, 'ArrowRotateCw'),
  arrowUp: adaptOfficialIcon(ArrowUp, 'ArrowUp'),
  atSign: adaptOfficialIcon(AtSign, 'AtSign'),
  avatarProfile: adaptOfficialIcon(AvatarProfile, 'AvatarProfile'),
  barChart: adaptOfficialIcon(BarChart, 'BarChart'),
  bell: adaptOfficialIcon(Bell, 'Bell'),
  bookOpen: adaptOfficialIcon(BookOpen, 'BookOpen'),
  brain: adaptOfficialIcon(Brain, 'Brain'),
  branch: adaptOfficialIcon(Branch, 'Branch'),
  branchAlt: adaptOfficialIcon(BranchAlt, 'BranchAlt'),
  bug: adaptOfficialIcon(Bug, 'Bug'),
  cabinet: adaptOfficialIcon(Cabinet, 'Cabinet'),
  calendar: adaptOfficialIcon(Calendar, 'Calendar'),
  chat: adaptOfficialIcon(Chat, 'Chat'),
  chatCompose: adaptOfficialIcon(ChatCompose, 'ChatCompose'),
  chatTripleDots: adaptOfficialIcon(ChatTripleDots, 'ChatTripleDots'),
  check: adaptOfficialIcon(Check, 'Check'),
  checkCircle: adaptOfficialIcon(CheckCircle, 'CheckCircle'),
  chevronDown: adaptOfficialIcon(ChevronDown, 'ChevronDown'),
  chevronLeft: adaptOfficialIcon(ChevronLeft, 'ChevronLeft'),
  chevronRight: adaptOfficialIcon(ChevronRight, 'ChevronRight'),
  chevronUp: adaptOfficialIcon(ChevronUp, 'ChevronUp'),
  circleDashed: adaptOfficialIcon(CircleDashed, 'CircleDashed'),
  clappingBoardClosed: adaptOfficialIcon(ClappingBoardClosed, 'ClappingBoardClosed'),
  clock: adaptOfficialIcon(Clock, 'Clock'),
  code: adaptOfficialIcon(Code, 'Code'),
  collapseLarge: adaptOfficialIcon(CollapseLarge, 'CollapseLarge'),
  colorTheme: adaptOfficialIcon(ColorTheme, 'ColorTheme'),
  compare: adaptOfficialIcon(Compare, 'Compare'),
  compareArrows: adaptOfficialIcon(CompareArrows, 'CompareArrows'),
  newChat: adaptOfficialIcon(ComposeEditSquare, 'ComposeEditSquare'),
  connect: adaptOfficialIcon(Connect, 'Connect'),
  copy: adaptOfficialIcon(Copy, 'Copy'),
  creditCard: adaptOfficialIcon(CreditCard, 'CreditCard'),
  cube: adaptOfficialIcon(Cube, 'Cube'),
  cursor: adaptOfficialIcon(Cursor, 'Cursor'),
  desktop: adaptOfficialIcon(Desktop, 'Desktop'),
  document: adaptOfficialIcon(Document, 'Document'),
  dotsHorizontal: adaptOfficialIcon(DotsHorizontal, 'DotsHorizontal'),
  download: adaptOfficialIcon(Download, 'Download'),
  editPencil: adaptOfficialIcon(EditPencil, 'EditPencil'),
  emptyCircle: adaptOfficialIcon(EmptyCircle, 'EmptyCircle'),
  error: adaptOfficialIcon(Error, 'Error'),
  exclamationMarkCircle: adaptOfficialIcon(ExclamationMarkCircle, 'ExclamationMarkCircle'),
  exitLogout: adaptOfficialIcon(ExitLogout, 'ExitLogout'),
  expand: adaptOfficialIcon(Expand, 'Expand'),
  expandLarge: adaptOfficialIcon(ExpandLarge, 'ExpandLarge'),
  externalLink: adaptOfficialIcon(ExternalLink, 'ExternalLink'),
  eye: adaptOfficialIcon(Eye, 'Eye'),
  eyeOff: adaptOfficialIcon(EyeOff, 'EyeOff'),
  fileBlank: adaptOfficialIcon(FileBlank, 'FileBlank'),
  fileCode: adaptOfficialIcon(FileCode, 'FileCode'),
  fileDocument: adaptOfficialIcon(FileDocument, 'FileDocument'),
  filePresentation: adaptOfficialIcon(FilePresentation, 'FilePresentation'),
  fileSpreadsheet: adaptOfficialIcon(FileSpreadsheet, 'FileSpreadsheet'),
  filter: adaptOfficialIcon(Filter, 'Filter'),
  flask: adaptOfficialIcon(Flask, 'Flask'),
  folder: adaptOfficialIcon(Folder, 'Folder'),
  folderDocumentsFinder: adaptOfficialIcon(FolderDocumentsFinder, 'FolderDocumentsFinder'),
  folderOpen: adaptOfficialIcon(FolderOpen, 'FolderOpen'),
  folderPlus: adaptOfficialIcon(FolderPlus, 'FolderPlus'),
  generateSuggestedEdits: adaptOfficialIcon(GenerateSuggestedEdits, 'GenerateSuggestedEdits'),
  globe: adaptOfficialIcon(Globe, 'Globe'),
  grid: adaptOfficialIcon(Grid, 'Grid'),
  handRaised: adaptOfficialIcon(HandRaised, 'HandRaised'),
  identity: adaptOfficialIcon(Identity, 'Identity'),
  imageSquare: adaptOfficialIcon(ImageSquare, 'ImageSquare'),
  infoCircle: adaptOfficialIcon(InfoCircle, 'InfoCircle'),
  invoice: adaptOfficialIcon(Invoice, 'Invoice'),
  key: adaptOfficialIcon(Key, 'Key'),
  keyboard: adaptOfficialIcon(Keyboard, 'Keyboard'),
  lightbulb: adaptOfficialIcon(Lightbulb, 'Lightbulb'),
  lock: adaptOfficialIcon(Lock, 'Lock'),
  lockKeyHole: adaptOfficialIcon(LockKeyHole, 'LockKeyHole'),
  mic: adaptOfficialIcon(MicLgDictate, 'MicLgDictate'),
  minus: adaptOfficialIcon(Minus, 'Minus'),
  mobile: adaptOfficialIcon(Mobile, 'Mobile'),
  moon: adaptOfficialIcon(Moon, 'Moon'),
  music: adaptOfficialIcon(Music, 'Music'),
  nodes: adaptOfficialIcon(Nodes, 'Nodes'),
  notebookPencil: adaptOfficialIcon(NotebookPencil, 'NotebookPencil'),
  on: adaptOfficialIcon(On, 'On'),
  pageBlank: adaptOfficialIcon(PageBlank, 'PageBlank'),
  paperclip: adaptOfficialIcon(Paperclip, 'Paperclip'),
  pauseOutline: adaptOfficialIcon(PauseOutline, 'PauseOutline'),
  pauseSm: adaptOfficialIcon(PauseSm, 'PauseSm'),
  pin: adaptOfficialIcon(Pin, 'Pin'),
  pinFilled: adaptOfficialIcon(PinFilled, 'PinFilled'),
  playOutline: adaptOfficialIcon(PlayOutline, 'PlayOutline'),
  playSm: adaptOfficialIcon(PlaySm, 'PlaySm'),
  plugin: adaptOfficialIcon(Plugin, 'Plugin'),
  plusCircle: adaptOfficialIcon(PlusCircle, 'PlusCircle'),
  plus: adaptOfficialIcon(PlusComposer, 'PlusComposer'),
  popOutWindow: adaptOfficialIcon(PopOutWindow, 'PopOutWindow'),
  pullRequestMerged: adaptOfficialIcon(PullRequestMerged, 'PullRequestMerged'),
  quote: adaptOfficialIcon(Quote, 'Quote'),
  quoteReplyFilledQuoteXs: adaptOfficialIcon(QuoteReplyFilledQuoteXs, 'QuoteReplyFilledQuoteXs'),
  reload: adaptOfficialIcon(Reload, 'Reload'),
  robotHead: adaptOfficialIcon(RobotHead, 'RobotHead'),
  search: adaptOfficialIcon(Search, 'Search'),
  settingsCog: adaptOfficialIcon(SettingsCog, 'SettingsCog'),
  settingsSlider: adaptOfficialIcon(SettingsSlider, 'SettingsSlider'),
  settingsWrench: adaptOfficialIcon(SettingsWrench, 'SettingsWrench'),
  share: adaptOfficialIcon(Share, 'Share'),
  shieldCheck: adaptOfficialIcon(ShieldCheck, 'ShieldCheck'),
  sidebar: adaptOfficialIcon(Sidebar, 'Sidebar'),
  sidebarCollapseRight: adaptOfficialIcon(SidebarCollapseRight, 'SidebarCollapseRight'),
  sidebarOpenRightAlt: adaptOfficialIcon(SidebarOpenRightAlt, 'SidebarOpenRightAlt'),
  simpleSmile: adaptOfficialIcon(SimpleSmile, 'SimpleSmile'),
  sparkles: adaptOfficialIcon(Sparkles, 'Sparkles'),
  spelling: adaptOfficialIcon(Spelling, 'Spelling'),
  stack: adaptOfficialIcon(Stack, 'Stack'),
  stop: adaptOfficialIcon(Stop, 'Stop'),
  storage: adaptOfficialIcon(Storage, 'Storage'),
  sun: adaptOfficialIcon(Sun, 'Sun'),
  tasks: adaptOfficialIcon(Tasks, 'Tasks'),
  terminal: adaptOfficialIcon(Terminal, 'Terminal'),
  text: adaptOfficialIcon(Text, 'Text'),
  timer: adaptOfficialIcon(Timer, 'Timer'),
  tools: adaptOfficialIcon(Tools, 'Tools'),
  trash: adaptOfficialIcon(Trash, 'Trash'),
  unarchive: adaptOfficialIcon(Unarchive, 'Unarchive'),
  undo: adaptOfficialIcon(Undo, 'Undo'),
  upgrade: adaptOfficialIcon(Upgrade, 'Upgrade'),
  upscale: adaptOfficialIcon(Upscale, 'Upscale'),
  user: adaptOfficialIcon(User, 'User'),
  users: adaptOfficialIcon(Users, 'Users'),
  video: adaptOfficialIcon(Video, 'Video'),
  voice5BarsSoundwave: adaptOfficialIcon(Voice5BarsSoundwave, 'Voice5BarsSoundwave'),
  warning: adaptOfficialIcon(Warning, 'Warning'),
  widget: adaptOfficialIcon(Widget, 'Widget'),
  x: adaptOfficialIcon(X, 'X'),
  xCircle: adaptOfficialIcon(XCircle, 'XCircle'),
} as const
