import { describe, expect, it } from 'vitest'
import type { KbAction, KbCellOut, TextItem } from '../types'
import { buildGraph, layout, screenHeight, targetsOf, type UxMapTables } from './uxmap'

const cell = (id: string, action: string, over: Partial<KbCellOut> = {}): KbCellOut => ({ id, action, conditions: [], locked: false, template: false, effective: { label: id, preview: id, style: '' }, ...over })
const item = (key: string, group: string, rows: KbCellOut[][] = []): TextItem => ({
  key, group, title: key, kind: 'message', default: '', body: 'тело', overridden: false, vars: [], when: '', buttons: [], style: '',
  keyboard: rows.length ? { rows, default: rows, overridden: false, locked: [] } : null,
})
const actions: KbAction[] = [{ id: 'route:trial', title: 'Пробный', kind: 'route', guard: 'trial_available', bound: false }]
const map: UxMapTables = {
  entries: [{ id: 'cmd_start', kind: 'command', title: '/start', when: '', targets: [{ to: 'menu' }] }, { id: 'cmd_x', kind: 'command', title: '/x', when: '', targets: [{ to: 'other' }] }],
  transitions: [
    { from: '*', action: 'route:pay', to: ['pay'] },
    { from: 'pay', action: 'route:pay', to: ['special'] },
    { from: '*', action: 'route:trial', to: ['trial'] },
  ],
  links: [{ from: 'pay', to: 'trial', label: 'ввод' }],
}

describe('uxmap', () => {
  it('own transition beats the generic one', () => {
    expect(targetsOf(map, 'menu', 'route:pay')).toEqual(['pay'])
    expect(targetsOf(map, 'pay', 'route:pay')).toEqual(['special'])
    expect(targetsOf(map, 'menu', 'copy:x')).toEqual([])
  })

  it('builds edges per visible button; conditions hide buttons and their arrows', () => {
    const items = [item('menu', 'Меню', [[cell('b_pay', 'route:pay'), cell('b_trial', 'route:trial')]]), item('pay', 'Оплата'), item('trial', 'Пробный')]
    const off = buildGraph(items, map, { on: new Set(), actions })
    expect(off.edges.filter((e) => e.kind === 'button').map((e) => `${e.sourceHandle}->${e.target}`)).toEqual(['b_pay->pay'])
    const on = buildGraph(items, map, { on: new Set(['trial_available']), actions })
    expect(on.edges.filter((e) => e.kind === 'button').map((e) => `${e.sourceHandle}->${e.target}`)).toEqual(['b_pay->pay', 'b_trial->trial'])
    expect(on.edges.find((e) => e.kind === 'link')).toMatchObject({ source: 'pay', target: 'trial', label: 'ввод' })
  })

  it('entries appear only with a shown target; group filter drops screens and their edges', () => {
    const items = [item('menu', 'Меню'), item('pay', 'Оплата')]
    const all = buildGraph(items, map, { on: new Set(), actions })
    expect(all.nodes.map((n) => n.id).sort()).toEqual(['entry:cmd_start', 'menu', 'pay'])
    const menuOnly = buildGraph(items, map, { on: new Set(), actions, groups: new Set(['Меню']) })
    expect(menuOnly.nodes.map((n) => n.id).sort()).toEqual(['entry:cmd_start', 'menu'])
    expect(menuOnly.edges.map((e) => e.id)).toEqual(['entry:cmd_start->menu'])
  })

  it('layout gives every node a distinct position, left to right', () => {
    const items = [item('menu', 'Меню', [[cell('b', 'route:pay')]]), item('pay', 'Оплата')]
    const g = buildGraph(items, map, { on: new Set(), actions })
    const laid = layout(g.nodes, g.edges)
    const at = (id: string) => laid.find((n) => n.id === id)!
    expect(at('entry:cmd_start').x).toBeLessThan(at('menu').x)
    expect(at('menu').x).toBeLessThan(at('pay').x)
    expect(new Set(laid.map((n) => `${n.x},${n.y}`)).size).toBe(laid.length)
  })

  it('height grows with text and keyboard rows', () => {
    expect(screenHeight('короткий', 1)).toBeLessThan(screenHeight('длинный\n'.repeat(5), 1))
    expect(screenHeight('x', 1)).toBeLessThan(screenHeight('x', 4))
  })
})
