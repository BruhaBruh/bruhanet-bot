import { expect, test } from 'vitest'
import { formatDate, formatRub, mskDate } from './format'

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
