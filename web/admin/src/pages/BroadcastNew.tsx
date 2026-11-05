import { useLocation, useNavigate } from 'react-router'
import { Button, toast } from '@/ui'
import { Composer, type ComposerDraft } from '../compose/Composer'
import { formatCount, plural } from '../format'

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
          toast(`Рассылка запущена — ${formatCount(b.total)} ${plural(b.total, ['получатель', 'получателя', 'получателей'])}`, {
            icon: 'check',
          })
          navigate('/broadcasts', { replace: true })
        }}
      />
    </>
  )
}
