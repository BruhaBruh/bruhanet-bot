import { describe, expect, it } from 'vitest'
import type { KbAction, KbCellOut } from '../types'
import { addButton, addTemplate, fromLayout, freeId, payload, sameKb, templatesLeft, toLayout, usedConditions, visibleRows } from './message-keyboard'

const c = (id: string, over: Partial<KbCellOut> = {}): KbCellOut => ({ id, action: 'route:menu', conditions: [], locked: false, template: false, ...over })
const actions: KbAction[] = [
  { id: 'route:menu', title: 'Меню', kind: 'route', bound: false },
  { id: 'route:trial', title: 'Пробный', kind: 'route', guard: 'trial_available', bound: false },
]

describe('message keyboard', () => {
  it('payload drops computed fields and empty optionals', () => {
    const x = c('a', { key: 'button.a', label: '', style: 'danger', locked: true, effective: { label: 'A', preview: 'A', style: 'danger' }, conditions: [{ id: 'in_panel', not: true }, { id: 'gifts_on' }] })
    expect(payload([[x]])).toEqual({ rows: [[{ id: 'a', key: 'button.a', style: 'danger', action: 'route:menu', conditions: [{ id: 'in_panel', not: true }, { id: 'gifts_on' }] }]] })
  })

  it('layout round trip keeps cell data and drops unknown ids', () => {
    const rows = [[c('a'), c('b')], [c('d')]]
    const l = toLayout(rows)
    expect(l.rows).toEqual([['a', 'b'], ['d']])
    expect(fromLayout(rows, { rows: [['d'], ['b', 'a'], ['zzz']], hidden: [] }).map((r) => r.map((x) => x.id))).toEqual([['d'], ['b', 'a'], []])
    expect(sameKb(rows, fromLayout(rows, l))).toBe(true)
  })

  it('freeId skips taken ids; templates are offered once', () => {
    const rows = addButton(addButton([]))
    expect(rows.flat().map((x) => x.id)).toEqual(['btn_1', 'btn_2'])
    expect(freeId(rows)).toBe('btn_3')
    const t = { id: 'tariffs', title: 'Тарифы', key: 'button.tariff' }
    const withT = addTemplate(rows, t)
    expect(withT.at(-1)![0]).toMatchObject({ id: 'tpl_tariffs', action: 'template:tariffs', template: true })
    expect(templatesLeft(withT, [t])).toEqual([])
    expect(templatesLeft(rows, [t])).toEqual([t])
  })

  it('visibleRows: conditions with НЕ, action guard, empty rows vanish', () => {
    const rows = [
      [c('a', { conditions: [{ id: 'in_panel' }] }), c('b', { conditions: [{ id: 'in_panel', not: true }] })],
      [c('t', { action: 'route:trial' })],
    ]
    const ids = (on: string[]) => visibleRows(rows, new Set(on), actions).map((r) => r.map((x) => x.id))
    expect(ids([])).toEqual([['b']])
    expect(ids(['in_panel'])).toEqual([['a']])
    expect(ids(['trial_available'])).toEqual([['b'], ['t']])
    expect(usedConditions(rows, actions).sort()).toEqual(['in_panel', 'trial_available'])
  })
})
