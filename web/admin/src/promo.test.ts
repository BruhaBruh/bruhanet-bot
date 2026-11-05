import { expect, test } from 'vitest'
import { generateCode, promoBody, promoStatus, promoUntil, promoValue, validatePromo, type PromoForm } from './promo'

test('promoValue formats each kind', () => {
  expect(promoValue({ kind: 'percent', value: 25 })).toBe('−25%')
  expect(promoValue({ kind: 'rub', value: 1500 })).toBe('−1\u00a0500\u00a0₽')
  expect(promoValue({ kind: 'days', value: 7 })).toBe('+7\u00a0дн.')
})

test('promoStatus: disabled, then expired, then limit', () => {
  const base = { disabled: false, validUntil: null, used: 0, maxActivations: null }
  const now = new Date('2026-11-06T12:00:00Z')
  expect(promoStatus(base, now)).toBe('active')
  expect(promoStatus({ ...base, maxActivations: 2, used: 1 }, now)).toBe('active')
  expect(promoStatus({ ...base, maxActivations: 2, used: 2 }, now)).toBe('limit')
  // validUntil включительно
  expect(promoStatus({ ...base, validUntil: '2026-11-06' }, now)).toBe('active')
  expect(promoStatus({ ...base, validUntil: '2026-11-05' }, now)).toBe('expired')
  // полночь по МСК наступает раньше, чем по UTC
  expect(promoStatus({ ...base, validUntil: '2026-11-06' }, new Date('2026-11-06T21:30:00Z'))).toBe('expired')
  expect(promoStatus({ ...base, validUntil: '2026-11-05', maxActivations: 1, used: 1 }, now)).toBe('expired')
  expect(promoStatus({ disabled: true, validUntil: '2026-11-05', maxActivations: 1, used: 1 }, now)).toBe('disabled')
})

test('promoUntil', () => {
  expect(promoUntil('2026-08-31')).toBe('до 31.08')
  expect(promoUntil(null)).toBe('—')
})

const base: PromoForm = { code: '', kind: 'percent', value: '25', maxActivations: '', validUntil: '', onlyNew: false, user: '' }

test('validatePromo ranges per kind', () => {
  expect(validatePromo(base)).toBe('')
  for (const value of ['0', '101', '', '2.5', '-1']) expect(validatePromo({ ...base, value })).not.toBe('')
  expect(validatePromo({ ...base, value: '100' })).toBe('')
  expect(validatePromo({ ...base, value: '1' })).toBe('')
  expect(validatePromo({ ...base, kind: 'rub', value: '1000000' })).toBe('')
  expect(validatePromo({ ...base, kind: 'rub', value: '1000001' })).not.toBe('')
  expect(validatePromo({ ...base, kind: 'days', value: '3650' })).toBe('')
  expect(validatePromo({ ...base, kind: 'days', value: '3651' })).not.toBe('')
})

test('validatePromo code, limit, user', () => {
  expect(validatePromo({ ...base, code: ' summer-25 ' })).toBe('')
  expect(validatePromo({ ...base, code: 'AB' })).not.toBe('')
  expect(validatePromo({ ...base, code: 'КОД123' })).not.toBe('')
  expect(validatePromo({ ...base, maxActivations: '0' })).not.toBe('')
  expect(validatePromo({ ...base, maxActivations: '5' })).toBe('')
  expect(validatePromo({ ...base, user: '@vasya' })).toBe('')
  expect(validatePromo({ ...base, user: '12345' })).toBe('')
  expect(validatePromo({ ...base, user: 'vasya' })).not.toBe('')
})

test('promoBody normalizes', () => {
  expect(promoBody({ ...base, code: ' sum25 ', maxActivations: '', validUntil: '', user: ' @v ' })).toEqual({
    code: 'SUM25', kind: 'percent', value: 25, maxActivations: null, validUntil: null, onlyNew: false, user: '@v',
  })
  expect(promoBody({ ...base, maxActivations: '5', validUntil: '2026-12-31' })).toMatchObject({ maxActivations: 5, validUntil: '2026-12-31' })
})

test('generateCode uses server alphabet', () => {
  for (let i = 0; i < 50; i++) expect(generateCode()).toMatch(/^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{8}$/)
})
