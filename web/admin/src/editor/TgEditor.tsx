import { Mark } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { forwardRef, useEffect, useId, useImperativeHandle, useRef, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { FormatBar, type FormatKey } from '@/ui'
import { Field, fieldBox } from '@/ui/input'
import { tgEntities } from '@/ui/tg-entities'
import { fromTelegramHtml, toTelegramHtml } from './serialize'

// Спойлер Telegram: <tg-spoiler> (и <span class="tg-spoiler"> при вставке).
const Spoiler = Mark.create({
  name: 'spoiler',
  parseHTML: () => [{ tag: 'tg-spoiler' }, { tag: 'span.tg-spoiler' }],
  renderHTML: () => ['tg-spoiler', 0],
})

// Кнопка FormatBar → узел или метка tiptap.
const NODES: Record<FormatKey, string> = {
  bold: 'bold',
  italic: 'italic',
  underline: 'underline',
  strike: 'strike',
  code: 'code',
  pre: 'codeBlock',
  spoiler: 'spoiler',
  quote: 'blockquote',
  link: 'link',
}
const FORMATS = Object.keys(NODES) as FormatKey[]

// Спойлер в превью скрыт, а при наборе текст должен быть виден. Селекторы сильнее, чем в tgEntities.
const CONTENT = cn(
  tgEntities,
  'min-w-0 flex-1',
  '[&_.ProseMirror]:min-h-40 [&_.ProseMirror]:whitespace-pre-wrap [&_.ProseMirror]:outline-none [&_p]:min-h-[1lh]',
  '[&_.ProseMirror_tg-spoiler]:bg-fill-tertiary [&_.ProseMirror_tg-spoiler]:text-text',
)

export interface TgEditorProps {
  value: string
  onChange(html: string): void
  disabled?: boolean
  /** Подпись для скринридера. */
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  /** Правый край FormatBar: счётчик символов. */
  right?: ReactNode
}

// insert — вставка текста (переменной) в позицию курсора.
export interface TgEditorHandle {
  insert(text: string): void
}

export const TgEditor = forwardRef<TgEditorHandle, TgEditorProps>(function TgEditor(
  { value, onChange, disabled, label = 'Текст сообщения', placeholder, hint, error, right },
  ref,
) {
  const id = useId()
  const changed = useRef(onChange)
  changed.current = onChange

  const editor = useEditor({
    content: fromTelegramHtml(value),
    editorProps: { attributes: { role: 'textbox', 'aria-multiline': 'true', 'aria-label': label } },
    shouldRerenderOnTransaction: true, // aria-pressed кнопок по курсору
    extensions: [
      StarterKit.configure({
        heading: false,
        bulletList: false,
        orderedList: false,
        listItem: false,
        listKeymap: false,
        horizontalRule: false,
        hardBreak: false,
        trailingNode: false,
        link: { openOnClick: false, autolink: false, protocols: ['tg', 'mailto'] },
      }),
      Spoiler,
    ],
    onUpdate: ({ editor }) => changed.current(toTelegramHtml(editor.getJSON())),
  })

  // внешнее значение (другой ключ, сброс после отправки) заменяет содержимое
  useEffect(() => {
    if (editor && value !== toTelegramHtml(editor.getJSON())) editor.commands.setContent(fromTelegramHtml(value), { emitUpdate: false })
  }, [editor, value])
  useEffect(() => editor?.setEditable(!disabled, false), [editor, disabled])

  useImperativeHandle(ref, () => ({ insert: (text) => editor?.chain().focus().insertContent({ type: 'text', text }).run() }), [editor])

  function link() {
    if (!editor) return
    const href = window.prompt('Ссылка (https://, tg://, mailto:)', String(editor.getAttributes('link').href ?? ''))
    if (href === null) return
    const c = editor.chain().focus().extendMarkRange('link')
    ;(href.trim() ? c.setLink({ href: href.trim() }) : c.unsetLink()).run()
  }

  function format(key: FormatKey) {
    if (!editor) return
    const c = editor.chain().focus()
    // Telegram не вкладывает pre в blockquote: блоки взаимно заменяют друг друга.
    if (key === 'link') link()
    else if (key === 'pre') c.unsetBlockquote().toggleCodeBlock().run()
    else if (key === 'quote') (editor.isActive('codeBlock') ? c.toggleCodeBlock() : c).toggleBlockquote().run()
    else c.toggleMark(key).run()
  }

  return (
    <div className="flex flex-col gap-3 text-text">
      <FormatBar
        active={FORMATS.filter((f) => editor?.isActive(NODES[f]))}
        disabled={disabled}
        onFormat={format}
        right={right}
      />
      <Field id={id} hint={hint} error={error}>
        <div
          className={cn(
            fieldBox,
            'relative cursor-text pt-2.75 pb-2',
            error && 'border-destructive focus-within:border-destructive',
            disabled && 'opacity-60',
          )}
        >
          {placeholder && editor?.isEmpty && (
            <span className="pointer-events-none absolute text-text-hint">{placeholder}</span>
          )}
          <EditorContent editor={editor} className={CONTENT} />
        </div>
      </Field>
    </div>
  )
})
