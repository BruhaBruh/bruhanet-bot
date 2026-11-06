import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { TextVar } from '../types'
import { VarChips } from './VarChips'

const v = (name: string, description: string, sample: string): TextVar => ({ name, description, sample, raw: false, max: 10 })

describe('VarChips', () => {
  it('keyboard focus shows description and sample, chip still inserts, no title attribute', async () => {
    const onInsert = vi.fn()
    render(<VarChips title="Общие" vars={[v('name', 'Имя пользователя', 'Иван'), v('promoLink', 'Своя ссылка', '')]} onInsert={onInsert} />)
    const chips = screen.getAllByRole('button')
    for (const c of chips) expect(c.hasAttribute('title')).toBe(false)

    await userEvent.tab()
    const tip = await screen.findByRole('tooltip')
    expect(tip.textContent).toContain('Имя пользователя')
    expect(tip.textContent).toContain('Например: Иван')

    await userEvent.keyboard('{Enter}')
    expect(onInsert).toHaveBeenCalledWith('name')

    await userEvent.tab()
    expect((await screen.findByRole('tooltip')).textContent).toBe('Своя ссылка') // без примера второй строки нет
  })

  it('hover shows the tooltip too', async () => {
    render(<VarChips title="Общие" vars={[v('name', 'Имя пользователя', 'Иван')]} onInsert={() => {}} />)
    await userEvent.hover(screen.getByRole('button'))
    expect((await screen.findByRole('tooltip')).textContent).toContain('Имя пользователя')
  })
})
