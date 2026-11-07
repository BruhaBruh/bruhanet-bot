import { describe, expect, it } from 'vitest'
import { moveSlot } from './keyboard-layout'
import { fromLayout, normalizeInline, toLayout } from './button-rows'

const b = (text: string, inline?: boolean) => ({ text, ...(inline ? { inline } : {}) })

describe('button rows via layout', () => {
  it('round-trips inline flags through the layout', () => {
    const items = [b('a'), b('b', true), b('c')]
    expect(toLayout(items).rows).toEqual([['0', '1'], ['2']])
    expect(fromLayout(items, toLayout(items))).toEqual(items)
  })

  it('a drop regroups the buttons and rewrites inline', () => {
    const items = [b('a'), b('b', true), b('c')]
    const moved = moveSlot(toLayout(items), '2', { kind: 'gap', index: 0 }, [])
    expect(fromLayout(items, moved)).toEqual([b('c'), b('a'), b('b', true)])
    const joined = moveSlot(toLayout([b('a'), b('b')]), '1', { kind: 'beside', id: '0', after: true }, [])
    expect(fromLayout([b('a'), b('b')], joined)).toEqual([b('a'), b('b', true)])
  })

  it('normalizeInline fixes orphans', () => {
    expect(normalizeInline([b('a', true), b('b', true), b('c', true), b('d', true)]).map((x) => !!x.inline)).toEqual([false, true, false, true])
  })
})
