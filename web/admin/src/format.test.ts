import { expect, test } from 'vitest'
import { formatDate, formatMonthYear, formatRub, mskDate, plural } from './format'

test('formatRub uses ru-RU grouping', () => {
  expect(formatRub(2350)).toBe('2\u00a0350\u00a0₽')
})

test('formatDate renders Moscow time', () => {
  expect(formatDate('2026-09-12T09:30:00Z')).toBe('12.09.2026 12:30')
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

test('formatMonthYear is genitive Moscow month (for «с … года»)', () => {
  expect(formatMonthYear('2026-09-12T09:30:00Z')).toBe('сентября 2026')
  expect(formatMonthYear('2026-10-31T22:30:00Z')).toBe('ноября 2026')
})
