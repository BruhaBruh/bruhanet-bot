import { describe, expect, it } from 'vitest'
import { checkPhoto } from './photo'

describe('checkPhoto', () => {
  const f = (type: string, size: number) => new File([new Uint8Array(size)], 'x', { type })
  it('accepts jpeg/png/webp up to 10 MB', () => {
    expect(checkPhoto(f('image/png', 10 * 1024 * 1024))).toBeUndefined()
    expect(checkPhoto(f('image/webp', 1))).toBeUndefined()
    expect(checkPhoto(f('image/jpeg', 1))).toBeUndefined()
  })
  it('rejects other types and big files', () => {
    expect(checkPhoto(f('image/gif', 1))).toBe('можно загрузить только JPEG, PNG или WebP')
    expect(checkPhoto(f('image/png', 10 * 1024 * 1024 + 1))).toBe('файл больше 10 МБ')
  })
})
