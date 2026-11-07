import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { Ref } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import type { TgEditorHandle, TgEditorProps } from '../editor/TgEditor'
import type { KbCellOut, MessageKeyboard, TextItem } from '../types'
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
  keyboard: null,
  ...over,
})

const cellOf = (id: string, over: Partial<KbCellOut> = {}): KbCellOut => ({
  id,
  key: `button.${id}`,
  action: `route:${id}`,
  conditions: [],
  locked: false,
  template: false,
  effective: { label: id, preview: id, style: '' },
  ...over,
})
const tplCell = (id: string): KbCellOut => ({ id: `tpl_${id}`, action: `template:${id}`, conditions: [], locked: true, template: true })
const rowsOf = (): KbCellOut[][] => [
  [cellOf('pay', { locked: true, effective: { label: 'Оплатить', preview: 'Оплатить', style: 'success' } }), cellOf('info', { effective: { label: 'Инфо', preview: 'Инфо', style: '' } })],
  [tplCell('tariffs')],
  [cellOf('trial', { conditions: [{ id: 'trial_available' }], effective: { label: 'Пробный', preview: 'Пробный', style: '' } })],
]
const menuKb = (over: Partial<MessageKeyboard> = {}): MessageKeyboard => ({ rows: rowsOf(), default: rowsOf(), overridden: false, locked: ['pay', 'tpl_tariffs'], ...over })
const catalogs = {
  conditions: [
    { id: 'in_panel', title: 'Юзер есть в панели', description: 'описание 1' },
    { id: 'trial_available', title: 'Пробный доступен', description: 'описание 2' },
  ],
  actions: [
    { id: 'route:pay', title: 'Оплата', kind: 'route', bound: false },
    { id: 'route:info', title: 'Информация', kind: 'route', bound: false },
    { id: 'route:menu', title: 'Меню', kind: 'route', bound: false },
    { id: 'route:trial', title: 'Пробный период', kind: 'route', guard: 'trial_available', bound: false },
    { id: 'template:tariffs', title: 'Тарифы', kind: 'template', bound: true },
  ],
  templates: [
    { id: 'tariffs', title: 'Тарифы', key: 'button.tariff' },
    { id: 'devices', title: 'Устройства', key: 'button.devices_add' },
  ],
}
beforeEach(() => {
  calls = []
  replies = {}
  kids.inserted = []
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
  replies['GET /api/settings'] = json(200, { prices: { 1: 100, 3: 270 }, devicePrice: 50 })
  replies['GET /api/texts'] = json(200, {
    items: [
      text('a', 'Текст A', 'тело A'),
      text('b', 'Текст B', 'тело B'),
      text('c', 'Текст C', 'своё', { default: 'станд', overridden: true }),
      text('x', 'Текст X', 'сломано', { default: 'станд', overridden: false, inactive: true, problem: 'тег не закрыт' }),
      text('v', 'С переменной', 'Привет, {{.name}}! <i>x</i>', {
        vars: [{ name: 'html', description: 'Разметка', sample: '<b>жир</b>', raw: true, max: 100 }],
      }),
      text('withKb', 'С клавиатурой', 'тело', { keyboard: menuKb() }),
      text('other', 'Другое', 'иное'),
      text('button.tariff', 'Тариф в списке', '{{.months}} за {{.price}} ₽', { kind: 'button', group: 'Кнопки', style: '', vars: [{ name: 'months', description: 'Срок', sample: '1 месяц', raw: false, max: 12 }, { name: 'price', description: 'Цена', sample: '100', raw: false, max: 8 }] }),
      text('loose', 'Без раскладки', 'тело'),
    ],
    globals: [{ name: 'name', description: 'Имя', sample: 'Иван', raw: false, max: 64 }],
    ...catalogs,
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

  it('«Отправить себе» sends the keyboard exactly as the preview shows it', async () => {
    replies['POST /api/texts/withKb/test'] = json(204)
    await open('/texts?key=withKb')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Пробный доступен' }))
    fireEvent.click(btn('Отправить себе'))
    await flush()
    expect(calls.filter((c) => c.method === 'POST')).toEqual([
      {
        url: '/api/texts/withKb/test',
        method: 'POST',
        body: {
          body: 'тело',
          keyboard: [
            [{ text: 'Оплатить', style: 'success' }, { text: 'Инфо', style: '' }],
            [{ text: '1 месяц за 100 ₽', style: '' }, { text: '3 месяца за 270 ₽', style: '' }],
            [{ text: 'Пробный', style: '' }],
          ],
        },
      },
    ])
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

  it('plain parts live on their own tab; opening a part by link selects it', async () => {
    const withParts = JSON.parse(JSON.stringify((await (replies['GET /api/texts']() as Response).json())))
    withParts.items.push({ ...text('st', 'Статус: активна', 'Активна', { kind: 'plain' }) })
    replies['GET /api/texts'] = json(200, withParts)
    await open('/texts')
    expect(list().queryByText('Статус: активна')).toBeNull()
    expect(list().getByText('Текст A')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: 'Части' }))
    await flush()
    expect(list().getByText('Статус: активна')).toBeTruthy()
    expect(list().queryByText('Текст A')).toBeNull()
    expect(screen.getByText(/подставляются в другие сообщения/)).toBeTruthy()
    // обратно: открывается первое сообщение
    fireEvent.click(screen.getByRole('radio', { name: 'Сообщения' }))
    await flush()
    expect(list().getByText('Текст A')).toBeTruthy()
  })

  it('a part opened by key shows the parts tab', async () => {
    const withParts = JSON.parse(JSON.stringify((await (replies['GET /api/texts']() as Response).json())))
    withParts.items.push({ ...text('st', 'Статус: активна', 'Активна', { kind: 'plain' }) })
    replies['GET /api/texts'] = json(200, withParts)
    await open('/texts?key=st')
    expect(list().getByText('Статус: активна')).toBeTruthy()
    expect(list().queryByText('Текст A')).toBeNull()
  })

  it('«Когда отправляется» comes from the registry', async () => {
    await open('/texts?key=b')
    expect(screen.getByText('Когда B')).toBeTruthy()
  })

  describe('keyboard constructor', () => {
    const kbButtons = () => within(preview()).queryAllByRole('button').map((b) => b.textContent)
    const click = async (name: string | RegExp) => {
      fireEvent.click(screen.getByRole('button', { name }))
      await flush()
    }
    const save = () => screen.getByRole('button', { name: 'Сохранить клавиатуру' }) as HTMLButtonElement
    const cellBody = (c: Partial<KbCellOut>) => ({ conditions: [], ...c })

    it('renders cells: id and action over the field, lock, template block; preview hides conditional buttons', async () => {
      await open('/texts?key=withKb')
      const input = screen.getByRole('textbox', { name: 'Текст кнопки Оплатить' }) as HTMLInputElement
      expect(input.placeholder).toBe('Оплатить')
      expect(input.value).toBe('')
      const id = screen.getAllByText('pay').find((e) => e.tagName === 'CODE')!
      expect(id.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
      expect(screen.getAllByText('→ Оплата').length).toBe(1)
      expect(screen.getByRole('combobox', { name: 'Стиль кнопки Оплатить' }).getAttribute('title')).toBe('Успех')
      expect(screen.getByRole('img', { name: /Нужна для работы бота: Оплатить/ })).toBeTruthy()
      expect(screen.queryByRole('button', { name: 'Убрать: Оплатить' })).toBeNull()
      expect((screen.getByRole('button', { name: 'Действие: Оплатить' }) as HTMLButtonElement).disabled).toBe(true)
      expect(screen.getByText('Шаблон: Тарифы')).toBeTruthy()
      expect(screen.getByRole('checkbox', { name: 'Пробный доступен' })).toBeTruthy()
      expect(save().disabled).toBe(true)
      // пробный скрыт, пока «Пробный доступен» не отмечено
      expect(kbButtons()).toEqual(['Оплатить', 'Инфо', '1 месяц за 100 ₽', '3 месяца за 270 ₽'])
      fireEvent.click(screen.getByRole('checkbox', { name: 'Пробный доступен' }))
      expect(kbButtons()).toEqual(['Оплатить', 'Инфо', '1 месяц за 100 ₽', '3 месяца за 270 ₽', 'Пробный'])
    })

    it('own label and style go to PUT /keyboard, one request per click, list is reloaded', async () => {
      replies['PUT /api/texts/withKb/keyboard'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
      await open('/texts?key=withKb')
      edit(screen.getByRole('textbox', { name: 'Текст кнопки Инфо' }), 'О боте')
      await pickStyle('Стиль кнопки О боте', 'Основная')
      expect(kbButtons()).toContain('О боте')
      const b = save()
      fireEvent.click(b)
      fireEvent.click(b)
      await wait(40)
      const sent = puts()
      expect(sent).toHaveLength(1)
      expect(sent[0].url).toBe('/api/texts/withKb/keyboard')
      expect((sent[0].body as { rows: unknown[][] }).rows[0][1]).toEqual(cellBody({ id: 'info', key: 'button.info', label: 'О боте', style: 'primary', action: 'route:info' }))
      expect(calls.filter((c) => c.url === '/api/texts' && c.method === 'GET')).toHaveLength(2)
    })

    it('«+ Кнопка» adds an editable cell; removing works only for non-locked cells', async () => {
      await open('/texts?key=withKb')
      await click('+ Кнопка')
      expect(screen.getByRole('textbox', { name: 'Текст кнопки Кнопка' })).toBeTruthy()
      expect(kbButtons()).toContain('Кнопка')
      await click('Убрать: Кнопка')
      expect(kbButtons()).not.toContain('Кнопка')
      await click('Убрать: Инфо')
      expect(kbButtons()).toEqual(['Оплатить', '1 месяц за 100 ₽', '3 месяца за 270 ₽'])
      expect(save().disabled).toBe(false)
      await click('Отменить')
      expect(kbButtons()).toEqual(['Оплатить', 'Инфо', '1 месяц за 100 ₽', '3 месяца за 270 ₽'])
    })

    it('action modal switches to a link; conditions modal sets НЕ', async () => {
      replies['PUT /api/texts/withKb/keyboard'] = json(204)
      await open('/texts?key=withKb')
      await click('Действие: Инфо')
      const sheet = () => within(screen.getByRole('dialog'))
      fireEvent.keyDown(sheet().getByRole('combobox', { name: 'Действие кнопки' }), { key: 'Enter' })
      fireEvent.click(await screen.findByRole('option', { name: 'Ссылка (https)' }))
      edit(sheet().getByRole('textbox', { name: 'Адрес ссылки' }), 'https://example.com/x')
      await click('Готово')
      await click('Условия: Инфо')
      fireEvent.click(sheet().getByRole('checkbox', { name: 'Юзер есть в панели' }))
      fireEvent.click(sheet().getByRole('checkbox', { name: 'НЕ (наоборот)' }))
      await click('Готово')
      expect(screen.getByText('НЕ Юзер есть в панели')).toBeTruthy()
      fireEvent.click(save())
      await flush()
      expect((puts()[0].body as { rows: unknown[][] }).rows[0][1]).toMatchObject({ id: 'info', action: 'url:https://example.com/x', conditions: [{ id: 'in_panel', not: true }] })
    })

    it('template: own button text is edited in the cell, preview shows the expanded result, saved without touching the layout', async () => {
      replies['PUT /api/texts/button.tariff'] = json(204)
      await open('/texts?key=withKb')
      edit(screen.getByRole('textbox', { name: 'Текст кнопок шаблона Тарифы' }), '{{.months}} — {{.price}} ₽')
      expect(kbButtons()).toContain('3 месяца — 270 ₽')
      expect(save().disabled).toBe(false)
      fireEvent.click(save())
      await flush()
      // раскладка не менялась: только текст кнопки
      expect(puts()).toEqual([{ url: '/api/texts/button.tariff', method: 'PUT', body: { body: '{{.months}} — {{.price}} ₽', style: '' } }])
    })

    it('template without a loaded button text falls back to its title in the preview', async () => {
      await open('/texts?key=withKb')
      fireEvent.keyDown(screen.getByRole('combobox', { name: 'Добавить шаблон' }), { key: 'Enter' })
      fireEvent.click(await screen.findByRole('option', { name: 'Устройства' }))
      await flush()
      // текста button.devices_add нет в фикстуре: блок показывается названием шаблона
      expect(kbButtons()).toContain('Устройства')
    })

    it('texts are written only after the layout is accepted', async () => {
      replies['PUT /api/texts/withKb/keyboard'] = json(400, { error: 'invalid', message: 'нельзя' })
      replies['PUT /api/texts/button.tariff'] = json(204)
      await open('/texts?key=withKb')
      edit(screen.getByRole('textbox', { name: 'Текст кнопок шаблона Тарифы' }), 'X {{.price}}')
      await click('+ Кнопка')
      fireEvent.click(save())
      await wait(10)
      expect(puts().map((c) => c.url)).toEqual(['/api/texts/withKb/keyboard'])
    })

    it('only templates this message can have are offered', async () => {
      replies['GET /api/texts'] = json(200, { items: [text('withKb', 'С клавиатурой', 'тело', { keyboard: menuKb({ templates: ['tariffs'] }) })], globals: [], ...catalogs })
      await open('/texts?key=withKb')
      expect(screen.queryByRole('combobox', { name: 'Добавить шаблон' })).toBeNull() // тарифы уже стоят, устройства сюда нельзя
    })

    it('template picker adds a template once', async () => {
      await open('/texts?key=withKb')
      fireEvent.keyDown(screen.getByRole('combobox', { name: 'Добавить шаблон' }), { key: 'Enter' })
      expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual(['Тарифы', 'Устройства'].filter((t) => t !== 'Тарифы'))
      fireEvent.click(screen.getByRole('option', { name: 'Устройства' }))
      await flush()
      expect(screen.getByText('Шаблон: Устройства')).toBeTruthy()
      expect(screen.queryByRole('combobox', { name: 'Добавить шаблон' })).toBeNull()
    })

    it('save error is shown next to the keyboard', async () => {
      replies['PUT /api/texts/withKb/keyboard'] = json(400, { error: 'invalid', message: 'в ряду не больше 2 кнопок' })
      await open('/texts?key=withKb')
      await click('+ Кнопка')
      fireEvent.click(save())
      await wait(10)
      expect(screen.getByRole('alert').textContent).toBe('в ряду не больше 2 кнопок')
    })

    it('reset goes through a dialog and sends DELETE /keyboard', async () => {
      replies['DELETE /api/texts/withKb/keyboard'] = json(204)
      replies['GET /api/texts'] = json(200, { items: [text('withKb', 'С клавиатурой', 'тело', { keyboard: menuKb({ overridden: true }) })], globals: [], ...catalogs })
      await open('/texts?key=withKb')
      await click('Вернуть как в коде')
      expect(screen.getByRole('alertdialog')).toBeTruthy()
      await click('Отмена')
      expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
      await click('Вернуть как в коде')
      await click('Вернуть')
      expect(calls.filter((c) => c.method === 'DELETE').map((c) => c.url)).toEqual(['/api/texts/withKb/keyboard'])
    })

    it('inactive stored keyboard is announced; message without keyboard has no card', async () => {
      replies['GET /api/texts'] = json(200, {
        items: [text('withKb', 'С клавиатурой', 'тело', { keyboard: menuKb({ overridden: true, inactive: true, problem: 'кнопку «Оплатить» нельзя убрать' }) }), text('plain', 'Без кнопок', 'x')],
        globals: [],
        ...catalogs,
      })
      await open('/texts?key=withKb')
      expect(screen.getByRole('alert').textContent).toContain('Сохранённая клавиатура не применяется')
      expect(screen.getByRole('alert').textContent).toContain('нельзя убрать')
      fireEvent.click(list().getByText('Без кнопок'))
      await flush()
      expect(screen.queryByRole('button', { name: 'Сохранить клавиатуру' })).toBeNull()
    })

    it('a message without buttons offers an empty keyboard: hint and «+ Кнопка»', async () => {
      replies['GET /api/texts'] = json(200, { items: [text('plainmsg', 'Реквизиты', 'x', { keyboard: { rows: [], default: [], overridden: false, locked: [] } })], globals: [], ...catalogs })
      await open('/texts?key=plainmsg')
      expect(screen.getByText(/Кнопок под этим сообщением пока нет/)).toBeTruthy()
      await click('+ Кнопка')
      expect(screen.getByRole('textbox', { name: 'Текст кнопки Кнопка' })).toBeTruthy()
      expect(screen.queryByText(/Кнопок под этим сообщением пока нет/)).toBeNull()
    })

    it('button texts are not in the list', async () => {
      replies['GET /api/texts'] = json(200, {
        items: [text('a', 'Текст A', 'x'), text('button.pay', 'Оплатить', 'Оплатить', { kind: 'button', group: 'Кнопки' })],
        globals: [],
        ...catalogs,
      })
      await open('/texts')
      expect(list().queryByText('Оплатить')).toBeNull()
      expect(screen.queryByRole('button', { name: 'Системные кнопки' })).toBeNull()
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
