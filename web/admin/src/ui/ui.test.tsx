import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MemoryRouter, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { notWired } from '@/lib/not-wired'
import {
  Button,
  Cell,
  Checkbox,
  DataTable,
  Dialog,
  FormatBar,
  Input,
  type DataTableColumn,
  SegmentedControl,
  Sheet,
  Sidebar,
  Switch,
  ToastProvider,
  toast,
} from '@/ui'

describe('Sidebar collapsed', () => {
  const items = [{ section: 'Данные' }, { key: 'pay', label: 'Платежи', icon: 'card' as const, badge: 5 }]

  it('подпись уходит в aria-label, секция — в разделитель, бейдж — в точку', () => {
    const { container } = render(<Sidebar items={items} value="pay" collapsed />)
    const item = screen.getByRole('button', { name: 'Платежи' })
    expect(item.getAttribute('aria-current')).toBe('page')
    expect(item.textContent).toBe('')
    expect(screen.queryByText('Данные')).toBeNull()
    expect(screen.getByRole('separator')).toBeTruthy()
    expect(container.querySelector('[data-dot]')).toBeTruthy()
    expect(screen.queryByText('5')).toBeNull()
  })

  it('развёрнутый: подпись и число видны, onToggle рисует кнопку', () => {
    const onToggle = vi.fn()
    const onChange = vi.fn()
    render(<Sidebar items={items} onChange={onChange} onToggle={onToggle} />)
    expect(screen.getByText('Данные')).toBeTruthy()
    expect(screen.getByText('5')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Свернуть меню' }))
    expect(onToggle).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByText('Платежи'))
    expect(onChange).toHaveBeenCalledWith('pay')
  })
})

describe('DataTable', () => {
  type Row = { id: number; name: string; sum: number }
  const rows: Row[] = [
    { id: 1, name: 'Б', sum: 10 },
    { id: 2, name: 'А', sum: 9 },
    { id: 3, name: 'В', sum: 100 },
  ]
  const columns: DataTableColumn<Row>[] = [
    { key: 'name', title: 'Имя', sortable: true },
    { key: 'sum', title: 'Сумма', numeric: true, sortable: true },
  ]
  const firstColumn = () =>
    within(screen.getByRole('table'))
      .getAllByRole('row')
      .slice(1)
      .map((r) => r.querySelector('td')?.textContent)

  it('numeric: правое выравнивание и табличные цифры у заголовка и ячеек', () => {
    render(<DataTable columns={columns} rows={rows} />)
    const header = screen.getByRole('columnheader', { name: 'Сумма' })
    const cell = screen.getByRole('cell', { name: '100' })
    for (const el of [header, cell]) {
      expect(el.classList.contains('text-right')).toBe(true)
      expect(el.classList.contains('tabular-nums')).toBe(true)
    }
  })

  it('сортировка: asc → desc → без сортировки', () => {
    render(<DataTable columns={columns} rows={rows} />)
    const sort = screen.getByRole('button', { name: 'Имя' })
    const header = screen.getByRole('columnheader', { name: 'Имя' })
    expect(firstColumn()).toEqual(['Б', 'А', 'В'])
    fireEvent.click(sort)
    expect(firstColumn()).toEqual(['А', 'Б', 'В'])
    expect(header.getAttribute('aria-sort')).toBe('ascending')
    fireEvent.click(sort)
    expect(firstColumn()).toEqual(['В', 'Б', 'А'])
    expect(header.getAttribute('aria-sort')).toBe('descending')
    fireEvent.click(sort)
    expect(firstColumn()).toEqual(['Б', 'А', 'В'])
    expect(header.getAttribute('aria-sort')).toBeNull()
  })

  it('numeric сортирует как числа', () => {
    render(<DataTable columns={columns} rows={rows} defaultSort={{ key: 'sum', dir: 'asc' }} />)
    expect(firstColumn()).toEqual(['А', 'Б', 'В'])
  })

  it('numeric понимает форматированные деньги с nbsp', () => {
    const money = [
      { id: 1, name: 'мало', sum: '900\u00a0₽' },
      { id: 2, name: 'много', sum: '11\u00a0900\u00a0₽' },
    ]
    render(
      <DataTable
        columns={[
          { key: 'name', title: 'Имя' },
          { key: 'sum', title: 'Сумма', numeric: true },
        ]}
        rows={money}
        defaultSort={{ key: 'sum', dir: 'desc' }}
      />,
    )
    expect(firstColumn()).toEqual(['много', 'мало'])
  })

  it('клик в rowActions не вызывает onRowClick, клик по строке и Enter — вызывают', () => {
    const onRowClick = vi.fn()
    const onAction = vi.fn()
    render(
      <DataTable
        columns={columns}
        rows={rows.slice(0, 1)}
        onRowClick={onRowClick}
        rowActions={() => <button onClick={onAction}>Действие</button>}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Действие' }))
    expect(onAction).toHaveBeenCalledOnce()
    expect(onRowClick).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('cell', { name: 'Б' }))
    expect(onRowClick).toHaveBeenCalledExactlyOnceWith(rows[0])
    fireEvent.keyDown(screen.getByRole('cell', { name: 'Б' }).closest('tr')!, { key: 'Enter' })
    expect(onRowClick).toHaveBeenCalledTimes(2)
    // false — обработчик вызвал preventDefault
    expect(fireEvent.keyDown(screen.getByRole('cell', { name: 'Б' }).closest('tr')!, { key: ' ' })).toBe(false)
    expect(onRowClick).toHaveBeenCalledTimes(3)
  })

  it('empty: свой текст или «Нет данных»', () => {
    const { rerender } = render(<DataTable columns={columns} rows={[]} empty="Платежей пока нет" />)
    expect(screen.getByRole('cell', { name: 'Платежей пока нет' }).getAttribute('colspan')).toBe('2')
    rerender(<DataTable columns={columns} rows={[]} />)
    expect(screen.getByRole('cell', { name: 'Нет данных' })).toBeTruthy()
  })

  it('pagination: счётчик и границы', () => {
    const onNext = vi.fn()
    render(
      <DataTable
        columns={columns}
        rows={rows}
        pagination={{ page: 1, pages: 2, from: 1, to: 3, total: 6, onNext }}
      />,
    )
    expect(screen.getByText('Показано 1–3 из 6')).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Предыдущая страница' }) as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
    expect(onNext).toHaveBeenCalledOnce()
  })

  it('mobileRow: Section из Cell в той же сортировке, переключение через CSS', () => {
    const { container } = render(
      <DataTable
        columns={columns}
        rows={rows}
        defaultSort={{ key: 'name', dir: 'asc' }}
        mobileRow={(r) => ({ title: `м-${r.name}`, value: r.sum })}
      />,
    )
    expect(screen.getByRole('table').closest('.max-md\\:hidden')).toBeTruthy()
    const mobile = container.querySelector('section.md\\:hidden')!
    expect(mobile).toBeTruthy()
    expect(
      within(mobile as HTMLElement)
        .getAllByText(/^м-/)
        .map((e) => e.textContent),
    ).toEqual(['м-А', 'м-Б', 'м-В'])
  })
})

describe('Dialog', () => {
  const setup = () => {
    const onCancel = vi.fn()
    const onDelete = vi.fn()
    render(
      <Dialog
        title="Удалить промокод?"
        message="Отменить нельзя"
        actions={[
          { label: 'Отмена', style: 'cancel', onClick: onCancel },
          { label: 'Удалить', style: 'destructive', onClick: onDelete },
        ]}
      />,
    )
    return { onCancel, onDelete }
  }

  it('рисует заголовок и сообщение в body, действия вызывают onClick', () => {
    const { onCancel, onDelete } = setup()
    const dialog = screen.getByRole('alertdialog', { name: 'Удалить промокод?' })
    expect(dialog).toBeTruthy()
    expect(screen.getByText('Отменить нельзя')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    expect(onDelete).toHaveBeenCalledOnce()
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('фокус на безопасном действии, Esc — как cancel', async () => {
    const { onCancel } = setup()
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Отмена' })))
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('после закрытия фокус возвращается на открывшую кнопку', async () => {
    function Host() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button onClick={() => setOpen(true)}>Открыть</button>
          {open && <Dialog title="Точно?" actions={[{ label: 'Закрыть', onClick: () => setOpen(false) }]} />}
        </>
      )
    }
    render(<Host />)
    const opener = screen.getByRole('button', { name: 'Открыть' })
    opener.focus()
    fireEvent.click(opener)
    await waitFor(() => expect(document.activeElement?.textContent).toBe('Закрыть'))
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })
})

describe('Sheet', () => {
  it('крестик и Esc вызывают onClose', () => {
    const onClose = vi.fn()
    render(<Sheet title="Фильтры" onClose={onClose} />)
    expect(screen.getByRole('dialog', { name: 'Фильтры' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(onClose).toHaveBeenCalledOnce()
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})

describe('Switch и Checkbox', () => {
  it('Switch: onChange(boolean), disabled молчит', () => {
    const onChange = vi.fn()
    const { rerender } = render(<Switch aria-label="Сеть" onChange={onChange} />)
    const sw = screen.getByRole('switch', { name: 'Сеть' })
    fireEvent.click(sw)
    expect(onChange).toHaveBeenLastCalledWith(true)
    rerender(<Switch aria-label="Сеть" checked onChange={onChange} />)
    expect(sw.getAttribute('aria-checked')).toBe('true')
    fireEvent.click(sw)
    expect(onChange).toHaveBeenLastCalledWith(false)
    onChange.mockClear()
    rerender(<Switch aria-label="Сеть" disabled onChange={onChange} />)
    fireEvent.click(sw)
    expect(onChange).not.toHaveBeenCalled()
  })

  it('Checkbox: onChange(boolean), клик по подписи тоже работает', async () => {
    const onChange = vi.fn()
    const { rerender } = render(<Checkbox label="Согласен" onChange={onChange} />)
    await userEvent.click(screen.getByText('Согласен'))
    expect(onChange).toHaveBeenLastCalledWith(true)
    rerender(<Checkbox label="Согласен" checked onChange={onChange} />)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Согласен' }))
    expect(onChange).toHaveBeenLastCalledWith(false)
  })
})

describe('SegmentedControl', () => {
  it('onChange с новым значением, повторный клик по активному молчит', () => {
    const onChange = vi.fn()
    render(<SegmentedControl options={['Все', { value: 'a', label: 'Активные' }]} value="a" onChange={onChange} />)
    fireEvent.click(screen.getByText('Активные'))
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.click(screen.getByText('Все'))
    expect(onChange).toHaveBeenCalledExactlyOnceWith('Все')
  })
})

describe('FormatBar', () => {
  it('active и aria-pressed, onFormat с ключом, formats сужает набор', () => {
    const onFormat = vi.fn()
    const { rerender } = render(<FormatBar active={['bold']} onFormat={onFormat} right="1 / 4096" />)
    expect(screen.getAllByRole('button')).toHaveLength(9)
    expect(screen.getByRole('button', { name: 'Жирный' }).getAttribute('aria-pressed')).toBe('true')
    const italic = screen.getByRole('button', { name: 'Курсив' })
    expect(italic.getAttribute('aria-pressed')).toBe('false')
    fireEvent.click(italic)
    expect(onFormat).toHaveBeenCalledExactlyOnceWith('italic')
    expect(screen.getByText('1 / 4096')).toBeTruthy()
    rerender(<FormatBar formats={['link']} />)
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})

describe('Button', () => {
  it('loading блокирует клик и прячет подпись', () => {
    const onClick = vi.fn()
    render(
      <Button loading onClick={onClick}>
        Сохранить
      </Button>,
    )
    const button = screen.getByRole('button') as HTMLButtonElement
    expect(button.disabled).toBe(true)
    expect(button.type).toBe('button')
    expect(screen.queryByText('Сохранить')).toBeNull()
    expect(screen.getByRole('status', { name: 'Загрузка' })).toBeTruthy()
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })
})

describe('Cell', () => {
  function Where() {
    return <span data-testid="where">{useLocation().pathname}</span>
  }

  it('внутренний href — переход через роутер, Enter и пробел работают', async () => {
    render(
      <MemoryRouter>
        <Cell title="Юзер" href="/users/5" chevron />
        <Where />
      </MemoryRouter>,
    )
    const link = screen.getByRole('link', { name: 'Юзер' })
    expect(link.getAttribute('href')).toBe('/users/5')
    await userEvent.click(link)
    expect(screen.getByTestId('where').textContent).toBe('/users/5')
  })

  it('пробел на ссылке открывает её', () => {
    render(
      <MemoryRouter>
        <Cell title="Юзер" href="/users/7" />
        <Where />
      </MemoryRouter>,
    )
    fireEvent.keyDown(screen.getByRole('link', { name: 'Юзер' }), { key: ' ' })
    expect(screen.getByTestId('where').textContent).toBe('/users/7')
  })

  it('внешний href — обычная ссылка, роутер не нужен', () => {
    render(<Cell title="Канал" href="https://t.me/x" />)
    expect(screen.getByRole('link', { name: 'Канал' }).getAttribute('href')).toBe('https://t.me/x')
  })

  it('onClick — кнопка, без действий — не интерактивна', () => {
    const onClick = vi.fn()
    render(
      <>
        <Cell title="Кнопка" onClick={onClick} />
        <Cell title="Текст" value="1 ₽" />
      </>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Кнопка' }))
    expect(onClick).toHaveBeenCalledOnce()
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})

describe('Toast', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('toast() показывает в body с role=status и скрывает через ~2,5 с', () => {
    vi.useFakeTimers()
    render(
      <div data-testid="host">
        <ToastProvider />
      </div>,
    )
    // Живая область смонтирована заранее, иначе скринридер не объявит тост.
    const node = screen.getByRole('status')
    act(() => toast('Промокод скопирован'))
    expect(node.textContent).toBe('Промокод скопирован')
    expect(node.closest('[data-testid=host]')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(2400)
    })
    expect(node.textContent).toBe('Промокод скопирован')
    act(() => {
      vi.advanceTimersByTime(200)
    })
    expect(node.textContent).toBe('')
  })

  it('новый тост заменяет старый и продлевает показ; notWired пишет «Пока не подключено»', () => {
    vi.useFakeTimers()
    render(<ToastProvider />)
    act(() => toast('Первый'))
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    act(() => notWired())
    expect(screen.getByRole('status').textContent).toBe('Пока не подключено')
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(screen.getByRole('status').textContent).toBe('Пока не подключено')
  })

  it('без провайдера toast() ничего не ломает', () => {
    expect(() => toast('x')).not.toThrow()
  })
})

describe('Input fit', () => {
  it('sizes to content only with fit', () => {
    const { rerender } = render(<Input aria-label="n" fit type="number" value="1600" onChange={() => {}} />)
    expect(screen.getByLabelText('n').style.width).toBe('4ch')
    rerender(<Input aria-label="n" fit type="number" value="" onChange={() => {}} />)
    expect(screen.getByLabelText('n').style.width).toBe('3ch')
    rerender(<Input aria-label="n" type="number" value="1600" onChange={() => {}} />)
    expect(screen.getByLabelText('n').style.width).toBe('')
  })
})
