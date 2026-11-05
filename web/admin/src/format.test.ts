import { describe, expect, test } from 'vitest'
import { formatCount, formatDelta, formatFullDay, formatRub, formatShortDate, mskDate, plural } from './format'

test('formatRub uses ru-RU grouping', () => {
  expect(formatRub(2350)).toBe('2\u00a0350\u00a0₽')
})

test('mskDate uses Moscow day and offset', () => {
  const now = new Date('2026-09-12T22:30:00Z') // 13.09 01:30 МСК
  expect(mskDate(now)).toBe('2026-09-13')
  expect(mskDate(now, -29)).toBe('2026-08-15')
})

test('plural picks russian forms', () => {
  const f: [string, string, string] = ['оплата', 'оплаты', 'оплат']
  expect([0, 1, 2, 4, 5, 11, 12, 14, 21, 22, 25, 101, 111].map((n) => plural(n, f))).toEqual([
    'оплат', 'оплата', 'оплаты', 'оплаты', 'оплат', 'оплат', 'оплат', 'оплат', 'оплата', 'оплаты', 'оплат', 'оплата', 'оплат',
  ])
})

test('formatFullDay keeps the year and uses the Moscow date', () => {
  expect(formatFullDay('2026-10-31T22:30:00Z')).toBe('1 нояб. 2026')
})

test('formatCount groups with nbsp', () => {
  expect(formatCount(3214)).toBe('3 214')
  expect(formatCount(7)).toBe('7')
})

describe('formatShortDate (МСК)', () => {
  const now = new Date('2026-11-06T12:00:00Z') // 15:00 МСК

  test('today and yesterday by Moscow day', () => {
    expect(formatShortDate('2026-11-06T11:32:00Z', now)).toBe('сегодня, 14:32')
    expect(formatShortDate('2026-11-05T21:30:00Z', now)).toBe('сегодня, 00:30') // в UTC ещё вчера
    expect(formatShortDate('2026-11-05T18:47:00Z', now)).toBe('вчера, 21:47')
    expect(formatShortDate('2026-11-04T20:59:00Z', now)).toBe('4 нояб., 23:59') // 23:59 МСК 4-го — уже не вчера
  })

  test('older: short month as in the macro', () => {
    expect(formatShortDate('2026-11-04T11:32:00Z', now)).toBe('4 нояб., 14:32')
    expect(formatShortDate('2026-03-03T06:22:00Z', now)).toBe('3 марта, 09:22')
    expect(formatShortDate('2026-06-15T06:22:00Z', now)).toBe('15 июня, 09:22')
  })

  test('another year is spelled out', () => {
    expect(formatShortDate('2025-11-04T11:32:00Z', now)).toBe('4 нояб. 2025, 14:32')
  })
})

describe('formatDelta', () => {
  test('signed percent, rounded', () => {
    expect(formatDelta(112, 100)).toBe('+12%')
    expect(formatDelta(96, 100)).toBe('-4%')
    expect(formatDelta(0, 100)).toBe('-100%')
    expect(formatDelta(100, 100)).toBe('0%')
    expect(formatDelta(1000, 3)).toBe('+33233%')
  })

  test('no base, no delta', () => {
    expect(formatDelta(500, 0)).toBeUndefined()
    expect(formatDelta(0, 0)).toBeUndefined()
  })
})
