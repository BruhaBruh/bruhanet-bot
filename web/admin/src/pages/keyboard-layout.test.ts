import { describe, expect, it } from 'vitest'
import { canJoin, hide, moveInRow, moveRow, show, toggleJoin, type Layout } from './keyboard-layout'

const L = (rows: string[][], hidden: string[] = []): Layout => ({ rows, hidden })

describe('keyboard layout', () => {
  it('moves rows and cells, ignoring the edges', () => {
    const l = L([['a'], ['b', 'c']])
    expect(moveRow(l, 0, 1).rows).toEqual([['b', 'c'], ['a']])
    expect(moveRow(l, 0, -1)).toBe(l)
    expect(moveInRow(l, 1, 0, 1).rows).toEqual([['a'], ['c', 'b']])
    expect(moveInRow(l, 1, 1, 1)).toBe(l)
  })

  it('splits the second cell out and joins the first into the previous row', () => {
    const l = L([['a', 'b'], ['c']])
    expect(toggleJoin(l, 0, 1, []).rows).toEqual([['a'], ['b'], ['c']])
    expect(canJoin(l, 1, 0, [])).toBe(false) // выше уже две
    expect(toggleJoin(L([['a'], ['b', 'c']]), 1, 0, []).rows).toEqual([['a', 'b'], ['c']])
    expect(toggleJoin(L([['a'], ['b']]), 1, 0, []).rows).toEqual([['a', 'b']])
  })

  it('blocks stay alone in a row', () => {
    const l = L([['a'], ['@t']])
    expect(canJoin(l, 1, 0, ['@t'])).toBe(false)
    expect(canJoin(L([['@t'], ['a']]), 1, 0, ['@t'])).toBe(false)
  })

  it('hide drops empty rows; show appends a row, not past 8 rows', () => {
    const h = hide(L([['a'], ['b', 'c']]), 'a')
    expect(h).toEqual({ rows: [['b', 'c']], hidden: ['a'] })
    expect(show(h, 'a')).toEqual({ rows: [['b', 'c'], ['a']], hidden: [] })
    const full = L(Array.from({ length: 8 }, (_, i) => [String(i)]), ['x'])
    expect(show(full, 'x')).toBe(full)
    expect(toggleJoin(L([...full.rows.slice(0, 7), ['p', 'q']]), 7, 1, []).rows).toHaveLength(8)
  })
})
