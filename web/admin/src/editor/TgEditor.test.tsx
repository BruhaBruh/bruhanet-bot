import { act, fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { expect, it, vi } from 'vitest'
import { TgEditor, type TgEditorHandle } from './TgEditor'

it('TgEditor loads Telegram HTML and emits it back on insert', async () => {
  const html = '<b>Оплата</b>\n\nна номер <code>{{.phone}}</code> <tg-spoiler>s</tg-spoiler>'
  const onChange = vi.fn()
  const ref = createRef<TgEditorHandle>()
  const { container } = render(<TgEditor ref={ref} value={html} onChange={onChange} />)
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
  const { container, rerender } = render(<TgEditor value="<b>a</b>" onChange={onChange} />)
  await act(() => new Promise((r) => setTimeout(r, 0)))
  rerender(<TgEditor value="<i>b</i>" onChange={onChange} />)
  await act(() => new Promise((r) => setTimeout(r, 0)))
  expect(container.querySelector('.ProseMirror i, .ProseMirror em')?.textContent).toBe('b')
  expect(container.querySelector('.ProseMirror b, .ProseMirror strong')).toBeNull()
  expect(onChange).not.toHaveBeenCalled()
})

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

async function setup(value: string) {
  const onChange = vi.fn()
  render(<TgEditor value={value} onChange={onChange} />)
  await flush()
  const format = (title: string) => {
    fireEvent.click(screen.getByRole('button', { name: title }))
    return onChange.mock.calls.at(-1)?.[0] as string
  }
  return { onChange, format }
}

it('FormatBar: quote and pre serialize to Telegram HTML', async () => {
  const { format } = await setup('hello')
  expect(format('Цитата')).toBe('<blockquote>hello</blockquote>')
  expect(screen.getByRole('button', { name: 'Цитата' }).getAttribute('aria-pressed')).toBe('true')
  // блоки взаимно заменяют друг друга: pre в blockquote Telegram не примет
  expect(format('Блок кода')).toBe('<pre>hello</pre>')
  expect(screen.getByRole('button', { name: 'Блок кода' }).getAttribute('aria-pressed')).toBe('true')
  expect(screen.getByRole('button', { name: 'Цитата' }).getAttribute('aria-pressed')).toBe('false')
  expect(format('Цитата')).toBe('<blockquote>hello</blockquote>')
  expect(format('Цитата')).toBe('hello')
})

it('FormatBar: all 9 formats are drawn', async () => {
  await setup('x')
  expect(screen.getByRole('toolbar').querySelectorAll('button')).toHaveLength(9)
})

it('FormatBar is disabled together with the editor', async () => {
  render(<TgEditor value="a" onChange={() => {}} disabled />)
  await flush()
  expect((screen.getByRole('button', { name: 'Жирный' }) as HTMLButtonElement).disabled).toBe(true)
})
