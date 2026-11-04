import { AppRoot } from '@telegram-apps/telegram-ui'
import { act, render } from '@testing-library/react'
import { createRef } from 'react'
import { expect, it, vi } from 'vitest'
import { TgEditor, type TgEditorHandle } from './TgEditor'

it('TgEditor loads Telegram HTML and emits it back on insert', async () => {
  const html = '<b>Оплата</b>\n\nна номер <code>{{.phone}}</code> <tg-spoiler>s</tg-spoiler>'
  const onChange = vi.fn()
  const ref = createRef<TgEditorHandle>()
  const { container } = render(<TgEditor ref={ref} value={html} onChange={onChange} />, { wrapper: AppRoot })
  await act(() => new Promise((r) => setTimeout(r, 0)))
  expect(container.querySelector('.ProseMirror b, .ProseMirror strong')?.textContent).toBe('Оплата')
  expect(container.querySelector('.ProseMirror tg-spoiler')).toBeTruthy()
  act(() => ref.current!.insert('{{.bank}}'))
  const out = onChange.mock.calls.at(-1)?.[0] as string
  expect(out).toContain('{{.bank}}')
  expect(out).toContain('<code>{{.phone}}</code>')
  expect(out).toContain('<tg-spoiler>s</tg-spoiler>')
})

it('external value change replaces the content without emitting', async () => {
  const onChange = vi.fn()
  const { container, rerender } = render(<TgEditor value="<b>a</b>" onChange={onChange} />, { wrapper: AppRoot })
  await act(() => new Promise((r) => setTimeout(r, 0)))
  rerender(<TgEditor value="<i>b</i>" onChange={onChange} />)
  await act(() => new Promise((r) => setTimeout(r, 0)))
  expect(container.querySelector('.ProseMirror i, .ProseMirror em')?.textContent).toBe('b')
  expect(container.querySelector('.ProseMirror b, .ProseMirror strong')).toBeNull()
  expect(onChange).not.toHaveBeenCalled()
})
