import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearQueryCache, useQuery } from './query'

afterEach(() => {
  vi.unstubAllGlobals()
  clearQueryCache()
})

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
const settle = () => act(async () => {})

// управляемый fetch: каждый вызов ждёт resolve()
function gate() {
  const waiting: Array<(r: Response) => void> = []
  const m = vi.fn(() => new Promise<Response>((res) => waiting.push(res)))
  vi.stubGlobal('fetch', m)
  return { m, resolve: (body: unknown, status = 200) => act(async () => waiting.shift()?.(json(body, status))) }
}

function View({ k = '/m/api/x' }: { k?: string }) {
  const q = useQuery<{ v: number }>(k)
  return (
    <p>
      {q.isLoading ? 'loading' : q.error ? 'error' : `v=${q.data?.v}`}
      {q.isRefreshing ? ' refreshing' : ''}
    </p>
  )
}

describe('useQuery', () => {
  it('shows loading on the first visit and the data afterwards', async () => {
    const g = gate()
    render(<View />)
    expect(screen.getByText('loading')).toBeTruthy()
    await g.resolve({ v: 1 })
    expect(screen.getByText('v=1')).toBeTruthy()
  })

  it('on a repeat visit shows the cached data at once and refreshes in the background', async () => {
    const g = gate()
    const first = render(<View />)
    await g.resolve({ v: 1 })
    first.unmount()
    render(<View />)
    // сразу кэш, без «loading», и идёт обновление
    expect(screen.getByText('v=1 refreshing')).toBeTruthy()
    expect(g.m).toHaveBeenCalledTimes(2)
    await g.resolve({ v: 2 })
    expect(screen.getByText('v=2')).toBeTruthy()
  })

  it('keeps showing the stale data when the refresh fails', async () => {
    const g = gate()
    const first = render(<View />)
    await g.resolve({ v: 1 })
    first.unmount()
    render(<View />)
    await g.resolve({ error: 'x' }, 502)
    expect(screen.getByText('v=1')).toBeTruthy()
  })

  it('shows the error only while there is nothing cached, and a retry shows loading again', async () => {
    const g = gate()
    function Retry() {
      const q = useQuery<{ v: number }>('/m/api/x')
      return q.error ? <button onClick={() => void q.refetch()}>again</button> : <p>{q.isLoading ? 'loading' : `v=${q.data?.v}`}</p>
    }
    render(<Retry />)
    await g.resolve({ error: 'x' }, 500)
    const btn = screen.getByRole('button', { name: 'again' })
    await act(async () => btn.click())
    expect(screen.getByText('loading')).toBeTruthy()
    await g.resolve({ v: 5 })
    expect(screen.getByText('v=5')).toBeTruthy()
  })

  it('sends one request for several simultaneous mounts of the same key', async () => {
    const g = gate()
    render(
      <>
        <View />
        <View />
      </>,
    )
    await settle()
    expect(g.m).toHaveBeenCalledTimes(1)
    await g.resolve({ v: 3 })
    expect(screen.getAllByText('v=3')).toHaveLength(2)
  })

  it('mutate updates every subscriber without a request', async () => {
    const g = gate()
    function Mut() {
      const q = useQuery<{ v: number }>('/m/api/x')
      return <button onClick={() => q.mutate((old) => ({ v: (old?.v ?? 0) + 10 }))}>bump</button>
    }
    render(
      <>
        <View />
        <Mut />
      </>,
    )
    await g.resolve({ v: 1 })
    await act(async () => screen.getByRole('button', { name: 'bump' }).click())
    expect(screen.getByText('v=11')).toBeTruthy()
    expect(g.m).toHaveBeenCalledTimes(1)
  })

  it('keys are independent', async () => {
    const g = gate()
    render(
      <>
        <View k="/m/api/a" />
        <View k="/m/api/b" />
      </>,
    )
    await settle()
    expect(g.m).toHaveBeenCalledTimes(2)
  })
})
