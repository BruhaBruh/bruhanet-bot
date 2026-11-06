import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { Ref } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import type { TgEditorHandle, TgEditorProps } from '../editor/TgEditor'
import type { TextItem } from '../types'
import Texts from './Texts'

// Редактор подменён: тест читает value и правит через onChange.
const kids = vi.hoisted(() => ({ editor: {} as TgEditorProps, inserted: [] as string[] }))
vi.mock('../editor/TgEditor', async () => {
  const { forwardRef: fr, useImperativeHandle: uih } = await import('react')
  return {
    TgEditor: fr(function Fake(p: TgEditorProps, ref: Ref<TgEditorHandle>) {
      uih(ref, () => ({ insert: (t: string) => kids.inserted.push(t) }))
      kids.editor = p
      return null
    }),
  }
})

type Call = { url: string; method: string; body: unknown }
let calls: Call[]
let replies: Record<string, () => Response | Promise<Response>>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const text = (key: string, title: string, body: string, over: Partial<TextItem> = {}): TextItem => ({
  key,
  group: 'Меню',
  title,
  kind: 'message',
  default: body,
  body,
  overridden: false,
  vars: [],
  when: `Когда ${title.slice(-1)}`,
  buttons: [],
  style: '',
  ...over,
})

beforeEach(() => {
  calls = []
  replies = {}
  kids.inserted = []
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
  replies['GET /api/texts'] = json(200, {
    items: [
      text('a', 'Текст A', 'тело A'),
      text('b', 'Текст B', 'тело B'),
      text('c', 'Текст C', 'своё', { default: 'станд', overridden: true }),
      text('v', 'С переменной', 'Привет, {{.name}}! <i>x</i>', {
        vars: [{ name: 'html', description: 'Разметка', sample: '<b>жир</b>', raw: true, max: 100 }],
      }),
      text('withBtn', 'С кнопкой', 'тело', { buttons: ['btn'] }),
      text('btn', 'Кнопка', 'Оплатить', { kind: 'button', group: 'Кнопки', route: 'pay', style: 'success' }),
    ],
    globals: [{ name: 'name', description: 'Имя', sample: 'Иван', raw: false, max: 64 }],
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const wait = (ms: number) => act(() => new Promise((r) => setTimeout(r, ms)))
const open = async (path: string) => {
  render(
    <MemoryRouter initialEntries={[path]}>
      <ToastProvider>
        <Texts />
      </ToastProvider>
    </MemoryRouter>,
  )
  await flush()
}
const btn = (name: string) => screen.getByRole('button', { name })
const edit = (el: HTMLElement, value: string) => fireEvent.change(el, { target: { value } })
const list = () => within(screen.getByRole('navigation', { name: 'Сообщения бота' }))
const preview = () => screen.getByRole('complementary', { name: 'Превью сообщения' })
const puts = () => calls.filter((c) => c.method === 'PUT')

describe('Texts', () => {
  it('switching keys replaces editor content, unsaved edits do not leak', async () => {
    await open('/texts?key=a')
    expect(kids.editor.value).toBe('тело A')
    act(() => kids.editor.onChange('правка A'))
    expect(preview().textContent).toContain('правка A')

    fireEvent.click(list().getByText('Текст B'))
    await flush()
    expect(kids.editor.value).toBe('тело B')
    expect(preview().textContent).toContain('тело B')
    expect(preview().textContent).not.toContain('правка A')
    expect(puts()).toHaveLength(0)

    // черновик A жив, пока не сохранён
    fireEvent.click(list().getByText('Текст A'))
    await flush()
    expect(kids.editor.value).toBe('правка A')
  })

  it('double «Сохранить» sends one PUT; pristine text cannot be saved', async () => {
    replies['PUT /api/texts/b'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
    await open('/texts?key=b')
    const save = btn('Сохранить') as HTMLButtonElement
    expect(save.disabled).toBe(true)
    act(() => kids.editor.onChange('новое B'))
    expect(save.disabled).toBe(false)
    fireEvent.click(save)
    fireEvent.click(save)
    await wait(30)
    expect(puts()).toEqual([{ url: '/api/texts/b', method: 'PUT', body: { body: 'новое B' } }])
    expect((btn('Сохранить') as HTMLButtonElement).disabled).toBe(true)
  })

  it('«Вернуть стандартный текст» sends DELETE and restores the default', async () => {
    replies['DELETE /api/texts/c'] = json(204)
    await open('/texts?key=c')
    expect(list().getByRole('img', { name: 'Изменено' })).toBeTruthy()
    fireEvent.click(btn('Вернуть стандартный текст'))
    await flush()
    expect(calls.filter((c) => c.method === 'DELETE').map((c) => c.url)).toEqual(['/api/texts/c'])
    expect(kids.editor.value).toBe('станд')
    expect(list().queryByRole('img', { name: 'Изменено' })).toBeNull()
    expect((btn('Вернуть стандартный текст') as HTMLButtonElement).disabled).toBe(true)
  })

  it('reset is disabled when the text is not overridden', async () => {
    await open('/texts?key=a')
    expect((btn('Вернуть стандартный текст') as HTMLButtonElement).disabled).toBe(true)
  })

  it('server error is shown for its key only', async () => {
    replies['PUT /api/texts/a'] = json(400, { message: 'неизвестная переменная x' })
    await open('/texts?key=a')
    act(() => kids.editor.onChange('{{.x}}'))
    fireEvent.click(btn('Сохранить'))
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('неизвестная переменная x')
    fireEvent.click(list().getByText('Текст B'))
    await flush()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('«Отправить себе» posts the current draft', async () => {
    replies['POST /api/texts/a/test'] = json(204)
    await open('/texts?key=a')
    act(() => kids.editor.onChange('черновик'))
    fireEvent.click(btn('Отправить себе'))
    await flush()
    expect(calls.filter((c) => c.method === 'POST')).toEqual([{ url: '/api/texts/a/test', method: 'POST', body: { body: 'черновик' } }])
  })

  it('variables: chip inserts the token, preview fills samples, table shows used ones', async () => {
    await open('/texts?key=v')
    fireEvent.click(btn('{{.html}}'))
    expect(kids.inserted).toEqual(['{{.html}}'])
    expect(btn('{{.name}}').title).toBe('Имя — например, Иван')

    const p = within(preview())
    expect(preview().querySelector('i')!.textContent).toBe('x')
    expect(preview().textContent).toContain('Привет, Иван!')
    expect(p.getByText('{{.name}}')).toBeTruthy()
    expect(p.getByText('Иван')).toBeTruthy()
    expect(p.queryByText('{{.html}}')).toBeNull()
  })

  it('preview drops tags outside the Telegram allow-list', async () => {
    await open('/texts?key=a')
    act(() => kids.editor.onChange('<img src=x onerror=alert(1)><b>ok</b>'))
    expect(preview().querySelector('img')).toBeNull()
    expect(preview().querySelector('b')!.textContent).toBe('ok')
  })

  it('button text is a plain input limited to 64 with a counter', async () => {
    await open('/texts?key=btn')
    const input = screen.getByRole('textbox', { name: 'Кнопка' }) as HTMLInputElement
    expect(input.maxLength).toBe(64)
    expect(input.value).toBe('Оплатить')
    fireEvent.change(input, { target: { value: 'Купить' } })
    expect(preview().textContent).toContain('Купить')
    expect(screen.getByText(/^6 \/ 64$/)).toBeTruthy()
  })

  it('search filters by title, key and text; current item is marked', async () => {
    await open('/texts?key=b')
    expect(list().getByText('Текст B').closest('button')!.getAttribute('aria-current')).toBe('page')
    expect(list().getByText('Текст A').closest('button')!.getAttribute('aria-current')).toBeNull()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'станд' } })
    expect(list().queryByText('Текст C')).toBeNull()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'своё' } })
    expect(list().getByText('Текст C')).toBeTruthy()
    expect(list().queryByText('Текст A')).toBeNull()
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'нет такого' } })
    expect(list().getByText('Ничего не найдено')).toBeTruthy()
  })

  it('chips: global and own variables in two groups, global on every text', async () => {
    await open('/texts?key=v')
    const group = (title: RegExp) => screen.getByText(title).parentElement!
    expect(within(group(/^Общие/)).getByRole('button', { name: '{{.name}}' })).toBeTruthy()
    expect(within(group(/^Этого сообщения/)).getByRole('button', { name: '{{.html}}' })).toBeTruthy()
  })

  it('text without own variables has only global chips, preview fills them', async () => {
    await open('/texts?key=a')
    expect(screen.queryByText(/^Этого сообщения/)).toBeNull()
    fireEvent.click(btn('{{.name}}'))
    expect(kids.inserted).toEqual(['{{.name}}'])
    act(() => kids.editor.onChange('Привет, {{.name}}'))
    expect(preview().textContent).toContain('Привет, Иван')
  })

  it('«Когда отправляется» comes from the registry', async () => {
    await open('/texts?key=b')
    expect(screen.getByText('Когда B')).toBeTruthy()
  })

  it('message buttons: label and style saved per button through PUT', async () => {
    replies['PUT /api/texts/btn'] = json(204)
    await open('/texts?key=withBtn')
    const save = screen.getByRole('button', { name: 'Сохранить кнопку Кнопка' }) as HTMLButtonElement
    expect(save.disabled).toBe(true)
    expect(screen.getByText('pay')).toBeTruthy()

    const group = screen.getByRole('radiogroup', { name: 'Стиль кнопки Кнопка' })
    expect(within(group).getByRole('radio', { name: 'Успех (зелёная)' }).getAttribute('aria-checked')).toBe('true')
    fireEvent.click(within(group).getByRole('radio', { name: 'Опасная (красная)' }))
    edit(screen.getByRole('textbox', { name: 'Текст кнопки Кнопка' }), 'Платить')
    expect(save.disabled).toBe(false)
    fireEvent.click(save)
    await flush()
    expect(calls.filter((c) => c.method === 'PUT')).toEqual([{ url: '/api/texts/btn', method: 'PUT', body: { body: 'Платить', style: 'danger' } }])
    expect(within(group).getByRole('radio', { name: 'Опасная (красная)' }).getAttribute('aria-checked')).toBe('true')
    expect((screen.getByRole('button', { name: 'Сохранить кнопку Кнопка' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('button save error is shown on the message screen', async () => {
    replies['PUT /api/texts/btn'] = json(400, { error: 'invalid', message: 'текст не может быть пустым' })
    await open('/texts?key=withBtn')
    edit(screen.getByRole('textbox', { name: 'Текст кнопки Кнопка' }), ' ')
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить кнопку Кнопка' }))
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('текст не может быть пустым')
  })

  it('no buttons card without buttons; a button edits its own style', async () => {
    await open('/texts?key=a')
    expect(screen.queryByRole('radiogroup')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /Кнопка/ }))
    expect(screen.getByRole('radiogroup', { name: 'Стиль кнопки Кнопка' })).toBeTruthy()
  })

  it('hint tells the truth about unknown variables', async () => {
    await open('/texts?key=a')
    expect(kids.editor.hint).toContain('Неизвестная переменная не сохранится')
  })

  it('load failure is reported', async () => {
    replies['GET /api/texts'] = json(500)
    await open('/texts')
    expect(screen.getByRole('alert').textContent).toBe('Не удалось загрузить')
  })
})
