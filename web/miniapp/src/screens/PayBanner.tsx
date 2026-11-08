import { Button } from '@/ui/primitives/button'
import { openBot } from '~/lib/telegram'
import type { Me } from '../App'

// Оплата, продление, докупка устройств и подарки живут только в боте (ручной перевод на карту).
export function PayBanner({ me }: { me: Me }) {
  return (
    <section className="mx-4 mb-4 flex flex-col gap-3 rounded-lg bg-accent-tint p-4">
      <p className="subheadline text-accent-strong">Оплатить и продлить можно только в боте.</p>
      {me.botUsername && (
        <Button size="md" variant="filled" onClick={() => openBot(me.botUsername)}>
          Открыть бота
        </Button>
      )}
    </section>
  )
}
