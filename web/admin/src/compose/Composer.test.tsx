import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { Ref } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import type { TgEditorHandle, TgEditorProps } from '../editor/TgEditor'
import { NO_ME, MeProvider, type Me } from '../me'
import { Composer, type ComposerDraft } from './Composer'
import type { PhotoPickerProps } from './PhotoPicker'

// Редактор и выбор фото подменены: тест управляет ими через пропсы, как в Vue-версии через $emit.
const kids = vi.hoisted(() => ({ editor: {} as TgEditorProps, picker: {} as PhotoPickerProps, inserted: [] as string[] }))
vi.mock('../editor/TgEditor', async () => {
  const { forwardRef: fr, useImperativeHandle: uih } = await import('react')
  return {
    TgEditor: fr(function Fake(p: TgEditorProps, ref: Ref<TgEditorHandle>) {
      uih(ref, () => ({ insert: (t: string) => kids.inserted.push(t) }))
      kids.editor = p
      return (
        <div>
          <div data-testid="count">{p.right}</div>
          {p.error}
        </div>
      )
    }),
  }
})
vi.mock('./PhotoPicker', () => ({
  PhotoPicker: (p: PhotoPickerProps) => {
    kids.picker = p
    return null
  },
}))

type Body = { idempotencyKey?: string; text?: string; photo?: string; audience?: string; audienceDays?: number | null; silent?: boolean; buttons?: unknown[] }
type Call = { url: string; method: string; body?: Body }
let calls: Call[]
let replies: Record<string, (() => Response | Promise<Response>)[]>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  me = NO_ME
  calls = []
  replies = {
    'GET /api/broadcasts/recipients?days=7': [json(200, { all: 3, active: 2, no_subscription: 1, expiring: 1 })],
    'GET /api/broadcasts/recipients?audience=all': [json(200, { count: 3 })],
  }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    const q = replies[`${method} ${url}`]
    return (q && q.length > 1 ? q.shift()! : (q?.[0] ?? json(500)))()
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const globals = [
  { name: 'name', description: 'Имя', sample: '<Иван>', raw: false, max: 64 },
  { name: 'promoLink', description: 'Своя', sample: 'https://x.io', raw: false, max: 256 },
]

let me: Me
const SEND = 'Отправить 3 получателям'
const CONFIRM = 'Отправить'
const TEST = 'Отправить мне'

async function setup(
  props:
    | { mode: 'broadcast'; initial?: ComposerDraft; broadcastId?: string; status?: 'draft' | 'scheduled' }
    | { mode: 'direct'; userId: number } = { mode: 'broadcast' },
) {
  const onSent = vi.fn()
  const onSaved = vi.fn()
  const r = render(
    props.mode === 'broadcast' ? (
      <Composer mode="broadcast" initial={props.initial} broadcastId={props.broadcastId} status={props.status} onSent={onSent} onSaved={onSaved} />
    ) : (
      <Composer mode="direct" userId={props.userId} onSent={onSent} />
    ),
    {
      wrapper: ({ children }) => (
        <MeProvider value={me}>
          <ToastProvider>{children}</ToastProvider>
        </MeProvider>
      ),
    },
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
  return { r, onSent, onSaved, setText, setPhoto, btn, click, count, countText }
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

  it('limit message mentions substitution only when the text has variables', async () => {
    replies['GET /api/texts'] = [json(200, { items: [], globals })]
    const { r, setText, setPhoto } = await setup()
    await setPhoto('file1')
    await setText('a'.repeat(1025))
    expect(r.container.textContent).not.toContain('с самыми длинными значениями переменных)')
    await setText('a'.repeat(1000) + '{{.name}}')
    expect(r.container.textContent).toContain('1024 символов (с самыми длинными значениями переменных). Сократите на 40.')
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
    replies['GET /api/broadcasts/recipients?audience=all'] = [json(200, { count: 21 })]
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    expect(screen.getByRole('alertdialog').textContent).toContain('получат 21 пользователь из группы')
  })

  it('recipients gone by the time of send: no dialog', async () => {
    replies['GET /api/broadcasts/recipients?audience=all'] = [json(502, { error: 'internal' })]
    const { r, setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(r.container.textContent).toContain('Не удалось получить количество получателей')
  })

  describe('while recipients are re-fetched', () => {
    async function asking(text: string) {
      let release!: () => void
      const gate = new Promise<void>((r) => (release = r))
      replies['GET /api/broadcasts/recipients?audience=all'] = [() => gate.then(() => json(200, { count: 3 })())]
      const s = await setup()
      await s.setText(text)
      fireEvent.click(s.btn(SEND)!)
      await flush()
      return { ...s, release: async () => (release(), await flush()) }
    }

    it('everything is locked, then the dialog opens', async () => {
      const { btn, release } = await asking('hi')
      expect(kids.editor.disabled).toBe(true)
      expect(kids.picker.disabled).toBe(true)
      expect(disabled(btn(TEST))).toBe(true)
      expect(screen.queryByRole('alertdialog')).toBeNull()
      await release()
      expect(kids.editor.disabled).toBe(false)
      expect(screen.getByRole('alertdialog')).toBeTruthy()
    })

    it('an upload started in the window: no dialog, even after it fails', async () => {
      const { release } = await asking('hi')
      act(() => kids.picker.onUploading?.(true))
      await release()
      expect(screen.queryByRole('alertdialog')).toBeNull()
      act(() => kids.picker.onUploading?.(false))
      await flush()
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })

    it('a photo arrived in the window and the text is over the limit: no dialog', async () => {
      const { setPhoto, release } = await asking('a'.repeat(1025))
      await setPhoto('file1')
      await release()
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
  })

  it('an upload started under the open dialog closes it, nothing is sent', async () => {
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    act(() => kids.picker.onUploading?.(true))
    await flush()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(posts()).toHaveLength(0)
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
    replies['GET /api/broadcasts/recipients?days=7'] = [json(502, { error: 'internal' })]
    const { r, setText, btn } = await setup()
    await setText('hi')
    expect(r.container.textContent).toContain('Не удалось получить количество получателей')
    expect(disabled(btn(CONFIRM))).toBe(true)
  })

  it('zero recipients: broadcast disabled, label shows 0', async () => {
    replies['GET /api/broadcasts/recipients?days=7'] = [json(200, { all: 0, active: 0, no_subscription: 0, expiring: 0 })]
    const { setText, btn } = await setup()
    await setText('hi')
    expect(disabled(btn('Отправить 0 получателям'))).toBe(true)
  })

  it('failed recipients load can be retried', async () => {
    replies['GET /api/broadcasts/recipients?days=7'] = [json(502, { error: 'internal' }), json(200, { all: 3 })]
    const { r, setText, btn, click } = await setup()
    await setText('hi')
    await click('Повторить')
    expect(btn('Повторить')).toBeUndefined()
    expect(r.container.textContent).not.toContain('Не удалось получить количество')
    expect(disabled(btn(SEND))).toBe(false)
  })

  it('a server refusal is shown with its message', async () => {
    replies['POST /api/broadcasts'] = [json(400, { error: 'invalid', message: 'время отправки уже прошло' })]
    const { setText, click } = await setup()
    await setText('hi')
    await click(SEND)
    await click(CONFIRM)
    expect(screen.getByRole('alert').textContent).toBe('время отправки уже прошло')
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
    expect(calls.some((c) => c.url.startsWith('/api/broadcasts/recipients'))).toBe(false)
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

  it('the test-send toast names the admin from /api/me, and nobody when unknown', async () => {
    replies['POST /api/broadcasts/preview-send'] = [json(204)]
    me = { ...NO_ME, username: 'owner_nick' }
    const first = await setup()
    await first.setText('hi')
    await first.click(TEST)
    expect(screen.getByRole('status').textContent).toBe('Тестовое сообщение отправлено вам, @owner_nick')
    first.r.unmount()
    me = NO_ME
    const second = await setup()
    await second.setText('hi')
    await second.click(TEST)
    expect(screen.getByRole('status').textContent).toBe('Тестовое сообщение отправлено вам')
  })

  describe('send later (МСК)', () => {
    const FUTURE = '2099-01-02T10:30'

    it('the switch reveals a datetime field; past time blocks sending, future allows it', async () => {
      const { setText, btn } = await setup()
      await setText('hi')
      expect(screen.queryByLabelText('Дата и время отправки')).toBeNull()
      expect(screen.getByText('по московскому времени (UTC+3)')).toBeTruthy()
      fireEvent.click(screen.getByRole('switch', { name: 'Отправить позже' }))
      await flush()
      const field = screen.getByLabelText('Дата и время отправки') as HTMLInputElement
      expect(field.type).toBe('datetime-local')
      expect(disabled(btn('Запланировать'))).toBe(true) // времени нет
      fireEvent.change(field, { target: { value: '2020-01-01T10:00' } })
      await flush()
      expect(screen.getByText('Это время уже прошло — выберите будущее')).toBeTruthy()
      expect(disabled(btn('Запланировать'))).toBe(true)
      fireEvent.change(field, { target: { value: FUTURE } })
      await flush()
      expect(disabled(btn('Запланировать'))).toBe(false)
    })

    it('schedules with the Moscow time string; the dialog names the time', async () => {
      replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1', status: 'scheduled' } })]
      replies['GET /api/broadcasts/recipients?audience=all'] = [json(200, { count: 3 })]
      const { onSent, setText, click } = await setup()
      await setText('hi')
      fireEvent.click(screen.getByRole('switch', { name: 'Отправить позже' }))
      fireEvent.change(screen.getByLabelText('Дата и время отправки'), { target: { value: FUTURE } })
      await flush()
      await click('Запланировать')
      const dialog = screen.getByRole('alertdialog')
      expect(within(dialog).getByText('Запланировать рассылку?')).toBeTruthy()
      expect(dialog.textContent!.replace(/\s/g, ' ')).toContain('2 янв. 2099 в 10:30 по Москве')
      fireEvent.click(within(dialog).getByRole('button', { name: 'Запланировать' }))
      await flush()
      expect(posts()[0].body).toMatchObject({ text: 'hi', scheduledAt: FUTURE })
      expect(onSent).toHaveBeenCalledTimes(1)
    })

    it('without «later» the request has no scheduledAt', async () => {
      replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
      const { setText, click } = await setup()
      await setText('hi')
      await click(SEND)
      await click(CONFIRM)
      expect(posts()[0].body).not.toHaveProperty('scheduledAt')
    })

    it('changing the time renews the idempotency key', async () => {
      replies['POST /api/broadcasts'] = [json(500, { error: 'internal' })]
      const { setText, click } = await setup()
      await setText('hi')
      fireEvent.click(screen.getByRole('switch', { name: 'Отправить позже' }))
      const field = screen.getByLabelText('Дата и время отправки')
      for (const v of [FUTURE, '2099-01-02T10:31']) {
        fireEvent.change(field, { target: { value: v } })
        await flush()
        await click('Запланировать')
        fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Запланировать' }))
        await flush()
      }
      expect(new Set(posts().map((c) => c.body?.idempotencyKey)).size).toBe(2)
    })

    it('editing a scheduled broadcast saves with PUT, no autosave, switch is locked on', async () => {
      replies['PUT /api/broadcasts/s1'] = [json(200, { broadcast: { id: 's1', status: 'scheduled' } })]
      const { onSaved, setText, click } = await setup({
        mode: 'broadcast',
        broadcastId: 's1',
        status: 'scheduled',
        initial: { text: 'old', scheduledAt: '2099-01-02T07:30:00Z' },
      })
      expect((screen.getByLabelText('Дата и время отправки') as HTMLInputElement).value).toBe(FUTURE)
      expect(screen.getByRole('switch', { name: 'Отправить позже' }).hasAttribute('disabled')).toBe(true)
      expect(screen.queryByText(/Черновик/)).toBeNull()
      expect(screen.queryByRole('button', { name: /Отправить \d/ })).toBeNull()
      await setText('new')
      await click('Сохранить изменения')
      const put = calls.find((c) => c.method === 'PUT')
      expect(put?.url).toBe('/api/broadcasts/s1')
      expect(put?.body).toMatchObject({ text: 'new', scheduledAt: FUTURE })
      expect(onSaved).toHaveBeenCalledTimes(1)
    })
  })

  describe('draft autosave', () => {
    const puts = () => calls.filter((c) => c.method === 'PUT')
    const advance = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)))
    const open = async (props?: Parameters<typeof setup>[0]) => {
      const s = await setup(props)
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
      return s
    }
    afterEach(() => vi.useRealTimers())

    it('opening the editor and waiting saves nothing', async () => {
      await open()
      await advance(10_000)
      expect(puts()).toEqual([])
      expect(screen.getByText('Черновик сохраняется автоматически')).toBeTruthy()
    })

    it('saves only changes, debounced, always to the same id', async () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000001')
      replies['PUT /api/broadcasts/00000000-0000-4000-8000-000000000001'] = [json(200, { broadcast: { id: 'x', status: 'draft' } })]
      await open()
      act(() => kids.editor.onChange('a'))
      await advance(1000)
      act(() => kids.editor.onChange('ab'))
      await advance(1000)
      expect(puts()).toHaveLength(0) // задержка сдвинулась правкой
      await advance(600)
      expect(puts()).toHaveLength(1)
      expect(puts()[0]).toMatchObject({ url: '/api/broadcasts/00000000-0000-4000-8000-000000000001', body: { text: 'ab', audience: 'all', buttons: [], silent: false, scheduledAt: '' } })
      expect(screen.getByText(/^Черновик сохранён · \d\d:\d\d$/)).toBeTruthy()
      act(() => kids.editor.onChange('ab')) // тот же текст — не правка
      await advance(5000)
      expect(puts()).toHaveLength(1)
      act(() => kids.editor.onChange('abc'))
      await advance(1600)
      expect(puts()).toHaveLength(2)
      expect(puts()[1].url).toBe(puts()[0].url)
      vi.restoreAllMocks()
    })

    it('a failed save is shown and retried by the next change', async () => {
      replies['PUT /api/broadcasts/00000000-0000-4000-8000-000000000002'] = [json(500, {}), json(200, { broadcast: {} })]
      vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000002')
      await open()
      act(() => kids.editor.onChange('a'))
      await advance(1600)
      expect(screen.getByRole('alert').textContent).toContain('Не удалось сохранить черновик')
      act(() => kids.editor.onChange('ab'))
      await advance(1600)
      expect(screen.queryByRole('alert')).toBeNull()
      expect(screen.getByText(/^Черновик сохранён/)).toBeTruthy()
      vi.restoreAllMocks()
    })

    it('an opened draft keeps its id and is not rewritten until changed', async () => {
      replies['PUT /api/broadcasts/d1'] = [json(200, { broadcast: { id: 'd1' } })]
      await open({ mode: 'broadcast', broadcastId: 'd1', status: 'draft', initial: { text: 'saved' } })
      await advance(5000)
      expect(puts()).toEqual([])
      act(() => kids.editor.onChange('saved!'))
      await advance(1600)
      expect(puts().map((c) => c.url)).toEqual(['/api/broadcasts/d1'])
    })

    it('sending a saved draft passes draftId; a pending autosave is dropped', async () => {
      replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'd1' } })]
      replies['PUT /api/broadcasts/d1'] = [json(200, { broadcast: { id: 'd1' } })]
      await open({ mode: 'broadcast', broadcastId: 'd1', status: 'draft', initial: { text: 'saved' } })
      act(() => kids.editor.onChange('final'))
      const send = (label: string) => act(async () => void fireEvent.click(screen.getAllByRole('button').find((b) => b.textContent === label)!))
      await send(SEND)
      await advance(10)
      await send(CONFIRM)
      await advance(5000)
      expect(posts()).toHaveLength(1)
      expect(posts()[0].body).toMatchObject({ draftId: 'd1', text: 'final' })
      expect(puts()).toEqual([]) // отложенное сохранение не уехало после отправки
    })

    it('leaving the page flushes the pending change', async () => {
      replies['PUT /api/broadcasts/d1'] = [json(200, { broadcast: { id: 'd1' } })]
      const { r } = await open({ mode: 'broadcast', broadcastId: 'd1', status: 'draft', initial: { text: 'saved' } })
      act(() => kids.editor.onChange('typed just now'))
      r.unmount()
      await advance(0)
      expect(puts().map((c) => (c.body as Body).text)).toEqual(['typed just now'])
    })
  })

  describe('buttons and silent', () => {
    const addButton = async (text: string, url: string) => {
      fireEvent.click(screen.getByRole('button', { name: 'Добавить кнопку' }))
      await flush()
      const i = screen.getAllByRole('textbox', { name: /^Текст кнопки/ }).length
      fireEvent.change(screen.getByRole('textbox', { name: `Текст кнопки ${i}` }), { target: { value: text } })
      fireEvent.change(screen.getByRole('textbox', { name: `Ссылка кнопки ${i}` }), { target: { value: url } })
      await flush()
    }

    it('buttons go to the server and to the preview keyboard; silent too', async () => {
      replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
      replies['POST /api/broadcasts/preview-send'] = [json(204)]
      const { r, setText, click } = await setup()
      await setText('hi')
      await addButton('Сайт', 'https://e.com/x')
      fireEvent.click(within(screen.getByRole('radiogroup', { name: 'Стиль кнопки 1' })).getByRole('radio', { name: 'Основная (синяя)' }))
      fireEvent.click(screen.getByRole('switch', { name: 'Без звука' }))
      await flush()
      const aside = within(r.container).getByRole('complementary', { name: 'Превью сообщения' })
      expect(within(aside).getByText('Сайт')).toBeTruthy()
      await click(TEST)
      expect(calls.find((c) => c.url === '/api/broadcasts/preview-send')?.body).toMatchObject({
        silent: true,
        buttons: [{ text: 'Сайт', url: 'https://e.com/x', style: 'primary' }],
      })
      await click(SEND)
      expect(screen.getByRole('alertdialog').textContent).toContain('Придёт без звука.')
      await click(CONFIRM)
      expect(posts()[0].body).toMatchObject({ silent: true, buttons: [{ text: 'Сайт', url: 'https://e.com/x', style: 'primary' }] })
    })

    it('not https, variables and blank fields block sending; fixed — allowed', async () => {
      const { setText, btn } = await setup()
      await setText('hi')
      await addButton('', '')
      expect(disabled(btn(SEND))).toBe(true)
      for (const [text, url, msg] of [
        ['Сайт', 'http://e.com', 'ссылка должна начинаться с https://'],
        ['Сайт', 'javascript:alert(1)', 'ссылка должна начинаться с https://'],
        ['Сайт', 'data:text/html,x', 'ссылка должна начинаться с https://'],
        ['Сайт', 'https://', 'ссылка должна начинаться с https://'],
        ['Привет {{.name}}', 'https://e.com', 'нельзя использовать переменные'],
        [' Сайт', 'https://e.com', 'уберите пробелы'],
      ]) {
        fireEvent.change(screen.getByRole('textbox', { name: 'Текст кнопки 1' }), { target: { value: text } })
        fireEvent.change(screen.getByRole('textbox', { name: 'Ссылка кнопки 1' }), { target: { value: url } })
        await flush()
        expect(screen.getAllByRole('alert').map((a) => a.textContent).join(' '), url).toContain(msg)
        expect(disabled(btn(SEND))).toBe(true)
      }
      fireEvent.change(screen.getByRole('textbox', { name: 'Текст кнопки 1' }), { target: { value: 'Сайт' } })
      fireEvent.change(screen.getByRole('textbox', { name: 'Ссылка кнопки 1' }), { target: { value: 'HTTPS://e.com' } })
      await flush()
      expect(disabled(btn(SEND))).toBe(false)
    })

    it('at most 3 buttons; delete frees a slot; text input is capped at 64', async () => {
      await setup()
      for (let i = 1; i <= 3; i++) await addButton(`b${i}`, 'https://e.com')
      expect((screen.getByRole('button', { name: 'Добавить кнопку' }) as HTMLButtonElement).disabled).toBe(true)
      expect((screen.getByRole('textbox', { name: 'Текст кнопки 1' }) as HTMLInputElement).maxLength).toBe(64)
      fireEvent.click(screen.getByRole('button', { name: 'Удалить кнопку 2' }))
      await flush()
      expect((screen.getByRole('button', { name: 'Добавить кнопку' }) as HTMLButtonElement).disabled).toBe(false)
      expect(screen.getAllByRole('textbox', { name: /^Текст кнопки/ })).toHaveLength(2)
    })

    it('changing buttons or silent renews the idempotency key', async () => {
      replies['POST /api/broadcasts'] = [json(500, { error: 'internal' })]
      const { setText, click } = await setup()
      const send = async () => {
        await click(SEND)
        await click(CONFIRM)
      }
      await setText('hi')
      await send()
      fireEvent.click(screen.getByRole('switch', { name: 'Без звука' }))
      await flush()
      await send()
      await addButton('a', 'https://e.com')
      await send()
      expect(new Set(posts().map((c) => c.body?.idempotencyKey)).size).toBe(3)
    })

    it('direct mode has neither buttons nor silent', async () => {
      await setup({ mode: 'direct', userId: 7 })
      expect(screen.queryByRole('button', { name: 'Добавить кнопку' })).toBeNull()
      expect(screen.queryByRole('switch', { name: 'Без звука' })).toBeNull()
    })
  })

  it('audience radios show counts and the choice goes to the server with days', async () => {
    replies['GET /api/broadcasts/recipients?days=3'] = [json(200, { all: 3, active: 2, no_subscription: 1, expiring: 5 })]
    replies['GET /api/broadcasts/recipients?audience=expiring&days=3'] = [json(200, { count: 5 })]
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    const { setText, click } = await setup()
    const counts = () => ['Все пользователи', 'Активные подписчики', 'Без подписки', 'Подписка истекает через'].map((n) => screen.getByLabelText(`Получателей: ${n}`).textContent)
    expect(counts()).toEqual(['3', '2', '1', '1'])
    expect(screen.getByRole('radio', { name: 'Все пользователи' }).getAttribute('aria-checked')).toBe('true')
    await setText('hi')
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Дней до окончания подписки' }), { target: { value: '3' } })
    await flush()
    expect(screen.getByRole('radio', { name: 'Подписка истекает через' }).getAttribute('aria-checked')).toBe('true')
    expect(counts()[3]).toBe('5')
    await click('Отправить 5 получателям')
    expect(screen.getByRole('alertdialog').textContent).toContain('получат 5 пользователей из группы «Подписка истекает через 3 дня»')
    await click(CONFIRM)
    expect(posts()[0].body).toMatchObject({ audience: 'expiring', audienceDays: 3 })
  })

  it('days outside 1..60 disable sending', async () => {
    const { setText, btn } = await setup()
    await setText('hi')
    const days = screen.getByRole('spinbutton', { name: 'Дней до окончания подписки' })
    for (const bad of ['0', '61', '', '1.5']) {
      fireEvent.change(days, { target: { value: bad } })
      await flush()
      expect(disabled(btn('Отправить')), bad).toBe(true)
    }
    fireEvent.change(days, { target: { value: '60' } })
    await flush()
    expect(screen.getByRole('radio', { name: 'Подписка истекает через' }).getAttribute('aria-checked')).toBe('true')
  })

  it('other audiences send null days', async () => {
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    replies['GET /api/broadcasts/recipients?audience=active'] = [json(200, { count: 2 })]
    const { setText, click } = await setup()
    await setText('hi')
    fireEvent.click(screen.getByRole('radio', { name: 'Активные подписчики' }))
    await flush()
    await click('Отправить 2 получателям')
    await click(CONFIRM)
    expect(posts()[0].body).toMatchObject({ audience: 'active', audienceDays: null })
  })

  it('preview shows sanitized text and the initial draft', async () => {
    const { r } = await setup({ mode: 'broadcast', initial: { text: '<b>hi</b><script>x</script>' } })
    const aside = within(r.container).getByRole('complementary', { name: 'Превью сообщения' })
    expect(aside.querySelector('b')?.textContent).toBe('hi')
    expect(aside.querySelector('script')).toBeNull()
    expect(kids.editor.value).toContain('<b>hi</b>')
  })

  it('variables: chips insert tokens, preview fills samples, limit is the worst case', async () => {
    replies['GET /api/texts'] = [json(200, { items: [], globals })]
    const { setText } = await setup()
    fireEvent.click(screen.getByRole('button', { name: '{{.name}}' }))
    expect(kids.inserted).toEqual(['{{.name}}'])

    await setText('Привет, {{.name}}')
    // текст уходит с токеном, а не с примером
    expect(kids.editor.value).toBe('Привет, {{.name}}')
    expect(screen.getByLabelText('Превью сообщения').textContent).toContain('Привет, <Иван>')
    expect(screen.getByTestId('count').textContent).toMatch(/^72 /) // 8 + 64

    await setText('a'.repeat(4033) + '{{.name}}')
    expect(screen.getByText(/Сократите на 1\./)).toBeTruthy()
    expect((screen.getByRole('button', { name: SEND }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('«Отправить мне» sends the token: the worker renders it for the admin', async () => {
    replies['GET /api/texts'] = [json(200, { items: [], globals })]
    replies['POST /api/broadcasts/preview-send'] = [json(204)]
    const { setText, click } = await setup()
    await setText('Привет, {{.name}}')
    await click(TEST)
    expect(calls.find((c) => c.url === '/api/broadcasts/preview-send')?.body).toEqual({ text: 'Привет, {{.name}}', silent: false, buttons: [] })
  })

  it('without globals (load failed) the composer still works', async () => {
    const { setText } = await setup()
    await setText('Привет')
    expect(screen.queryByRole('button', { name: '{{.name}}' })).toBeNull()
    expect(screen.getByTestId('count').textContent).toMatch(/^6 /)
  })
})
