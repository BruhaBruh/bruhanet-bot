import { AppRoot } from '@telegram-apps/telegram-ui'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Composer } from './Composer'

// Редактор и выбор фото подменены: тест управляет ими через пропсы, как в Vue-версии через $emit.
const kids = vi.hoisted(() => ({ editor: {} as any, picker: {} as any }))
vi.mock('../editor/TgEditor', () => ({
  TgEditor: (p: any) => {
    kids.editor = p
    return null
  },
}))
vi.mock('./PhotoPicker', () => ({
  PhotoPicker: (p: any) => {
    kids.picker = p
    return null
  },
}))

type Call = { url: string; method: string; body: any }
let calls: Call[]
let replies: Record<string, (() => Response | Promise<Response>)[]>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  calls = []
  replies = { 'GET /api/broadcasts/recipients': [json(200, { count: 3 })] }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    const q = replies[`${method} ${url}`]
    return (q && q.length > 1 ? q.shift()! : (q?.[0] ?? json(500)))()
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

async function setup(props: { mode: 'broadcast' } | { mode: 'direct'; userId: number; onSent?: (m: any) => void } = { mode: 'broadcast' }) {
  const onSent = vi.fn()
  const r = render(props.mode === 'broadcast' ? <Composer mode="broadcast" onSent={onSent} /> : <Composer mode="direct" userId={props.userId} onSent={onSent} />, { wrapper: AppRoot })
  await flush()
  const setText = async (t: string) => {
    act(() => kids.editor.onChange(t))
    await flush()
  }
  const setPhoto = async (p: string | undefined) => {
    act(() => kids.picker.onChange(p))
    await flush()
  }
  const btn = (label: string) => screen.queryAllByRole('button').find((b) => b.textContent === label)
  const click = async (label: string) => {
    fireEvent.click(btn(label)!)
    await flush()
  }
  const count = () => r.container.querySelector('.count')!
  return { r, onSent, setText, setPhoto, btn, click, count }
}

const posts = () => calls.filter((c) => c.method === 'POST' && c.url === '/api/broadcasts')
const disabled = (b?: HTMLElement) => (b as HTMLButtonElement).disabled

describe('Composer', () => {
  it('limit is 1024 with photo, 4096 without', async () => {
    const { setText, setPhoto, btn, count } = await setup()
    await setText('a'.repeat(1025))
    expect(count().textContent).toBe('1025 / 4096')
    expect(disabled(btn('Разослать всем (3)'))).toBe(false)
    await setPhoto('file1')
    expect(count().textContent).toBe('1025 / 1024')
    expect(count().classList).toContain('over')
    expect(disabled(btn('Разослать всем (3)'))).toBe(true)
    await setText('a'.repeat(1024))
    expect(disabled(btn('Разослать всем (3)'))).toBe(false)
  })

  it('empty text without photo cannot be sent, photo alone can', async () => {
    const { setText, setPhoto, btn } = await setup()
    await setText(' ')
    expect(disabled(btn('Разослать всем (3)'))).toBe(true)
    await setPhoto('file1')
    expect(disabled(btn('Разослать всем (3)'))).toBe(false)
  })

  it('broadcast needs a second confirm click; cancel goes back', async () => {
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    const { onSent, setText, btn, click } = await setup()
    await setText('hi')
    await click('Разослать всем (3)')
    expect(posts()).toHaveLength(0)
    expect(btn('Точно разослать 3 юзерам?')).toBeTruthy()
    await click('Отмена')
    expect(btn('Точно разослать 3 юзерам?')).toBeUndefined()
    await click('Разослать всем (3)')
    await click('Точно разослать 3 юзерам?')
    expect(posts()).toHaveLength(1)
    expect(posts()[0].body).toMatchObject({ text: 'hi' })
    expect(onSent).toHaveBeenCalledTimes(1)
  })

  it('idempotency key is stable until success, then renewed', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' }), json(201, { broadcast: { id: 'b1' } }), json(201, { broadcast: { id: 'b2' } })]
    const { r, setText, click } = await setup()
    const send = async () => {
      await click('Разослать всем (3)')
      await click('Точно разослать 3 юзерам?')
    }
    await setText('hi')
    await send()
    expect(r.container.querySelector('.tg-err')).toBeTruthy()
    await send()
    await setText('next')
    await send()
    const keys = posts().map((c) => c.body.idempotencyKey)
    expect(keys).toHaveLength(3)
    expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/)
    expect(keys[1]).toBe(keys[0])
    expect(keys[2]).not.toBe(keys[0])
  })

  it('editing the draft after a failure gives a new key', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' }), json(500, { error: 'internal' }), json(500, { error: 'internal' })]
    const { setText, setPhoto, click } = await setup()
    const send = async () => {
      await click('Разослать всем (3)')
      await click('Точно разослать 3 юзерам?')
    }
    await setText('hi')
    await send()
    await setText('hi2')
    await send()
    await setPhoto('file1')
    await send()
    const keys = posts().map((c) => c.body.idempotencyKey)
    expect(new Set(keys).size).toBe(3)
  })

  it('send buttons are disabled while a photo uploads', async () => {
    const { setText, btn } = await setup()
    await setText('hi')
    act(() => kids.picker.onUploading(true))
    await flush()
    expect(disabled(btn('Разослать всем (3)'))).toBe(true)
    expect(disabled(btn('Отправить себе'))).toBe(true)
    act(() => kids.picker.onUploading(false))
    await flush()
    expect(disabled(btn('Разослать всем (3)'))).toBe(false)
  })

  it('panel down: no recipients count, broadcast disabled', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(502, { error: 'panel_unavailable' })]
    const { r, setText } = await setup()
    await setText('hi')
    expect(r.container.textContent).toContain('Панель недоступна, количество получателей неизвестно')
    const b = screen.getAllByRole('button').find((x) => x.textContent!.startsWith('Разослать всем'))
    expect(disabled(b)).toBe(true)
  })

  it('zero recipients: broadcast disabled, label shows 0', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(200, { count: 0 })]
    const { setText, btn } = await setup()
    await setText('hi')
    expect(disabled(btn('Разослать всем (0)'))).toBe(true)
  })

  it('failed recipients load can be retried', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(502, { error: 'panel_unavailable' }), json(200, { count: 3 })]
    const { r, setText, btn, click } = await setup()
    await setText('hi')
    await click('Повторить')
    expect(btn('Повторить')).toBeUndefined()
    expect(r.container.textContent).not.toContain('количество получателей неизвестно')
    expect(disabled(btn('Разослать всем (3)'))).toBe(false)
  })

  it('panel down on send: explains that no broadcast was created', async () => {
    replies['POST /api/broadcasts'] = [json(502, { error: 'panel_unavailable' })]
    const { r, setText, click } = await setup()
    await setText('hi')
    await click('Разослать всем (3)')
    await click('Точно разослать 3 юзерам?')
    expect(r.container.querySelector('.tg-err')?.textContent).toBe('Панель недоступна, рассылка не создана')
  })

  it('editor and photo picker are locked while a send is in flight', async () => {
    let release!: () => void
    const gate = new Promise<void>((r) => (release = r))
    replies['POST /api/broadcasts'] = [() => gate.then(() => json(201, { broadcast: { id: 'b1' } })())]
    const { setText, click } = await setup()
    await setText('hi')
    await click('Разослать всем (3)')
    await click('Точно разослать 3 юзерам?')
    expect(kids.editor.disabled).toBe(true)
    expect(kids.picker.disabled).toBe(true)
    release()
    await flush()
    expect(kids.editor.disabled).toBe(false)
  })

  it('direct mode posts to the user without confirm', async () => {
    replies['POST /api/users/7/messages'] = [json(201, { id: 5 })]
    const { onSent, setText, click } = await setup({ mode: 'direct', userId: 7 })
    expect(calls.some((c) => c.url === '/api/broadcasts/recipients')).toBe(false)
    await setText('hi')
    await click('Отправить')
    expect(calls.at(-1)).toMatchObject({ url: '/api/users/7/messages', method: 'POST', body: { text: 'hi' } })
    expect(onSent.mock.calls[0]?.[0]).toMatchObject({ id: 5, text: 'hi', status: 'pending', telegramId: 7 })
  })

  it('double click on «Отправить себе» sends one request', async () => {
    replies['POST /api/broadcasts/preview-send'] = [json(204)]
    const { setText, btn } = await setup()
    await setText('hi')
    const b = btn('Отправить себе')!
    // оба клика до перерисовки: кнопка ещё не disabled, защищает только синхронный флаг
    act(() => {
      b.click()
      b.click()
    })
    await flush()
    expect(calls.filter((c) => c.url === '/api/broadcasts/preview-send')).toHaveLength(1)
    expect(screen.getByRole('status').textContent).toBe('Отправлено в Telegram')
  })
})
