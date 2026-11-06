import { useEffect, useState } from 'react'
import { Navigate, useNavigate, useParams } from 'react-router'
import { Button, Spinner } from '@/ui'
import { get } from '../api'
import { announceSent } from '../compose/announce'
import { Composer } from '../compose/Composer'
import type { Broadcast } from '../types'

// Правка черновика (автосохранение) или запланированной рассылки («Сохранить изменения»).
export default function BroadcastEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [state, setState] = useState<{ id?: string; broadcast?: Broadcast; failed?: boolean }>({})

  useEffect(() => {
    let stale = false
    get<{ broadcast: Broadcast }>(`/api/broadcasts/${id}`).then(
      (r) => !stale && setState({ id, broadcast: r.broadcast }),
      () => !stale && setState({ id, failed: true }),
    )
    return () => {
      stale = true
    }
  }, [id])

  const back = (
    <div className="-my-2 -ml-2">
      <Button variant="plain" icon="chevron-left" onClick={() => navigate(`/broadcasts/${id}`)}>
        К рассылке
      </Button>
    </div>
  )
  if (state.id !== id) return <Spinner className="self-center p-8" />
  if (state.failed || !state.broadcast) return <p role="alert">Не удалось загрузить рассылку</p>
  const b = state.broadcast
  if (b.status !== 'draft' && b.status !== 'scheduled') return <Navigate to={`/broadcasts/${b.id}`} replace />

  return (
    <>
      {back}
      <Composer
        key={b.id}
        mode="broadcast"
        broadcastId={b.id}
        status={b.status}
        initial={{ text: b.text, photo: b.photo, audience: b.audience, audienceDays: b.audienceDays, silent: b.silent, buttons: b.buttons, scheduledAt: b.scheduledAt }}
        onSent={(sent) => {
          announceSent(sent)
          navigate(`/broadcasts/${b.id}`, { replace: true })
        }}
        onSaved={() => navigate(`/broadcasts/${b.id}`, { replace: true })}
      />
    </>
  )
}
