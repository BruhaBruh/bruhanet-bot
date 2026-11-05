import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Cell, type CellProps } from './cell'
import { Icon } from './icon'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './primitives/table'
import { Section } from './section'

export interface DataTableColumn<R = unknown> {
  key: string
  title: ReactNode
  /** Деньги, счётчики: вправо, табличные цифры, без переноса. */
  numeric?: boolean
  align?: 'left' | 'right' | 'center'
  width?: number | string
  minWidth?: number | string
  maxWidth?: number | string
  mono?: boolean
  secondary?: boolean
  strong?: boolean
  /** По умолчанию без переноса; `wrap` разрешает, `truncate` режет многоточием по maxWidth. */
  wrap?: boolean
  truncate?: boolean
  sortable?: boolean
  sortValue?: (row: R) => string | number
  render?: (row: R) => ReactNode
}

export interface DataTablePagination {
  page: number
  pages: number
  from: number
  to: number
  total: string | number
  onPrev?: () => void
  onNext?: () => void
}

type Sort = { key: string; dir: 'asc' | 'desc' }

export interface DataTableProps<R = unknown> {
  columns: DataTableColumn<R>[]
  rows: R[]
  title?: ReactNode
  toolbar?: ReactNode
  footer?: ReactNode
  pagination?: DataTablePagination
  rowActions?: (row: R) => ReactNode
  defaultSort?: Sort
  empty?: ReactNode
  onRowClick?: (row: R) => void
  /** Ниже 768px вместо таблицы — Section из Cell. Без onClick и href строка получает onRowClick. */
  mobileRow?: (row: R) => CellProps
  className?: string
}

const field = (row: unknown, key: string) => (row as Record<string, unknown>)[key]

function numOf(v: unknown) {
  if (typeof v === 'number') return v
  const m = String(v ?? '')
    .replace(/\s/g, '')
    .replace(',', '.')
    .match(/[-−]?\d+(\.\d+)?/)
  return m ? parseFloat(m[0].replace('−', '-')) : -Infinity
}

function sortRows<R>(rows: R[], columns: DataTableColumn<R>[], sort: Sort | null) {
  const col = sort && columns.find((c) => c.key === sort.key)
  if (!sort || !col) return rows
  const get = col.sortValue ?? ((r: R) => field(r, col.key))
  return rows.slice().sort((a, b) => {
    const x = get(a)
    const y = get(b)
    const r =
      col.numeric || (typeof x === 'number' && typeof y === 'number')
        ? numOf(x) - numOf(y)
        : String(x ?? '').localeCompare(String(y ?? ''), 'ru')
    return sort.dir === 'desc' ? -r : r
  })
}

const rowKey = (row: unknown, i: number) =>
  (field(row, 'id') ?? field(row, 'code') ?? field(row, 'uid') ?? i) as string | number

const ALIGN = { left: 'text-left', right: 'text-right', center: 'text-center' }

const pagerButton =
  'focus-ring inline-flex size-8 cursor-pointer items-center justify-center rounded-sm bg-fill-tertiary text-text disabled:cursor-default disabled:opacity-35'

export function DataTable<R>({
  columns,
  rows,
  title,
  toolbar,
  footer,
  pagination: pg,
  rowActions,
  defaultSort,
  empty = 'Нет данных',
  onRowClick,
  mobileRow,
  className,
}: DataTableProps<R>) {
  const [sort, setSort] = useState<Sort | null>(defaultSort ?? null)
  const sorted = sortRows(rows, columns, sort)
  const align = (c: DataTableColumn<R>) => ALIGN[c.align ?? (c.numeric ? 'right' : 'left')]

  const nextSort = (key: string): Sort | null => {
    if (sort?.key !== key) return { key, dir: 'asc' }
    return sort.dir === 'asc' ? { key, dir: 'desc' } : null
  }

  const foot =
    footer ||
    (pg && (
      <>
        <span className="tabular-nums">
          Показано {pg.from}–{pg.to} из {pg.total}
        </span>
        <span className="inline-flex items-center gap-2">
          <button
            type="button"
            className={pagerButton}
            disabled={pg.page <= 1}
            onClick={pg.onPrev}
            aria-label="Предыдущая страница"
          >
            <Icon name="chevron-left" size={16} strokeWidth={2.2} />
          </button>
          <span className="min-w-16 text-center text-text tabular-nums">
            {pg.page} из {pg.pages}
          </span>
          <button
            type="button"
            className={pagerButton}
            disabled={pg.page >= pg.pages}
            onClick={pg.onNext}
            aria-label="Следующая страница"
          >
            <Icon name="chevron-right" size={16} strokeWidth={2.2} />
          </button>
        </span>
      </>
    ))

  return (
    <div className={cn('overflow-hidden rounded-lg bg-surface text-text shadow-card', className)}>
      {(title || toolbar) && (
        <div className="flex flex-wrap items-center gap-3 px-5 py-3 shadow-[inset_0_-0.5px_0_var(--separator)]">
          {title && <h3 className="headline flex-1">{title}</h3>}
          <div className="flex items-center gap-2">{toolbar}</div>
        </div>
      )}
      <div className={cn(mobileRow && 'max-md:hidden')}>
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((c) => {
                const on = sort?.key === c.key
                return (
                  <TableHead
                    key={c.key}
                    scope="col"
                    className={cn(align(c), c.numeric && 'tabular-nums')}
                    aria-sort={on ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                    style={{ width: c.width, minWidth: c.minWidth }}
                  >
                    {c.sortable ? (
                      <button
                        type="button"
                        className={cn(
                          'group/sort focus-ring inline-flex cursor-pointer items-center gap-1 uppercase hover:text-text',
                          on && 'text-text',
                        )}
                        onClick={() => setSort(nextSort(c.key))}
                      >
                        {c.title}
                        <Icon
                          name="chevron-down"
                          size={12}
                          strokeWidth={2.6}
                          className={cn(
                            'opacity-0 transition duration-150 group-hover/sort:opacity-50',
                            on && 'opacity-100 group-hover/sort:opacity-100',
                            on && sort.dir === 'asc' && 'rotate-180',
                          )}
                        />
                      </button>
                    ) : (
                      c.title
                    )}
                  </TableHead>
                )
              })}
              {rowActions && (
                <TableHead className="w-[1%] text-right">
                  <span className="sr-only">Действия</span>
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.length ? (
              sorted.map((row, i) => (
                <TableRow
                  key={rowKey(row, i)}
                  className={cn(
                    'group/row',
                    onRowClick &&
                      'cursor-pointer -outline-offset-2 outline-accent hover:[&>td]:bg-[color-mix(in_srgb,var(--cell-highlight)_35%,var(--surface))] focus-visible:outline-2',
                  )}
                  tabIndex={onRowClick ? 0 : undefined}
                  onClick={onRowClick && (() => onRowClick(row))}
                  onKeyDown={
                    onRowClick &&
                    ((e) => {
                      if (e.key === 'Enter' && e.target === e.currentTarget) onRowClick(row)
                    })
                  }
                >
                  {columns.map((c) => (
                    <TableCell
                      key={c.key}
                      style={{ maxWidth: c.maxWidth }}
                      className={cn(
                        align(c),
                        c.numeric && 'tabular-nums',
                        c.mono && 'font-mono text-[13px] tracking-normal',
                        c.secondary && 'text-text-secondary',
                        c.strong && 'font-semibold',
                        c.wrap && !c.numeric ? 'whitespace-normal' : 'whitespace-nowrap',
                        c.truncate && 'truncate',
                      )}
                    >
                      {c.render ? c.render(row) : (field(row, c.key) as ReactNode)}
                    </TableCell>
                  ))}
                  {rowActions && (
                    <TableCell
                      className="w-[1%] pl-0 text-right whitespace-nowrap group-hover/row:[&>*]:opacity-100 [&>*]:opacity-55 [&>*]:transition-opacity [&>*]:duration-150 [&>*:focus-visible]:opacity-100"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {rowActions(row)}
                    </TableCell>
                  )}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columns.length + (rowActions ? 1 : 0)}
                  className="h-30 text-center text-text-secondary"
                >
                  {empty}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      {mobileRow && (
        <Section plain className="mb-0 md:hidden">
          {sorted.length ? (
            sorted.map((row, i) => {
              const props = mobileRow(row)
              return (
                <Cell
                  key={rowKey(row, i)}
                  onClick={!props.onClick && !props.href && onRowClick ? () => onRowClick(row) : undefined}
                  {...props}
                />
              )
            })
          ) : (
            <div className="admin-table flex h-30 items-center justify-center text-text-secondary">{empty}</div>
          )}
        </Section>
      )}
      {foot && (
        <div className="admin-label flex flex-wrap items-center justify-between gap-3 px-5 py-3 font-normal text-text-secondary shadow-[inset_0_0.5px_0_var(--separator)]">
          {foot}
        </div>
      )}
    </div>
  )
}
