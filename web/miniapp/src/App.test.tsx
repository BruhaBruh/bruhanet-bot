import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from './App'

describe('App', () => {
  it('renders with the admin design primitives', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: 'Кабинет' })).toBeTruthy()
  })
})
