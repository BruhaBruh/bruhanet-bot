// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { TgAvatar, TgBadge, TgButton, TgCell, TgInput, TgSelect } from './index'
import { readFileSync } from 'node:fs'

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: {} }, { path: '/x', component: {} }] })

// vitest stubs css imports, so read the file directly (cwd = web/admin)
const css = readFileSync('src/ui/theme.css', 'utf8')

describe('ui kit', () => {
  it('TgAvatar: same text -> same color class', () => {
    const cls = (t: string) => mount(TgAvatar, { props: { text: t } }).classes().find((c) => /^tg-av-c\d$/.test(c))
    expect(cls('@vasya')).toBeTruthy()
    expect(cls('@vasya')).toBe(cls('@vasya'))
  })
  it('TgAvatar: explicit color wins', () => {
    expect(mount(TgAvatar, { props: { text: 'a', color: 3 } }).classes()).toContain('tg-av-c3')
  })
  it('TgBadge: count 0 renders nothing', () => {
    expect(mount(TgBadge, { props: { count: 0 } }).find('.tg-badge').exists()).toBe(false)
    expect(mount(TgBadge, { props: { count: 3 } }).text()).toBe('3')
  })
  it('TgCell: to -> link, otherwise div', () => {
    const g = { global: { plugins: [router] } }
    expect(mount(TgCell, { props: { title: 't', to: '/x' }, ...g }).element.tagName).toBe('A')
    expect(mount(TgCell, { props: { title: 't' }, ...g }).element.tagName).toBe('DIV')
  })
  it('TgCell: subtitle is single-line ellipsis, text block can shrink', () => {
    const w = mount(TgCell, { props: { title: 't', subtitle: 'очень длинная подпись' } })
    const sub = w.find('.tg-sub')
    expect(sub.exists()).toBe(true)
    expect(css).toMatch(/\.tg-sub\s*\{[^}]*text-overflow:\s*ellipsis/)
    expect(css).toMatch(/\.tg-cell-body\s*\{[^}]*min-width:\s*0/)
    expect(w.find('.tg-cell-body').exists()).toBe(true)
  })
  it('TgInput/TgSelect: aria-label reaches the native control', () => {
    expect(mount(TgInput, { props: { modelValue: '' }, attrs: { 'aria-label': 'Поиск' } }).find('input').attributes('aria-label')).toBe('Поиск')
    expect(mount(TgInput, { props: { modelValue: '', placeholder: 'ID' } }).find('input').attributes('aria-label')).toBe('ID')
    const sel = mount(TgSelect, { props: { modelValue: 'a', options: [{ value: 'a', label: 'A' }] }, attrs: { 'aria-label': 'Статус' } })
    expect(sel.find('select').attributes('aria-label')).toBe('Статус')
    expect(sel.find('label').attributes('aria-label')).toBeUndefined()
  })
  it('TgButton: type=button by default, overridable; loading sets aria-busy', () => {
    expect(mount(TgButton).attributes('type')).toBe('button')
    expect(mount(TgButton, { attrs: { type: 'submit' } }).attributes('type')).toBe('submit')
    const b = mount(TgButton, { props: { loading: true } })
    expect(b.attributes('aria-busy')).toBe('true')
    expect(b.find('[role=status]').exists()).toBe(false)
  })
})
