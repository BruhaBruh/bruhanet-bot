import { expect, test } from 'vitest'
import { generateCode, promoBody, promoGives, promoUntil, promoUsage, validatePromo, type PromoForm } from './promo'

test('promoGives formats each kind', () => {
  expect(promoGives({ kind: 'percent', value: 25 })).toBe('−25% к оплате')
  expect(promoGives({ kind: 'rub', value: 1500 })).toBe('−1 500 ₽')
  expect(promoGives({ kind: 'days', value: 7 })).toBe('+7 дней')
  expect(promoGives({ kind: 'days', value: 1 })).toBe('+1 день')
  expect(promoGives({ kind: 'days', value: 3 })).toBe('+3 дня')
})

test('promoUsage shows ∞ without limit', () => {
  expect(promoUsage({ used: 3, maxActivations: null })).toBe('использовано 3 / ∞')
  expect(promoUsage({ used: 17, maxActivations: 50 })).toBe('использовано 17 / 50')
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
