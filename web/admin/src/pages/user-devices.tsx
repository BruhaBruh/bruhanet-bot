import { useCallback, useEffect, useState } from 'react'
import { Button, Cell, RowAction, Spinner } from '@/ui'
import { get } from '../api'
import { formatShortDate, plural } from '../format'
import type { UserDevice, UserDevices } from '../types'
import { DeleteDialog } from './delete-dialog'

const UA_MAX = 48

const clip = (s: string) => (s.length > UA_MAX ? `${s.slice(0, UA_MAX)}…` : s)

const deviceTitle = (d: UserDevice) => [d.platform, d.deviceModel].filter(Boolean).join(' · ') || 'Устройство'

/** Блок «Устройства»: HWID-устройства из панели, удаление одного и всех. */
export function DevicesCard({ userId, reloadKey }: { userId: number; reloadKey: number }) {
  const [data, setData] = useState<UserDevices>()
  const [failed, setFailed] = useState(false)
  const [dialog, setDialog] = useState<UserDevice | 'all'>()
  const [version, setVersion] = useState(0)
  const base = `/api/users/${userId}/devices`

  useEffect(() => {
    let live = true
    get<UserDevices>(base).then(
      (r) => {
        if (!live) return
        setFailed(!Array.isArray(r.items))
        setData(r)
      },
      () => live && setFailed(true),
    )
    return () => {
      live = false
    }
  }, [base, reloadKey, version])

  const refresh = useCallback(() => setVersion((n) => n + 1), [])
  const items = Array.isArray(data?.items) ? data.items : []
  const count = items.length
  const aside = data?.limit == null ? String(count) : `${count} из ${data.limit}`

  return (
    <section className="admin-body flex flex-col gap-3 rounded-lg bg-surface p-5 shadow-card">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-3">
          <h2 className="headline">Устройства</h2>
          {data && !failed && <span className="footnote text-text-secondary">{aside}</span>}
        </div>
        {count > 0 && (
          <Button size="sm" variant="destructive" icon="trash" onClick={() => setDialog('all')}>
            Удалить все
          </Button>
        )}
      </div>
      {failed ? (
        <p role="alert" className="text-text-secondary">
          Не удалось загрузить устройства: панель недоступна
        </p>
      ) : !data ? (
        <Spinner className="self-center p-4" />
      ) : !count ? (
        <p className="text-text-secondary">Нет подключённых устройств</p>
      ) : (
        <ul className="overflow-hidden rounded-md">
          {items.map((d) => (
            <li key={d.hwid}>
              <Cell
                title={deviceTitle(d)}
                subtitle={
                  <span className="flex min-w-0 flex-col">
                    {(d.osVersion || d.userAgent) && (
                      <span className="truncate" title={d.userAgent ?? undefined}>
                        {[d.osVersion, d.userAgent && clip(d.userAgent)].filter(Boolean).join(' · ')}
                      </span>
                    )}
                    <span>добавлено {formatShortDate(d.createdAt)}</span>
                    <span>последний раз {formatShortDate(d.updatedAt)}</span>
                  </span>
                }
                after={<RowAction icon="trash" label={`Удалить устройство ${deviceTitle(d)}`} onClick={() => setDialog(d)} />}
              />
            </li>
          ))}
        </ul>
      )}
      {dialog === 'all' && (
        <DeleteDialog
          title="Удалить все устройства?"
          message={`${count} ${plural(count, ['устройство', 'устройства', 'устройств'])} отключатся от подписки, юзер сможет добавить их снова.`}
          url={base}
          done="Устройства удалены"
          conflict=""
          onClose={() => setDialog(undefined)}
          onDeleted={refresh}
        />
      )}
      {dialog && dialog !== 'all' && (
        <DeleteDialog
          title="Удалить устройство?"
          message="Оно отключится от подписки, юзер сможет добавить его снова."
          url={`${base}/${encodeURIComponent(dialog.hwid)}`}
          done="Устройство удалено"
          conflict=""
          onClose={() => setDialog(undefined)}
          onDeleted={refresh}
        />
      )}
    </section>
  )
}
