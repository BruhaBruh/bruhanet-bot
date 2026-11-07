import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import Settings from './Settings'

type Call = { url: string; method: string; body: unknown }
let calls: Call[]
let replies: Record<string, () => Response | Promise<Response>>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const s = {
  prices: { '3': 1290, '1': 490 },
  phone: '+7900',
  bank: 'Банк',
  comments: ['На кофе'],
  supportUrl: 'https://t.me/x',
  reminderTimes: ['12:00'],
  advanceLimit: 3,
  advanceHours: 24,
  reminderWeekDays: 7,
  reminderDayHours: 24,
  devicesBase: 5,
  devicePrice: 0,
  deviceOptions: [1, 2, 3, 5],
  deviceMax: 10,
  miniAppUrl: '',
  trialDays: 3,
  trialDevices: 2,
  referralDays: 7,
  vars: { promoLink: 'https://x.io' },
}

beforeEach(() => {
  calls = []
  replies = { 'GET /api/settings': json(200, s), 'PUT /api/settings': json(204) }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const open = async () => {
  render(
    <ToastProvider>
      <Settings />
    </ToastProvider>,
  )
  await flush()
}
const btn = (name: string) => screen.getByRole('button', { name }) as HTMLButtonElement
const field = (name: string) => screen.getByLabelText(name) as HTMLInputElement
const edit = (el: HTMLElement, value: string) => fireEvent.change(el, { target: { value } })
const puts = () => calls.filter((c) => c.method === 'PUT')
const months = () => screen.getAllByRole('spinbutton', { name: 'Срок в месяцах' })
const prices = () => screen.getAllByRole('spinbutton', { name: 'Цена' })

describe('Settings', () => {
  it('double save sends one PUT with all keys, comments included', async () => {
    replies['PUT /api/settings'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
    await open()
    edit(field('Банк'), 'Т-Банк')
    const save = btn('Сохранить изменения')
    fireEvent.click(save)
    fireEvent.click(save)
    await act(() => new Promise((r) => setTimeout(r, 30)))
    expect(puts()).toEqual([{ url: '/api/settings', method: 'PUT', body: { ...s, bank: 'Т-Банк' } }])
    expect(Object.keys(puts()[0].body as object).sort()).toEqual(Object.keys(s).sort())
    expect(screen.getByText('Все изменения сохранены')).toBeTruthy()
    expect(btn('Сохранить изменения').disabled).toBe(true)
  })

  it('bot card sends numbers and time', async () => {
    await open()
    edit(field('Время напоминания'), '09:30')
    edit(field('Лимит авансов'), '0')
    edit(field('Длина аванса'), '48')
    edit(field('Первое напоминание'), '10')
    edit(field('Второе напоминание'), '12')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ reminderTimes: ['09:30'], advanceLimit: 0, advanceHours: 48, reminderWeekDays: 10, reminderDayHours: 12 })
  })

  it('reminder times: add, remove, sorted on save, duplicates and empties refused', async () => {
    await open()
    expect(btn('Удалить время').disabled).toBe(true) // последнее не удалить
    fireEvent.click(btn('Добавить время'))
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toEqual([])
    expect(screen.getByRole('alert').textContent).toContain('Заполните все времена')
    edit(screen.getAllByLabelText('Время напоминания')[1], '12:00')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toEqual([])
    expect(screen.getByRole('alert').textContent).toContain('12:00 указано дважды')
    edit(screen.getAllByLabelText('Время напоминания')[1], '09:00')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ reminderTimes: ['09:00', '12:00'] })
  })

  it('reminder times: at most six', async () => {
    await open()
    for (let i = 0; i < 5; i++) fireEvent.click(btn('Добавить время'))
    expect(screen.getAllByLabelText('Время напоминания')).toHaveLength(6)
    expect(btn('Добавить время').disabled).toBe(true)
    fireEvent.click(screen.getAllByRole('button', { name: 'Удалить время' })[0])
    expect(btn('Добавить время').disabled).toBe(false)
  })

  it('custom variables: edit, add, remove, duplicate refused', async () => {
    await open()
    expect(field('Имя переменной').value).toBe('promoLink')
    edit(field('Значение переменной'), 'https://y.io')
    fireEvent.click(btn('Добавить переменную'))
    edit(screen.getAllByLabelText('Имя переменной')[1], 'promoLink')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toEqual([])
    expect(screen.getByRole('alert').textContent).toContain('указана дважды')
    edit(screen.getAllByLabelText('Имя переменной')[1], 'note')
    edit(screen.getAllByLabelText('Значение переменной')[1], 'текст')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect((puts()[0].body as typeof s).vars).toEqual({ promoLink: 'https://y.io', note: 'текст' })
  })

  it('stored variable over 96 chars: warning, save refused until shortened, 96 passes', async () => {
    const long = 'a'.repeat(97)
    replies['GET /api/settings'] = json(200, { ...s, vars: { promoLink: long }, problems: ['Переменная promoLink длиннее 96 символов: сократите'] })
    await open()
    expect(screen.getByText(/Сохранить настройки нельзя/)).toBeTruthy()
    expect(screen.getByText(/Переменная promoLink длиннее 96 символов: сократите/)).toBeTruthy()
    edit(field('Банк'), 'Т-Банк')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toEqual([])
    edit(field('Значение переменной'), 'a'.repeat(96))
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toHaveLength(1)
    expect(screen.queryByText(/Сохранить настройки нельзя/)).toBeNull()
  })

  it('refused removal shows the keys from the server', async () => {
    replies['PUT /api/settings'] = json(400, { error: 'invalid', message: 'переменная promoLink используется в текстах: payment.tariffs', problems: ['переменная promoLink используется в текстах: payment.tariffs'] })
    await open()
    fireEvent.click(btn('Удалить переменную'))
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(screen.getByRole('alert').textContent).toContain('payment.tariffs')
  })

  it('edited comments go into the PUT', async () => {
    await open()
    edit(screen.getByRole('textbox', { name: 'Комментарий' }), 'Перекус')
    fireEvent.click(btn('Добавить комментарий'))
    edit(screen.getAllByRole('textbox', { name: 'Комментарий' })[1], 'Вернул долг')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect((puts()[0].body as typeof s).comments).toEqual(['Перекус', 'Вернул долг'])
  })

  it('«В месяц» is price / months, rounded, with the profit against the first tariff', async () => {
    await open()
    expect(screen.getByText(/^490\s₽ \/ мес\.$/)).toBeTruthy()
    expect(screen.getByText(/^430\s₽ \/ мес\. · выгода 12%$/)).toBeTruthy()
    edit(months()[1], '')
    expect(screen.getByText('—')).toBeTruthy()
  })

  it('tariffs are sorted by months; add and remove work, the last one stays', async () => {
    await open()
    expect(months().map((m) => (m as HTMLInputElement).value)).toEqual(['1', '3'])
    fireEvent.click(btn('Добавить тариф'))
    expect(months()).toHaveLength(3)
    expect((months()[2] as HTMLInputElement).value).toBe('')
    fireEvent.click(screen.getAllByRole('button', { name: 'Удалить тариф' })[2])
    fireEvent.click(screen.getAllByRole('button', { name: 'Удалить тариф' })[1])
    expect(months()).toHaveLength(1)
    expect(btn('Удалить тариф').disabled).toBe(true)
  })

  it('rejects duplicate months and out-of-range values without a request', async () => {
    await open()
    edit(months()[1], '1')
    fireEvent.click(btn('Сохранить изменения'))
    expect(screen.getByRole('alert').textContent).toBe('Тариф на 1 мес. указан дважды')
    edit(months()[1], '121')
    fireEvent.click(btn('Сохранить изменения'))
    expect(screen.getByRole('alert').textContent).toContain('от 1 до 120')
    edit(months()[1], '3')
    edit(prices()[1], '0')
    fireEvent.click(btn('Сохранить изменения'))
    expect(screen.getByRole('alert').textContent).toContain('больше 0')
    expect(puts()).toHaveLength(0)
  })

  it('«Отменить» restores the loaded values and clears the error', async () => {
    await open()
    expect(btn('Отменить').disabled).toBe(true)
    expect(screen.getByText('Все изменения сохранены')).toBeTruthy()
    edit(months()[1], '1')
    fireEvent.click(btn('Сохранить изменения'))
    expect(screen.getByText('Есть несохранённые изменения')).toBeTruthy()
    fireEvent.click(btn('Отменить'))
    expect((months()[1] as HTMLInputElement).value).toBe('3')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(btn('Сохранить изменения').disabled).toBe(true)
  })

  it('requisites line follows the fields, dashes when empty', async () => {
    await open()
    expect(screen.getByText('+7900 · Банк')).toBeTruthy()
    edit(field('Номер телефона'), '')
    edit(field('Банк'), '')
    expect(screen.getByText('— · —')).toBeTruthy()
  })

  it('support link must be http(s)', async () => {
    await open()
    edit(field('Ссылка на поддержку'), 't.me/x')
    expect(screen.getByText('Ссылка должна начинаться с https://')).toBeTruthy()
    expect(btn('Сохранить изменения').disabled).toBe(true)
  })

  it('devices base and price are editable and saved; 0 price is explained as off', async () => {
    await open()
    expect(field('Устройств в подписке').value).toBe('5')
    expect(field('Цена дополнительного устройства').value).toBe('0')
    expect(screen.getByText(/0 — продажа устройств выключена/)).toBeTruthy()
    edit(field('Устройств в подписке'), '4')
    edit(field('Цена дополнительного устройства'), '60')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toHaveLength(1)
    expect(puts()[0].body).toMatchObject({ devicesBase: 4, devicePrice: 60 })
  })

  it('device purchase options: parsed and saved; unsorted or over the maximum is rejected before sending', async () => {
    await open()
    expect(field('Варианты в меню докупки').value).toBe('1, 2, 3, 5')
    expect(field('Наибольшее число за раз').value).toBe('10')
    edit(field('Варианты в меню докупки'), '1, 4, 2')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toHaveLength(0)
    expect(screen.getByRole('alert').textContent).toContain('по возрастанию')
    edit(field('Варианты в меню докупки'), '2, 20')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toHaveLength(0)
    edit(field('Варианты в меню докупки'), '1 4,8')
    edit(field('Наибольшее число за раз'), '8')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ deviceOptions: [1, 4, 8], deviceMax: 8 })
  })

  it('MiniApp link: optional, https only, saved trimmed', async () => {
    await open()
    expect(field('Ссылка на MiniApp для удаления устройств').value).toBe('')
    edit(field('Ссылка на MiniApp для удаления устройств'), 'http://x.example')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toHaveLength(0)
    expect(screen.getByRole('alert').textContent).toContain('https://')
    edit(field('Ссылка на MiniApp для удаления устройств'), '  https://sub.example/app  ')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ miniAppUrl: 'https://sub.example/app' })
  })

  it('trial days and devices are editable and saved; 0 days is explained as off', async () => {
    await open()
    expect(field('Дней пробного периода').value).toBe('3')
    expect(field('Устройств на пробном периоде').value).toBe('2')
    expect(screen.getByText('0 — выключено (по умолчанию): кнопки в боте нет')).toBeTruthy()
    edit(field('Дней пробного периода'), '0')
    edit(field('Устройств на пробном периоде'), '4')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ trialDays: 0, trialDevices: 4 })
  })

  it('referral days are editable and saved; 0 is explained as off', async () => {
    await open()
    expect(field('Дней за приглашённого друга').value).toBe('7')
    expect(screen.getByText(/0 — выключено \(по умолчанию\): кнопки в боте нет, бонусы/)).toBeTruthy()
    edit(field('Дней за приглашённого друга'), '0')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()[0].body).toMatchObject({ referralDays: 0 })
  })

  it('usingDefaults: banner with problems, saving works without edits and hides the banner', async () => {
    replies['GET /api/settings'] = json(200, { ...s, usingDefaults: true, problems: ['нет тарифов', 'нет банка'] })
    await open()
    expect(screen.getByRole('alert').textContent).toContain('нет тарифов; нет банка')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(puts()).toEqual([{ url: '/api/settings', method: 'PUT', body: s }])
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('server errors are shown: message or the problems list', async () => {
    replies['PUT /api/settings'] = json(400, { message: 'неверно', problems: ['нет банка', 'нет телефона'] })
    await open()
    edit(field('Банк'), 'x')
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(screen.getAllByRole('listitem').map((li) => li.textContent)).toEqual(['нет банка', 'нет телефона'])
    replies['PUT /api/settings'] = json(500)
    fireEvent.click(btn('Сохранить изменения'))
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('Не удалось сохранить, попробуйте ещё раз')
  })

  it('load failure is reported', async () => {
    replies['GET /api/settings'] = json(500)
    await open()
    expect(screen.getByRole('alert').textContent).toBe('Не удалось загрузить')
  })
})
