// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TgEditor from '../editor/TgEditor.vue'
import Composer from './Composer.vue'
import PhotoPicker from './PhotoPicker.vue'
import { checkPhoto } from './photo'

type Call = { url: string; method: string; body: any }
let calls: Call[]
let replies: Record<string, (() => Response)[]>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  calls = []
  replies = { 'GET /api/broadcasts/recipients': [json(200, { count: 3 })] }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    const q = replies[`${method} ${url}`]
    return (q && q.length > 1 ? q.shift()! : q?.[0] ?? json(500))()
  })
})
afterEach(() => vi.unstubAllGlobals())

async function setup(props: { mode: 'broadcast' | 'direct'; userId?: number } = { mode: 'broadcast' }) {
  const w = mount(Composer, { props, attachTo: document.body })
  await flushPromises()
  const setText = async (t: string) => {
    w.findComponent(TgEditor).vm.$emit('update:modelValue', t)
    await flushPromises()
  }
  const setPhoto = async (p: string | undefined) => {
    w.findComponent(PhotoPicker).vm.$emit('update:modelValue', p)
    await flushPromises()
  }
  const btn = (label: string) => w.findAll('button').find((b) => b.text() === label)
  return { w, setText, setPhoto, btn }
}

const posts = () => calls.filter((c) => c.method === 'POST' && c.url === '/api/broadcasts')

describe('Composer', () => {
  it('limit is 1024 with photo, 4096 without', async () => {
    const { w, setText, setPhoto, btn } = await setup()
    await setText('a'.repeat(1025))
    expect(w.find('.count').text()).toBe('1025 / 4096')
    expect(btn('Разослать всем (3)')!.attributes('disabled')).toBeUndefined()
    await setPhoto('file1')
    expect(w.find('.count').text()).toBe('1025 / 1024')
    expect(w.find('.count').classes()).toContain('over')
    expect(btn('Разослать всем (3)')!.attributes('disabled')).toBeDefined()
    await setText('a'.repeat(1024))
    expect(btn('Разослать всем (3)')!.attributes('disabled')).toBeUndefined()
  })

  it('empty text without photo cannot be sent, photo alone can', async () => {
    const { setText, setPhoto, btn } = await setup()
    await setText(' ')
    expect(btn('Разослать всем (3)')!.attributes('disabled')).toBeDefined()
    await setPhoto('file1')
    expect(btn('Разослать всем (3)')!.attributes('disabled')).toBeUndefined()
  })

  it('broadcast needs a second confirm click; cancel goes back', async () => {
    replies['POST /api/broadcasts'] = [json(201, { broadcast: { id: 'b1' } })]
    const { w, setText, btn } = await setup()
    await setText('hi')
    await btn('Разослать всем (3)')!.trigger('click')
    expect(posts()).toHaveLength(0)
    expect(btn('Точно разослать 3 юзерам?')).toBeTruthy()
    await btn('Отмена')!.trigger('click')
    expect(btn('Точно разослать 3 юзерам?')).toBeUndefined()
    await btn('Разослать всем (3)')!.trigger('click')
    await btn('Точно разослать 3 юзерам?')!.trigger('click')
    await flushPromises()
    expect(posts()).toHaveLength(1)
    expect(posts()[0].body).toMatchObject({ text: 'hi' })
    expect(w.emitted('sent')).toHaveLength(1)
  })

  it('idempotency key is stable until success, then renewed', async () => {
    replies['POST /api/broadcasts'] = [json(500, { error: 'internal' }), json(201, { broadcast: { id: 'b1' } }), json(201, { broadcast: { id: 'b2' } })]
    const { w, setText, btn } = await setup()
    const send = async () => {
      await btn('Разослать всем (3)')!.trigger('click')
      await btn('Точно разослать 3 юзерам?')!.trigger('click')
      await flushPromises()
    }
    await setText('hi')
    await send()
    expect(w.find('.err').exists()).toBe(true)
    await send()
    await setText('next')
    await send()
    const keys = posts().map((c) => c.body.idempotencyKey)
    expect(keys).toHaveLength(3)
    expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/)
    expect(keys[1]).toBe(keys[0])
    expect(keys[2]).not.toBe(keys[0])
  })

  it('panel down: no recipients count, broadcast disabled', async () => {
    replies['GET /api/broadcasts/recipients'] = [json(502, { error: 'panel_unavailable' })]
    const { w, setText } = await setup()
    await setText('hi')
    expect(w.text()).toContain('Панель недоступна, количество получателей неизвестно')
    const b = w.findAll('button').find((x) => x.text().startsWith('Разослать всем'))!
    expect(b.attributes('disabled')).toBeDefined()
  })

  it('direct mode posts to the user without confirm', async () => {
    replies['POST /api/users/7/messages'] = [json(201, { id: 5 })]
    const { w, setText, btn } = await setup({ mode: 'direct', userId: 7 })
    expect(calls.some((c) => c.url === '/api/broadcasts/recipients')).toBe(false)
    await setText('hi')
    await btn('Отправить')!.trigger('click')
    await flushPromises()
    expect(calls.at(-1)).toMatchObject({ url: '/api/users/7/messages', method: 'POST', body: { text: 'hi' } })
    expect(w.emitted('sent')?.[0]?.[0]).toMatchObject({ id: 5, text: 'hi', status: 'pending' })
  })
})

describe('checkPhoto', () => {
  const f = (type: string, size: number) => new File([new Uint8Array(size)], 'x', { type })
  it('accepts jpeg/png/webp up to 10 MB', () => {
    expect(checkPhoto(f('image/png', 10 * 1024 * 1024))).toBeUndefined()
    expect(checkPhoto(f('image/webp', 1))).toBeUndefined()
    expect(checkPhoto(f('image/jpeg', 1))).toBeUndefined()
  })
  it('rejects other types and big files', () => {
    expect(checkPhoto(f('image/gif', 1))).toBe('можно загрузить только JPEG, PNG или WebP')
    expect(checkPhoto(f('image/png', 10 * 1024 * 1024 + 1))).toBe('файл больше 10 МБ')
  })
})
