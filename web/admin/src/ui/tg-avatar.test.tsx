import { fireEvent, render } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { TgAvatar } from './tg-avatar'
import { failedAvatars } from './tg-avatar-failed'

beforeEach(() => failedAvatars.clear())

describe('TgAvatar', () => {
  it('renders a lazy decorative img from the avatars endpoint', () => {
    const { container } = render(<TgAvatar tg={42} name="Анна" />)
    const img = container.querySelector('img')!
    expect(img.getAttribute('src')).toBe('/api/avatars/42')
    expect(img.getAttribute('loading')).toBe('lazy')
    expect(img.getAttribute('decoding')).toBe('async')
    expect(img.getAttribute('alt')).toBe('')
  })

  it('uses /bot for the bot and the letter avatar without an id', () => {
    const bot = render(<TgAvatar tg="bot" name="Бот" />)
    expect(bot.container.querySelector('img')!.getAttribute('src')).toBe('/api/avatars/bot')
    const none = render(<TgAvatar name="Анна К" />)
    expect(none.container.querySelector('img')).toBeNull()
    expect(none.container.textContent).toBe('АК')
  })

  it('falls back to the letter avatar on error and remembers it', () => {
    const first = render(<TgAvatar tg={7} name="Иван Петров" />)
    fireEvent.error(first.container.querySelector('img')!)
    expect(first.container.querySelector('img')).toBeNull()
    expect(first.container.textContent).toBe('ИП')
    // другой экземпляр того же id не просит картинку снова
    const second = render(<TgAvatar tg={7} name="Иван Петров" />)
    expect(second.container.querySelector('img')).toBeNull()
    // чужой id не затронут
    expect(render(<TgAvatar tg={8} name="X" />).container.querySelector('img')).not.toBeNull()
  })
})
