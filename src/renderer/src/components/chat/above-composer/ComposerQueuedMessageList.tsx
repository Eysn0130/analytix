import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent
} from '@dnd-kit/core'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Check, GripVertical, Pencil, RotateCcw, SendHorizontal, Trash2, X } from 'lucide-react'
import { useState, type CSSProperties, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import type { QueuedUserMessage } from '../../../store/chat-store-types'
import { useChatStore } from '../../../store/chat-store'
import { AboveComposerPanelRow } from './AboveComposerPanelRow'
import { AboveComposerActionRow } from './AboveComposerActionRow'

type Props = {
  messages: QueuedUserMessage[]
  onRemove?: (id: string) => void
}

function summary(message: QueuedUserMessage, t: (key: string, options?: Record<string, unknown>) => string): string {
  const text = projectOrdinaryPublicText(message.displayText ?? message.text).trim()
  if (text) return text
  if (message.attachments?.length) return t('queuedMessageAttachmentSummary', { count: message.attachments.length })
  if (message.fileReferences?.length) return t('queuedMessageFileReferenceSummary', { count: message.fileReferences.length })
  return t('queuedMessageFallbackSummary')
}

function SortableQueuedMessage({
  message,
  onRemove
}: {
  message: QueuedUserMessage
  onRemove?: (id: string) => void
}): ReactElement {
  const { t } = useTranslation('common')
  const [editing, setEditing] = useState(false)
  const publicText = projectOrdinaryPublicText(message.displayText ?? message.text)
  const [draft, setDraft] = useState(publicText)
  const editQueuedMessage = useChatStore((s) => s.editQueuedMessage)
  const removeQueuedMessage = useChatStore((s) => s.removeQueuedMessage)
  const sendQueuedMessageNow = useChatStore((s) => s.sendQueuedMessageNow)
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: message.id })
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition
  }

  const saveEdit = (): void => {
    editQueuedMessage(message.id, draft)
    setEditing(false)
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`group flex min-w-0 items-center gap-2 rounded-lg px-0 py-1 text-[13px] leading-5 ${isDragging ? 'opacity-60' : ''}`}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="relative -ml-1 flex h-6 w-6 shrink-0 cursor-grab items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink active:cursor-grabbing"
        aria-label={t('queuedMessageReorder')}
        title={t('queuedMessageReorder')}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5" strokeWidth={1.9} />
      </button>
      <div className="min-w-0 flex-1">
        {editing ? (
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') saveEdit()
              if (event.key === 'Escape') {
                setDraft(publicText)
                setEditing(false)
              }
            }}
            className="h-7 w-full rounded-lg border border-ds-border bg-ds-main px-2 text-[13px] text-ds-ink outline-none focus:border-ds-ink/35"
            aria-label={t('queuedMessageEditInput')}
          />
        ) : (
          <div className="truncate text-ds-muted" title={summary(message, t)}>
            {summary(message, t)}
          </div>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-0.5 opacity-100 md:opacity-0 md:transition md:group-hover:opacity-100">
        {editing ? (
          <>
            <button
              type="button"
              onClick={saveEdit}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('queuedMessageSaveEdit')}
              title={t('queuedMessageSaveEdit')}
            >
              <Check className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
            <button
              type="button"
              onClick={() => {
                setDraft(publicText)
                setEditing(false)
              }}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('queuedMessageCancelEdit')}
              title={t('queuedMessageCancelEdit')}
            >
              <X className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={() => {
                setDraft(publicText)
                setEditing(true)
              }}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('queuedMessageEdit')}
              title={t('queuedMessageEdit')}
            >
              <Pencil className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
            <button
              type="button"
              onClick={() => void sendQueuedMessageNow(message.id)}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('queuedMessageSendNow')}
              title={t('queuedMessageSendNow')}
            >
              <SendHorizontal className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
            <button
              type="button"
              onClick={() => {
                if (onRemove) onRemove(message.id)
                else removeQueuedMessage(message.id)
              }}
              className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('queuedMessageRemove')}
              title={t('queuedMessageRemove')}
            >
              <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
          </>
        )}
      </div>
    </div>
  )
}

export function ComposerQueuedMessageList({ messages, onRemove }: Props): ReactElement | null {
  const { t } = useTranslation('common')
  const pausedReason = useChatStore((s) => s.queuedMessagesPausedReason)
  const reorderQueuedMessages = useChatStore((s) => s.reorderQueuedMessages)
  const resumeInterruptedQueue = useChatStore((s) => s.resumeInterruptedQueue)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  if (messages.length === 0) return null

  const ids = messages.map((message) => message.id)
  const onDragEnd = (event: DragEndEvent): void => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = ids.indexOf(String(active.id))
    const newIndex = ids.indexOf(String(over.id))
    if (oldIndex === -1 || newIndex === -1) return
    reorderQueuedMessages(arrayMove(ids, oldIndex, newIndex))
  }

  return (
    <AboveComposerPanelRow>
      <div className="vertical-scroll-fade-mask hide-scrollbar flex max-h-[30dvh] flex-col gap-px overflow-x-hidden overflow-y-auto px-3 py-2">
        {pausedReason ? (
          <AboveComposerActionRow
            icon={<RotateCcw className="h-3.5 w-3.5 text-ds-faint" strokeWidth={1.9} />}
            title={<span className="text-[13px] font-medium text-ds-ink">{pausedReason === 'interrupted' ? t('queuedMessageInterruptedQueue') : t('queuedMessageFailedQueue')}</span>}
            actions={
              <button
                type="button"
                onClick={resumeInterruptedQueue}
                className="h-7 rounded-lg px-2 text-[12px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('queuedMessageResume')}
                title={t('queuedMessageResume')}
              >
                {t('queuedMessageResume')}
              </button>
            }
          />
        ) : null}
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          modifiers={[restrictToVerticalAxis]}
          onDragEnd={onDragEnd}
        >
          <SortableContext items={ids} strategy={verticalListSortingStrategy}>
            {messages.map((message) => (
              <SortableQueuedMessage key={message.id} message={message} onRemove={onRemove} />
            ))}
          </SortableContext>
        </DndContext>
      </div>
    </AboveComposerPanelRow>
  )
}
