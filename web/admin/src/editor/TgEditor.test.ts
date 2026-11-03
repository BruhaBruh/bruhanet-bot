// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import TgEditor from './TgEditor.vue'

it('TgEditor loads Telegram HTML and emits it back on insert', async () => {
  const html = '<b>Оплата</b>\n\nна номер <code>{{.phone}}</code> <tg-spoiler>s</tg-spoiler>'
  const w = mount(TgEditor, { props: { modelValue: html }, attachTo: document.body })
  await new Promise((r) => setTimeout(r, 0)) // useEditor создаёт редактор в onMounted
  expect(w.find('.ProseMirror b, .ProseMirror strong').text()).toBe('Оплата')
  expect(w.find('.ProseMirror tg-spoiler').exists()).toBe(true)
  ;(w.vm as unknown as { insert(t: string): void }).insert('{{.bank}}')
  const out = w.emitted('update:modelValue')?.at(-1)?.[0] as string
  expect(out).toContain('{{.bank}}')
  expect(out).toContain('<code>{{.phone}}</code>')
  expect(out).toContain('<tg-spoiler>s</tg-spoiler>')
  w.unmount()
})
