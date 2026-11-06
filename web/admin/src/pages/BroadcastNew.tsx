import { useLocation, useNavigate } from 'react-router'
import { Button } from '@/ui'
import { Composer, type ComposerDraft } from '../compose/Composer'
import { announceSent } from '../compose/announce'

export default function BroadcastNew() {
  const navigate = useNavigate()
  const { state } = useLocation()

  return (
    <>
      <div className="-my-2 -ml-2">
        <Button variant="plain" icon="chevron-left" onClick={() => navigate('/broadcasts')}>
          Все рассылки
        </Button>
      </div>
      <Composer
        mode="broadcast"
        initial={state as ComposerDraft | null}
        onSent={(b) => {
          announceSent(b)
          navigate(`/broadcasts/${b.id}`, { replace: true })
        }}
      />
    </>
  )
}
