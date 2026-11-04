import { Button } from '@telegram-apps/telegram-ui'
import { Mark } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { fromTelegramHtml, toTelegramHtml } from './serialize'

// Спойлер Telegram: <tg-spoiler> (и <span class="tg-spoiler"> при вставке).
const Spoiler = Mark.create({
  name: 'spoiler',
  parseHTML: () => [{ tag: 'tg-spoiler' }, { tag: 'span.tg-spoiler' }],
  renderHTML: () => ['tg-spoiler', 0],
})

const marks = [
  { mark: 'bold', label: 'B', title: 'Жирный' },
  { mark: 'italic', label: 'I', title: 'Курсив' },
  { mark: 'underline', label: 'U', title: 'Подчёркнутый' },
  { mark: 'strike', label: 'S', title: 'Зачёркнутый' },
  { mark: 'code', label: '</>', title: 'Код' },
  { mark: 'spoiler', label: 'Спойлер', title: 'Спойлер' },
] as const

export interface TgEditorProps {
  value: string
  onChange(html: string): void
  disabled?: boolean
}

// insert — вставка текста (переменной) в позицию курсора.
export interface TgEditorHandle {
  insert(text: string): void
}

export const TgEditor = forwardRef<TgEditorHandle, TgEditorProps>(function TgEditor({ value, onChange, disabled }, ref) {
  const changed = useRef(onChange)
  changed.current = onChange

  const editor = useEditor({
    content: fromTelegramHtml(value),
    editorProps: { attributes: { class: 'tg-msg' } },
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

  return (
    <div className="tg-editor">
      <div className="toolbar" role="toolbar" aria-label="Форматирование">
        {marks.map((m) => (
          <Button
            key={m.mark}
            mode="bezeled"
            size="s"
            title={m.title}
            aria-label={m.title}
            aria-pressed={editor?.isActive(m.mark) ? 'true' : 'false'}
            disabled={disabled}
            onClick={() => editor?.chain().focus().toggleMark(m.mark).run()}
          >
            {m.label}
          </Button>
        ))}
        <Button
          mode="bezeled"
          size="s"
          title="Ссылка"
          aria-label="Ссылка"
          aria-pressed={editor?.isActive('link') ? 'true' : 'false'}
          disabled={disabled}
          onClick={link}
        >
          🔗
        </Button>
      </div>
      <EditorContent editor={editor} className="area" />
    </div>
  )
})
