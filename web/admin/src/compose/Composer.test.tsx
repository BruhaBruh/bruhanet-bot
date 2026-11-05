import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import type { TgEditorProps } from '../editor/TgEditor'
import { Composer } from './Composer'
import type { PhotoPickerProps } from './PhotoPicker'

// Редактор и выбор фото подменены: тест управляет ими через пропсы, как в Vue-версии через $emit.
const kids = vi.hoisted(() => ({ editor: {} as TgEditorProps, picker: {} as PhotoPickerProps }))
vi.mock('../editor/TgEditor', () => ({
  TgEditor: (p: TgEditorProps) => {
    kids.editor = p
    return (
      <div>
        <div data-testid="count">{p.right}</div>
        {p.error}
      </div>
    )
  },
}))
vi.mock('./PhotoPicker', () => ({
  PhotoPicker: (p: PhotoPickerProps) => {
    kids.picker = p
    return null
  },
}))

type Body = { idempotencyKey?: string; text?: string; photo?: string }
type Call = { url: string; method: string; body?: Body }
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

const SEND = 'Отправить 3 получателям'
const CONFIRM = 'Отправить'
const TEST = 'Отправить мне'

async function setup(props: { mode: 'broadcast'; initial?: { text: string } } | { mode: 'direct'; userId: number } = { mode: 'broadcast' }) {
  const onSent = vi.fn()
  const r = render(
    props.mode === 'broadcast' ? (
      <Composer mode="broadcast" initial={props.initial} onSent={onSent} />
    ) : (
      <Composer mode="direct" userId={props.userId} onSent={onSent} />
    ),
    { wrapper: ToastProvider },
  )
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
  const count = () => screen.getByTestId('count').firstElementChild!
  const countText = () => count().textContent!.replace(/\s/g, ' ')
  return { r, onSent, setText, setPhoto, btn, click, count, countText }
}

const posts = () => calls.filter((c) => c.method === 'POST' && c.url === '/api/broadcasts')
const disabled = (b?: HTMLElement) => (b as HTMLButtonElement).disabled

describe('Composer', () => {
  it('limit is 1024 with photo, 4096 without', async () => {
    const { r, setText, setPhoto, btn, count, countText } = await setup()
    await setText('a'.repeat(1025))
    expect(countText()).toBe('1 025 / 4 096')
    expect(disabled(btn(SEND))).toBe(false)
    await setPhoto('file1')
    expect(countText()).toBe('1 025 / 1 024')
    expect(count().classList).toContain('text-destructive-strong')
    expect(r.container.textContent).toContain('С фото текст становится подписью — не больше 1024 символов. Сократите на 1.')
    expect(disabled(btn(SEND))).toBe(true)
    await setText('a'.repeat(1024))
    expect(disabled(btn(SEND))).toBe(false)
  })

  it('empty text without photo cannot be sent, photo alone can', async () => {
    const { setText, setPhoto, btn } = await setup()
    await setText(' ')
    expect(disabled(btn(SEND))).toBe(true)
    await setPhoto('file1')
    expect(disabled(btn(SEND))).toBe(false)
  })

  it('broadcast needs a second confirm click; cancel goes back', async () => {
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    const { onSent, setText, btn, click } = await setup()
    await setText('hi')
    await click(SEND)
    expect(posts()).toHaveLength(0)
    const dialog = screen.getByRole('alertdialog')
    expect(within(dialog).getByText('Отправить рассылку?')).toBeTruthy()
    expect(dialog.textContent!.replace(/\s/g, ' ')).toContain(
      'Сообщение сразу получат 3 пользователя из группы «Все пользователи». Отменить отправленное нельзя.',
    )
    await click('Отмена')
    expect(screen.queryByRole('alertdialog')).toBeNull()
    await click(SEND)
    await click(CONFIRM)
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(posts()).toHaveLength(1)
    expect(posts()[0].body).toMatchObject({ text: 'hi' })
    expect(onSent).toHaveBeenCalledTimes(1)
    expect(btn(SEND)).toBeTruthy()
  })

  it('confirm dialog shows the last-minute recipients count', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(200, { count: 3 }), json(200, { count: 21 })]
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    expect(screen.getByRole('alertdialog').textContent).toContain('получат 21 пользователь из группы')
  })

  it('recipients gone by the time of send: no dialog', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(200, { count: 3 }), json(502, { error: 'panel_unavailable' })]
    const { r, setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(r.container.textContent).toContain('Панель недоступна, количество получателей неизвестно')
  })

  it('double click on Dialog confirm sends one POST', async () => {
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    const { setText, click, btn } = await setup()
    await setText('hi')
    await click(SEND)
    const ok = btn(CONFIRM)!
    // оба клика до перерисовки: диалог ещё открыт, защищает только синхронный флаг
    act(() => {
      ok.click()
      ok.click()
    })
    await flush()
    expect(posts()).toHaveLength(1)
  })

  it('idempotency key is stable until success, then renewed', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' }), json(201, { broadcast: { id: 'b1' } }), json(201, { broadcast: { id: 'b2' } })]
    const { setText, click } = await setup()
    const send = async () => {
      await click(SEND)
      await click(CONFIRM)
    }
    await setText('hi')
    await send()
    expect(screen.getByRole('alert').textContent).toBe('Не удалось отправить, попробуйте ещё раз')
    await send()
    await setText('next')
    await send()
    const keys = posts().map((c) => c.body?.idempotencyKey)
    expect(keys).toHaveLength(3)
    expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/)
    expect(keys[1]).toBe(keys[0])
    expect(keys[2]).not.toBe(keys[0])
  })

  it('editing the draft after a failure gives a new key', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' }), json(500, { error: 'internal' }), json(500, { error: 'internal' })]
    const { setText, setPhoto, click } = await setup()
    const send = async () => {
      await click(SEND)
      await click(CONFIRM)
    }
    await setText('hi')
    await send()
    await setText('hi2')
    await send()
    await setPhoto('file1')
    await send()
    const keys = posts().map((c) => c.body?.idempotencyKey)
    expect(new Set(keys).size).toBe(3)
  })

  it('an editor update with the same text keeps the key', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' })]
    const { setText, click } = await setup()
    const send = async () => {
      await click(SEND)
      await click(CONFIRM)
    }
    await setText('hi')
    await send()
    await setText('hi')
    await send()
    const keys = posts().map((c) => c.body?.idempotencyKey)
    expect(keys[1]).toBe(keys[0])
  })

  it('text typed during a photo upload is kept', async () => {
    const { setText } = await setup()
    const uploadDone = kids.picker.onChange
    await setText('typed')
    act(() => uploadDone('file1'))
    await flush()
    expect(kids.editor.value).toBe('typed')
    expect(kids.picker.value).toBe('file1')
  })

  it('send buttons are disabled while a photo uploads', async () => {
    const { setText, btn } = await setup()
    await setText('hi')
    act(() => kids.picker.onUploading?.(true))
    await flush()
    expect(disabled(btn(SEND))).toBe(true)
    expect(disabled(btn(TEST))).toBe(true)
    act(() => kids.picker.onUploading?.(false))
    await flush()
    expect(disabled(btn(SEND))).toBe(false)
  })

  it('panel down: no recipients count, broadcast disabled', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(502, { error: 'panel_unavailable' })]
    const { r, setText, btn } = await setup()
    await setText('hi')
    expect(r.container.textContent).toContain('Панель недоступна, количество получателей неизвестно')
    expect(disabled(btn(CONFIRM))).toBe(true)
  })

  it('zero recipients: broadcast disabled, label shows 0', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(200, { count: 0 })]
    const { setText, btn } = await setup()
    await setText('hi')
    expect(disabled(btn('Отправить 0 получателям'))).toBe(true)
  })

  it('failed recipients load can be retried', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(502, { error: 'panel_unavailable' }), json(200, { count: 3 })]
    const { r, setText, btn, click } = await setup()
    await setText('hi')
    await click('Повторить')
    expect(btn('Повторить')).toBeUndefined()
    expect(r.container.textContent).not.toContain('количество получателей неизвестно')
    expect(disabled(btn(SEND))).toBe(false)
  })

  it('panel down on send: explains that no broadcast was created', async () => {
    replies['POST /api/broadcasts'] = [json(502, { error: 'panel_unavailable' })]
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    await click(CONFIRM)
    expect(screen.getByRole('alert').textContent).toBe('Панель недоступна, рассылка не создана')
  })

  it('editor and photo picker are locked while a send is in flight', async () => {
    let release!: () => void
    const gate = new Promise<void>((r) => (release = r))
    replies['POST /api/broadcasts'] = [() => gate.then(() => json(201, { broadcast: { id: 'b1' } })())]
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    await click(CONFIRM)
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
    const b = btn(TEST)!
    // оба клика до перерисовки: кнопка ещё не disabled, защищает только синхронный флаг
    act(() => {
      b.click()
      b.click()
    })
    await flush()
    expect(calls.filter((c) => c.url === '/api/broadcasts/preview-send')).toHaveLength(1)
    expect(screen.getByRole('status').textContent).toContain('Тестовое сообщение отправлено вам')
  })

  it('controls without backend are disabled', async () => {
    await setup()
    for (const name of ['Отправить позже', 'Без звука']) {
      expect(screen.getByRole('switch', { name }).hasAttribute('disabled')).toBe(true)
    }
    for (const box of screen.getAllByRole('checkbox')) expect(box.hasAttribute('disabled')).toBe(true)
    expect((screen.getByRole('textbox', { name: 'Текст кнопки' }) as HTMLInputElement).disabled).toBe(true)
  })

  it('preview shows sanitized text and the initial draft', async () => {
    const { r } = await setup({ mode: 'broadcast', initial: { text: '<b>hi</b><script>x</script>' } })
    const aside = within(r.container).getByRole('complementary', { name: 'Превью сообщения' })
    expect(aside.querySelector('b')?.textContent).toBe('hi')
    expect(aside.querySelector('script')).toBeNull()
    expect(kids.editor.value).toContain('<b>hi</b>')
  })
})
