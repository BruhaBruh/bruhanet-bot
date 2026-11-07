import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { KbCellOut, TextItem } from '../types'
import UxMap from './UxMap'

// xyflow в jsdom не измеряет узлы: подмена рисует узлы как есть, а стрелки — списком «из → в».
vi.mock('@xyflow/react', () => ({
  ReactFlow: ({ nodes, edges, nodeTypes }: { nodes: { id: string; type: string; data: unknown }[]; edges: { id: string }[]; nodeTypes: Record<string, (p: { data: unknown }) => ReactNode> }) => (
    <div>
      {nodes.map((n) => {
        const Node = nodeTypes[n.type]
        return (
          <div key={n.id} data-testid={`node:${n.id}`}>
            <Node data={n.data} />
          </div>
        )
      })}
      <ul aria-label="стрелки">
        {edges.map((e) => (
          <li key={e.id}>{e.id}</li>
        ))}
      </ul>
    </div>
  ),
  Handle: () => null,
  Background: () => null,
  Controls: () => null,
  MarkerType: { ArrowClosed: 'arrowclosed' },
  Position: { Left: 'left', Right: 'right' },
}))

type Reply = () => Response
const json = (body: unknown): Reply => () => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
const cell = (id: string, action: string, over: Partial<KbCellOut> = {}): KbCellOut => ({ id, action, conditions: [], locked: false, template: false, effective: { label: id, preview: id, style: '' }, ...over })
const item = (key: string, group: string, body: string, rows: KbCellOut[][] = []): TextItem => ({
  key, group, title: key, kind: 'message', default: body, body, overridden: false, vars: [], when: '', buttons: [], style: '',
  keyboard: rows.length ? { rows, default: rows, overridden: false, locked: [] } : null,
})

let replies: Record<string, Reply>
beforeEach(() => {
  replies = {
    'GET /api/texts': json({
      items: [
        item('menu', 'Меню', 'Привет, {{.name}}', [[cell('go_pay', 'route:pay', { effective: { label: 'Оплата', preview: 'Оплата', style: '' } }), cell('go_trial', 'route:trial', { effective: { label: 'Пробный', preview: 'Пробный', style: '' } })]]),
        item('pay', 'Оплата', 'Выбор тарифа'),
        item('trial', 'Пробный', 'Пробный период'),
      ],
      globals: [{ name: 'name', description: '', sample: 'Иван', raw: false, max: 64 }],
      conditions: [
        { id: 'trial_available', title: 'Пробный доступен', description: '' },
        { id: 'gifts_on', title: 'Подарки доступны', description: '' },
      ],
      actions: [{ id: 'route:trial', title: 'Пробный', kind: 'route', guard: 'trial_available', bound: false }],
      templates: [],
    }),
    'GET /api/uxmap': json({
      entries: [{ id: 'cmd_start', kind: 'command', title: '/start', when: 'Юзер запускает бота', targets: [{ to: 'menu' }] }],
      transitions: [
        { from: '*', action: 'route:pay', to: ['pay'] },
        { from: '*', action: 'route:trial', to: ['trial'] },
      ],
      links: [],
    }),
  }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => (replies[`${init.method ?? 'GET'} ${url}`] ?? (() => new Response(null, { status: 500 })))())
})
afterEach(() => vi.unstubAllGlobals())

const open = async () => {
  render(
    <MemoryRouter>
      <UxMap />
    </MemoryRouter>,
  )
  await act(() => new Promise((r) => setTimeout(r, 0)))
}
const arrows = () => within(screen.getByRole('list', { name: 'стрелки' })).queryAllByRole('listitem').map((li) => li.textContent)

describe('UxMap', () => {
  it('draws screens with preview text and keyboard, entries and arrows; guarded button is hidden by default', async () => {
    await open()
    expect(screen.getByTestId('node:menu').textContent).toContain('Привет, Иван')
    expect(within(screen.getByTestId('node:menu')).getByText('Оплата')).toBeTruthy()
    expect(within(screen.getByTestId('node:menu')).queryByText('Пробный')).toBeNull()
    expect(screen.getByTestId('node:entry:cmd_start').textContent).toContain('/start')
    expect(arrows()).toEqual(['menu:go_pay->pay', 'entry:cmd_start->menu'])
    // «Править» ведёт в редактор именно этого сообщения
    expect(within(screen.getByTestId('node:pay')).getByRole('link', { name: 'Править' }).getAttribute('href')).toBe('/texts?key=pay')
  })

  it('fake condition shows the button and its arrow; only conditions used in keyboards are offered', async () => {
    await open()
    expect(screen.queryByRole('checkbox', { name: 'Подарки доступны' })).toBeNull()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Пробный доступен' }))
    expect(within(screen.getByTestId('node:menu')).getByText('Пробный')).toBeTruthy()
    expect(arrows()).toContain('menu:go_trial->trial')
  })

  it('group chips filter screens and their arrows', async () => {
    await open()
    fireEvent.click(screen.getByRole('button', { name: 'Меню' }))
    expect(screen.queryByTestId('node:pay')).toBeNull()
    expect(screen.getByTestId('node:menu')).toBeTruthy()
    expect(arrows()).toEqual(['entry:cmd_start->menu'])
  })

  it('load failure is reported', async () => {
    replies['GET /api/uxmap'] = () => new Response(null, { status: 500 })
    await open()
    expect(screen.getByRole('alert').textContent).toBe('Не удалось загрузить')
  })
})
