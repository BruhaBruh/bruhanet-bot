<script setup lang="ts">
import { Mark } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { onBeforeUnmount, watch } from 'vue'
import { TgButton } from '../ui'
import { fromTelegramHtml, toTelegramHtml } from './serialize'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

// Спойлер Telegram: <tg-spoiler> (и <span class="tg-spoiler"> при вставке).
const Spoiler = Mark.create({
  name: 'spoiler',
  parseHTML: () => [{ tag: 'tg-spoiler' }, { tag: 'span.tg-spoiler' }],
  renderHTML: () => ['tg-spoiler', 0],
})

const editor = useEditor({
  content: fromTelegramHtml(props.modelValue),
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
  onUpdate: ({ editor }) => emit('update:modelValue', toTelegramHtml(editor.getJSON())),
})

watch(
  () => props.modelValue,
  (v) => {
    const e = editor.value
    if (e && v !== toTelegramHtml(e.getJSON())) e.commands.setContent(fromTelegramHtml(v), { emitUpdate: false })
  },
)
onBeforeUnmount(() => editor.value?.destroy())

const marks = [
  { mark: 'bold', label: 'B', title: 'Жирный' },
  { mark: 'italic', label: 'I', title: 'Курсив' },
  { mark: 'underline', label: 'U', title: 'Подчёркнутый' },
  { mark: 'strike', label: 'S', title: 'Зачёркнутый' },
  { mark: 'code', label: '</>', title: 'Код' },
  { mark: 'spoiler', label: 'Спойлер', title: 'Спойлер' },
] as const

function toggle(mark: string) {
  editor.value?.chain().focus().toggleMark(mark).run()
}

function link() {
  const e = editor.value
  if (!e) return
  const href = window.prompt('Ссылка (https://, tg://, mailto:)', String(e.getAttributes('link').href ?? ''))
  if (href === null) return
  const c = e.chain().focus().extendMarkRange('link')
  ;(href.trim() ? c.setLink({ href: href.trim() }) : c.unsetLink()).run()
}

// Вставка текста (переменной) в позицию курсора.
function insert(text: string) {
  editor.value?.chain().focus().insertContent({ type: 'text', text }).run()
}
defineExpose({ insert })
</script>

<template>
  <div class="tg-editor">
    <div class="toolbar" role="toolbar" aria-label="Форматирование">
      <TgButton
        v-for="m in marks"
        :key="m.mark"
        mode="bezeled"
        size="s"
        :title="m.title"
        :aria-label="m.title"
        :aria-pressed="editor?.isActive(m.mark) ? 'true' : 'false'"
        @click="toggle(m.mark)"
      >{{ m.label }}</TgButton>
      <TgButton mode="bezeled" size="s" title="Ссылка" aria-label="Ссылка" :aria-pressed="editor?.isActive('link') ? 'true' : 'false'" @click="link">🔗</TgButton>
    </div>
    <EditorContent :editor="editor" class="area" />
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px; }
.toolbar [aria-pressed='true'] { background: var(--tg-button); color: var(--tg-button-text); }
.area :deep(.ProseMirror) { min-height: 120px; padding: 12px 16px; background: var(--tg-bg); border-radius: 12px; outline: none; white-space: pre-wrap; }
.area :deep(.ProseMirror:focus-visible) { box-shadow: 0 0 0 2px var(--tg-link); }
.area :deep(p) { margin: 0; min-height: 1.3em; }
.area :deep(pre), .area :deep(blockquote) { margin: 0; }
.area :deep(blockquote) { border-left: 3px solid var(--tg-link); padding-left: 8px; }
.area :deep(tg-spoiler) { background: var(--tg-sep); border-radius: 4px; }
</style>
