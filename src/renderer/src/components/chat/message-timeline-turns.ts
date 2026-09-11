export {
  blockHasPendingRuntimeWork,
  findTrailingAssistantContentStart,
  groupThreadTurns as groupTurns,
  isProcessBlock,
  sameThreadTurnContent as sameTurnContent,
  splitThink,
  stableThreadTurnKey as stableTurnKey,
  type ThreadTurn as Turn
} from '../../thread/projection/thread-turns'
