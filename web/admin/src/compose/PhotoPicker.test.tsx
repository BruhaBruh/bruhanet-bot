import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { PhotoPicker } from './PhotoPicker'

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const png = new File(['x'], 'a.png', { type: 'image/png' })
const gif = new File(['x'], 'a.gif', { type: 'image/gif' })

let uploads: number

beforeEach(() => {
  uploads = 0
  vi.stubGlobal('fetch', async () => {
    uploads++
    return new Response(JSON.stringify({ photoId: `file${uploads}` }), { status: 200 })
  })
  URL.createObjectURL = () => 'blob:x'
  URL.revokeObjectURL = () => {}
})
afterEach(() => vi.unstubAllGlobals())

const zone = () => screen.getByRole('button', { name: /Добавить фото/ }).parentElement!

it('drag-and-drop uploads the file and reports photoId with preview url', async () => {
  const onChange = vi.fn()
  render(<PhotoPicker onChange={onChange} />)
  fireEvent.drop(zone(), { dataTransfer: { files: [png] } })
  await flush()
  expect(onChange).toHaveBeenCalledWith('file1', 'blob:x')
})

it('dropped file goes through checkPhoto', async () => {
  const onChange = vi.fn()
  render(<PhotoPicker onChange={onChange} />)
  fireEvent.drop(zone(), { dataTransfer: { files: [gif] } })
  await flush()
  expect(uploads).toBe(0)
  expect(onChange).not.toHaveBeenCalled()
  expect(screen.getByRole('alert').textContent).toBe('можно загрузить только JPEG, PNG или WebP')
})

it('disabled picker ignores drops', async () => {
  render(<PhotoPicker disabled onChange={() => {}} />)
  fireEvent.drop(zone(), { dataTransfer: { files: [png] } })
  await flush()
  expect(uploads).toBe(0)
})

it('an uploaded picture is previewed from the server, a legacy file_id gets a placeholder', () => {
  const a = render(<PhotoPicker value="pic1" onChange={() => {}} />)
  expect(a.container.querySelector('img')!.getAttribute('src')).toBe('/api/photos/pic1')
  a.unmount()
  const b = render(<PhotoPicker legacy onChange={() => {}} />)
  expect(b.container.querySelector('img')).toBeNull()
  expect(screen.getByText('Фото из рассылки')).toBeTruthy()
})

it('replace uploads a new file, delete clears the value', async () => {
  const onChange = vi.fn()
  const { container } = render(<PhotoPicker value="old" onChange={onChange} />)
  expect(screen.getByText('Фото из рассылки')).toBeTruthy()
  fireEvent.change(container.querySelector('input')!, { target: { files: [png] } })
  await flush()
  expect(onChange).toHaveBeenLastCalledWith('file1', 'blob:x')
  expect(screen.getByText('a.png')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
  expect(onChange).toHaveBeenLastCalledWith(undefined)
})
