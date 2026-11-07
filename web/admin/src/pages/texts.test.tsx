import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { Ref } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import type { TgEditorHandle, TgEditorProps } from '../editor/TgEditor'
import type { KeyboardInfo, TextItem } from '../types'
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
  keyboards: [],
  ...over,
})

const slotOf = (id: string, over = {}) => ({ id, kind: 'button' as const, label: id, removable: true, style: '', ...over })
const menuKb = (over: Partial<KeyboardInfo> = {}): KeyboardInfo => ({
  id: 'menu',
  title: 'Меню',
  messages: ['withKb', 'other'],
  slots: [
    slotOf('button.pay', { label: 'Оплатить', removable: false, style: 'success' }),
    slotOf('button.info', { label: 'Инфо' }),
    slotOf('button.help', { label: 'Помощь' }),
    { id: '@tariffs', kind: 'dynamic' as const, label: 'тарифы (из настроек)' },
  ],
  rows: [['button.pay', 'button.info'], ['@tariffs'], ['button.help']],
  hidden: [],
  default: [['button.pay', 'button.info'], ['@tariffs'], ['button.help']],
  overridden: false,
  ...over,
})
const btnText = (key: string, title: string, body: string, over: Partial<TextItem> = {}) =>
  text(key, title, body, { kind: 'button', group: 'Кнопки', route: key.slice(7), style: '', ...over })

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
      text('x', 'Текст X', 'сломано', { default: 'станд', overridden: false, inactive: true, problem: 'тег не закрыт' }),
      text('v', 'С переменной', 'Привет, {{.name}}! <i>x</i>', {
        vars: [{ name: 'html', description: 'Разметка', sample: '<b>жир</b>', raw: true, max: 100 }],
      }),
      text('withBtn', 'С кнопкой', 'тело', { buttons: ['btn'] }),
      text('btn', 'Кнопка', 'Оплатить', { kind: 'button', group: 'Кнопки', route: 'pay', style: 'success' }),
      text('withKb', 'С клавиатурой', 'тело', { keyboards: [menuKb()], buttons: ['button.pay', 'button.info', 'button.help'] }),
      text('other', 'Другое', 'иное', { keyboards: [menuKb()] }),
      text('two', 'Две клавиатуры', 'две', { keyboards: [menuKb({ messages: ['two'] }), menuKb({ id: 'second', title: 'Вторая', messages: ['two'], slots: [slotOf('button.back', { label: 'Назад', removable: false })], rows: [['button.back']], default: [['button.back']] })] }),
      btnText('button.pay', 'Оплатить', 'Оплатить', { style: 'success' }),
      btnText('button.info', 'Информация', 'Инфо'),
      btnText('button.help', 'Помощь', 'Помощь'),
      text('loose', 'Без раскладки', 'тело', { buttons: ['btn'] }),
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

const pickStyle = async (name: string, label: string) => {
  fireEvent.keyDown(screen.getByRole('combobox', { name }), { key: 'Enter' })
  fireEvent.click(await screen.findByRole('option', { name: label }))
  await new Promise((r) => setTimeout(r, 0))
}

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

  it('inactive override: tag in the list, banner with the reason; saving clears both', async () => {
    replies['PUT /api/texts/x'] = json(204)
    await open('/texts?key=x')
    expect(list().getAllByText('не применяется')).toHaveLength(1)
    expect(screen.getByText(/^Сохранённый текст не применяется/).textContent).toBe('Сохранённый текст не применяется, пользователи видят стандартный: тег не закрыт')
    act(() => kids.editor.onChange('починено'))
    fireEvent.click(btn('Сохранить'))
    await flush()
    expect(screen.queryByText(/^Сохранённый текст не применяется/)).toBeNull()
    expect(list().queryByText('не применяется')).toBeNull()
  })

  it('no banner for a healthy override', async () => {
    await open('/texts?key=c')
    expect(screen.queryByText(/^Сохранённый текст не применяется/)).toBeNull()
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
    expect(btn('{{.name}}').hasAttribute('title')).toBe(false)

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

  describe('keyboard editor', () => {
    const preview = () => screen.getByRole('complementary', { name: 'Превью сообщения' })
    const kbButtons = () => within(preview()).queryAllByRole('button').map((b) => b.textContent)
    const click = async (name: string | RegExp) => {
      fireEvent.click(screen.getByRole('button', { name }))
      await flush()
    }
    const save = () => screen.getByRole('button', { name: 'Сохранить клавиатуру' }) as HTMLButtonElement

    it('renders rows from the API: id above the field, style select, dynamic block placeholder', async () => {
      await open('/texts?key=withKb')
      const input = screen.getByRole('textbox', { name: 'Текст кнопки Оплатить' })
      const id = screen.getAllByText('button.pay').find((e) => e.tagName === 'CODE')!
      expect(id.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
      expect(screen.getByText('→ pay')).toBeTruthy()
      const style = screen.getByRole('combobox', { name: 'Стиль кнопки Оплатить' })
      expect(style.getAttribute('title')).toBe('Успех')
      fireEvent.keyDown(style, { key: 'Enter' })
      expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual(['Обычная', 'Основная', 'Успех', 'Опасная'])
      fireEvent.keyDown(screen.getAllByRole('option')[0], { key: 'Escape' })
      expect(screen.queryByRole('radiogroup')).toBeNull()
      expect(screen.getAllByText('тарифы (из настроек)').length).toBeGreaterThan(0)
      expect(screen.getByText('Общая для 2 сообщений')).toBeTruthy()
      expect(screen.getByText('С клавиатурой, Другое')).toBeTruthy()
      expect(kbButtons()).toEqual(['Оплатить', 'Инфо', 'тарифы (из настроек)', 'Помощь'])
      expect(save().disabled).toBe(true)
    })

    it('every cell has a drag handle (not for arrows); no move or split buttons remain', async () => {
      await open('/texts?key=withKb')
      for (const n of ['Оплатить', 'Информация', 'Помощь', 'тарифы (из настроек)']) {
        expect(screen.getByRole('button', { name: `Перетащить: ${n}` })).toBeTruthy()
      }
      expect(screen.queryByRole('button', { name: /^(Выше|Ниже|Левее|Правее|В отдельную строку|В строку с предыдущей)/ })).toBeNull()
      expect(screen.queryByText('Новый ряд')).toBeNull() // зона «новый ряд» показывается только при перетаскивании
    })

    it('hide only removable buttons; locked one has a tooltip hint; chips bring them back', async () => {
      await open('/texts?key=withKb')
      expect(screen.queryByRole('button', { name: 'Скрыть: Оплатить' })).toBeNull()
      expect(screen.getByRole('img', { name: /Нужна для работы бота: Оплатить/ })).toBeTruthy()
      await click('Скрыть: Информация')
      expect(kbButtons()).toEqual(['Оплатить', 'тарифы (из настроек)', 'Помощь'])
      expect(screen.getByText('Скрытые кнопки')).toBeTruthy()
      await click('Показать: Информация')
      expect(kbButtons()).toEqual(['Оплатить', 'тарифы (из настроек)', 'Помощь', 'Инфо'])
    })

    it('save: button PUTs first, then the layout; one request per click; list is reloaded', async () => {
      replies['PUT /api/texts/button.info'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
      replies['PUT /api/keyboards/menu'] = json(204)
      await open('/texts?key=withKb')
      edit(screen.getByRole('textbox', { name: 'Текст кнопки Информация' }), 'О боте')
      await pickStyle('Стиль кнопки Информация', 'Основная')
      expect(kbButtons()).toContain('О боте')
      await click('Скрыть: Помощь')
      const saveBtn = save()
      fireEvent.click(saveBtn)
      fireEvent.click(saveBtn)
      await wait(40)
      expect(puts()).toEqual([
        { url: '/api/texts/button.info', method: 'PUT', body: { body: 'О боте', style: 'primary' } },
        { url: '/api/keyboards/menu', method: 'PUT', body: { rows: [['button.pay', 'button.info'], ['@tariffs']], hidden: ['button.help'] } },
      ])
      expect(calls.filter((c) => c.url === '/api/texts' && c.method === 'GET')).toHaveLength(2)
    })

    it('save error is shown next to the keyboard; reset needs confirmation', async () => {
      replies['PUT /api/keyboards/menu'] = json(400, { error: 'invalid', message: 'в ряду не больше 2 кнопок' })
      await open('/texts?key=withKb')
      await click('Скрыть: Информация')
      fireEvent.click(save())
      await wait(10)
      expect(screen.getByRole('alert').textContent).toBe('в ряду не больше 2 кнопок')

      expect((screen.getByRole('button', { name: 'Вернуть как в коде' }) as HTMLButtonElement).disabled).toBe(true) // не переопределена
    })

    it('reset goes through a dialog and sends DELETE', async () => {
      replies['DELETE /api/keyboards/menu'] = json(204)
      replies['GET /api/texts'] = json(200, {
        items: [text('withKb', 'С клавиатурой', 'тело', { keyboards: [menuKb({ overridden: true, rows: [['button.info'], ['button.pay']] })] })],
        globals: [],
      })
      await open('/texts?key=withKb')
      await click('Вернуть как в коде')
      expect(screen.getByRole('alertdialog')).toBeTruthy()
      await click('Отмена')
      expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
      await click('Вернуть как в коде')
      await click('Вернуть')
      expect(calls.filter((c) => c.method === 'DELETE').map((c) => c.url)).toEqual(['/api/keyboards/menu'])
    })

    it('inactive stored layout is announced', async () => {
      replies['GET /api/texts'] = json(200, {
        items: [text('withKb', 'С клавиатурой', 'тело', { keyboards: [menuKb({ overridden: true, inactive: true, problem: 'кнопку «Оплатить» нельзя убрать' })] })],
        globals: [],
      })
      await open('/texts?key=withKb')
      expect(screen.getByRole('alert').textContent).toContain('Сохранённая раскладка не применяется')
      expect(screen.getByRole('alert').textContent).toContain('нельзя убрать')
    })

    it('two keyboards: both cards, preview switches between them', async () => {
      await open('/texts?key=two')
      expect(screen.getByText('Клавиатура: Меню')).toBeTruthy()
      expect(screen.getByText('Клавиатура: Вторая')).toBeTruthy()
      expect(kbButtons()).toContain('Оплатить')
      fireEvent.click(within(preview()).getByRole('radio', { name: 'Вторая' }))
      await flush()
      expect(kbButtons()).toEqual(['Назад'])
    })

    it('button text: label input, style select and id above; saved with style', async () => {
      replies['PUT /api/texts/btn'] = json(204)
      await open('/texts?key=btn')
      expect(screen.getAllByText('btn').some((e) => e.tagName === 'CODE')).toBe(true)
      expect(screen.queryByRole('radiogroup')).toBeNull()
      await pickStyle('Стиль кнопки', 'Опасная')
      edit(screen.getByRole('textbox', { name: 'Кнопка' }), 'Платить')
      fireEvent.click(btn('Сохранить'))
      await flush()
      expect(puts()).toEqual([{ url: '/api/texts/btn', method: 'PUT', body: { body: 'Платить', style: 'danger' } }])
    })

    it('button texts are not in the list; system buttons live in a modal with only unplaced buttons', async () => {
      replies['PUT /api/texts/btn'] = json(204)
      replies['DELETE /api/texts/btn'] = json(204)
      await open('/texts?key=a')
      expect(list().queryByText('Кнопка')).toBeNull()
      expect(list().queryByText('Информация')).toBeNull()
      fireEvent.click(btn('Системные кнопки'))
      await flush()
      const sheet = within(screen.getByRole('dialog'))
      expect(sheet.getByRole('textbox', { name: 'Текст кнопки Кнопка' })).toBeTruthy()
      expect(sheet.queryByRole('textbox', { name: 'Текст кнопки Оплатить' })).toBeNull() // стоит в раскладке
      edit(sheet.getByRole('textbox', { name: 'Текст кнопки Кнопка' }), 'Платить')
      const save = sheet.getByRole('button', { name: 'Сохранить кнопку Кнопка' })
      fireEvent.click(save)
      fireEvent.click(save)
      await flush()
      expect(puts()).toEqual([{ url: '/api/texts/btn', method: 'PUT', body: { body: 'Платить', style: 'success' } }])
      expect(screen.queryByText('Сохранить кнопки')).toBeNull()
    })
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
