// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { TgAvatar, TgBadge, TgCell } from './index'
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
})
