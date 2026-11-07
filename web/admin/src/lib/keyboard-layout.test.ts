import { describe, expect, it } from 'vitest'
import { hide, moveSlot, show, type Layout } from './keyboard-layout'

const L = (rows: string[][], hidden: string[] = []): Layout => ({ rows, hidden })
const cell = (id: string, h: 'left' | 'right' = 'right', v: 'top' | 'bottom' = 'bottom') => ({ kind: 'cell' as const, id, h, v })

describe('moveSlot', () => {
  it('joins a single-cell row on the pointer side; the emptied row disappears', () => {
    const l = L([['a'], ['b'], ['c']])
    expect(moveSlot(l, 'c', cell('a', 'right'), []).rows).toEqual([['a', 'c'], ['b']])
    expect(moveSlot(l, 'c', cell('a', 'left'), []).rows).toEqual([['c', 'a'], ['b']])
    expect(moveSlot(l, 'a', { kind: 'beside', id: 'b', after: true }, []).rows).toEqual([['b', 'a'], ['c']])
  })

  it('swaps inside a row and starts a new row when the target row is full', () => {
    const l = L([['a', 'b'], ['c']])
    expect(moveSlot(l, 'a', cell('b', 'right'), []).rows).toEqual([['b', 'a'], ['c']])
    expect(moveSlot(l, 'c', cell('a', 'right', 'top'), []).rows).toEqual([['c'], ['a', 'b']])
    expect(moveSlot(l, 'c', cell('b', 'left', 'bottom'), []).rows).toEqual([['a', 'b'], ['c']]) // то же место
    expect(moveSlot(l, 'c', cell('b', 'left', 'bottom'), [])).toBe(l)
  })

  it('splits a cell out of a pair via a gap; gap index counts the original rows', () => {
    const l = L([['a', 'b'], ['c']])
    expect(moveSlot(l, 'b', { kind: 'gap', index: 2 }, []).rows).toEqual([['a'], ['c'], ['b']])
    expect(moveSlot(l, 'b', { kind: 'gap', index: 0 }, []).rows).toEqual([['b'], ['a'], ['c']])
    expect(moveSlot(l, 'c', { kind: 'gap', index: 0 }, []).rows).toEqual([['c'], ['a', 'b']])
    expect(moveSlot(l, 'c', { kind: 'gap', index: 2 }, [])).toBe(l) // уже последний
    expect(moveSlot(l, 'c', { kind: 'gap', index: 1 }, [])).toBe(l) // на месте
  })

  it('blocks are always alone: nothing joins them, they join nothing', () => {
    const l = L([['a'], ['@t'], ['b']])
    expect(moveSlot(l, 'b', cell('@t', 'right', 'top'), ['@t']).rows).toEqual([['a'], ['b'], ['@t']])
    expect(moveSlot(l, '@t', cell('a', 'right', 'bottom'), ['@t']).rows).toEqual([['a'], ['@t'], ['b']].slice(0, 3))
    expect(moveSlot(l, '@t', { kind: 'beside', id: 'a', after: true }, ['@t'])).toBe(l)
    expect(moveSlot(L([['a'], ['b'], ['@t']]), '@t', cell('a', 'left', 'top'), ['@t']).rows).toEqual([['@t'], ['a'], ['b']])
  })

  it('never goes past 8 rows', () => {
    const full = L(Array.from({ length: 8 }, (_, i) => [String(i)]).slice(0, 7).concat([['p', 'q']]))
    expect(moveSlot(full, 'p', { kind: 'gap', index: 0 }, [])).toBe(full)
    expect(moveSlot(full, '0', cell('p', 'right', 'top'), []).rows).toHaveLength(8)
  })

  it('hide drops empty rows; show appends a row, not past 8 rows', () => {
    const h = hide(L([['a'], ['b', 'c']]), 'a')
    expect(h).toEqual({ rows: [['b', 'c']], hidden: ['a'] })
    expect(show(h, 'a')).toEqual({ rows: [['b', 'c'], ['a']], hidden: [] })
    const full = L(Array.from({ length: 8 }, (_, i) => [String(i)]), ['x'])
    expect(show(full, 'x')).toBe(full)
  })
})
