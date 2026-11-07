import { describe, expect, it } from 'vitest'
import { moveSlot, type Layout } from './keyboard-layout'

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

  it('an over-limit layout (old data) can still shrink, but never grow', () => {
    const big = L(Array.from({ length: 10 }, (_, i) => [String(i)]))
    expect(moveSlot(big, '9', cell('0', 'right'), []).rows).toHaveLength(9) // склейка уменьшает число рядов
    expect(moveSlot(big, '9', { kind: 'gap', index: 0 }, []).rows).toHaveLength(10) // перестановка без роста — можно
    const pair = L([...Array.from({ length: 9 }, (_, i) => [String(i)]), ['p', 'q']])
    expect(moveSlot(pair, 'q', { kind: 'gap', index: 0 }, [])).toBe(pair) // разделить пару — рост до 11
  })
})
