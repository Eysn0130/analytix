import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactElement
} from 'react'
import { baseKeymap } from '@tiptap/pm/commands'
import { history, redo, undo } from '@tiptap/pm/history'
import { keymap } from '@tiptap/pm/keymap'
import { EditorState, Selection, TextSelection, Plugin, PluginKey } from '@tiptap/pm/state'
import type { EditorState as ProseMirrorEditorState } from '@tiptap/pm/state'
import { Decoration, DecorationSet, EditorView } from '@tiptap/pm/view'
import {
  composerPromptSchema,
  composerRawOffsetToPmPos,
  parseComposerPromptDoc,
  pmPosToComposerRawOffset,
  serializeComposerPromptDoc,
  type ComposerPromptMentionMetadata
} from '../../lib/composer-prompt-document'

type ComposerPromptDoc = ReturnType<typeof parseComposerPromptDoc>

export type ComposerPromptEditorHandle = {
  focus: () => void
  setRawSelection: (cursor: number) => void
  getRawSelection: () => { from: number; to: number } | null
  getElement: () => HTMLDivElement | null
}

type ComposerPromptEditorProps = {
  value: string
  metadata?: ComposerPromptMentionMetadata
  placeholder?: string
  disabled?: boolean
  compact?: boolean
  spellCheck?: boolean
  onChange: (value: string) => void
  onCursorChange?: (cursor: number) => void
  onFocus?: () => void
  onBlur?: () => void
  onCompositionStart?: () => void
  onCompositionEnd?: () => void
  onKeyDownCapture?: (event: ReactKeyboardEvent<HTMLDivElement>) => void
  onElement?: (element: HTMLDivElement | null) => void
}

const placeholderPluginKey = new PluginKey('composerPromptPlaceholder')

function metadataSignature(metadata: ComposerPromptMentionMetadata | undefined): string {
  const plugins = (metadata?.plugins ?? [])
    .map((item) => [
      item.pluginId,
      item.title,
      item.description ?? '',
      item.iconUrl ?? '',
      item.brandColor ?? ''
    ].join('\u001f'))
    .sort()
    .join('\u001e')
  const skills = (metadata?.skills ?? [])
    .map((item) => [
      item.skillId,
      item.title,
      item.description ?? ''
    ].join('\u001f'))
    .sort()
    .join('\u001e')
  return `${plugins}\u001d${skills}`
}

function isEmptyPromptDoc(state: ProseMirrorEditorState): boolean {
  return state.doc.childCount === 1 && state.doc.firstChild?.childCount === 0
}

function createPlaceholderPlugin(placeholder: string | undefined): Plugin {
  return new Plugin({
    key: placeholderPluginKey,
    props: {
      decorations(state) {
        if (!placeholder || !isEmptyPromptDoc(state)) return DecorationSet.empty
        const paragraph = state.doc.firstChild
        if (!paragraph) return DecorationSet.empty
        return DecorationSet.create(state.doc, [
          Decoration.node(0, paragraph.nodeSize, {
            class: 'composer-prompt-editor-empty',
            'data-placeholder': placeholder
          })
        ])
      }
    }
  })
}

function createEditorState({
  value,
  metadata,
  placeholder,
  rawCursor
}: {
  value: string
  metadata?: ComposerPromptMentionMetadata
  placeholder?: string
  rawCursor?: number
}): ProseMirrorEditorState {
  const doc = parseComposerPromptDoc(value, metadata)
  const selectionPos = composerRawOffsetToPmPos(
    doc,
    rawCursor ?? value.length
  )
  return createStateWithSelection(doc, selectionPos, placeholder)
}

function createStateWithSelection(
  doc: ComposerPromptDoc,
  selectionPos: number,
  placeholder: string | undefined
): ProseMirrorEditorState {
  const plugins = [
    history(),
    keymap({
      'Mod-z': undo,
      'Mod-y': redo,
      'Shift-Mod-z': redo
    }),
    keymap(baseKeymap),
    createPlaceholderPlugin(placeholder)
  ]
  const state = EditorState.create({
    schema: composerPromptSchema,
    doc,
    plugins
  })
  const selection = safeTextSelection(state.doc, selectionPos)
  return state.apply(state.tr.setSelection(selection))
}

function safeTextSelection(doc: ComposerPromptDoc, pos: number): Selection {
  const bounded = Math.max(1, Math.min(doc.content.size - 1, pos))
  try {
    return TextSelection.create(doc, bounded)
  } catch {
    return Selection.near(doc.resolve(bounded))
  }
}

export const ComposerPromptEditor = forwardRef<ComposerPromptEditorHandle, ComposerPromptEditorProps>(
  function ComposerPromptEditor({
    value,
    metadata,
    placeholder,
    disabled = false,
    compact = false,
    spellCheck = false,
    onChange,
    onCursorChange,
    onFocus,
    onBlur,
    onCompositionStart,
    onCompositionEnd,
    onKeyDownCapture,
    onElement
  }, ref): ReactElement {
    const mountRef = useRef<HTMLDivElement | null>(null)
    const viewRef = useRef<EditorView | null>(null)
    const valueRef = useRef(value)
    const metadataRef = useRef(metadata)
    const placeholderRef = useRef(placeholder)
    const metadataSignatureRef = useRef(metadataSignature(metadata))
    const appliedMetadataSignatureRef = useRef(metadataSignature(metadata))
    const appliedPlaceholderRef = useRef(placeholder)
    const onElementRef = useRef(onElement)
    const latestPropsRef = useRef({
      disabled,
      spellCheck,
      onChange,
      onCursorChange,
      onFocus,
      onBlur,
      onCompositionStart,
      onCompositionEnd
    })

    latestPropsRef.current = {
      disabled,
      spellCheck,
      onChange,
      onCursorChange,
      onFocus,
      onBlur,
      onCompositionStart,
      onCompositionEnd
    }
    valueRef.current = value
    metadataRef.current = metadata
    placeholderRef.current = placeholder
    metadataSignatureRef.current = metadataSignature(metadata)
    onElementRef.current = onElement

    useEffect(() => {
      const mount = mountRef.current
      if (!mount || viewRef.current) return

      const state = createEditorState({
        value: valueRef.current,
        metadata: metadataRef.current,
        placeholder: placeholderRef.current
      })
      const view = new EditorView(mount, {
        state,
        editable: () => !latestPropsRef.current.disabled,
        attributes: {
          class: 'ds-no-drag',
          spellcheck: latestPropsRef.current.spellCheck ? 'true' : 'false',
          'aria-label': placeholderRef.current || ''
        },
        dispatchTransaction(transaction) {
          const nextState = view.state.apply(transaction)
          view.updateState(nextState)
          const nextValue = serializeComposerPromptDoc(nextState.doc)
          if (nextValue !== valueRef.current) {
            valueRef.current = nextValue
            latestPropsRef.current.onChange(nextValue)
          }
          latestPropsRef.current.onCursorChange?.(
            pmPosToComposerRawOffset(nextState.doc, nextState.selection.head)
          )
        },
        handleDOMEvents: {
          focus: () => {
            latestPropsRef.current.onFocus?.()
            return false
          },
          blur: () => {
            latestPropsRef.current.onBlur?.()
            return false
          },
          compositionstart: () => {
            latestPropsRef.current.onCompositionStart?.()
            return false
          },
          compositionend: () => {
            latestPropsRef.current.onCompositionEnd?.()
            return false
          }
        }
      })
      viewRef.current = view
      onElementRef.current?.(view.dom as HTMLDivElement)

      return () => {
        onElementRef.current?.(null)
        view.destroy()
        viewRef.current = null
      }
    }, [])

    useEffect(() => {
      const view = viewRef.current
      if (!view) return
      const metadataChanged = appliedMetadataSignatureRef.current !== metadataSignatureRef.current
      const placeholderChanged = appliedPlaceholderRef.current !== placeholder
      const currentValue = serializeComposerPromptDoc(view.state.doc)
      if (currentValue === value && !metadataChanged && !placeholderChanged) return
      const rawCursor = pmPosToComposerRawOffset(view.state.doc, view.state.selection.head)
      const nextState = createEditorState({
        value,
        metadata,
        placeholder,
        rawCursor
      })
      view.updateState(nextState)
      valueRef.current = value
      appliedMetadataSignatureRef.current = metadataSignatureRef.current
      appliedPlaceholderRef.current = placeholder
      latestPropsRef.current.onCursorChange?.(
        pmPosToComposerRawOffset(nextState.doc, nextState.selection.head)
      )
    }, [metadata, placeholder, value])

    useEffect(() => {
      const view = viewRef.current
      if (!view) return
      view.setProps({
        editable: () => !latestPropsRef.current.disabled,
        attributes: {
          class: 'ds-no-drag',
          spellcheck: spellCheck ? 'true' : 'false',
          'aria-label': placeholder || ''
        }
      })
    }, [disabled, placeholder, spellCheck])

    useImperativeHandle(ref, () => ({
      focus: () => {
        viewRef.current?.focus()
      },
      setRawSelection: (cursor: number) => {
        const view = viewRef.current
        if (!view) return
        const pos = composerRawOffsetToPmPos(view.state.doc, cursor)
        view.dispatch(view.state.tr.setSelection(safeTextSelection(view.state.doc, pos)).scrollIntoView())
        view.focus()
      },
      getRawSelection: () => {
        const view = viewRef.current
        if (!view) return null
        return {
          from: pmPosToComposerRawOffset(view.state.doc, view.state.selection.from),
          to: pmPosToComposerRawOffset(view.state.doc, view.state.selection.to)
        }
      },
      getElement: () => (viewRef.current?.dom as HTMLDivElement | undefined) ?? null
    }), [])

    return (
      <div
        className={`composer-prompt-editor w-full min-w-0 ${
          compact ? 'composer-prompt-editor-compact text-[14px]' : ''
        } ${disabled ? 'opacity-80' : ''}`}
        data-composer-prompt-editor="true"
        aria-disabled={disabled}
        onKeyDownCapture={onKeyDownCapture}
      >
        <div ref={mountRef} />
      </div>
    )
  }
)
